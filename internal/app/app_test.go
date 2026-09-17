package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/ddunford/goretrotv/internal/app"
	"github.com/ddunford/goretrotv/internal/config"
	"github.com/ddunford/goretrotv/internal/web"
)

func TestPageServesOnlyBuiltAssets(t *testing.T) {
	root := t.TempDir()
	assets := map[string]string{
		"index.html": "<title>Sky box</title>", "styles.css": "body{color:blue}",
		"favicon.svg": "<svg></svg>", "dist/app.js": "import './wire.js'",
		"dist/wire.js": "export const wire = 1", "dist/wire_generated.js": "export const version = 1",
		"app.ts": "private TypeScript source",
	}
	for name, content := range assets {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{WebDir: root}
	transport := web.NewTransport()
	handler, err := app.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), transport)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	for path, want := range map[string]string{
		"/": "Sky box", "/styles.css": "body{color:blue}", "/dist/app.js": "import './wire.js'",
	} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
			t.Fatalf("GET %s: status=%d body=%q err=%v", path, response.StatusCode, body, err)
		}
	}
	for _, path := range []string{"/app.ts", "/missing.js", "/dist/missing.js", "/firmware/FLASH_U202.bin"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, response.StatusCode)
		}
	}
	if err := transport.PushState("ready", "ready"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		t.Fatalf("middleware blocked WebSocket upgrade: status=%v err=%v", response, err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

func TestMissingBuiltAssetPreventsServerStart(t *testing.T) {
	_, err := app.New(&config.Config{WebDir: t.TempDir()}, slog.Default(), web.NewTransport())
	if err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("missing built page accepted: %v", err)
	}
}
