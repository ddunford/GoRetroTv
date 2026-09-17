package blitter

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/memory"
)

func boardRAM(t *testing.T) *memory.RAM {
	t.Helper()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	return ram
}

func TestBit24FillAndBit24ClearPackedCopy(t *testing.T) {
	t.Parallel()
	ram := boardRAM(t)
	b := New(ram)
	var fill Command
	fill[0] = 0x018c0000 // bit 24 set, bit 23 also set, 8bpp
	fill[7] = 0x1000
	fill[11] = 7         // pitch 8 pixels
	fill[13] = 1<<16 | 3 // 4x2
	fill[14] = 0xdc
	if err := b.Execute(fill); err != nil {
		t.Fatal(err)
	}
	for _, off := range []uint32{0x1000, 0x1003, 0x1008, 0x100b} {
		if got := ram.Read(off, bus.Byte); got != 0xdc {
			t.Fatalf("filled byte %#x = %#x", off, got)
		}
	}
	for i, v := range []byte{1, 2, 3, 4, 5, 6, 7, 8} {
		ram.Write(0x2000+uint32(i), bus.Byte, uint32(v)) // #nosec G115 -- small fixture
	}
	var copyCmd Command
	copyCmd[0] = 0x008c0000 // bit 23 set, bit 24 clear: copy, not fill
	copyCmd[1] = 0x2000
	copyCmd[5] = 0x8042f354 // implausible source descriptor; source rows are packed
	copyCmd[7] = 0x3000
	copyCmd[11] = 7 // destination pitch 8, source stride 4
	copyCmd[13] = 1<<16 | 3
	copyCmd[14] = 0xdeadbeef // unused stack data, not a colour
	if err := b.Execute(copyCmd); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ off, want uint32 }{{0x3000, 1}, {0x3003, 4}, {0x3008, 5}, {0x300b, 8}} {
		if got := ram.Read(tc.off, bus.Byte); got != tc.want {
			t.Fatalf("copied byte %#x = %#x, want %#x", tc.off, got, tc.want)
		}
	}
}

func TestBlitterHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(nil) },
		Mutate: func(device bus.Device) {
			b := device.(*Blitter)
			b.Write(0x00, bus.Word, 0x018c0000)
			b.Write(0x38, bus.Word, 0xdc)
			b.Write(0x3c, bus.Word, 0x11)
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"ram"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSixteenBitCellFillCoversWholeSurface(t *testing.T) {
	t.Parallel()
	ram := boardRAM(t)
	b := New(ram)
	var command Command
	command[0] = 0x418f0000
	command[7] = 0x4000
	command[11] = 3 // four 16-bit cells per row
	command[13] = 1<<16 | 3
	command[14] = 0x10100000
	if err := b.Execute(command); err != nil {
		t.Fatal(err)
	}
	if got := ram.Read(0x4000+14, bus.Half); got != 0x1010 {
		t.Fatalf("last 16-bit cell = %#x", got)
	}
}

func TestPlanarCopyCarriesChroma(t *testing.T) {
	t.Parallel()
	ram := boardRAM(t)
	b := New(ram)
	for i := uint32(0); i < 8; i++ {
		ram.Write(0x5000+i, bus.Byte, i+1)
	}
	for i := uint32(0); i < 4; i++ {
		ram.Write(0x6000+i, bus.Byte, 0x80+i)
	}
	var command Command
	command[0] = 0x001c0000 // 4:2:0 copy, bit 24 clear
	command[1], command[3], command[5] = 0x5000, 0x6000, 3
	command[7], command[9], command[11] = 0x7000, 0x8000, 3
	command[13] = 1<<16 | 3
	if err := b.Execute(command); err != nil {
		t.Fatal(err)
	}
	if got := ram.Read(0x7007, bus.Byte); got != 8 {
		t.Fatalf("last luma = %#x", got)
	}
	if got := ram.Read(0x8003, bus.Byte); got != 0x83 {
		t.Fatalf("last chroma byte = %#x", got)
	}
}
