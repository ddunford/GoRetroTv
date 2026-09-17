package dma

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func (d *Controller) readDRAM(addr uint32, size bus.Size) (uint32, error) {
	off, err := d.dramRange(addr, uint64(size))
	if err != nil {
		return 0, err
	}
	return d.ram.Read(off, size), nil
}

func (d *Controller) dramRange(addr uint32, length uint64) (uint32, error) {
	if d.ram == nil {
		return 0, fmt.Errorf("board DRAM is not bound")
	}
	// Descriptors and source buffers may use a physical address or either
	// unmapped DRAM alias. Reject other segments before masking their high bits.
	if addr >= 0x20000000 && (addr < 0x80000000 || addr >= 0xc0000000) {
		return 0, fmt.Errorf("DRAM address %#08x is not an unmapped alias", addr)
	}
	off := addr & 0x1fffffff
	if length == 0 || uint64(off)+length > uint64(d.ram.Size()) || uint64(addr)+length > 1<<32 {
		return 0, fmt.Errorf("DRAM address %#08x outside board memory", addr)
	}
	return off, nil
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
	if _, err := d.dramRange(descriptor, DescriptorSize); err != nil {
		return fmt.Errorf("channel %d descriptor: %w", channel, err)
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
	length := uint64(ctl & 0x3fff)
	if end >= src {
		length = uint64(end) - uint64(src) + 1
	}
	if length == 0 || length > memory.DRAMSize {
		return fmt.Errorf("channel %d length %d exceeds board DRAM", channel, length)
	}
	if channel == 8 || channel == 12 {
		if _, err := d.dramRange(src, length); err != nil {
			return fmt.Errorf("channel %d source: %w", channel, err)
		}
	}
	switch channel {
	case 5:
		if d.transport == nil || d.flash == nil {
			return fmt.Errorf("channel 5 transport path is not bound")
		}
		if length > 1<<20 {
			return fmt.Errorf("channel 5 transfer exceeds 1 MiB")
		}
		data := make([]byte, int(length)) // #nosec G115 -- capped at 1 MiB above
		const flashPhysical = memory.FlashU202 & 0x1fffffff
		if src >= flashPhysical && uint64(src-flashPhysical)+length <= uint64(d.flash.Size()) {
			for i := range data {
				data[i] = byte(d.flash.Read(src-flashPhysical+uint32(i), bus.Byte)) // #nosec G115 -- transfer capped above
			}
		} else {
			if _, err := d.dramRange(src, length); err != nil {
				return fmt.Errorf("channel 5 source: %w", err)
			}
			for i := range data {
				data[i] = byte(d.ram.Read((src&0x1fffffff)+uint32(i), bus.Byte)) // #nosec G115 -- DRAM range checked above
			}
		}
		if err := d.transport.PushTransport(data); err != nil {
			return fmt.Errorf("channel 5 transport: %w", err)
		}
	case 12:
		if d.blitter == nil {
			return fmt.Errorf("channel 12 blitter is not bound")
		}
		if length < 15*4 {
			return fmt.Errorf("channel 12 command has %d bytes; need 60", length)
		}
		words := length / 4
		if words > 15 {
			words = 15
		}
		for i := uint64(0); i < words; i++ {
			value, err := d.readDRAM(src+uint32(i*4), bus.Word) // #nosec G115 -- at most 15 words
			if err != nil {
				return fmt.Errorf("channel 12 command word %d: %w", i, err)
			}
			d.blitter.Write(uint32(i*4), bus.Word, value) // #nosec G115 -- at most 15 words
		}
		if err := d.blitter.ExecuteRegisters(); err != nil {
			return fmt.Errorf("channel 12 blitter: %w", err)
		}
	case 8:
		if d.video == nil {
			return fmt.Errorf("channel 8 video RAM is not bound")
		}
		plane := make([]byte, int(length)) // #nosec G115 -- capped at 32 MiB above
		for i := uint64(0); i < length; i++ {
			value, err := d.readDRAM(src+uint32(i), bus.Byte) // #nosec G115 -- capped at 32 MiB above
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
