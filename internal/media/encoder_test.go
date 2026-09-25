package media_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/media"
)

func TestEncoderStartsAtResolvedLivePositionAndEmitsDeclaredPIDs(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for encoder acceptance")
	}
	// The decoder tests already prove this generated transport becomes RGBA and PCM. This test pins
	// the schedule-facing half: the source process emits packet-aligned video/audio PIDs.
	root := t.TempDir()
	path := generateAVFile(t, ffmpeg, root, "programme.mp4", 3*time.Second)
	playlist := media.Playlist{Items: []media.Item{{Path: path, Duration: 3 * time.Second}},
		Total: 3 * time.Second, Loop: true}
	encoder, err := media.StartEncoder(context.Background(), ffmpeg, playlist, 4*time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	seen := map[uint16]bool{}
	deadline := time.After(10 * time.Second)
	for !seen[0x101] || !seen[0x102] {
		select {
		case chunk, ok := <-encoder.Packets():
			if !ok {
				t.Fatalf("encoder ended before both PIDs; fault=%v seen=%v", encoder.Fault(), seen)
			}
			if len(chunk)%188 != 0 {
				t.Fatalf("transport chunk length = %d", len(chunk))
			}
			for at := 0; at < len(chunk); at += 188 {
				pid := uint16(chunk[at+1]&0x1f)<<8 | uint16(chunk[at+2])
				seen[pid] = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for programme PIDs; fault=%v seen=%v", encoder.Fault(), seen)
		}
	}
}

func generateAVFile(t *testing.T, ffmpeg, root, name string, duration time.Duration) string {
	t.Helper()
	path := root + "/" + name
	seconds := duration.Seconds()
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", // #nosec G204 -- test-resolved executable and test-owned path
		"testsrc2=size=64x64:rate=25:duration="+durationString(seconds), "-f", "lavfi", "-i",
		"sine=frequency=1000:sample_rate=48000:duration="+durationString(seconds), "-shortest", "-y", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate AV file: %v: %s", err, output)
	}
	return path
}

func durationString(seconds float64) string {
	return time.Duration(seconds * float64(time.Second)).String()
}
