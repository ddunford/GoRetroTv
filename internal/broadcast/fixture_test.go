package broadcast_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/firmware"
)

// cachedLineup holds a machine that has already been given a clock and a
// channel list, so the tests that need that state restore it instead of
// re-deriving it.
//
// Deriving it costs ~38 million emulated instructions, and running that once
// per subtest took internal/broadcast past Go's ten-minute timeout under the
// race detector -- which is how `ctl.sh test` went from fitting in the quality
// hook's budget to not finishing at all. Snapshot and restore exist for
// precisely this: phase 4's justification was that a hypothesis costing six
// minutes of boot should cost seconds from a snapshot, and a test suite is the
// same hypothesis run repeatedly.
var cachedLineup []byte

// restoredBox gives a machine in the verified post-acquisition state, or skips.
// Neither the flash images nor the snapshot are redistributable, so a machine
// without them must say so rather than quietly testing nothing.
func restoredBox(t *testing.T) *board.Runtime {
	t.Helper()
	dir := filepath.Join("..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		t.Skip("private firmware is not installed")
	}
	snapshot := filepath.Join("..", "..", "snapshots", "post-acquisition.snapshot")
	if _, err := os.Stat(snapshot); os.IsNotExist(err) {
		t.Skip("private post-acquisition snapshot is not installed")
	}
	images, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	box, err := board.New(images, true)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(snapshot) // #nosec G304 -- fixed local private test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Restore(file); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if box.Machine.Retired != 1_100_000_000 {
		t.Fatalf("unexpected fixture state: %d retired", box.Machine.Retired)
	}
	return box
}

// guestSubscription reads the ids the box is actually asking for out of its own
// section filters, and fails if it is not asking. Choosing them instead is the
// mistake this project has already paid for: a bouquet id the filter does not
// match is a section the hardware never delivers, and from outside that is
// indistinguishable from a box that received it and ignored it.
func guestSubscription(t *testing.T, box *board.Runtime) (bouquetID, networkID uint16) {
	t.Helper()
	table, ok := box.Demux.Match(3, 0)
	if !ok || table.Value != 0x4a || table.Mask != 0xff {
		t.Fatalf("fixture is not asking for the BAT: %+v", table)
	}
	high, okHigh := box.Demux.Match(3, 1)
	low, okLow := box.Demux.Match(3, 2)
	if !okHigh || !okLow || high.Mask != 0xff || low.Mask != 0xff {
		t.Fatal("fixture lacks an exact bouquet match, so the id cannot be read off it")
	}
	bouquetID = uint16(high.Value)<<8 | uint16(low.Value)

	network, okNetwork := box.Demux.Match(1, 0)
	if !okNetwork || network.Value != 0x40 {
		t.Fatalf("fixture is not asking for the NIT: %+v", network)
	}
	networkHigh, okNH := box.Demux.Match(1, 1)
	networkLow, okNL := box.Demux.Match(1, 2)
	if !okNH || !okNL {
		t.Fatal("fixture lacks a network id to seed the transport with")
	}
	networkID = uint16(networkHigh.Value)<<8 | uint16(networkLow.Value)

	armed := false
	for _, pid := range box.Demux.ArmedPIDs() {
		if pid == 0x11 {
			armed = true
		}
	}
	if !armed {
		t.Fatal("PID 0x11 is not armed, so a BAT pushed to it would never reach the guest")
	}
	return bouquetID, networkID
}

// boxWithLineup returns a machine that has a clock and a channel list, and so
// is asking for its listings. The first call derives that state and snapshots
// it; every later call restores it.
func boxWithLineup(t *testing.T, feed func(*testing.T, *board.Runtime)) *board.Runtime {
	t.Helper()
	box := restoredBox(t)
	if cachedLineup != nil {
		if err := box.Restore(bytes.NewReader(cachedLineup)); err != nil {
			t.Fatal(err)
		}
		return box
	}
	feed(t, box)
	var state bytes.Buffer
	if err := box.Machine.Snapshot(&state); err != nil {
		t.Fatal(err)
	}
	cachedLineup = state.Bytes()
	return box
}
