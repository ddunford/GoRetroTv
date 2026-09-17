package dma

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func (d *Controller) readDRAM(addr uint32, size bus.Size) (uint32, error) {
	if d.ram == nil {
		return 0, fmt.Errorf("board DRAM is not bound")
	}
	off := addr & 0x1fffffff
	if uint64(off)+uint64(size) > uint64(d.ram.Size()) {
		return 0, fmt.Errorf("DRAM address %#08x outside board memory", addr)
	}
	return d.ram.Read(off, size), nil
}

// Run executes one descriptor and raises its completion bit. A bad descriptor
// fails visibly; it never reports a successful DMA transfer of invented bytes.
func (d *Controller) Run(channel uint8) error {
	if channel >= ChannelCount {
		return fmt.Errorf("channel %d out of range", channel)
	}
	descriptor := d.descriptors[channel]
	if descriptor == 0 {
		return fmt.Errorf("channel %d has no descriptor", channel)
	}
	src, err := d.readDRAM(descriptor+12, bus.Word)
	if err != nil {
		return fmt.Errorf("channel %d source: %w", channel, err)
	}
	ctl, err := d.readDRAM(descriptor+20, bus.Word)
	if err != nil {
		return fmt.Errorf("channel %d control: %w", channel, err)
	}
	end, err := d.readDRAM(descriptor+32, bus.Word)
	if err != nil {
		return fmt.Errorf("channel %d end: %w", channel, err)
	}
	length := ctl & 0x3fff
	if end >= src {
		length = end - src + 1
	}
	if length > memory.DRAMSize {
		return fmt.Errorf("channel %d length %d exceeds board DRAM", channel, length)
	}
	switch channel {
	case 12:
		if d.blitter == nil {
			return fmt.Errorf("channel 12 blitter is not bound")
		}
		words := length / 4
		if words > 15 {
			words = 15
		}
		for i := uint32(0); i < words; i++ {
			value, err := d.readDRAM(src+i*4, bus.Word)
			if err != nil {
				return fmt.Errorf("channel 12 command word %d: %w", i, err)
			}
			d.blitter.Write(i*4, bus.Word, value)
		}
		if err := d.blitter.ExecuteRegisters(); err != nil {
			return fmt.Errorf("channel 12 blitter: %w", err)
		}
	case 8:
		if d.video == nil {
			return fmt.Errorf("channel 8 video RAM is not bound")
		}
		plane := make([]byte, length)
		for i := uint32(0); i < length; i++ {
			value, err := d.readDRAM(src+i, bus.Byte)
			if err != nil {
				return fmt.Errorf("channel 8 source byte %d: %w", i, err)
			}
			plane[i] = byte(value) // #nosec G115 -- bus.Byte reads at most 8 bits
		}
		d.video.Upload(d.video.Read(0xd0, bus.Word), plane)
	}
	d.statusA |= 1 << channel
	d.updateLine()
	return nil
}
