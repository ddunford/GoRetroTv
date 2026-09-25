package media_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/media"
)

func TestScheduledEncoderFeedsDecoder(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for pipeline acceptance")
	}
	path := generateAVFile(t, ffmpeg, t.TempDir(), "programme.mp4", 3*time.Second)
	playlist := media.Playlist{Items: []media.Item{{Path: path, Duration: 3 * time.Second}},
		Total: 3 * time.Second, Loop: true}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	encoder, err := media.StartEncoder(ctx, ffmpeg, playlist, 4*time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	decoder, err := media.Start(ctx, media.Config{Executable: ffmpeg, InputQueue: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	chunks := 0
	packets := encoder.Packets()
	for {
		select {
		case chunk, ok := <-packets:
			if !ok {
				decoder.Finish()
				packets = nil
				if fault := encoder.Fault(); fault != nil {
					t.Fatal(fault)
				}
				continue
			}
			chunks++
			if err := decoder.Enqueue(chunk); err != nil {
				t.Fatal(err)
			}
		case frame, ok := <-decoder.Video():
			if !ok {
				t.Fatalf("decoder ended after %d chunks: %v", chunks, decoder.Fault())
			}
			if len(frame.RGBA) != media.VideoBytes {
				t.Fatalf("video bytes = %d", len(frame.RGBA))
			}
			return
		case <-ctx.Done():
			t.Fatalf("pipeline timed out after %d chunks: encoder=%v decoder=%v", chunks, encoder.Fault(), decoder.Fault())
		}
	}
}
