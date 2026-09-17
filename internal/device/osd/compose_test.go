package osd

import (
	"image/color"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func TestComposeUsesProgrammedFieldsAtTwoFourAndEightBits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ depth, code uint32 }{{2, 1}, {4, 2}, {8, 3}} {
		t.Run(string(rune('0'+tc.depth)), func(t *testing.T) {
			t.Parallel()
			ram, err := memory.NewRAM("dram", memory.DRAMSize)
			if err != nil {
				t.Fatal(err)
			}
			d := NewDisplay()
			if err := d.BindRAM(ram); err != nil {
				t.Fatal(err)
			}
			const desc = 0x10000
			const field0 = 0x11000
			const field1 = 0x12000
			ram.Write(desc+0x08, bus.Half, 1) // two interlaced rows
			ram.Write(desc+0x0a, bus.Half, 7) // eight pixels wide
			ram.Write(desc+0x10, bus.Word, (tc.code<<25)|field0)
			ram.Write(desc+0x14, bus.Word, (tc.code<<25)|field1)
			for x := uint32(0); x < 8; x++ {
				putIndex := func(base, value uint32) {
					at := base + x*tc.depth/8
					old := ram.Read(at, bus.Byte)
					shift := 8 - tc.depth - (x * tc.depth & 7)
					mask := uint32((1<<tc.depth)-1) << shift
					ram.Write(at, bus.Byte, old&^mask|value<<shift&mask)
				}
				putIndex(field0, x%4)
				putIndex(field1, (7-x)%4)
			}
			d.Write(0x200, bus.Word, 0x01000000|desc)
			frame, err := d.Compose()
			if err != nil {
				t.Fatal(err)
			}
			if frame.Rect.Dx() != 8 || frame.Rect.Dy() != 2 {
				t.Fatalf("geometry %v", frame.Rect)
			}
			if got := frame.ColorIndexAt(2, 0); got != 2 {
				t.Fatalf("depth %d field 0 index = %d", tc.depth, got)
			}
			if got := frame.ColorIndexAt(2, 1); got != 1 {
				t.Fatalf("depth %d field 1 index = %d", tc.depth, got)
			}
		})
	}
}

func TestComposeDecodesPackedCLUTAndLeavesNoSignalBlack(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDisplay()
	if err := d.BindRAM(ram); err != nil {
		t.Fatal(err)
	}
	black, err := d.Compose()
	if err != nil {
		t.Fatal(err)
	}
	if black.Rect.Dx() != 720 || black.Rect.Dy() != 576 || black.ColorIndexAt(0, 0) != 0 {
		t.Fatalf("unprogrammed frame = %v", black.Rect)
	}
	const desc = 0x10000
	ram.Write(desc+0x08, bus.Half, 1)
	ram.Write(desc+0x0a, bus.Half, 7)
	ram.Write(desc+0x0c, bus.Word, 0x01013000)
	ram.Write(desc+0x10, bus.Word, (1<<25)|0x11000)
	ram.Write(0x13000, bus.Word, 0x04883a88) // black and bright neutral in packed Y/Cb/Cr
	ram.Write(0x11000, bus.Byte, 0x10)       // second pixel uses palette entry 1
	d.Write(0x200, bus.Word, 0x01010000)
	frame, err := d.Compose()
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.ColorIndexAt(1, 0); got != 1 {
		t.Fatalf("CLUT pixel index = %d", got)
	}
	bright := frame.Palette[1].(color.RGBA)
	if bright.R < 200 || bright.G < 200 || bright.B < 200 {
		t.Fatalf("packed CLUT entry = %+v", bright)
	}
}
