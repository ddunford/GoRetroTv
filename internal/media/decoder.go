// Package media owns host-side MPEG decoding at the edge of the emulator.
// It never mutates guest state: its only input is transport already admitted by the guest-programmed
// demux PIDs, and its decoded outputs are consumed by presentation code outside the instruction loop.
package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	// VideoWidth is the decoded SD raster width announced by the test service.
	VideoWidth = 352
	// VideoHeight is the decoded SD raster height announced by the test service.
	VideoHeight = 288
	// VideoBytes is one RGBA frame.
	VideoBytes = VideoWidth * VideoHeight * 4
	// AudioSampleRate is the decoded PCM rate announced by the test service.
	AudioSampleRate = 48_000
	// AudioChannels is the decoded stereo channel count.
	AudioChannels = 2
	audioSamples  = 1152
	// AudioBytes is one 1152-sample stereo s16le chunk.
	AudioBytes       = audioSamples * AudioChannels * 2
	defaultQueueSize = 8
	maxInputChunk    = 2 * 1024 * 1024
)

var (
	// ErrClosed reports an enqueue attempted after the decoder input was finished or closed.
	ErrClosed = errors.New("media decoder is closed")
	// ErrQueueFull reports that an instruction-loop input or decoded output could not enter its
	// bounded queue. The decoder stops rather than silently discard programme data.
	ErrQueueFull = errors.New("media decoder queue is full")
)

// Config controls bounded host queues. Zero values select the production defaults.
type Config struct {
	Executable string
	InputQueue int
	VideoQueue int
	AudioQueue int
}

// VideoFrame is one decoded RGBA frame in display order.
type VideoFrame struct {
	Sequence uint64
	RGBA     []byte
}

// AudioChunk is one MPEG Layer II frame's worth of signed little-endian stereo PCM.
type AudioChunk struct {
	Sequence uint64
	PCM      []byte
}

// Decoder supervises one ffmpeg process. All blocking pipe work stays in edge goroutines; callers
// on the instruction loop only perform bounded, non-blocking queue operations.
type Decoder struct {
	input  chan []byte
	video  chan VideoFrame
	audio  chan AudioChunk
	done   chan struct{}
	cancel context.CancelFunc

	finishOnce sync.Once
	mu         sync.Mutex
	fault      error
	closing    bool
	finished   bool
	stderr     bytes.Buffer
}

// Start launches ffmpeg with separate raw-video and PCM pipes. No shell is involved and the
// executable is resolved before the process starts.
func Start(parent context.Context, config Config) (*Decoder, error) {
	executable := config.Executable
	if executable == "" {
		var err error
		executable, err = exec.LookPath("ffmpeg")
		if err != nil {
			return nil, fmt.Errorf("media: find ffmpeg: %w", err)
		}
	} else {
		resolved, err := exec.LookPath(executable)
		if err != nil {
			return nil, fmt.Errorf("media: find ffmpeg %q: %w", executable, err)
		}
		executable = resolved
	}
	inputSize, err := queueSize("input", config.InputQueue)
	if err != nil {
		return nil, err
	}
	videoSize, err := queueSize("video", config.VideoQueue)
	if err != nil {
		return nil, err
	}
	audioSize, err := queueSize("audio", config.AudioQueue)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(parent)
	d := &Decoder{input: make(chan []byte, inputSize), video: make(chan VideoFrame, videoSize),
		audio: make(chan AudioChunk, audioSize), done: make(chan struct{}), cancel: cancel}
	videoR, videoW, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("media: video pipe: %w", err)
	}
	audioR, audioW, err := os.Pipe()
	if err != nil {
		cancel()
		_ = videoR.Close()
		_ = videoW.Close()
		return nil, fmt.Errorf("media: audio pipe: %w", err)
	}
	cmd := exec.CommandContext(ctx, executable, // #nosec G204 -- executable is resolved directly, with no shell
		"-v", "error", "-nostdin", "-fflags", "+discardcorrupt", "-f", "mpegts", "-i", "pipe:0",
		"-map", "0:v:0", "-pix_fmt", "rgba", "-f", "rawvideo", "pipe:3",
		"-map", "0:a:0", "-ac", "2", "-ar", "48000", "-f", "s16le", "pipe:4")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		closePipes(videoR, videoW, audioR, audioW)
		return nil, fmt.Errorf("media: ffmpeg stdin: %w", err)
	}
	cmd.ExtraFiles = []*os.File{videoW, audioW}
	cmd.Stderr = &d.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		closePipes(videoR, videoW, audioR, audioW)
		return nil, fmt.Errorf("media: start ffmpeg: %w", err)
	}
	_ = videoW.Close()
	_ = audioW.Close()

	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		defer func() { _ = videoR.Close() }()
		d.readFrames(videoR)
	}()
	go func() {
		defer readers.Done()
		defer func() { _ = audioR.Close() }()
		d.readAudio(audioR)
	}()
	go d.writeInput(stdin)
	go func() {
		err := cmd.Wait()
		readers.Wait()
		d.mu.Lock()
		closing := d.closing
		d.mu.Unlock()
		if err != nil && !closing {
			d.setFault(fmt.Errorf("media: ffmpeg exited: %w%s", err, d.stderrSuffix()))
		}
		close(d.video)
		close(d.audio)
		close(d.done)
	}()
	return d, nil
}

