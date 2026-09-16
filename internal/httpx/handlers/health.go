// Package handlers holds the server's HTTP endpoints.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ddunford/goretrotv/internal/version"
)

// HealthResponse is what GET /health answers with.
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Timestamp string `json:"timestamp"`
}

// Health reports that the process is up and says which build it is.
//
// Which build matters more than it looks: a gate that passes against a binary nobody can identify
// has proved nothing about the tree it was supposed to be checking.
func Health(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Version:   version.Version,
		Commit:    version.Commit,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// WriteJSON encodes v as the response body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
