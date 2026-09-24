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
	"github.com/ddunford/goretrotv/internal/device/csi"
	"github.com/ddunford/goretrotv/internal/wire"
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
	if field[string](t, palette, "type") != "palette" || field[uint64](t, palette, "epoch") != 1 ||
		len(field[[]byte](t, palette, "rgb")) != 768 || len(field[[]byte](t, palette, "alpha")) != 256 {
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

func TestTransportSendsGuestSelectedMediaToCurrentAndFutureClients(t *testing.T) {
	transport := NewTransport()
	transport.PushMedia(true, "Sky One", "Dream Team", "test-pattern")
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	message := readMessage(t, conn)
	if field[string](t, message, "type") != "media" || field[uint8](t, message, "active") != 1 ||
		field[string](t, message, "service") != "Sky One" ||
		field[string](t, message, "programme") != "Dream Team" ||
		field[string](t, message, "source") != "test-pattern" {
		t.Fatalf("initial media message = %v", message)
	}
	transport.PushMedia(false, "", "", "")
	message = readMessage(t, conn)
	if field[uint8](t, message, "active") != 0 {
		t.Fatalf("stopped media message = %v", message)
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

func TestBrowserKeyReachesCSILinkInInstructionLoop(t *testing.T) {
	transport := NewTransport()
	if err := transport.PushState("ready", "The guest acquired its services"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"key","version":3,"raw":125,"source":0}`)); err != nil {
		t.Fatal(err)
	}
	link := csi.New(nil)
	deadline := time.After(2 * time.Second)
	for link.Pending() == 0 {
		if err := transport.DrainKeys(link.Key); err != nil {
			t.Fatal(err)
		}
		select {
		case <-deadline:
			t.Fatal("socket key never reached CSI link")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if link.Pending() != len(csi.Encode([]byte{5, 0x80, 2, 0, 7, 0xd0})) {
		t.Fatalf("queued wire bytes = %d", link.Pending())
	}
	if err := transport.DrainKeys(link.Key); err != nil || link.Pending() != 8 {
		t.Fatalf("duplicate key delivery: pending=%d err=%v", link.Pending(), err)
	}
}

func TestTransportRejectsInvalidHandsetMessages(t *testing.T) {
	cases := []string{
		`{"type":"frame","version":3,"raw":125,"source":0}`,
		`{"type":"key","version":2,"raw":125,"source":0}`,
		`{"type":"key","version":3,"raw":125,"source":2}`,
		`{"type":"key","version":3,"raw":99,"source":0}`,
		`{"type":"key","version":3,"raw":256,"source":0}`,
		`{"type":"key","version":3,"raw":125,"source":0,"other":1}`,
		`{"type":"key","version":3,"raw":125,"source":0}{}`,
	}
	for _, payload := range cases {
		var key wire.KeyMessage
		if err := decodeKey([]byte(payload), &key); err == nil {
			t.Errorf("accepted %s", payload)
		}
	}
	for _, raw := range []uint8{0, 9, 0x3c, 0x58, 0x5c, 0x6d, 0x70, 0x7d, 0x7e, 0x80, 0xcc, 0xf5} {
		data, err := json.Marshal(wire.KeyMessage{Type: "key", Version: wire.Version, Raw: raw, Source: 0})
		if err != nil {
			t.Fatal(err)
		}
		var key wire.KeyMessage
		if err := decodeKey(data, &key); err != nil {
			t.Errorf("documented raw %02x rejected: %v", raw, err)
		}
	}
}

func TestTransportClosesSocketOnInvalidKey(t *testing.T) {
	transport := NewTransport()
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"key","version":3,"raw":125,"source":2}`)); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("invalid handset source close status = %v, want policy violation", err)
	}
	if len(transport.keys) != 0 {
		t.Fatal("rejected key entered input queue")
	}
}

