package web

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// This fixture comes from the actual WebSocket writer, through an HTTP upgrade.
// WIRE_CAPTURE_PATH allows an intentional refresh after a wire version change.
func TestCapturedWireFixture(t *testing.T) {
	transport := NewTransport()
	if err := transport.PushFrame(testFrame()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 20)
	read := func() []byte {
		t.Helper()
		typ, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if typ != websocket.MessageText {
			t.Fatalf("message type %v is not text", typ)
		}
		return data
	}
	palette := read()
	_ = read() // The initial full frame makes the following message a dirty rectangle.
	frame := testFrame()
	frame.Pix[7*FrameWidth+5] = 1
	if err := transport.PushFrame(frame); err != nil {
		t.Fatal(err)
	}
	dirty := read()
	captured := append(append(append([]byte{}, palette...), '\n'), dirty...)
	captured = append(captured, '\n')
	if path := os.Getenv("WIRE_CAPTURE_PATH"); path != "" {
		if err := os.WriteFile(path, captured, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	fixture, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "wire.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(captured, fixture) {
		t.Fatal("captured Go WebSocket bytes differ from committed wire fixture")
	}
}
