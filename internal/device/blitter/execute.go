package blitter

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

// Command is the fifteen-word hardware descriptor delivered by DMA channel 12.
type Command [15]uint32

type pixelFormat struct {
	bpp    uint32
	planar uint32 // 0: indexed/cell, 1: YUV422, 2: YUV420
}

type surface struct {
	f0, f1, c0, c1 uint32
	pitchPx, x, y  uint32
}

func format(op uint32) (pixelFormat, error) {
	if op&0xf0000 == 0xf0000 {
		return pixelFormat{bpp: 16}, nil
	}
	switch f := (op >> 18) & 7; f {
	case 0, 1, 2, 3:
		return pixelFormat{bpp: 1 << f}, nil
	case 6:
		return pixelFormat{bpp: 8, planar: 1}, nil
	case 7:
		return pixelFormat{bpp: 8, planar: 2}, nil
	default:
		return pixelFormat{}, fmt.Errorf("blitter: unsupported pixel format %d", f)
	}
}

func physical(v uint32) uint32 { return v&0x1fffffff | memory.DRAMBase }

func side(words Command, at int) surface {
	return surface{
		f0: physical(words[at]), f1: physical(words[at+1]),
		c0: physical(words[at+2]), c1: physical(words[at+3]),
		pitchPx: (words[at+4] & 0xffff) + 1,
		x:       words[at+5] & 0x7ff, y: (words[at+5] >> 16) & 0x3ff,
	}
}

func sourceDescriptorPresent(words Command) bool {
	s := side(words, 1)
	return s.f0 >= memory.DRAMBase && s.f0 < memory.DRAMBase+memory.DRAMSize &&
		s.pitchPx <= 2048 && words[6]>>26 == 0
}

func rowAddress(s surface, line uint32, fieldPair bool, chroma bool, f pixelFormat) uint32 {
	base, row := s.f0, line
	if chroma {
		base = s.c0
	}
	if fieldPair {
		row = line >> 1
		if line&1 != 0 {
			base = s.f1
			if chroma {
				base = s.c1
			}
		}
	}
	if chroma && f.planar == 2 {
		if fieldPair {
			row = line >> 2
		} else {
			row = line >> 1
		}
	}
	return base + row*((s.pitchPx*f.bpp)>>3)
}

func (b *Blitter) read(addr uint32, size bus.Size) (uint32, error) {
	if b.ram == nil {
		return 0, fmt.Errorf("blitter: board DRAM is not bound")
	}
	if addr < memory.DRAMBase || uint64(addr-memory.DRAMBase)+uint64(size) > uint64(b.ram.Size()) {
		return 0, fmt.Errorf("blitter: source address %#08x outside board DRAM", addr)
	}
	return b.ram.Read(addr-memory.DRAMBase, size), nil
}

func (b *Blitter) write(addr uint32, size bus.Size, value uint32) error {
	if b.ram == nil {
		return fmt.Errorf("blitter: board DRAM is not bound")
	}
	if addr < memory.DRAMBase || uint64(addr-memory.DRAMBase)+uint64(size) > uint64(b.ram.Size()) {
		return fmt.Errorf("blitter: destination address %#08x outside board DRAM", addr)
	}
	b.ram.Write(addr-memory.DRAMBase, size, value)
	return nil
}

func (b *Blitter) get(row, col, bpp uint32) (uint32, error) {
	switch bpp {
	case 16:
		return b.read(row+col*2, bus.Half)
	case 8:
		return b.read(row+col, bus.Byte)
	default:
		at := row + col*bpp/8
		word, err := b.read(at, bus.Byte)
		shift := 8 - bpp - (col * bpp & 7)
		return (word >> shift) & ((1 << bpp) - 1), err
	}
}

func (b *Blitter) put(row, col, bpp, value uint32) error {
	switch bpp {
	case 16:
		return b.write(row+col*2, bus.Half, value&0xffff)
	case 8:
		return b.write(row+col, bus.Byte, value&0xff)
	default:
		at := row + col*bpp/8
		old, err := b.read(at, bus.Byte)
		if err != nil {
			return err
		}
		shift := 8 - bpp - (col * bpp & 7)
		mask := uint32((1<<bpp)-1) << shift
		return b.write(at, bus.Byte, old&^mask|value<<shift&mask)
	}
}

// Execute loads one command and performs its fill or copy synchronously.
func (b *Blitter) Execute(cmd Command) error {
	b.words = cmd
	return b.ExecuteRegisters()
}

// ExecuteRegisters runs the descriptor currently loaded through MMIO or DMA.
func (b *Blitter) ExecuteRegisters() error {
	w := Command(b.words)
	f, err := format(w[0])
	if err != nil {
		return err
	}
	width, height := (w[13]&0x7ff)+1, ((w[13]>>16)&0x3ff)+1
	dst := side(w, 7)
	fieldPair := w[0]&0x600000 != 0
	if w[0]&0x1000000 != 0 { // bit 24 selects fill; bit 23 does not
		value := w[14] & 0xff
		if f.bpp == 16 {
			value = w[14] >> 16
		}
		for y := uint32(0); y < height; y++ {
			row := rowAddress(dst, dst.y+y, fieldPair, false, f)
			for x := uint32(0); x < width; x++ {
				if err := b.put(row, dst.x+x, f.bpp, value); err != nil {
					return err
				}
			}
		}
		return nil
	}

	packed := !sourceDescriptorPresent(w)
	if packed && f.planar != 0 {
		return fmt.Errorf("blitter: planar copy needs a complete source descriptor")
	}
	src := side(w, 1)
	if packed {
		src = surface{f0: physical(w[1]), pitchPx: width}
	}
	// Read before writing: the firmware copies between overlapping surfaces.
	pixels := make([]uint16, int(width*height))
	for y := uint32(0); y < height; y++ {
		row := rowAddress(src, src.y+y, !packed && fieldPair, false, f)
		for x := uint32(0); x < width; x++ {
			pixel, err := b.get(row, src.x+x, f.bpp)
			if err != nil {
				return err
			}
			pixels[y*width+x] = uint16(pixel) // #nosec G115 -- at most 16 bits per pixel
		}
	}
	for y := uint32(0); y < height; y++ {
		row := rowAddress(dst, dst.y+y, fieldPair, false, f)
		for x := uint32(0); x < width; x++ {
			if err := b.put(row, dst.x+x, f.bpp, uint32(pixels[y*width+x])); err != nil {
				return err
			}
		}
	}
	if f.planar != 0 {
		cx0 := src.x &^ 1
		cx1 := (src.x + width + 1) &^ 1
		dcx0 := dst.x &^ 1
		step := uint32(1)
		if f.planar == 2 {
			step = 2
		}
		chroma := make([]byte, 0, (cx1-cx0)*((height+step-1)/step))
		for y := uint32(0); y < height; y += step {
			row := rowAddress(src, src.y+y, fieldPair, true, f)
			for x := cx0; x < cx1; x++ {
				value, err := b.read(row+x, bus.Byte)
				if err != nil {
					return err
				}
				chroma = append(chroma, byte(value)) // #nosec G115 -- bus.Byte reads at most 8 bits
			}
		}
		index := 0
		for y := uint32(0); y < height; y += step {
			row := rowAddress(dst, dst.y+y, fieldPair, true, f)
			for x := uint32(0); x < cx1-cx0; x++ {
				if err := b.write(row+dcx0+x, bus.Byte, uint32(chroma[index])); err != nil {
					return err
				}
				index++
			}
		}
	}
	return nil
}
