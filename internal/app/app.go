// Package app wires the process's parts together and hands back something runnable.
//
// Dependencies point inward (CLAUDE.md -> Architecture Decisions): this package may know about the
// web layer and the emulator, and neither of those may know about it.
package app

import (
	"log/slog"
	"net/http"

	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/httpx/handlers"
	"github.com/ddunford/goretrotv/internal/httpx/middleware"
)

// New builds the HTTP handler for the whole server.
func New(cfg *config.Config, logger *slog.Logger) (http.Handler, error) {
	mux := http.NewServeMux()
	registerRoutes(mux, cfg)

	return middleware.Chain(mux,
		middleware.Recover(logger),
		middleware.AccessLog(logger),
	), nil
}

func registerRoutes(mux *http.ServeMux, cfg *config.Config) {
	mux.HandleFunc("GET /health", handlers.Health)

	if cfg.EnablePprof {
		handlers.MountPprof(mux)
	}
}
