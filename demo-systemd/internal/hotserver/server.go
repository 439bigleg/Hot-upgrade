// Package hotserver：监听来源仅支持 1) 应用内热升级 fd 2) systemd socket activation（无直接 bind）。
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

const DefaultListenEnv = "HOTUPGRADE_LISTEN_FD"

type Server struct {
	Handler http.Handler

	Logger *log.Logger

	ListenEnvKey string

	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	DrainTimeout      time.Duration

	UpgradeSignal syscall.Signal

	ExecPath string
	ExecArgs []string

	Exit func(code int)
}

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

	ln, err := inheritOrListen(envKey, h)
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

func inheritOrListen(envKey string, log *log.Logger) (net.Listener, error) {
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
		log.Printf("inherited listener (app handoff) on %s", ln.Addr())
		return ln, nil
	}

	listeners, err := systemdListeners()
	if err != nil {
		return nil, err
	}
	if len(listeners) > 0 {
		ln := pickListener(listeners)
		log.Printf("systemd socket activation on %s", ln.Addr())
		return ln, nil
	}

	return nil, fmt.Errorf("hotserver: need systemd socket activation (LISTEN_FDS) or %s from parent upgrade", envKey)
}

func pickListener(listeners []net.Listener) net.Listener {
	for _, ln := range listeners {
		if _, ok := ln.(*net.TCPListener); ok {
			return ln
		}
	}
	return listeners[0]
}

type forkUpgradeConfig struct {
	listenEnv  string
	drain      time.Duration
	execPath   string
	execArgs   []string
	logger     *log.Logger
	firstExtra int
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
