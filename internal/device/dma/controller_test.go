package dma

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/blitter"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/device/osd"
	"github.com/ddunford/goretrotv/internal/memory"
)

func testRAM(t *testing.T) *memory.RAM {
	t.Helper()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	return ram
}

func descriptor(ram *memory.RAM, channel uint8, src, length uint32) uint32 {
	at := DescriptorBase + uint32(channel)*DescriptorSize
	off := at - memory.DRAMBase
	ram.Write(off+12, bus.Word, src)
	ram.Write(off+20, bus.Word, length)
	ram.Write(off+32, bus.Word, src+length-1)
	return at & 0x1fffffff
}

func TestChannel12CompletesAndAcknowledges(t *testing.T) {
	t.Parallel()
	ram := testRAM(t)
	graphics := blitter.New(ram)
	var lines []uint8
	interrupts := irq.New(func(ip uint8) { lines = append(lines, ip) })
	d := New(ram, graphics, nil, interrupts)
	interrupts.Write(0x40, bus.Word, IRQMask)
	command := [15]uint32{}
	command[0] = 0x018c0000
	command[7] = 0x3000
	command[11] = 3
	command[13] = 1<<16 | 3
	command[14] = 0xdc
	for i, word := range command {
		ram.Write(0x2000+uint32(i)*4, bus.Word, word) // #nosec G115 -- fifteen-word fixture
	}
	d.Write(0x40+12*0x10, bus.Word, descriptor(ram, 12, 0x80002000, 60))
	d.Write(0x10, bus.Word, 1<<12)
	if err := d.Fault(); err != nil {
		t.Fatal(err)
	}
	if got := ram.Read(0x3007, bus.Byte); got != 0xdc {
		t.Fatalf("blitter did not fill last byte: %#x", got)
	}
	if got := d.Read(0x122, bus.Half); got != 1<<12 {
		t.Fatalf("DMA completion halfword = %#x", got)
	}
	if got := interrupts.Read(0x30, bus.Word); got != IRQMask || len(lines) != 1 || lines[0] != 2 {
		t.Fatalf("dispatch pending %#x, lines %v", got, lines)
	}
	if got := d.Read(0x10, bus.Word); got != 1<<12 {
		t.Fatalf("enable readback = %#x", got)
	}
	d.Write(0x10, bus.Word, d.Read(0x10, bus.Word)&^(1<<12))
	d.Write(0x130, bus.Word, 1<<12)
	if got := d.Read(0x120, bus.Word) | interrupts.Read(0x30, bus.Word); got != 0 {
		t.Fatalf("ack left completion or pending %#x", got)
	}
}

func TestChannel8UploadsPlaneAndBootChannel5DoesNotInterrupt(t *testing.T) {
	t.Parallel()
	ram := testRAM(t)
	video := osd.NewVideo()
	interrupts := irq.New(nil)
	d := New(ram, nil, video, interrupts)
	for i := uint32(0); i < 4; i++ {
		ram.Write(0x4000+i, bus.Byte, 0x10+i)
	}
	video.Write(0xd0, bus.Word, 0x80000)
	d.Write(0x40+8*0x10, bus.Word, descriptor(ram, 8, 0x80004000, 4))
	d.Write(0x10, bus.Word, 1<<8)
	video.Write(0xbc, bus.Word, 0x80000)
	if got := video.Read(0xb0, bus.Word); got != 0x10111213 {
		t.Fatalf("uploaded video plane = %#08x", got)
	}
	d.Write(0x40+5*0x10, bus.Word, descriptor(ram, 5, 0x80005000, 16))
	d.Write(0x10, bus.Word, (1<<8)|(1<<5))
	if got := d.Read(0x120, bus.Word) & (1 << 5); got == 0 {
		t.Fatal("bootloader channel 5 did not complete")
	}
	d.Write(0x130, bus.Word, 1<<8)
	if got := interrupts.Read(0x30, bus.Word); got != 0 {
		t.Fatalf("channel 5 improperly raised board IP2 pending %#x", got)
	}
}

func TestControllerHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(nil, nil, nil, nil) },
		Mutate: func(device bus.Device) {
			d := device.(*Controller)
			d.descriptors[12] = 0x108a60
			d.enable = 1 << 12
			d.statusA = 1 << 12
			d.statusB = 1
			d.fault = "test fault"
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"ram", "blitter", "video", "interrupt"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
