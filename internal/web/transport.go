// Package web carries the emulator's indexed framebuffer to browser clients.
// Socket I/O stays outside the deterministic guest instruction loop.
package web

import (
	"bytes"
	"context"
	"encoding/binary"
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
	// resetInterval is the shortest gap between two restores. The box is public
	// and unauthenticated, so a held-down reset must cost one rebuild rather
	// than one per message.
	resetInterval = 3 * time.Second
)

type frameData struct {
	seq     uint64
	epoch   uint64
	pixels  []byte
	palette []byte
	alpha   []byte
}

type client struct {
	latest chan *frameData
	state  chan wire.StateMessage
	media  chan wire.MediaMessage
	video  chan []byte
	audio  chan []byte
}

// offerLatest never waits for a concurrent consumer. A plain "default, receive old, send new"
// sequence has two races: the consumer can empty the channel before the receive, or accept a value
// before the replacement send. Either turns a lossy presentation queue into a blocking operation
// on the emulator's instruction-loop goroutine.
func offerLatest[T any](queue chan T, value T) {
	select {
	case queue <- value:
		return
	default:
	}
	select {
	case <-queue:
	default:
	}
	select {
	case queue <- value:
	default:
	}
}

const mediaWireHeader = 16

// PushVideo publishes the latest decoded 352x288 RGBA frame. Slow viewers skip frames rather than
// holding up the emulated receiver.
func (t *Transport) PushVideo(sequence uint64, rgba []byte) {
	t.pushBinary(1, sequence, rgba, true)
}

// PushAudio publishes one 48 kHz stereo signed-16 PCM chunk at the guest instruction which
// released it for presentation. The timestamp is the emulator's clock; decoder sequence numbers
// depend on host scheduling and therefore cannot be the browser's ordering authority.
func (t *Transport) PushAudio(instruction uint64, pcm []byte) {
	t.pushBinary(2, instruction, pcm, false)
}

func (t *Transport) pushBinary(kind byte, sequence uint64, payload []byte, latest bool) {
	message := make([]byte, mediaWireHeader+len(payload))
	copy(message, "GRTV")
	message[4], message[5] = 2, kind
	binary.BigEndian.PutUint64(message[8:16], sequence)
	copy(message[mediaWireHeader:], payload)
	t.mu.Lock()
	defer t.mu.Unlock()
	for c := range t.clients {
		queue := c.audio
		if latest {
			queue = c.video
		}
		offerLatest(queue, message)
	}
}

// Transport broadcasts indexed OSD frames. A slow browser receives the newest
// image when it catches up; its dirty rectangle is calculated against the last
// image it actually received, so dropped updates cannot corrupt its canvas.
type Transport struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	latest  *frameData
	keys    chan wire.KeyMessage
	// resets holds at most one pending rebuild. It is a channel rather than a
	// flag so a loop with no machine left to step can block on it.
	resets    chan struct{}
	lastReset time.Time
	state     *wire.StateMessage
	media     *wire.MediaMessage
}

