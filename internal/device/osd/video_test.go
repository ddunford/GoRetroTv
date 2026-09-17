package osd

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/memory"
)

func TestVRAMPortAddressingAndFramebufferGeometry(t *testing.T) {
	t.Parallel()
	if FramebufferBase != 0x80584048 || FrameWidth != 720 || FrameHeight != 576 {
		t.Fatal("application framebuffer geometry changed")
	}
	v := NewVideo()
	v.Write(0xbc, bus.Word, 0x20)
	v.Write(0xb0, bus.Word, 0xaaaaaaaa)
	v.Write(0xbc, bus.Word, 0x20)
	if got := v.Read(0xb0, bus.Word); got != 0xaaaaaaaa {
		t.Fatalf("video RAM readback = %#08x", got)
	}
	if got := v.Read(0xbc, bus.Word); got != 0x24 {
		t.Fatalf("auto-incremented address = %#x", got)
	}
	v.Write(0xbc, bus.Word, VRAMSize-2)
	v.Write(0xb0, bus.Word, 0x12345678)
	v.Write(0xbc, bus.Word, VRAMSize-2)
	if got := v.Read(0xb0, bus.Word); got != 0x12345678 {
		t.Fatalf("wrapped word = %#08x", got)
	}
	if got := v.Read(0xb8, bus.Word); got != 0 {
		t.Fatalf("busy status = %#x", got)
	}
	v.Write(0x98, bus.Word, 0x80000)
	if got := v.Read(0x98, bus.Word); got != 0x80000 {
		t.Fatalf("plane address readback = %#x", got)
	}
}

func TestFramebufferLivesInSharedBoardDRAM(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	b := bus.New()
	if err := b.Attach(memory.DRAMBase, memory.DRAMSize, ram); err != nil {
		t.Fatal(err)
	}
	last := FramebufferBase + FrameWidth*FrameHeight - 1
	b.Write(last, bus.Byte, 0xdc)
	if got := b.Read(last+0x20000000, bus.Byte); got != 0xdc {
		t.Fatalf("uncached framebuffer alias = %#x", got)
	}
}

func TestVideoHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return NewVideo() },
		Mutate: func(device bus.Device) {
			v := device.(*Video)
			v.Write(0xbc, bus.Word, 0x20)
			v.Write(0xb0, bus.Word, 0xaaaaaaaa)
			v.Write(0xb4, bus.Word, 0x31)
			v.Write(0x98, bus.Word, 0x80000)
		},
		Disturb: func(device bus.Device) {
			v := device.(*Video)
			v.Write(0xbc, bus.Word, 0x40)
			v.Write(0xb0, bus.Word, 0x55555555)
			v.Write(0xb4, bus.Word, 0)
			v.Write(0x98, bus.Word, 0)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDisplayHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return NewDisplay() },
		Mutate: func(device bus.Device) {
			d := device.(*Display)
			d.Write(0x200, bus.Word, 0x01013c0c)
			d.Write(0x204, bus.Word, 0x01583fe8)
		},
		Disturb: func(device bus.Device) { device.Reset() },
	})
	if err != nil {
		t.Fatal(err)
	}
}
