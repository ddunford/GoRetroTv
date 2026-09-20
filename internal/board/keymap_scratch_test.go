package board_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

func TestProbeFullKeyMap(t *testing.T) {
	dir := filepath.Join("..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		t.Skip("no firmware")
	}
	images, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join("..", "..", "snapshots", "post-acquisition.snapshot")
	known := map[string]string{
		"A6A21DC5": "no change (startup blue)",
		"FE8D1CCC": "SKY MENU on BOX OFFICE",
		"B424095B": "TV GUIDE",
		"64AF0A8D": "SKY MENU on SERVICES",
		"20DB8CF2": "INTERACTIVE",
	}
	for code := 0; code <= 0xff; code++ {
		box, err := board.New(images, true)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(snapshot)
		if err != nil {
			t.Skip("no snapshot")
		}
		if err := box.Restore(f); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if err := box.CSI.Key(uint8(code), 0); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 40_000_000; i++ {
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		frame, err := box.Compose()
		if err != nil {
			t.Fatal(err)
		}
		h := fmt.Sprintf("%08X", statehash.HashBytes(frame.Pix))
		if h == "A6A21DC5" {
			continue
		}
		if name, ok := known[h]; ok {
			t.Logf("key %#02x -> %s  %s", code, h, name)
			continue
		}
		t.Logf("key %#02x -> %s  *** UNSEEN SCREEN ***", code, h)
	}
}
