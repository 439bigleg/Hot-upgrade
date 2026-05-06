// Package hotserver 封装「可热升级的 HTTP 监听组件」：继承监听 FD、SIGUSR2 拉起新进程并排空当前实例。
package hotserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// DefaultListenEnv 子进程继承监听 fd 时使用的环境变量名（值为 fd 编号）。
const DefaultListenEnv = "HOTUPGRADE_LISTEN_FD"

// Server 表示一个带热升级能力的 HTTP 组件实例。
type Server struct {
	Addr    string        // 监听地址，如 ":8085"
	Handler http.Handler  // 业务路由，必填

	Logger *log.Logger // 可选；默认使用 log.Default()

	// ListenEnvKey 读取/写入继承监听 fd 的环境变量名；空则使用 DefaultListenEnv。
	ListenEnvKey string

	ReadHeaderTimeout time.Duration // 请求头读超时；0 则默认 15s
	ShutdownTimeout   time.Duration // SIGINT/SIGTERM 时 Shutdown 超时；0 则默认 120s
	DrainTimeout      time.Duration // 升级后父进程排空超时；0 则默认 120s

	// UpgradeSignal 触发热升级的信号；0 则默认 SIGUSR2。
	UpgradeSignal syscall.Signal

	// ExecPath、ExecArgs 用于 exec 子进程；空则分别为 os.Args[0]、os.Args[1:]。
	ExecPath string
	ExecArgs []string

	// Exit 进程退出函数；用于测试注入。nil 时为 os.Exit。
	Exit func(code int)
}

// Run 阻塞运行：启动监听与 HTTP 服务，直到优雅退出或热升级（成功后当前进程会 Exit(0)）。
func (s *Server) Run() error {
	if s.Handler == nil {
		return fmt.Errorf("hotserver: Handler is required")
	}
	h := s.Logger
	if h == nil {
		h = log.Default()
	}
	envKey := s.ListenEnvKey
	if envKey == "" {
		envKey = DefaultListenEnv
	}
	readHdr := s.ReadHeaderTimeout
	if readHdr == 0 {
		readHdr = 15 * time.Second
	}
	shutdownTO := s.ShutdownTimeout
	if shutdownTO == 0 {
		shutdownTO = 120 * time.Second
	}
	drainTO := s.DrainTimeout
	if drainTO == 0 {
		drainTO = 120 * time.Second
	}
	upSig := s.UpgradeSignal
	if upSig == 0 {
		upSig = syscall.SIGUSR2
	}
	exit := s.Exit
	if exit == nil {
		exit = os.Exit
	}

	ln, err := inheritOrListen(s.Addr, envKey, h)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           s.Handler,
		ReadHeaderTimeout: readHdr,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			h.Fatal(err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, upSig)

	for sig := range sigCh {
		switch sig {
		case upSig:
			if err := forkUpgrade(srv, ln, forkUpgradeConfig{
				listenEnv:  envKey,
				drain:      drainTO,
				execPath:   s.ExecPath,
				execArgs:   s.ExecArgs,
				logger:     h,
				firstExtra: 3,
			}); err != nil {
				h.Printf("upgrade failed: %v", err)
				continue
			}
			h.Println("parent exiting after drain")
			exit(0)
		default:
			h.Println("shutdown...")
			ctx, cancel := context.WithTimeout(context.Background(), shutdownTO)
			_ = srv.Shutdown(ctx)
			cancel()
			exit(0)
		}
	}
	return nil
}

func inheritOrListen(addr, envKey string, log *log.Logger) (net.Listener, error) {
	if v := os.Getenv(envKey); v != "" {
		fd, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("bad %s: %w", envKey, err)
		}
		f := os.NewFile(uintptr(fd), "listen")
		ln, err := net.FileListener(f)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		_ = os.Unsetenv(envKey)
		log.Printf("inherited listener on %s", ln.Addr())
		return ln, nil
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	log.Printf("listening on %s", ln.Addr())
	return ln, nil
}

type forkUpgradeConfig struct {
	listenEnv  string
	drain      time.Duration
	execPath   string
	execArgs   []string
	logger     *log.Logger
	firstExtra int // 第一个 ExtraFiles 对应 fd，一般为 3
}

func forkUpgrade(srv *http.Server, ln net.Listener, cfg forkUpgradeConfig) error {
	tcpLn, ok := ln.(*net.TCPListener)
	if !ok {
		return fmt.Errorf("need *net.TCPListener, got %T", ln)
	}

	f, err := tcpLn.File()
	if err != nil {
		return fmt.Errorf("TCPListener.File: %w", err)
	}

	path := cfg.execPath
	if path == "" {
		path = os.Args[0]
	}
	args := cfg.execArgs
	if args == nil {
		args = os.Args[1:]
	}

	cmd := exec.Command(path, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", cfg.listenEnv, cfg.firstExtra))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{f}

	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return fmt.Errorf("start child: %w", err)
	}
	_ = f.Close()

	cfg.logger.Printf("child pid=%d (same binary), draining parent connections...", cmd.Process.Pid)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.drain)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
