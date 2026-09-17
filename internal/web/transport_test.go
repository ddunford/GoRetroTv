package web

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func testFrame() *image.Paletted {
	return image.NewPaletted(image.Rect(0, 0, FrameWidth, FrameHeight),
		color.Palette{color.Black, color.White})
}

func readMessage(t *testing.T, conn *websocket.Conn) map[string]json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	typ, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("message type = %v", typ)
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func field[T any](t *testing.T, msg map[string]json.RawMessage, name string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(msg[name], &value); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return value
}

func TestTransportSendsFullThenDirtyThenPaletteRefresh(t *testing.T) {
	transport := NewTransport()
	first := testFrame()
	if err := transport.PushFrame(first); err != nil {
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
	palette := readMessage(t, conn)
	if field[string](t, palette, "type") != "palette" || field[uint64](t, palette, "epoch") != 1 || len(field[[]byte](t, palette, "rgb")) != 768 {
		t.Fatalf("initial palette = %v", palette)
	}
	full := readMessage(t, conn)
	if field[string](t, full, "type") != "frame" || field[int](t, full, "w") != FrameWidth || field[int](t, full, "h") != FrameHeight || len(field[[]byte](t, full, "pixels")) != FrameWidth*FrameHeight {
		t.Fatalf("initial frame header = %v", full)
	}
	// PushFrame copies input. Mutating the caller's image after publication
	// cannot change the baseline against which the next update is measured.
	first.Pix[0] = 1
	second := testFrame()
	second.Pix[7*FrameWidth+5] = 1
	if err := transport.PushFrame(second); err != nil {
		t.Fatal(err)
	}
	dirty := readMessage(t, conn)
	if field[string](t, dirty, "type") != "frame" || field[int](t, dirty, "x") != 5 || field[int](t, dirty, "y") != 7 || field[int](t, dirty, "w") != 1 || field[int](t, dirty, "h") != 1 || field[[]byte](t, dirty, "pixels")[0] != 1 {
		t.Fatalf("dirty frame = %v", dirty)
	}
	third := testFrame()
	third.Pix[7*FrameWidth+5] = 1
	third.Palette[1] = color.RGBA{R: 255, A: 255}
	if err := transport.PushFrame(third); err != nil {
		t.Fatal(err)
	}
	palette = readMessage(t, conn)
	full = readMessage(t, conn)
	if field[string](t, palette, "type") != "palette" || field[uint64](t, palette, "epoch") != 2 || field[int](t, full, "w") != FrameWidth || field[int](t, full, "h") != FrameHeight {
		t.Fatalf("palette change did not refresh full frame: palette=%v frame=%v", palette, full)
	}
}

func TestTransportRejectsForeignBrowserOrigin(t *testing.T) {
	server := httptest.NewServer(NewTransport())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"),
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://other.example"}}})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin accepted: status=%v err=%v", response, err)
	}
}

func TestTransportRejectsIncompleteFrame(t *testing.T) {
	frame := testFrame()
	frame.Pix = frame.Pix[:10]
	if err := NewTransport().PushFrame(frame); err == nil {
		t.Fatal("incomplete frame accepted")
	}
}

func TestSlowClientCannotBlockPublisher(t *testing.T) {
	transport := NewTransport()
	slow := &client{latest: make(chan *frameData, 1)}
	transport.clients[slow] = struct{}{}
	first := testFrame()
	if err := transport.PushFrame(first); err != nil {
		t.Fatal(err)
	}
	second := testFrame()
	second.Pix[0] = 1
	done := make(chan error, 1)
	go func() { done <- transport.PushFrame(second) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow client blocked frame publication")
	}
	if latest := <-slow.latest; latest.seq != 1 {
		t.Fatalf("queued sequence = %d, want newest sequence 1", latest.seq)
	}
}
