// Package web carries the emulator's indexed framebuffer to browser clients.
// Socket I/O stays outside the deterministic guest instruction loop.
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/ddunford/goretrotv/internal/wire"
)

// FrameWidth and FrameHeight are the Digibox display raster from the shared
// browser wire schema.
const (
	FrameWidth    = wire.FrameWidth
	FrameHeight   = wire.FrameHeight
	framePeriod   = 100 * time.Millisecond
	writeLimit    = 10 * time.Second
	keyQueueLimit = 64
)

type frameData struct {
	seq     uint64
	epoch   uint64
	pixels  []byte
	palette []byte
}

type client struct{ latest chan *frameData }

// Transport broadcasts indexed OSD frames. A slow browser receives the newest
// image when it catches up; its dirty rectangle is calculated against the last
// image it actually received, so dropped updates cannot corrupt its canvas.
type Transport struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	latest  *frameData
	keys    chan wire.KeyMessage
}

// NewTransport creates a framebuffer broadcaster.
func NewTransport() *Transport {
	return &Transport{clients: make(map[*client]struct{}), keys: make(chan wire.KeyMessage, keyQueueLimit)}
}

// DrainKeys hands queued browser input to the caller's instruction loop. The
// socket reader never mutates a guest device, and FIFO order is preserved.
func (t *Transport) DrainKeys(send func(raw, source uint8) error) error {
	for {
		select {
		case key := <-t.keys:
			if err := send(key.Raw, key.Source); err != nil {
				return fmt.Errorf("web: deliver key: %w", err)
			}
		default:
			return nil
		}
	}
}

// PushFrame copies one full compositor image and makes it available to clients.
// Callers retain ownership of image memory and may change it immediately after
// this returns. Client writes run independently of the caller.
func (t *Transport) PushFrame(frame *image.Paletted) error {
	if frame == nil || frame.Rect != image.Rect(0, 0, FrameWidth, FrameHeight) || len(frame.Palette) > 256 {
		return fmt.Errorf("web: expected a 720x576 indexed frame with at most 256 palette entries")
	}
	if frame.Stride < FrameWidth || len(frame.Pix) < (FrameHeight-1)*frame.Stride+FrameWidth {
		return fmt.Errorf("web: incomplete indexed frame")
	}
	pixels := make([]byte, FrameWidth*FrameHeight)
	for y := 0; y < FrameHeight; y++ {
		copy(pixels[y*FrameWidth:(y+1)*FrameWidth], frame.Pix[y*frame.Stride:y*frame.Stride+FrameWidth])
	}
	palette := make([]byte, 256*3)
	for i, colour := range frame.Palette {
		if colour == nil {
			return fmt.Errorf("web: palette entry %d has no colour", i)
		}
		rgb := color.RGBAModel.Convert(colour).(color.RGBA)
		palette[i*3], palette[i*3+1], palette[i*3+2] = rgb.R, rgb.G, rgb.B
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.latest != nil && bytes.Equal(t.latest.pixels, pixels) && bytes.Equal(t.latest.palette, palette) {
		return nil
	}
	next := &frameData{pixels: pixels, palette: palette}
	if t.latest != nil {
		next.seq = t.latest.seq + 1
		next.epoch = t.latest.epoch
	}
	if t.latest == nil || !bytes.Equal(t.latest.palette, palette) {
		next.epoch++
	}
	t.latest = next
	for c := range t.clients {
		select {
		case c.latest <- next:
		default:
			select {
			case <-c.latest:
			default: // The writer drained the slot between the two checks.
			}
			select {
			case c.latest <- next:
			default: // The writer already has a newer pending frame.
			}
		}
	}
	return nil
}

// ServeHTTP upgrades one browser connection. The library's default same-origin
// check rejects cross-site browser handshakes; no wildcard origin is configured.
func (t *Transport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Preserve request values, but not its cancellation: an upgraded request's
	// context is not a reliable lifetime for the WebSocket connection.
	baseCtx := context.WithoutCancel(r.Context())
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept writes the HTTP error response.
	}
	sessionCtx, cancel := context.WithCancel(baseCtx)
	defer func() {
		cancel()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	conn.SetReadLimit(1024)
	c := &client{latest: make(chan *frameData, 1)}
	t.mu.Lock()
	t.clients[c] = struct{}{}
	if t.latest != nil {
		c.latest <- t.latest
	}
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.clients, c)
		t.mu.Unlock()
	}()
	disconnected := make(chan struct{})
	go func(ctx context.Context) {
		defer close(disconnected)
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if typ != websocket.MessageText {
				_ = conn.Close(websocket.StatusUnsupportedData, "key must be text")
				return
			}
			var key wire.KeyMessage
			if err := decodeKey(data, &key); err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid key")
				return
			}
			select {
			case t.keys <- key:
			default:
				_ = conn.Close(websocket.StatusPolicyViolation, "key queue full")
				return
			}
		}
	}(sessionCtx)
	var last *frameData
	ticker := time.NewTicker(framePeriod)
	defer ticker.Stop()
	var pending *frameData
	for {
		select {
		case <-disconnected:
			return
		case pending = <-c.latest:
			if last == nil {
				if err := writeFrame(sessionCtx, conn, last, pending); err != nil {
					return
				}
				last, pending = pending, nil
			}
		case <-ticker.C:
			if pending != nil {
				if err := writeFrame(sessionCtx, conn, last, pending); err != nil {
					return
				}
				last, pending = pending, nil
			}
		}
	}
}

