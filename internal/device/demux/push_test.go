package demux

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func TestPushWritesSectionAndHardwareByteToBoardDRAM(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New()
	if err := b.Attach(memory.DRAMBase, memory.DRAMSize, ram); err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	d.Write(0xD8, bus.Word, 1<<22)
	d.Write(0x14+4*22, bus.Word, 0x14014)
	section := []byte{0x70, 0x70, 0x05, 0xc3, 0x50, 0x00, 0x00, 0x00}
	if err := d.Push(0x14, section); err != nil {
		t.Fatal(err)
	}
	base := RingBase(22)
	for i, want := range append(append([]byte(nil), section...), 0) {
		if got := b.Read(base+uint32(i), bus.Byte); got != uint32(want) { // #nosec G115 -- small fixture index
			t.Fatalf("ring byte %d = %#x, want %#x", i, got, want)
		}
	}
	d.Write(0x124, bus.Word, 0x4000|(22<<2))
	if got := d.Read(0x128, bus.Word); got != 22*RingSize+uint32(len(section))+1 { // #nosec G115 -- small fixture
		t.Fatalf("write pointer = %#x", got)
	}
	if got := d.Read(0xB8, bus.Word); got != 1<<22 {
		t.Fatalf("completion status = %#x", got)
	}
	if err := d.Push(0x14, section); err != nil {
		t.Fatal(err)
	}
	if got := b.Read(base+uint32(len(section))+1, bus.Byte); got != uint32(section[0]) { // #nosec G115 -- small fixture
		t.Fatalf("second section did not start after the appended byte: %#x", got)
	}
}

func TestPushRejectsUnrequestedAndMalformedSections(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	section := []byte{0x70, 0x70, 0x05, 0xc3, 0x50, 0x00, 0x00, 0x00}
	if err := d.Push(0x14, section); err == nil {
		t.Fatal("unrequested PID was accepted")
	}
	d.Write(0xD8, bus.Word, 1<<22)
	d.Write(0x14+4*22, bus.Word, 0x14014)
	if err := d.Push(0x14, section[:7]); err == nil {
		t.Fatal("truncated section was accepted")
	}
	if got := d.Read(0xB8, bus.Word); got != 0 {
		t.Fatalf("rejected section raised completion %#x", got)
	}
}
