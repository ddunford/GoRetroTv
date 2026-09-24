package board_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

func verifiedImages(t *testing.T) *firmware.Set {
	t.Helper()
	dir := filepath.Join("..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		t.Skip("private firmware is not installed")
	}
	images, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return images
}

func TestColdBootMatchesFirmwaretraceOracleAnchor(t *testing.T) {
	images := verifiedImages(t)
	r, err := board.New(images, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100_000; i++ {
		if err := r.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if r.Machine.Retired != 100_000 {
		t.Fatalf("retired %d", r.Machine.Retired)
	}
	h, err := statehash.New(r.RAM)
	if err != nil {
		t.Fatal(err)
	}
	// Firmwaretrace's checkpoint at 100000 is observed before stepping that
	// instruction, and the oracle independently pinned the same value.
	const want uint32 = 0xCB99AE4A
	if got := h.Hash(r.Machine.Core.State()); got != want {
		t.Fatalf("100000-instruction state hash %08X, want %08X", got, want)
	}
	if err := h.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestPrivatePostAcquisitionSnapshotRestoresAndRuns(t *testing.T) {
	images := verifiedImages(t)
	path := filepath.Join("..", "..", "snapshots", "post-acquisition.snapshot")
	f, err := os.Open(path) // #nosec G304 -- fixed local private test fixture path
	if os.IsNotExist(err) {
		t.Skip("private post-acquisition snapshot is not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := board.New(images, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Restore(f); err != nil {
		t.Fatal(err)
	}
	if r.Machine.Retired != 1_100_000_000 || !r.Machine.Handoff.Done() || !r.Machine.SkyGates.Done() {
		t.Fatalf("snapshot state retired=%d handoff=%v sky=%v", r.Machine.Retired,
			r.Machine.Handoff.Done(), r.Machine.SkyGates.Done())
	}
	h, err := statehash.New(r.RAM)
	if err != nil {
		t.Fatal(err)
	}
	const want uint32 = 0x8B2A7E0B
	if got := h.Hash(r.Machine.Core.State()); got != want {
		t.Fatalf("restored state hash %08X, want %08X", got, want)
	}
	frame, err := r.Compose()
	if err != nil {
		t.Fatal(err)
	}
	if frame.Rect.Dx() != 720 || frame.Rect.Dy() != 576 {
		t.Fatalf("restored display geometry %v", frame.Rect)
	}
	if got := statehash.HashBytes(frame.Pix); got != 0xA6A21DC5 {
		t.Fatalf("restored composed framebuffer hash %08X, want A6A21DC5", got)
	}
	for i := 0; i < 100_000; i++ {
		if err := r.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if r.Machine.Retired != 1_100_100_000 {
		t.Fatalf("retired %d", r.Machine.Retired)
	}
	const afterWant uint32 = 0x8DD8BC2E // firmwaretrace from the same private snapshot
	if got := h.Hash(r.Machine.Core.State()); got != afterWant {
		t.Fatalf("run-on state hash %08X, want %08X", got, afterWant)
	}
	if err := h.Err(); err != nil {
		t.Fatal(err)
	}
	// The run-on hash above and the Sky route below are separate measured anchors. Restore the
	// route's declared precondition instead of injecting its key into the state altered by the
	// run-on probe. CSI's idle-key tests independently prove that waiting cannot splice a reply
	// into the first handset frame.
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	r, err = board.New(images, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Restore(f); err != nil {
		t.Fatal(err)
	}
	if err := r.CSI.Key(0x7D, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20_000_000; i++ {
		if err := r.Step(); err != nil {
			t.Fatal(err)
		}
	}
	menu, err := r.Compose()
	if err != nil {
		t.Fatal(err)
	}
	if got := statehash.HashBytes(menu.Pix); got != 0xFE8D1CCC {
		t.Fatalf("post-Sky composed framebuffer hash %08X, want FE8D1CCC", got)
	}
}
