package main

import (
	"crypto/sha256"
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

const (
	surfaceBase   = 0x80584048
	surfaceLength = 720 * 576
)

// surfaceDigest hashes the raw guest RAM bytes backing the documented OSD
// surface, independent of the display's palette or browser presentation.
func surfaceDigest(ram *memory.RAM, base, length uint32) (uint32, int, [sha256.Size]byte, error) {
	if ram == nil || base < memory.DRAMBase || length == 0 ||
		uint64(base-memory.DRAMBase)+uint64(length) > uint64(ram.Size()) {
		return 0, 0, [sha256.Size]byte{}, fmt.Errorf("raw OSD surface is outside bound DRAM")
	}
	bytes := make([]byte, length)
	var seen [256]bool
	distinct := 0
	for off := uint32(0); off < length; off++ {
		b := byte(ram.Read(base-memory.DRAMBase+off, bus.Byte)) // #nosec G115 -- bus.Byte yields one byte.
		bytes[off] = b
		if !seen[b] {
			seen[b] = true
			distinct++
		}
	}
	return statehash.HashBytes(bytes), distinct, sha256.Sum256(bytes), nil
}
