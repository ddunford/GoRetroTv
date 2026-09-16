// Package httpx holds the server plumbing: its timeouts, its lifecycle and its middleware.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ShutdownGrace is how long in-flight requests get to finish once a signal arrives.
const ShutdownGrace = 15 * time.Second

// Server is the process's HTTP listener.
type Server struct {
	http   *http.Server
	logger *slog.Logger
}

// NewServer builds a listener on addr serving h.
func NewServer(addr string, h http.Handler, logger *slog.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
			// No WriteTimeout. The framebuffer stream is long-lived by design and a write
			// deadline would sever it mid-frame; per-handler deadlines cover the short routes.
		},
		logger: logger,
	}
}

// Addr reports the address the server was configured to listen on.
func (s *Server) Addr() string { return s.http.Addr }

// Run serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	errs := make(chan error, 1)

	go func() {
		s.logger.Info("listening", "addr", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("serve: %w", err)
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down", "grace", ShutdownGrace.String())
		// Deliberately rooted at Background and not at ctx: ctx is already cancelled -- that is
		// why we are here -- and a shutdown context derived from it would be born expired,
		// turning every graceful shutdown into an immediate connection kill.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownGrace)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // see above
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return <-errs
	}
}
