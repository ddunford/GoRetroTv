package osd

import (
	"fmt"
	"image"
	"image/color"

	"github.com/ddunford/goretrotv/internal/bus"
)

// Descriptor is the firmware-programmed OSD plane at the display-list root.
type Descriptor struct {
	Width, Height  uint32
	Depth          uint8
	Field0, Field1 uint32
	CLUT           uint32
}

func (d *Display) readRAM(addr uint32, size bus.Size) (uint32, error) {
	if d.ram == nil {
		return 0, fmt.Errorf("osd: board DRAM is not bound")
	}
	off, ok := bus.Physical(addr)
	if !ok || uint64(off)+uint64(size) > uint64(d.ram.Size()) {
		return 0, fmt.Errorf("osd: display address %#08x outside board DRAM", addr)
	}
	return d.ram.Read(off, size), nil
}

func displayPointer(word uint32) uint32 {
	if word&0x00ffffff == 0 {
		return 0
	}
	return 0xA0000000 | (word & 0x00ffffff)
}

// Descriptor reads the first valid display-list entry. A zero root means the
// firmware has not programmed a picture yet and returns no descriptor.
func (d *Display) Descriptor() (Descriptor, bool, error) {
	if d.roots[0]&0x01000000 == 0 {
		return Descriptor{}, false, nil
	}
	at := displayPointer(d.roots[0])
	if at == 0 {
		return Descriptor{}, false, fmt.Errorf("osd: enabled root has no descriptor address")
	}
	hMinus1, err := d.readRAM(at+0x08, bus.Half)
	if err != nil {
		return Descriptor{}, false, err
	}
	wMinus1, err := d.readRAM(at+0x0a, bus.Half)
	if err != nil {
		return Descriptor{}, false, err
	}
	clutWord, err := d.readRAM(at+0x0c, bus.Word)
	if err != nil {
		return Descriptor{}, false, err
	}
	f0Word, err := d.readRAM(at+0x10, bus.Word)
	if err != nil {
		return Descriptor{}, false, err
	}
	f1Word, err := d.readRAM(at+0x14, bus.Word)
	if err != nil {
		return Descriptor{}, false, err
	}
	desc := Descriptor{
		Width: wMinus1 + 1, Height: hMinus1 + 1,
		Depth:  uint8(1 << ((f0Word >> 25) & 3)), // #nosec G115 -- result is one of 1,2,4,8
		Field0: displayPointer(f0Word), Field1: displayPointer(f1Word),
		CLUT: displayPointer(clutWord),
	}
	if desc.Width < 2 || desc.Width > 2048 || desc.Height < 2 || desc.Height > 1152 || desc.Field0 == 0 {
		return Descriptor{}, false, fmt.Errorf("osd: invalid display-list geometry or field pointer")
	}
	return desc, true, nil
}

func clampColour(value float64) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value) // #nosec G115 -- clamped to [0,255]
}

func decodeColour(y, cb, cr uint32) color.RGBA {
	yy := 1.164 * (float64(y) - 16)
	cbDelta, crDelta := float64(cb)-128, float64(cr)-128
	return color.RGBA{
		R: clampColour(yy + 1.596*crDelta),
		G: clampColour(yy - 0.392*cbDelta - 0.813*crDelta),
		B: clampColour(yy + 2.017*cbDelta), A: 255,
	}
}

func (d *Display) palette(desc Descriptor) (color.Palette, error) {
	n := 1 << desc.Depth
	palette := make(color.Palette, n)
	for i := 0; i < n; i++ {
		var y, cb, cr uint32
		if desc.CLUT == 0 {
			y = uint32(i * 255 / (n - 1)) // #nosec G115 -- palette size is at most 256
			cb, cr = 128, 128
		} else {
			word, err := d.readRAM(desc.CLUT+uint32(i/2)*4, bus.Word) // #nosec G115 -- palette size is at most 256
			if err != nil {
				return nil, err
			}
			entry := word >> 16
			if i&1 != 0 {
				entry = word & 0xffff
			}
			y = ((entry >> 8) & 0x3f) << 2
			cb, cr = ((entry>>4)&0xf)<<4, (entry&0xf)<<4
		}
		palette[i] = decodeColour(y, cb, cr)
	}
	return palette, nil
}

// Compose renders the first display-list plane as indexed pixels and its CLUT.
// Before programming, the output is a black 720x576 frame, never guessed RAM.
func (d *Display) Compose() (*image.Paletted, error) {
	desc, programmed, err := d.Descriptor()
	if err != nil {
		return nil, err
	}
	if !programmed {
		return image.NewPaletted(image.Rect(0, 0, FrameWidth, FrameHeight), color.Palette{color.RGBA{A: 255}}), nil
	}
	palette, err := d.palette(desc)
	if err != nil {
		return nil, err
	}
	frame := image.NewPaletted(image.Rect(0, 0, int(desc.Width), int(desc.Height)), palette)
	stride := (desc.Width*uint32(desc.Depth) + 7) / 8
	for y := uint32(0); y < desc.Height; y++ {
		base, row := desc.Field0, y
		if desc.Field1 != 0 {
			row = y / 2
			if y&1 != 0 {
				base = desc.Field1
			}
		}
		for x := uint32(0); x < desc.Width; x++ {
			bit := x * uint32(desc.Depth)
			value, err := d.readRAM(base+row*stride+bit/8, bus.Byte)
			if err != nil {
				return nil, err
			}
			shift := 8 - uint32(desc.Depth) - (bit & 7)
			index := (value >> shift) & ((1 << desc.Depth) - 1)
			frame.Pix[y*uint32(frame.Stride)+x] = uint8(index) // #nosec G115 -- at most 8-bit palette index
		}
	}
	return frame, nil
}
