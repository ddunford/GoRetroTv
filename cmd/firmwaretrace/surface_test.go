package main

import (
	"crypto/sha256"
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
	got, distinct, sha, err := surfaceDigest(ram, base, 4)
	if err != nil {
		t.Fatal(err)
	}
	const want = 0xD4A9E9C4 // independently calculated FNV-1a 32-bit for 12 34 12 FF.
	if got != want || distinct != 3 || sha != sha256.Sum256([]byte{0x12, 0x34, 0x12, 0xff}) {
		t.Fatalf("digest=%08X distinct=%d sha256=%x, want %08X and 3 with matching SHA-256", got, distinct, sha, want)
	}
	if _, _, _, err := surfaceDigest(ram, memory.DRAMBase+memory.DRAMSize-2, 4); err == nil {
		t.Fatal("out-of-bounds surface was accepted")
	}
}
