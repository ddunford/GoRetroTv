package machine

import (
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/memory"
)

func handoffFixture(t *testing.T, valid bool) (*Handoff, *cpu.Core, *bus.Bus, *memory.RAM) {
	t.Helper()
	board := bus.New()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	image := make([]byte, memory.FlashSize)
	copy(image[0x12584:], []byte{0x4a, 0x42, 0xa0, 0x07})
	if valid {
		copy(image[0x20000:], []byte{0x4a, 0x42, 0xa0, 0x07})
	}
	// The supplied image has an instruction here, so the policy must use its
	// declared flash entry stub rather than treating this word as a pointer.
	copy(image[0x2002c:], []byte{0x3c, 0x08, 0xbf, 0xc2})
	flash, err := memory.NewFlash("U202", image)
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range []struct {
		base, size uint32
		device     bus.Device
	}{
		{memory.DRAMBase, memory.DRAMSize, ram},
		{memory.FlashU202, memory.FlashSize, flash},
	} {
		if err := board.Attach(region.base, region.size, region.device); err != nil {
			t.Fatal(err)
		}
	}
	board.Write(0x800083cc, bus.Word, 0x100)
	core := cpu.New(board, 0xBFC00000)
	core.GPR[31] = 0x12345678
	return &Handoff{}, core, board, ram
}

func TestHandoffRequiresContinuousIdleAndRunsOnlyOnce(t *testing.T) {
	h, core, board, ram := handoffFixture(t, true)
	check := func(want bool) {
		t.Helper()
		got, err := h.Tick(core, board)
		if err != nil || got != want {
			t.Fatalf("applied=%v, err=%v; want applied=%v", got, err, want)
		}
	}
	for i := 0; i < 12_499; i++ {
		check(false)
	}
	board.Write(0x800083d0, bus.Word, 1)
	check(false)
	board.Write(0x800083d0, bus.Word, 0)
	for i := 0; i < 12_499; i++ {
		check(false)
	}
	check(true)
	if core.PC != 0xBFC2048C || core.ISA || core.GPR[31] != 0 {
		t.Fatalf("handoff state PC=%08X ISA=%v RA=%08X", core.PC, core.ISA, core.GPR[31])
	}
	if ram.Read(0x9f4, bus.Word) != 0 {
		t.Fatal("host policy modified application RAM")
	}
	core.PC = 0x800009f4
	check(false)
	if core.PC != 0x800009f4 {
		t.Fatal("one-shot policy changed PC again")
	}
}

func TestHandoffRejectsWrongFlashHeader(t *testing.T) {
	h, core, board, _ := handoffFixture(t, false)
	for i := 0; i < 12_499; i++ {
		if _, err := h.Tick(core, board); err != nil {
			t.Fatal(err)
		}
	}
	applied, err := h.Tick(core, board)
	if applied || err == nil || !strings.Contains(err.Error(), "flash header") {
		t.Fatalf("applied=%v, err=%v", applied, err)
	}
	if h.Done() || core.PC != 0xBFC00000 || core.GPR[31] != 0x12345678 {
		t.Fatal("rejected handoff changed CPU state")
	}
}

func TestHandoffRestoresIdleDeadline(t *testing.T) {
	h, core, board, _ := handoffFixture(t, true)
	if _, err := h.Tick(core, board); err != nil {
		t.Fatal(err)
	}
	blob, err := h.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var restored Handoff
	if err := restored.Restore(blob); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12_498; i++ {
		if applied, err := restored.Tick(core, board); err != nil || applied {
			t.Fatalf("handoff fired before restored deadline: applied=%v err=%v", applied, err)
		}
	}
	if applied, err := restored.Tick(core, board); err != nil || !applied {
		t.Fatalf("handoff missed restored deadline: applied=%v err=%v", applied, err)
	}
}
