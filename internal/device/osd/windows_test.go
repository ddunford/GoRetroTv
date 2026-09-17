package osd

import (
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func TestWindowZeroRefusedAndVisibleRecordReadWithoutMutation(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	write := func(addr, value uint32) { ram.Write(addr-memory.DRAMBase, bus.Word, value) }
	const base = 0x80020000
	write(windowArrayPointer, base)
	write(windowCountAddress, 2)
	write(windowGateAddress, 0)
	visible := uint32(base + windowRecordSize)
	write(visible+0x00, 1)
	write(visible+0x40, 8)
	write(visible+0x50, 4)
	write(visible+0x54, 3)
	write(visible+0x58, 1)
	write(visible+0x5c, 0xdcdcdcdc)
	write(visible+0x60, 0x80430000)
	table := NewWindowTable(ram)
	if _, err := table.Read(0); err == nil || !strings.Contains(err.Error(), "refuses window 0") {
		t.Fatalf("window 0 was not safely refused: %v", err)
	}
	w, err := table.Read(1)
	if err != nil {
		t.Fatal(err)
	}
	if w.Kind != 1 || w.Depth != 8 || w.Produce != 4 || w.Consume != 3 ||
		!w.BackgroundSet || w.BackgroundColor != 0xdcdcdcdc || w.RootObject != 0x80430000 {
		t.Fatalf("visible window record = %+v", w)
	}
	if got := ram.Read(visible-memory.DRAMBase+0x50, bus.Word); got != 4 {
		t.Fatalf("view mutated firmware-owned produce index to %d", got)
	}
}

func TestWindowTableRejectsOutOfBoundsExtent(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	ram.Write(windowArrayPointer-memory.DRAMBase, bus.Word, memory.DRAMBase+memory.DRAMSize-50)
	ram.Write(windowCountAddress-memory.DRAMBase, bus.Word, 2)
	if _, err := NewWindowTable(ram).Read(1); err == nil {
		t.Fatal("window table extending beyond DRAM was accepted")
	}
}