// PushMedia publishes the guest's measured MPEG-service selection. It carries no host control:
// the instruction loop calls it only when the real firmware executes its MPEG callback.
func (t *Transport) PushMedia(requested, active bool, service, programme, source string) {
	next := wire.MediaMessage{Type: "media", Version: wire.Version, Service: service,
		Programme: programme, Source: source}
	if requested {
		next.Requested = 1
	}
	if active {
		next.Active = 1
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.media != nil && *t.media == next {
		return
	}
	t.media = &next
	for c := range t.clients {
		offerLatest(c.media, next)
	}
}

// PushState publishes the latest observed machine phase to current and future
// browsers. The caller derives it from guest state, never elapsed wall time.
func (t *Transport) PushState(phase, reason string) error {
	switch phase {
	case "booting", "flash-check", "channel-list", "ready", "halted":
	default:
		return fmt.Errorf("web: unsupported machine phase %q", phase)
	}
	next := wire.StateMessage{Type: "state", Version: wire.Version, Phase: phase, Reason: reason}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != nil && *t.state == next {
		return nil
	}
	t.state = &next
	for c := range t.clients {
		offerLatest(c.state, next)
	}
	return nil
}

// NewTransport creates a framebuffer broadcaster.
func NewTransport() *Transport {
	return &Transport{clients: make(map[*client]struct{}), keys: make(chan wire.KeyMessage, keyQueueLimit),
		resets: make(chan struct{}, 1)}
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

// requestReset queues one rebuild for the instruction loop, folding a second
// request inside resetInterval into the first. The wall clock is the right
// clock here and only here: this rate-limits a socket, not the guest, whose
// only clock is the instruction counter.
func (t *Transport) requestReset() {
	t.mu.Lock()
	now := time.Now()
	if !t.lastReset.IsZero() && now.Sub(t.lastReset) < resetInterval {
		t.mu.Unlock()
		return
	}
	t.lastReset = now
	t.mu.Unlock()
	select {
	case t.resets <- struct{}{}:
	default: // One rebuild already pending; it satisfies this request too.
	}
}

// TakeReset reports whether a browser has asked for the box to be rebuilt.
// The instruction loop calls it at a safe point, because Runtime admits no
// owner but that loop.
func (t *Transport) TakeReset() bool {
	select {
	case <-t.resets:
		return true
	default:
		return false
	}
}

// Resets lets a loop with no machine to step wait for a rebuild. A halted box
// is the state the reset control exists to leave, so that wait is the point.
func (t *Transport) Resets() <-chan struct{} { return t.resets }

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
	alpha := make([]byte, 256)
	for i, colour := range frame.Palette {
		if colour == nil {
			return fmt.Errorf("web: palette entry %d has no colour", i)
		}
		rgb := color.RGBAModel.Convert(colour).(color.RGBA)
		palette[i*3], palette[i*3+1], palette[i*3+2] = rgb.R, rgb.G, rgb.B
		alpha[i] = rgb.A
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.latest != nil && bytes.Equal(t.latest.pixels, pixels) && bytes.Equal(t.latest.palette, palette) && bytes.Equal(t.latest.alpha, alpha) {
		return nil
	}
	next := &frameData{pixels: pixels, palette: palette, alpha: alpha}
	if t.latest != nil {
		next.seq = t.latest.seq + 1
		next.epoch = t.latest.epoch
	}
	if t.latest == nil || !bytes.Equal(t.latest.palette, palette) || !bytes.Equal(t.latest.alpha, alpha) {
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
	c := &client{latest: make(chan *frameData, 1), state: make(chan wire.StateMessage, 1),
		media: make(chan wire.MediaMessage, 1), video: make(chan []byte, 1), audio: make(chan []byte, 64)}
	t.mu.Lock()
	t.clients[c] = struct{}{}
	if t.latest != nil {
		c.latest <- t.latest
	}
	if t.state != nil {
		c.state <- *t.state
	}
	if t.media != nil {
		c.media <- *t.media
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
			kind, err := clientMessageType(data)
			if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid client message")
				return
			}
			if kind == "reset" {
				if err := decodeReset(data); err != nil {
					_ = conn.Close(websocket.StatusPolicyViolation, "invalid reset")
					return
				}
				// Accepted in EVERY phase on purpose. The halted box is the one
				// case this control exists for, and the phase gate below would
				// refuse it exactly there.
				t.requestReset()
				continue
			}
			var key wire.KeyMessage
			if err := decodeKey(data, &key); err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid key")
				return
			}
			t.mu.Lock()
			accepting := t.state != nil && t.state.Phase == "ready"
			t.mu.Unlock()
			if !accepting {
				_ = conn.Close(websocket.StatusPolicyViolation, "box is not ready")
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
		case state := <-c.state:
			if err := writeJSON(sessionCtx, conn, state); err != nil {
				return
			}
		case media := <-c.media:
			if err := writeJSON(sessionCtx, conn, media); err != nil {
				return
			}
		case video := <-c.video:
			if err := conn.Write(sessionCtx, websocket.MessageBinary, video); err != nil {
				return
			}
		case audio := <-c.audio:
			if err := conn.Write(sessionCtx, websocket.MessageBinary, audio); err != nil {
				return
			}
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

// clientMessageType reads only the discriminator, and permissively, because
// the strict decoders reject unknown fields and a key's fields are unknown to
// a reset.
func clientMessageType(data []byte) (string, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	return envelope.Type, nil
}

// decodeStrict reads exactly one JSON object, rejecting unknown fields and
// trailing values. Both browser messages share it so neither can drift into
// accepting something the other refuses.
func decodeStrict(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("multiple JSON values")
	} else if err != io.EOF {
		return err
	}
	return nil
}

func decodeKey(data []byte, key *wire.KeyMessage) error {
	if err := decodeStrict(data, key); err != nil {
		return err
	}
	if key.Type != "key" || key.Version != wire.Version || key.Source != 0 || !handsetRaw(key.Raw) {
		return fmt.Errorf("unsupported handset key")
	}
	return nil
}

// decodeReset accepts the browser's request to rebuild the box. It carries no
// guest-visible payload, so its whole validation is type and wire version.
func decodeReset(data []byte) error {
	var reset wire.ResetMessage
	if err := decodeStrict(data, &reset); err != nil {
		return err
	}
	if reset.Type != "reset" || reset.Version != wire.Version {
		return fmt.Errorf("unsupported reset request")
	}
	return nil
}

func handsetRaw(raw uint8) bool {
	if raw <= 9 || raw >= 0x6d && raw <= 0x70 {
		return true
	}
	switch raw {
	// 0x7e is the Sky menu's SERVICES tab, measured against frame 64AF0A8D from
	// the post-acquisition snapshot and tuned-state handset measurements. Keep
	// this aligned with the controls rendered by web/index.html: an omitted real
	// key closes the viewer's socket with "invalid key" instead of reaching CSI.
	case 0x0c, 0x20, 0x21, 0x3c, 0x58, 0x59, 0x5a, 0x5b, 0x5c,
		0x7d, 0x7e, 0x80, 0x81, 0x83, 0xcb, 0xcc, 0xf5:
		return true
	}
	return false
}

func writeFrame(ctx context.Context, conn *websocket.Conn, previous, current *frameData) error {
	if previous == nil || previous.epoch != current.epoch {
		if err := writeJSON(ctx, conn, wire.PaletteMessage{Type: "palette", Version: wire.Version, Epoch: current.epoch, RGB: current.palette, Alpha: current.alpha}); err != nil {
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
