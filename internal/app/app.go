// Package app wires the process's parts together and hands back something runnable.
//
// Dependencies point inward (CLAUDE.md -> Architecture Decisions): this package may know about the
// web layer and the emulator, and neither of those may know about it.
package app

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/httpx/handlers"
	"github.com/ddunford/goretrotv/internal/httpx/middleware"
	"github.com/ddunford/goretrotv/internal/web"
)

// New builds the HTTP handler for the whole server.
func New(cfg *config.Config, logger *slog.Logger, transport *web.Transport) (http.Handler, error) {
	if transport == nil {
		return nil, fmt.Errorf("app: browser transport is nil")
	}
	assets := os.DirFS(cfg.WebDir)
	for _, name := range []string{"index.html", "styles.css", "favicon.svg", "dist/app.js", "dist/wire.js", "dist/wire_generated.js"} {
		if _, err := fs.Stat(assets, name); err != nil {
			return nil, fmt.Errorf("app: required page asset %s: %w", name, err)
		}
	}
	mux := http.NewServeMux()
	registerRoutes(mux, cfg, assets, transport)

	return middleware.Chain(mux,
		middleware.Recover(logger),
		middleware.AccessLog(logger),
	), nil
}

func registerRoutes(mux *http.ServeMux, cfg *config.Config, assets fs.FS, transport *web.Transport) {
	mux.HandleFunc("GET /health", handlers.Health)
	mux.Handle("GET /ws", transport)
	for path, name := range map[string]string{
		"/": "index.html", "/styles.css": "styles.css", "/favicon.svg": "favicon.svg",
		"/dist/app.js": "dist/app.js", "/dist/wire.js": "dist/wire.js", "/dist/wire_generated.js": "dist/wire_generated.js",
	} {
		asset := name
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				http.NotFound(w, r)
				return
			}
			http.ServeFileFS(w, r, assets, asset)
		})
	}

	if cfg.EnablePprof {
		handlers.MountPprof(mux)
	}
}
