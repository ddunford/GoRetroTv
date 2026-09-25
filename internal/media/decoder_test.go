package media_test

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/media"
)

func TestDecoderRejectsAnUnresolvableExecutable(t *testing.T) {
	t.Parallel()
	_, err := media.Start(context.Background(), media.Config{Executable: "goretrotv-no-such-ffmpeg"})
	if err == nil {
		t.Fatal("Start accepted a missing ffmpeg executable")
	}
}

func TestDecoderProducesBoundedVideoAndAudio(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for the decoder acceptance test")
	}
	transport := generateTransport(t, ffmpeg)
	decoder, err := media.Start(context.Background(), media.Config{Executable: ffmpeg, VideoQueue: 32, AudioQueue: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	if err := decoder.Enqueue(transport); err != nil {
		t.Fatal(err)
	}
	decoder.Finish()

	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	var frame media.VideoFrame
	var audio media.AudioChunk
	for frame.RGBA == nil || audio.PCM == nil {
		select {
		case got, ok := <-decoder.Video():
			if ok && frame.RGBA == nil {
				frame = got
			}
		case got, ok := <-decoder.Audio():
			if ok && audio.PCM == nil {
				audio = got
			}
		case <-decoder.Done():
			if err := decoder.Fault(); err != nil {
				t.Fatal(err)
			}
			if frame.RGBA == nil || audio.PCM == nil {
				t.Fatalf("ffmpeg ended before both outputs: video=%d audio=%d", len(frame.RGBA), len(audio.PCM))
			}
		case <-timeout.C:
			t.Fatalf("timed out waiting for ffmpeg: %v", decoder.Fault())
		}
	}
	if len(frame.RGBA) != media.VideoBytes || len(audio.PCM) != media.AudioBytes {
		t.Fatalf("decoded sizes video=%d audio=%d", len(frame.RGBA), len(audio.PCM))
	}
	if bytes.Equal(frame.RGBA, make([]byte, len(frame.RGBA))) {
		t.Fatal("decoded video is an all-zero placeholder")
	}
}

func TestDecoderMakesOutputBackpressureVisible(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for the decoder backpressure test")
	}
	decoder, err := media.Start(context.Background(), media.Config{Executable: ffmpeg, VideoQueue: 1, AudioQueue: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := decoder.Enqueue(generateTransport(t, ffmpeg)); err != nil {
		t.Fatal(err)
	}
	decoder.Finish()
	select {
	case <-decoder.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("decoder did not terminate after bounded output queue filled")
	}
	if err := decoder.Fault(); !errors.Is(err, media.ErrQueueFull) {
		t.Fatalf("fault = %v, want ErrQueueFull", err)
	}
}

func TestDecoderRefusesInputAfterFinish(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for the decoder lifecycle test")
	}
	decoder, err := media.Start(context.Background(), media.Config{Executable: ffmpeg})
	if err != nil {
		t.Fatal(err)
	}
	decoder.Finish()
	if err := decoder.Enqueue(make([]byte, 188)); !errors.Is(err, media.ErrClosed) {
		t.Fatalf("Enqueue after Finish = %v, want ErrClosed", err)
	}
	if err := decoder.Close(); err != nil {
		t.Fatal(err)
	}
}

func generateTransport(t *testing.T, ffmpeg string) []byte {
	t.Helper()
	cmd := exec.Command(ffmpeg, // #nosec G204 -- test-resolved executable, fixed arguments
		"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=352x288:rate=25:duration=0.4",
		"-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000:duration=0.4",
		"-map", "0:v:0", "-map", "1:a:0", "-c:v", "mpeg2video", "-bf", "0", "-c:a", "mp2",
		"-b:a", "192k", "-mpegts_service_id", "100", "-streamid", "0:257", "-streamid", "1:258",
		"-f", "mpegts", "pipe:1")
	transport, err := cmd.Output()
	if err != nil {
		t.Fatalf("generate transport: %v", err)
	}
	return transport
}
