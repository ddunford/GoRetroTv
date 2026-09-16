package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ddunford/goretrotv/internal/httpx/handlers"
)

func TestHealthReportsOKAndIdentifiesTheBuild(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	handlers.Health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var body handlers.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	// An unidentifiable build is the failure this asserts: a gate that passes against a binary
	// nobody can name has proved nothing about the tree it was checking.
	if body.Version == "" {
		t.Error("version is empty")
	}
	if body.Timestamp == "" {
		t.Error("timestamp is empty")
	}
}
