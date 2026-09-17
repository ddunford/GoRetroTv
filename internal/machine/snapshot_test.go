package machine_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/machine"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/clock"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

func snapshotMachine(t *testing.T) (*machine.Machine, *memory.RAM) {
	t.Helper()
	board := bus.New()
	ram, err := memory.NewRAM("dram", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if err := board.Attach(memory.DRAMBase, 4096, ram); err != nil {
		t.Fatal(err)
	}
	for _, part := range []struct {
		name string
		base uint32
	}{
		{"U202", memory.FlashU202},
		{"U203", memory.FlashU203},
	} {
		flash, err := memory.NewFlash(part.name, make([]byte, 4096))
		if err != nil {
			t.Fatal(err)
		}
		if err := board.Attach(part.base, 4096, flash); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.New()
	handler := func(uint64) error { return nil }
	if _, err := clk.At(16, "board-pump", handler); err != nil {
		t.Fatal(err)
	}
	box, err := machine.New(cpu.New(board, memory.FlashU202), board, clk, &machine.Handoff{},
		machine.NewSkyGates(true), map[string]clock.Handler{"board-pump": handler})
	if err != nil {
		t.Fatal(err)
	}
	return box, ram
}

func TestWholeMachineSnapshotRestoresEveryOwnerAndClockDeadline(t *testing.T) {
	t.Parallel()
	box, ram := snapshotMachine(t)
	box.Core.GPR[3] = 0x12345678
	box.Retired = 9
	ram.Write(0x20, bus.Word, 0xdeadbeef)
	if err := box.Clock.Advance(7); err != nil {
		t.Fatal(err)
	}
	var saved bytes.Buffer
	if err := box.Snapshot(&saved); err != nil {
		t.Fatal(err)
	}
	box.Core.GPR[3] = 0
	box.Retired = 100
	ram.Write(0x20, bus.Word, 0)
	if err := box.Clock.Advance(20); err != nil {
		t.Fatal(err)
	}
	if err := box.Restore(bytes.NewReader(saved.Bytes())); err != nil {
		t.Fatal(err)
	}
	if box.Core.GPR[3] != 0x12345678 || box.Retired != 9 || ram.Read(0x20, bus.Word) != 0xdeadbeef {
		t.Fatal("CPU, retired count or RAM did not restore")
	}
	if box.Clock.Now() != 7 {
		t.Fatalf("clock now = %d, want 7", box.Clock.Now())
	}
	if due, ok := box.Clock.NextDue(); !ok || due != 16 {
		t.Fatalf("next pump = %d/%v, want 16/true", due, ok)
	}
	var restored bytes.Buffer
	if err := box.Snapshot(&restored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved.Bytes(), restored.Bytes()) {
		t.Fatal("restored machine snapshot differs from saved state")
	}
	if err := box.Clock.Advance(8); err != nil {
		t.Fatal(err)
	}
	if box.Clock.Pending() != 1 {
		t.Fatal("pump fired before restored deadline")
	}
	if err := box.Clock.Advance(1); err != nil {
		t.Fatal(err)
	}
	if box.Clock.Now() != 16 || box.Clock.Pending() != 0 {
		t.Fatalf("pump did not fire at restored instruction 16: now=%d pending=%d", box.Clock.Now(), box.Clock.Pending())
	}
}

func TestWholeMachineRefusesAnOmittedOwnerWithoutMutation(t *testing.T) {
	t.Parallel()
	box, _ := snapshotMachine(t)
	var complete bytes.Buffer
	if err := box.Snapshot(&complete); err != nil {
		t.Fatal(err)
	}
	set, err := snapcodec.OpenSet(complete.Bytes(), "goretrotv-machine", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := snapcodec.NewSetWriter("goretrotv-machine", 1)
	for _, name := range set.Names() {
		if name == "sky-gates" {
			continue
		}
		blob, _ := set.Member(name)
		if err := w.Add(name, blob); err != nil {
			t.Fatal(err)
		}
	}
	broken, err := w.Blob()
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Restore(bytes.NewReader(broken)); !errors.Is(err, snapcodec.ErrMemberMissing) {
		t.Fatalf("omitted owner: got %v, want ErrMemberMissing", err)
	}
	var after bytes.Buffer
	if err := box.Snapshot(&after); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(complete.Bytes(), after.Bytes()) {
		t.Fatal("refused restore mutated the machine")
	}
}

func TestWholeMachineRollsBackWhenCPURefusesAfterBusRestore(t *testing.T) {
	t.Parallel()
	box, ram := snapshotMachine(t)
	var target bytes.Buffer
	if err := box.Snapshot(&target); err != nil {
		t.Fatal(err)
	}
	set, err := snapcodec.OpenSet(target.Bytes(), "goretrotv-machine", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := snapcodec.NewSetWriter("goretrotv-machine", 1)
	for _, name := range set.Names() {
		blob, _ := set.Member(name)
		if name == "cpu" {
			blob = blob[:len(blob)-1]
		}
		if err := w.Add(name, blob); err != nil {
			t.Fatal(err)
		}
	}
	broken, err := w.Blob()
	if err != nil {
		t.Fatal(err)
	}
	ram.Write(0x20, bus.Word, 0xfeedface)
	box.Core.GPR[3] = 0xaabbccdd
	var before bytes.Buffer
	if err := box.Snapshot(&before); err != nil {
		t.Fatal(err)
	}
	if err := box.Restore(bytes.NewReader(broken)); err == nil {
		t.Fatal("truncated CPU state was accepted")
	}
	var after bytes.Buffer
	if err := box.Snapshot(&after); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Bytes(), after.Bytes()) {
		t.Fatal("failed CPU restore left bus or register state mutated")
	}
}
