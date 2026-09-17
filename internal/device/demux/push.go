package demux

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

// BindRAM connects the section engine to the board's existing DRAM device.
// The 32 rings are a subregion of that DRAM and share its snapshot.
func (d *Demux) BindRAM(ram *memory.RAM) error {
	if ram == nil || ram.Size() < (SectionRAMBase&0x1fffffff)+SectionRAMSize {
		return fmt.Errorf("demux: section RAM needs board DRAM through %#x", (SectionRAMBase&0x1fffffff)+SectionRAMSize)
	}
	d.ram = ram
	return nil
}

// Push delivers a DVB section to the first armed PID channel that requested it.
// PID and filter remain distinct values even when their numbers overlap.
func (d *Demux) Push(pid uint16, section []byte) error {
	if d.ram == nil {
		return fmt.Errorf("demux: board DRAM is not bound")
	}
	if pid > 0x1fff {
		return fmt.Errorf("demux: PID %#x out of range", pid)
	}
	if err := validateSection(section); err != nil {
		return err
	}
	for filter := uint8(0); filter < FilterCount; filter++ {
		if !d.pidWritten[filter] || d.enable[2]&(1<<filter) == 0 ||
			d.pidChannels[filter]&0x1fff != uint32(pid) {
			continue
		}
		return d.pushFilter(filter, section)
	}
	return fmt.Errorf("demux: no armed filter requested PID %#x", pid)
}

func validateSection(section []byte) error {
	if len(section) < 3 || len(section)+1 > RingSize {
		return fmt.Errorf("demux: invalid section size %d", len(section))
	}
	declared := int(section[1]&0x0f)<<8 | int(section[2])
	if declared+3 != len(section) {
		return fmt.Errorf("demux: section length %d differs from %d bytes", declared, len(section))
	}
	if section[1]&0x80 != 0 || section[0] == 0x73 {
		if len(section) < 7 || mpegCRC(section) != 0 {
			return fmt.Errorf("demux: invalid MPEG section CRC")
		}
	}
	return nil
}

func mpegCRC(data []byte) uint32 {
	crc := ^uint32(0)
	for _, b := range data {
		crc ^= uint32(b) << 24
		for bit := 0; bit < 8; bit++ {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func (d *Demux) pushFilter(filter uint8, section []byte) error {
	physicalBase, err := ringOffset(filter)
	if err != nil {
		return err
	}
	base := uint32(filter) * RingSize
	ptr := d.writePointer[filter]
	if ptr == 0 {
		ptr = base
	}
	n := uint32(len(section)) // #nosec G115 -- validateSection bounds this to RingSize
	within := ptr - base
	if within >= RingSize || within+n+1 > RingSize {
		ptr = base
	}
	for i, b := range section {
		d.ram.Write(physicalBase+(ptr-base)+uint32(i), bus.Byte, uint32(b)) // #nosec G115 -- section length is bounded by RingSize
	}
	d.ram.Write(physicalBase+(ptr-base)+n, bus.Byte, 0)
	d.writePointer[filter] = ptr + n + 1
	d.complete(filter)
	return nil
}
