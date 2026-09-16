package handlers

import (
	"net/http"
	"net/http/pprof"
)

// MountPprof registers the profiling endpoints on mux.
//
// Gated by GORETROTV_ENABLE_PPROF and off by default. pprof is a developer surface, and the
// recorded decision is that developer surfaces are not exposed (CLAUDE.md -> Deployment and
// access). Reaching it needs both the flag AND a listener the caller can get to; the default
// listener is loopback.
func MountPprof(mux *http.ServeMux) {
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
}
