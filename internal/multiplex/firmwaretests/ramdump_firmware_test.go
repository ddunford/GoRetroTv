package firmwaretests_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
)

// Dump guest RAM regions so they can be DISASSEMBLED, which is the one thing
// this project could not do until now.
//
// The application runs decompressed in RAM, and the addresses the instruments
// report -- the 544 a 0xC1 section wakes, the grid's row loop, anything else
// found by census -- have only ever been numbers. There is no MIPS
// disassembler in the Go toolchain and none was installed here, so every
// address in the record is named by BEHAVIOUR rather than by reading the code.
//
// This writes the bytes out at their guest addresses. It deliberately dumps
// from a RESTORED BOX rather than from firmware/application-ram-image.bin,
// because the image's load base would have to be assumed and an off-by-one
// there produces a disassembly that looks plausible and decodes the wrong
// instruction boundary -- the same failure mode as reading a section one byte
// out. What the CPU executes is what gets dumped.
//
// Render with, for a region based at 0x800A8150:
//
//	mips-linux-gnu-objdump -D -b binary -m mips:16 -EB \
//	  --adjust-vma=0x800A8150 .artifacts/ram-800A8150.bin
//
// MIPS16 is the right mode: these regions step two bytes at a time, which is
// what the census showed.
func TestDumpGuestRAMForDisassembly(t *testing.T) {
	box := restoredBox(t)
	regions := []struct {
		name   string
		lo, hi uint32
		what   string
	}{
		{"80004740", 0x80004740, 0x80004790, "the three PCs near the base"},
		{"8007F0E0", 0x8007F0E0, 0x8007F220, "beside alloc2 at 0x8007F77C"},
		{"800A8150", 0x800A8150, 0x800A8740, "the hottest region, 864 executions at 0x800A8176"},
		{"800C4C20", 0x800C4C20, 0x800C5160, "the largest region, the 0xC1 consumer proper"},
		{"800CC220", 0x800CC220, 0x800CCF00, "includes 0x800CCECC, the allocator the record names"},
		{"800D1B20", 0x800D1B20, 0x800D1B60, "the tail"},
	}
	dir := filepath.Join("..", "..", "..", ".artifacts")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, r := range regions {
		blob := make([]byte, 0, r.hi-r.lo)
		for a := r.lo; a < r.hi; a++ {
			// KSEG0/KSEG1 both map straight onto physical DRAM here.
			blob = append(blob, byte(box.RAM.Read(a&0x1fffffff, bus.Byte)))
		}
		nonZero := 0
		for _, b := range blob {
			if b != 0 {
				nonZero++
			}
		}
		// A region of zeroes means the dump missed -- wrong base, or a page the
		// application never filled. That is a harness failure, not a result.
		if nonZero*4 < len(blob) {
			t.Errorf("%s is %d/%d non-zero bytes; that is not code, so this dump is wrong",
				r.name, nonZero, len(blob))
			continue
		}
		name := filepath.Join(dir, fmt.Sprintf("ram-%s.bin", r.name))
		if err := os.WriteFile(name, blob, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s  %5d bytes, %4d non-zero  %s", filepath.Base(name), len(blob), nonZero, r.what)
	}
}