func TestTransportPublishesLatestMachineStateOnConnectAndChange(t *testing.T) {
	transport := NewTransport()
	if err := transport.PushState("channel-list", "The guest is acquiring services"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	state := readMessage(t, conn)
	if field[string](t, state, "type") != "state" || field[string](t, state, "phase") != "channel-list" {
		t.Fatalf("initial guest state = %v", state)
	}
	if err := transport.PushState("halted", "guest instruction fault"); err != nil {
		t.Fatal(err)
	}
	state = readMessage(t, conn)
	if field[string](t, state, "phase") != "halted" || field[string](t, state, "reason") != "guest instruction fault" {
		t.Fatalf("halt state = %v", state)
	}
	if err := transport.PushState("finished", ""); err == nil {
		t.Fatal("invalid phase accepted")
	}
}

// dialTransport opens one browser-equivalent socket against the transport.
func dialTransport(t *testing.T, ctx context.Context, server *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body != nil {
		t.Cleanup(func() { response.Body.Close() })
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })
	return conn
}

// The halted box is the whole reason the reset exists, so the phase gate that
// refuses keys must not refuse a reset. A reset accepted only while the box
// already works is a control that cannot do its one job.
func TestTransportAcceptsResetWhileHaltedAndStillRefusesKeys(t *testing.T) {
	transport := NewTransport()
	if err := transport.PushState("halted", "guest instruction fault"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(transport)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn := dialTransport(t, ctx, server)
	readMessage(t, conn) // the halted state the transport replays on connect
	if transport.TakeReset() {
		t.Fatal("a reset was pending before the browser asked for one")
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"reset","version":3}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-transport.Resets():
	case <-ctx.Done():
		t.Fatal("reset sent while halted never reached the instruction loop")
	}

	// The same socket, the same phase: a key must still be refused, or the
	// reset would have opened a hole in the gate rather than an exception to it.
	keyed := dialTransport(t, ctx, server)
	readMessage(t, keyed)
	if err := keyed.Write(ctx, websocket.MessageText, []byte(`{"type":"key","version":3,"raw":125,"source":0}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := keyed.Read(ctx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("key accepted while halted: close status = %v", err)
	}
	if len(transport.keys) != 0 {
		t.Fatal("key entered the input queue while the box was halted")
	}
}

// A public unauthenticated control must cost one rebuild however hard it is
// pressed. The fold is silent by design: one restore satisfies every request
// inside the window, and the state push that follows tells all of them.
func TestTransportFoldsResetsInsideTheMinimumInterval(t *testing.T) {
	transport := NewTransport()
	transport.requestReset()
	transport.requestReset()
	if !transport.TakeReset() {
		t.Fatal("the first reset was not queued")
	}
	if transport.TakeReset() {
		t.Fatal("two presses with nothing drained between them queued two rebuilds")
	}

	// The queue is empty now, so anything the next press queues came past the
	// interval rather than past the channel's one slot. Without this drain the
	// test passes with no rate limit at all, which is how it was first written.
	transport.requestReset()
	if transport.TakeReset() {
		t.Fatal("a press inside the minimum interval queued a second rebuild")
	}

	// Once the interval has passed the next press is a real request again.
	transport.mu.Lock()
	transport.lastReset = time.Now().Add(-resetInterval - time.Millisecond)
	transport.mu.Unlock()
	transport.requestReset()
	if !transport.TakeReset() {
		t.Fatal("a reset after the interval was folded")
	}
}

func TestTransportRejectsMalformedResetsAndUnknownTypes(t *testing.T) {
	for _, payload := range []string{
		`{"type":"reset","version":2}`,
		`{"type":"reset","version":3,"raw":125}`,
		`{"type":"reset","version":3}{}`,
		`{"type":"restart","version":3}`,
	} {
		transport := NewTransport()
		server := httptest.NewServer(transport)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Errorf("accepted %s: close status = %v", payload, err)
		}
		if transport.TakeReset() {
			t.Errorf("%s queued a rebuild", payload)
		}
		conn.Close(websocket.StatusNormalClosure, "")
		if response.Body != nil {
			response.Body.Close()
		}
		cancel()
		server.Close()
	}
}
