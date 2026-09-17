package cpu_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

func TestCheckpointLoopSkipsReachableBranchSlot(t *testing.T) {
	c, _ := machine(t, ri(4, 0, 0, 2), 0, 0, 0)
	ram, err := memory.NewRAM("checkpoint-test", memory.DirtyPageLen)
	if err != nil {
		t.Fatal(err)
	}
	h, err := statehash.New(ram)
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	e, err := statehash.NewEmitter(&stream, 1, h)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < 4; i++ {
		if err := c.ObserveCheckpoint(e, i); err != nil {
			t.Fatal(err)
		}
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stream.String(), "\n0 ") || !strings.Contains(stream.String(), "\n2 ") || strings.Contains(stream.String(), "\n1 ") {
		t.Fatalf("branch/slot sampling instants wrong:\n%s", stream.String())
	}
	// Negative control: a raw observer does emit the forbidden sample, so the assertion above
	// would reject the real loop if its pending-branch guard were removed.
	c2, _ := machine(t, ri(4, 0, 0, 2), 0, 0, 0)
	h2, err := statehash.New(ram)
	if err != nil {
		t.Fatal(err)
	}
	var bad bytes.Buffer
	e2, err := statehash.NewEmitter(&bad, 1, h2)
	if err != nil {
		t.Fatal(err)
	}
	if err := e2.Observe(0, c2.State()); err != nil {
		t.Fatal(err)
	}
	if err := c2.Step(); err != nil {
		t.Fatal(err)
	}
	if !c2.HasPendingBranch() {
		t.Fatal("negative control has no live branch")
	}
	if err := e2.Observe(1, c2.State()); err != nil {
		t.Fatal(err)
	}
	if err := e2.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bad.String(), "\n1 ") {
		t.Fatal("negative control did not produce the forbidden mid-pair sample")
	}
}

func TestCheckpointSamplesMIPS16DelaySlot(t *testing.T) {
	c, b := machine(t)
	c.PC, c.ISA = codeBase+0x40, true
	b.Write(c.PC, bus.Half, 0x1c00) // JALX index follows
	b.Write(c.PC+2, bus.Half, 4)
	b.Write(c.PC+4, bus.Half, 0x6803) // MIPS16 delay slot
	ram, err := memory.NewRAM("mips16-checkpoint", memory.DirtyPageLen)
	if err != nil {
		t.Fatal(err)
	}
	h, err := statehash.New(ram)
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	e, err := statehash.NewEmitter(&stream, 1, h)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ObserveCheckpoint(e, 0); err != nil {
		t.Fatal(err)
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if !c.HasPendingBranch() || !c.ISA {
		t.Fatal("MIPS16 slot was not pending")
	}
	if err := c.ObserveCheckpoint(e, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stream.String(), "\n1 ") {
		t.Fatalf("MIPS16 slot checkpoint missing:\n%s", stream.String())
	}
}
