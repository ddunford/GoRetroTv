package demux

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/irq"
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

func TestSectionCompletionDrivesDispatchRowAndAcknowledges(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	var interrupts []uint8
	controller := irq.New(func(ip uint8) { interrupts = append(interrupts, ip) })
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	d.BindIRQ(controller)
	controller.Write(0x40, bus.Word, irq.DemuxMask)
	d.Write(0xD8, bus.Word, 1<<22)
	d.Write(0x14+4*22, bus.Word, 0x14014)
	section := []byte{0x70, 0x70, 0x05, 0xc3, 0x50, 0, 0, 0}
	if err := d.Push(0x14, section); err != nil {
		t.Fatal(err)
	}
	if got := controller.Read(0x30, bus.Word); got != irq.DemuxMask {
		t.Fatalf("dispatch pending = %#x", got)
	}
	if len(interrupts) != 1 || interrupts[0] != 2 {
		t.Fatalf("demux IP lines = %v", interrupts)
	}
	d.Write(0xB8, bus.Word, ^uint32(1<<22))
	if got := controller.Read(0x30, bus.Word); got != 0 {
		t.Fatalf("acknowledged dispatch pending = %#x", got)
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
	badCRC := []byte{0x42, 0xb0, 0x04, 0, 0, 0, 0}
	if err := d.Push(0x14, badCRC); err == nil {
		t.Fatal("bad CRC was accepted")
	}
}

func TestSectionRingWrapKeepsWholeSectionTogether(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := New()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	d.Write(0xD8, bus.Word, 1<<22)
	d.Write(0x14+4*22, bus.Word, 0x14014)
	d.writePointer[22] = 22*RingSize + RingSize - 3
	section := []byte{0x70, 0x70, 0x05, 0xc3, 0x50, 0, 0, 0}
	if err := d.Push(0x14, section); err != nil {
		t.Fatal(err)
	}
	if got := d.writePointer[22]; got != 22*RingSize+9 {
		t.Fatalf("wrapped pointer = %#x", got)
	}
	physical, err := ringOffset(22)
	if err != nil {
		t.Fatal(err)
	}
	if got := ram.Read(physical, bus.Byte); got != 0x70 {
		t.Fatalf("wrapped section start = %#x", got)
	}
	if got := ram.Read(physical+8, bus.Byte); got != 0 {
		t.Fatalf("wrapped status byte = %#x", got)
	}
}

func TestMPEGCRCAnchor(t *testing.T) {
	t.Parallel()
	if got := mpegCRC([]byte("123456789")); got != 0x0376e6e7 {
		t.Fatalf("MPEG CRC of standard check vector = %#08x", got)
	}
}
