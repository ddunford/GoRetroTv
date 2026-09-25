package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const transportChunk = 188 * 32

// Encoder supervises the conversion of one programme timeline into the MPEG-2/MP2 transport the
// guest has requested. Its output queue is bounded and packet aligned.
type Encoder struct {
	packets chan []byte
	done    chan struct{}
	cancel  context.CancelFunc
	mu      sync.Mutex
	fault   error
	closing bool
	stderr  bytes.Buffer
}

// StartEncoder starts at the live programme-relative offset and emits at most limit of playout.
func StartEncoder(parent context.Context, ffmpeg string, playlist Playlist, elapsed, limit time.Duration) (*Encoder, error) {
	executable, err := exec.LookPath(ffmpeg)
	if err != nil {
		return nil, fmt.Errorf("media: find ffmpeg: %w", err)
	}
	position, err := playlist.At(elapsed)
	if err != nil {
		return nil, err
	}
	script, err := concatScript(playlist, position)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	e := &Encoder{packets: make(chan []byte, defaultQueueSize), done: make(chan struct{}), cancel: cancel}
	args := []string{"-v", "error", "-nostdin", "-re"}
	if playlist.Loop {
		args = append(args, "-stream_loop", "-1")
	}
	args = append(args, "-ss", durationArgument(position.Offset), "-f", "concat", "-safe", "0", "-i", script,
		"-map", "0:v:0", "-map", "0:a:0", "-vf", "scale=352:288:force_original_aspect_ratio=decrease,pad=352:288:(ow-iw)/2:(oh-ih)/2",
		"-r", "25", "-c:v", "mpeg2video", "-profile:v", "main", "-level:v", "main", "-g", "12", "-bf", "0",
		"-c:a", "mp2", "-ar", "48000", "-ac", "2", "-b:a", "192k", "-mpegts_service_id", "100",
		"-streamid", "0:257", "-streamid", "1:258")
	if limit > 0 {
		args = append(args, "-t", durationArgument(limit))
	}
	args = append(args, "-f", "mpegts", "pipe:1")
	cmd := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- resolved executable, fixed arguments and prevalidated paths
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("media: ffmpeg transport pipe: %w", err)
	}
	cmd.Stderr = &e.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.Remove(script)
		return nil, fmt.Errorf("media: start programme encoder: %w", err)
	}
	go func() {
		defer func() { _ = os.Remove(script) }()
		defer close(e.packets)
		defer close(e.done)
	readLoop:
		for {
			chunk := make([]byte, transportChunk)
			n, readErr := io.ReadFull(stdout, chunk)
			if n > 0 {
				if n%188 != 0 {
					e.setEncoderFault(fmt.Errorf("media: ffmpeg emitted %d trailing bytes outside a transport packet", n))
					break
				}
				select {
				case e.packets <- chunk[:n]:
				case <-ctx.Done():
					break readLoop
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
					e.setEncoderFault(fmt.Errorf("media: read encoded transport: %w", readErr))
				}
				break
			}
		}
		waitErr := cmd.Wait()
		e.mu.Lock()
		closing := e.closing
		e.mu.Unlock()
		if waitErr != nil && !closing && e.Fault() == nil {
			e.setEncoderFault(fmt.Errorf("media: programme encoder exited: %w: %s", waitErr, strings.TrimSpace(e.stderr.String())))
		}
	}()
	return e, nil
}

func durationArgument(value time.Duration) string {
	return strconv.FormatFloat(value.Seconds(), 'f', 6, 64)
}

func concatScript(playlist Playlist, position Position) (string, error) {
	start := -1
	for i := range playlist.Items {
		if playlist.Items[i].Path == position.Item.Path {
			start = i
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("media: live item is absent from playlist")
	}
	items := append([]Item(nil), playlist.Items[start:]...)
	if playlist.Loop {
		items = append(items, playlist.Items[:start]...)
	}
	var body strings.Builder
	body.WriteString("ffconcat version 1.0\n")
	for _, item := range items {
		if strings.ContainsAny(item.Path, "\r\n") {
			return "", fmt.Errorf("media: filename contains a newline")
		}
		quoted := strings.ReplaceAll(item.Path, "'", "'\\''")
		fmt.Fprintf(&body, "file '%s'\nduration %s\n", quoted, durationArgument(item.Duration))
	}
	file, err := os.CreateTemp("", "goretrotv-*.ffconcat")
	if err != nil {
		return "", fmt.Errorf("media: create playlist: %w", err)
	}
	name := file.Name()
	if _, err := file.WriteString(body.String()); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("media: write playlist: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("media: close playlist: %w", err)
	}
	return name, nil
}

func (e *Encoder) setEncoderFault(err error) {
	e.mu.Lock()
	if e.fault == nil && !e.closing {
		e.fault = err
		e.cancel()
	}
	e.mu.Unlock()
}

// Packets returns packet-aligned encoded transport chunks.
func (e *Encoder) Packets() <-chan []byte { return e.packets }

// Done closes when ffmpeg and the output reader stop.
func (e *Encoder) Done() <-chan struct{} { return e.done }

// Fault reports the first process, pipe or backpressure failure.
func (e *Encoder) Fault() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.fault
}

// Close terminates the encoder and waits for its pipe reader.
func (e *Encoder) Close() error {
	e.mu.Lock()
	e.closing = true
	e.mu.Unlock()
	e.cancel()
	<-e.done
	return e.Fault()
}

// BaseName returns the operator-facing name of an item without exposing its host path.
func (i Item) BaseName() string { return filepath.Base(i.Path) }