func decodeKey(data []byte, key *wire.KeyMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(key); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("multiple JSON values")
	} else if err != io.EOF {
		return err
	}
	if key.Type != "key" || key.Version != wire.Version || key.Source != 0 || !handsetRaw(key.Raw) {
		return fmt.Errorf("unsupported handset key")
	}
	return nil
}

func handsetRaw(raw uint8) bool {
	if raw <= 9 || raw >= 0x6d && raw <= 0x70 {
		return true
	}
	switch raw {
	case 0x3c, 0x58, 0x59, 0x5a, 0x5b, 0x5c, 0x7d, 0x80, 0xcc, 0xf5:
		return true
	}
	return false
}

func writeFrame(ctx context.Context, conn *websocket.Conn, previous, current *frameData) error {
	if previous == nil || previous.epoch != current.epoch {
		if err := writeJSON(ctx, conn, wire.PaletteMessage{Type: "palette", Version: wire.Version, Epoch: current.epoch, RGB: current.palette}); err != nil {
			return err
		}
	}
	rect := dirtyRect(previous, current)
	if rect.Empty() {
		return nil
	}
	pixels := make([]byte, rect.Dx()*rect.Dy())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		copy(pixels[(y-rect.Min.Y)*rect.Dx():(y-rect.Min.Y+1)*rect.Dx()],
			current.pixels[y*FrameWidth+rect.Min.X:y*FrameWidth+rect.Max.X])
	}
	return writeJSON(ctx, conn, wire.FrameMessage{Type: "frame", Version: wire.Version, Seq: current.seq,
		X: uint32(rect.Min.X), Y: uint32(rect.Min.Y), W: uint32(rect.Dx()), H: uint32(rect.Dy()), // #nosec G115 -- frame rectangle is within 720x576.
		Epoch: current.epoch, Pixels: pixels})
}

func dirtyRect(previous, current *frameData) image.Rectangle {
	if previous == nil || previous.epoch != current.epoch {
		return image.Rect(0, 0, FrameWidth, FrameHeight)
	}
	left, top, right, bottom := FrameWidth, FrameHeight, 0, 0
	for y := 0; y < FrameHeight; y++ {
		for x := 0; x < FrameWidth; x++ {
			at := y*FrameWidth + x
			if previous.pixels[at] == current.pixels[at] {
				continue
			}
			if x < left {
				left = x
			}
			if y < top {
				top = y
			}
			if x+1 > right {
				right = x + 1
			}
			if y+1 > bottom {
				bottom = y + 1
			}
		}
	}
	return image.Rect(left, top, right, bottom)
}

func writeJSON(ctx context.Context, conn *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, writeLimit)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, data)
}
