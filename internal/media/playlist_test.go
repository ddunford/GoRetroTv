package media_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/media"
)

func TestPlaylistUsesTheLiveProgrammeOffsetAcrossFiles(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is required for playlist acceptance")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "channel")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for playlist acceptance")
	}
	for _, name := range []string{"02-second.mp4", "01-first.mp4"} {
		cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=red:size=32x32:rate=1:duration=2", // #nosec G204 -- test-resolved executable and owned path
			"-an", "-y", filepath.Join(dir, name))
		if output, runErr := cmd.CombinedOutput(); runErr != nil {
			t.Fatalf("generate %s: %v: %s", name, runErr, output)
		}
	}
	playlist, err := media.ProbePlaylist(context.Background(), ffprobe, root, "folder",
		media.Source{Path: "channel", Loop: true}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	position, err := playlist.At(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(position.Item.Path) != "01-first.mp4" || position.Offset != time.Second {
		t.Fatalf("position at +5s = %s + %s, want lexically first file +1s after wrapping",
			filepath.Base(position.Item.Path), position.Offset)
	}
}

func TestPlaylistRefusesTraversalAndInsufficientDuration(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is required for playlist acceptance")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("not media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.mp4")); err != nil {
		t.Fatal(err)
	}
	_, err = media.ProbePlaylist(context.Background(), ffprobe, root, "file",
		media.Source{Path: "escape.mp4", Loop: true}, time.Hour)
	if err == nil {
		t.Fatal("playlist accepted a symlink outside the media root")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}
