package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/httpx/handlers"
	"github.com/ddunford/goretrotv/internal/version"
)

// TestHealthReportsTheBuildItWasLinkedWith asserts what a unit test can actually assert: that the
// handler reports faithfully whatever internal/version holds, rather than hardcoding, dropping or
// transposing a field.
//
// It deliberately does NOT claim to prove the build is identifiable, which an earlier version of
// this test did by requiring version to be non-empty. Under `go test` there are no -ldflags, so
// version is always "dev" and commit always "unknown" -- the linker defaults, which are exactly
// what an unidentifiable build reports, and which are non-empty. That assertion could not fail for
// the reason it gave. The same defect was found and fixed in the boot gate (gort-6ar.27), and
// stage 4 of that gate -- comparing the reported commit to this working tree's HEAD against a
// binary built through the Makefile's ldflags -- is where build identity is actually proved.
// Do not cite this test for that.
func TestHealthReportsTheBuildItWasLinkedWith(t *testing.T) {
	// Not parallel: it sets the version package's globals, which are process-wide.
	//
	// They have to be SET rather than read as they stand, and that is not fussiness. Under
	// `go test` they hold the linker defaults, so `body.Version != version.Version` is satisfied
	// just as happily by a handler that hardcodes the literal "dev". Proved by mutation: with the
	// values left alone, a hardcoded version and a dropped commit both SURVIVED this test. Giving
	// them values nothing would otherwise produce is what makes the comparison able to fail.
	originalVersion, originalCommit := version.Version, version.Commit
	t.Cleanup(func() {
		version.Version, version.Commit = originalVersion, originalCommit
	})
	version.Version = "9.9.9-under-test"
	version.Commit = "0ddba11"

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
	// Equality with the version package, not non-emptiness. This can be false: a handler that
	// hardcodes a string, drops a field or swaps version for commit fails it.
	if body.Version != version.Version {
		t.Errorf("version = %q, want %q (whatever the linker put there)", body.Version, version.Version)
	}
	if body.Commit != version.Commit {
		t.Errorf("commit = %q, want %q", body.Commit, version.Commit)
	}
	if _, err := time.Parse(time.RFC3339, body.Timestamp); err != nil {
		t.Errorf("timestamp %q is not RFC3339: %v", body.Timestamp, err)
	}
}
