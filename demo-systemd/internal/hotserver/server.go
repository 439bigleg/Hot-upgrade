// Package hotserver：HTTP 服务仅通过 systemd socket activation 取得监听（LISTEN_FDS），无 SIGUSR2 / 无直接 bind。
package hotserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Server struct {
	Handler http.Handler

	Logger *log.Logger

	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration

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
	readHdr := s.ReadHeaderTimeout
	if readHdr == 0 {
		readHdr = 15 * time.Second
	}
	shutdownTO := s.ShutdownTimeout
	if shutdownTO == 0 {
		shutdownTO = 120 * time.Second
	}
	exit := s.Exit
	if exit == nil {
		exit = os.Exit
	}

	listeners, err := systemdListeners()
	if err != nil {
		return err
	}
	if len(listeners) == 0 {
		return fmt.Errorf("hotserver: need systemd socket activation (LISTEN_FDS / LISTEN_PID)")
	}
	ln := pickListener(listeners)
	h.Printf("systemd socket activation on %s", ln.Addr())

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
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	h.Println("shutdown...")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTO)
	_ = srv.Shutdown(ctx)
	cancel()
	exit(0)
	return nil // unreachable; satisfies func error signature
}

func pickListener(listeners []net.Listener) net.Listener {
	for _, ln := range listeners {
		if _, ok := ln.(*net.TCPListener); ok {
			return ln
		}
	}
	return listeners[0]
}
