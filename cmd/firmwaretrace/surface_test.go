package main

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

func TestSurfaceDigestUsesGuestBytesInAddressOrder(t *testing.T) {
	t.Parallel()
	ram, err := memory.NewRAM("test", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	base := uint32(memory.DRAMBase + 0x100)
	for i, b := range []byte{0x12, 0x34, 0x12, 0xff} {
		ram.Write(base-memory.DRAMBase+uint32(i), bus.Byte, uint32(b)) // #nosec G115 -- test index is at most three.
	}
	got, distinct := surfaceDigest(ram, base, 4)
	const want = 0xD4A9E9C4 // independently calculated FNV-1a 32-bit for 12 34 12 FF.
	if got != want || distinct != 3 {
		t.Fatalf("digest=%08X distinct=%d, want %08X and 3", got, distinct, want)
	}
}
