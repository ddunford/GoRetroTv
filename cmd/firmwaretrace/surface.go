package main

import (
	"hash/fnv"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

const (
	surfaceBase   = 0x80584048
	surfaceLength = 720 * 576
)

// surfaceDigest hashes the raw guest RAM bytes backing the documented OSD
// surface, independent of the display's palette or browser presentation.
func surfaceDigest(ram *memory.RAM, base, length uint32) (uint32, int) {
	h := fnv.New32a()
	var seen [256]bool
	distinct := 0
	var one [1]byte
	for off := uint32(0); off < length; off++ {
		one[0] = byte(ram.Read(base-memory.DRAMBase+off, bus.Byte)) // #nosec G115 -- bus.Byte yields one byte.
		_, _ = h.Write(one[:])
		if !seen[one[0]] {
			seen[one[0]] = true
			distinct++
		}
	}
	return h.Sum32(), distinct
}