func queueSize(name string, size int) (int, error) {
	if size < 0 {
		return 0, fmt.Errorf("media: %s queue size %d is negative", name, size)
	}
	if size == 0 {
		return defaultQueueSize, nil
	}
	return size, nil
}

func closePipes(pipes ...*os.File) {
	for _, pipe := range pipes {
		_ = pipe.Close()
	}
}

func (d *Decoder) stderrSuffix() string {
	message := strings.TrimSpace(d.stderr.String())
	if message == "" {
		return ""
	}
	return ": " + message
}

func (d *Decoder) writeInput(stdin io.WriteCloser) {
	defer func() { _ = stdin.Close() }()
	for data := range d.input {
		if _, err := stdin.Write(data); err != nil {
			d.setFault(fmt.Errorf("media: write ffmpeg transport: %w", err))
			return
		}
	}
}

func (d *Decoder) readFrames(src io.Reader) {
	for sequence := uint64(0); ; sequence++ {
		frame := make([]byte, VideoBytes)
		if _, err := io.ReadFull(src, frame); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				d.setFault(fmt.Errorf("media: read ffmpeg video: %w", err))
			}
			return
		}
		select {
		case d.video <- VideoFrame{Sequence: sequence, RGBA: frame}:
		default:
			d.setFault(fmt.Errorf("media: video: %w", ErrQueueFull))
			return
		}
	}
}

func (d *Decoder) readAudio(src io.Reader) {
	for sequence := uint64(0); ; sequence++ {
		pcm := make([]byte, AudioBytes)
		if _, err := io.ReadFull(src, pcm); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				d.setFault(fmt.Errorf("media: read ffmpeg audio: %w", err))
			}
			return
		}
		select {
		case d.audio <- AudioChunk{Sequence: sequence, PCM: pcm}:
		default:
			d.setFault(fmt.Errorf("media: audio: %w", ErrQueueFull))
			return
		}
	}
}

func (d *Decoder) setFault(err error) {
	d.mu.Lock()
	if d.fault == nil && !d.closing {
		d.fault = err
		d.cancel()
	}
	d.mu.Unlock()
}

// Enqueue copies one admitted transport burst without blocking the instruction loop.
func (d *Decoder) Enqueue(data []byte) error {
	if len(data) == 0 || len(data) > maxInputChunk || len(data)%188 != 0 {
		return fmt.Errorf("media: invalid transport chunk length %d", len(data))
	}
	copyOfData := append([]byte(nil), data...)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fault != nil {
		return d.fault
	}
	if d.finished || d.closing {
		return ErrClosed
	}
	select {
	case d.input <- copyOfData:
		return nil
	default:
		err := fmt.Errorf("media: input: %w", ErrQueueFull)
		d.fault = err
		d.cancel()
		return err
	}
}

// Finish closes the transport input after all queued bursts. It is safe to call more than once.
func (d *Decoder) Finish() {
	d.finishOnce.Do(func() {
		d.mu.Lock()
		d.finished = true
		close(d.input)
		d.mu.Unlock()
	})
}

// Video returns decoded frames in display order.
func (d *Decoder) Video() <-chan VideoFrame { return d.video }

// Audio returns decoded PCM chunks in playback order.
func (d *Decoder) Audio() <-chan AudioChunk { return d.audio }

// Done closes after ffmpeg and both output readers have stopped.
func (d *Decoder) Done() <-chan struct{} { return d.done }

// Fault reports the first subprocess or queue failure.
func (d *Decoder) Fault() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fault
}

// Close terminates the subprocess and waits for every pipe goroutine to finish.
func (d *Decoder) Close() error {
	d.mu.Lock()
	d.closing = true
	d.mu.Unlock()
	d.Finish()
	d.cancel()
	<-d.done
	if err := d.Fault(); err != nil {
		return err
	}
	return nil
}
