// Package demux models the EMMA section filter and its 32 section RAM rings.
package demux

import (
	"fmt"
)

const (
	// FilterCount is the number of hardware section buffers and PID channels.
	FilterCount = 32
	// RingSize is the byte length of each filter's circular section buffer.
	RingSize = 0x3000
	// SectionRAMBase is the uncached address of the first ring.
	SectionRAMBase = 0xA07A0000
	// SectionRAMSize spans all 32 rings in one physical bus region.
	SectionRAMSize  = FilterCount * RingSize
	filterRecordRAM = 0x80142D34
	filterRecordLen = 20
)

// RingBase is the firmware-visible uncached start of one filter's 12 KB ring.
func RingBase(filter uint8) uint32 { return SectionRAMBase + uint32(filter)*RingSize }

// RecordBase is the firmware's five-word record for one ring, in ordinary DRAM.
// The firmware owns those records; the demux owns the section RAM they describe.
func RecordBase(filter uint8) uint32 { return filterRecordRAM + uint32(filter)*filterRecordLen }

// ringOffset is the offset inside board DRAM. The section rings are a DRAM
// subregion, not a second physical memory device or a second snapshot key.
func ringOffset(filter uint8) (uint32, error) {
	if filter >= FilterCount {
		return 0, fmt.Errorf("demux: filter %d out of range", filter)
	}
	return (SectionRAMBase & 0x1fffffff) + uint32(filter)*RingSize, nil
}
