package firmwaretests_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/firmware"
)

// EVERY NATIVE MODULE, NOT JUST MODULE 1.
//
// tools/opentv-natives.py resolves 236 natives and every one of them is MODULE 1, because that is
// the only function array whose address anyone has written down (0x9FC29F04). So a census can say
// "(1,0x5B) ran 20 times" and name it, and a call to any other module is a number and nothing else.
//
// That is now in the way. The ALL CHANNELS grid keeps a 336-byte record per row and tests a
// halfword in it against 2 before drawing a programme; the record is CREATED with that halfword
// set to 1, by `scall (0x0c, 0x08)` at o-code 0x9FC77F23 -- module TWELVE, which the shim table
// does not cover. Naming it is the difference between "a native creates the row" and a function
// Ghidra can read.
//
// THE TABLE IS BUILT AT BOOT IN DRAM, which is why this is a firmware probe and not an addition to
// the static tool. opentv-natives.py's own header records the shape: the dispatcher indexes a table
// of {function array, count} pairs held at the word in 0x8006E71C, and a probe against the running
// box confirmed it holds the same record pointers as the flash copy for module 1.
//
// SO MODULE 1 IS THE CONTROL, and it is the whole point of doing it this way. This walk must
// independently rediscover that module 1's array is at 0x9FC29F04 with 236 entries. If it does not,
// the stride or the base is wrong and every other module it reports is noise -- which is exactly
// how a table walk goes wrong while looking self-consistent.
//
// IT ONLY READS.
func TestEveryNativeModuleTheDispatcherKnows(t *testing.T) {
	const (
		modulePool = 0x8006E71C // the word here is the module table's base
		// Module 1, from tools/opentv-natives.py. The control.
		wantModule1Array = 0x9FC29F04
		wantModule1Count = 236
		maxModules       = 64
	)
	box := restoredBox(t)
	// THE TABLE IS IN DRAM AND THE ARRAYS IT POINTS AT ARE IN FLASH, so one reader will not do.
	// Reading a 0x9FC... address through box.RAM masks it to 0x1FC..., which is past the end of
	// DRAM and comes back as a confident zero -- which is exactly what the first run of this
	// reported about module 12's function 8.
	flash, err := os.ReadFile(filepath.Join("..", "..", "..", "firmware", firmware.FileU202))
	if err != nil {
		t.Skipf("the flash image is not installed: %v", err)
	}
	word := func(virtual uint32) uint32 {
		if virtual >= 0x9FC00000 && virtual < 0xA0000000 {
			off := int(virtual - 0x9FC00000)
			if off < 0 || off+4 > len(flash) {
				t.Fatalf("harness: flash address %08X is outside the %d-byte image",
					virtual, len(flash))
			}
			return binary.BigEndian.Uint32(flash[off:])
		}
		return box.RAM.Read(virtual&0x1fffffff, bus.Word)
	}
	plausible := func(at uint32) bool {
		return (at >= 0x80000000 && at < 0x80800000) || (at >= 0x9FC00000 && at < 0xA0000000)
	}

	base := word(modulePool)
	t.Logf("the module table is at %08X (the word in %08X)", base, uint32(modulePool))
	if !plausible(base) || base&3 != 0 {
		t.Fatalf("harness: %08X is not a word-aligned flash or DRAM address, so it is not the "+
			"module table and nothing below would mean anything", base)
	}

	type module struct {
		n     int
		array uint32
		count uint32
	}
	var modules []module
	for n := 0; n < maxModules; n++ {
		array := word(base + uint32(n)*8)     // #nosec G115 -- bounded by maxModules
		count := word(base + uint32(n)*8 + 4) // #nosec G115 -- bounded by maxModules
		if array == 0 && count == 0 {
			continue
		}
		modules = append(modules, module{n: n, array: array, count: count})
	}
	if len(modules) == 0 {
		t.Fatalf("harness: not one entry in the table at %08X is non-zero, so the stride of 8 or "+
			"the base is wrong", base)
	}

	// THE CONTROL, CHECKED BEFORE ANYTHING IS BELIEVED.
	var found *module
	for i := range modules {
		if modules[i].n == 1 {
			found = &modules[i]
		}
	}
	if found == nil {
		t.Fatalf("harness: the walk found %d modules and none of them is module 1, which is the "+
			"one module whose address is known", len(modules))
	}
	t.Logf("module 1 reads as array %08X count %d; expected %08X and %d",
		found.array, found.count, uint32(wantModule1Array), wantModule1Count)
	if found.array != wantModule1Array || found.count != wantModule1Count {
		t.Fatalf("harness: module 1 does not match the address tools/opentv-natives.py resolves " +
			"236 natives from, so this walk's stride or base is wrong and every other module it " +
			"reports below is noise")
	}

	t.Logf("=== every native module the dispatcher knows ===")
	for _, m := range modules {
		note := ""
		switch {
		case m.n == 1:
			note = "  <- the one tools/opentv-natives.py covers"
		case m.n == 12:
			note = "  <- MODULE 12: scall(0x0c,0x08) creates the grid's row record"
		}
		t.Logf("    module %2d  array %08X  %4d functions%s", m.n, m.array, m.count, note)
	}

	// AND THE ONE THIS WAS BUILT FOR.
	for _, m := range modules {
		if m.n != 12 {
			continue
		}
		const fn = 8
		if fn >= int(m.count) {
			t.Fatalf("module 12 declares %d functions, so there is no function %d and the o-code's "+
				"scall(0x0c,0x08) is not what it appears to be", m.count, fn)
		}
		rec := word(m.array + uint32(fn)*4) // #nosec G115 -- small constant
		if !plausible(rec) {
			t.Fatalf("module 12 entry %d points at %08X, which is not an address", fn, rec)
		}
		impl := word(rec)
		desc := word(rec + 4)
		t.Logf("=== module 12, function 8 -- the call that creates a grid row ===")
		t.Logf("    record   %08X", rec)
		t.Logf("    impl     %08X   (MIPS16 bit %d)", impl&^1, impl&1)
		t.Logf("    argdesc  %08X", desc)
		t.Logf("    decompile it:  ./ctl.sh ghidra:decompile %#08x", impl&^1)
		return
	}
	t.Fatalf("harness: the table has no module 12 at all, yet the grid's o-code calls " +
		"scall(0x0c,0x08) six times. Either the module number is not the first operand or this " +
		"walk is reading the wrong table")
}
