package memory_test

import (
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/memory"
)

// The cached and uncached windows onto the same DRAM, and the two flash chips' mirrors, written
// as the record and the oracle's memory map write them.
const (
	dramCached    = memory.DRAMBase // 0x80000000
	dramUncached  = 0xA0000000
	flashU202     = memory.FlashU202 // 0xBFC00000, the reset vector
	flashU202Mirr = 0x9FC00000
	flashU203     = memory.FlashU203 // 0xBF800000
	flashU203Mirr = 0x9F800000
)

func newRAM(t *testing.T, size uint32) *memory.RAM {
	t.Helper()
	r, err := memory.NewRAM("dram", size)
	if err != nil {
		t.Fatalf("NewRAM: %v", err)
	}
	return r
}

func newFlash(t *testing.T, name string, image []byte) *memory.Flash {
	t.Helper()
	f, err := memory.NewFlash(name, image)
	if err != nil {
		t.Fatalf("NewFlash: %v", err)
	}
	return f
}

// A stand-in for a flash image: not the real firmware, which is not redistributable and is not
// needed to prove a decode.
func image(size int, seed byte) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = seed ^ byte(i) ^ byte(i>>8)
	}
	return b
}

// board wires the three memories at the addresses the board puts them at.
func board(t *testing.T) (*bus.Bus, *memory.RAM, *memory.Flash, *memory.Flash) {
	t.Helper()
	b := bus.New()
	ram := newRAM(t, memory.DRAMSize)
	u202 := newFlash(t, "U202", image(memory.FlashSize, 0xA5))
	u203 := newFlash(t, "U203", image(memory.FlashSize, 0x5A))
	for _, m := range []struct {
		base uint32
		size uint32
		dev  bus.Device
	}{
		{memory.DRAMBase, memory.DRAMSize, ram},
		{memory.FlashU202, memory.FlashSize, u202},
		{memory.FlashU203, memory.FlashSize, u203},
	} {
		if err := b.Attach(m.base, m.size, m.dev); err != nil {
			t.Fatalf("Attach %s at %#08x: %v", m.dev.Name(), m.base, err)
		}
	}
	return b, ram, u202, u203
}

// TC-1.3: the two DRAM windows are one memory, in both directions and at every width.
func TestDramAliasesAgree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		size  bus.Size
		off   uint32
		value uint32
	}{
		{"byte", bus.Byte, 0x000000, 0xA5},
		{"half", bus.Half, 0x000100, 0xBEEF},
		{"word", bus.Word, 0x105C44, 0x80081C58}, // an address the record quotes
		{"word at the top of DRAM", bus.Word, memory.DRAMSize - 4, 0xDEADBEEF},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// A bus is single-threaded by design, so a parallel subtest gets its own board
			// rather than sharing one.
			b, _, _, _ := board(t)

			b.Write(dramCached+tc.off, tc.size, tc.value)
			if got := b.Read(dramUncached+tc.off, tc.size); got != tc.value {
				t.Fatalf("written cached at %#08x, read uncached: %#x, want %#x",
					dramCached+tc.off, got, tc.value)
			}

			b.Write(dramUncached+tc.off, tc.size, ^tc.value&mask(tc.size))
			if got := b.Read(dramCached+tc.off, tc.size); got != ^tc.value&mask(tc.size) {
				t.Fatalf("written uncached at %#08x, read cached: %#x, want %#x",
					dramUncached+tc.off, got, ^tc.value&mask(tc.size))
			}
		})
	}
}

func mask(s bus.Size) uint32 {
	//exhaustive:ignore
	switch s {
	case bus.Byte:
		return 0xFF
	case bus.Half:
		return 0xFFFF
	default:
		return 0xFFFFFFFF
	}
}

// The machine is big-endian, so the top byte of a word is the one at the lowest address. Getting
// this the other way round produces values that are plausible and wrong, which is the whole genre.
func TestDramIsBigEndian(t *testing.T) {
	t.Parallel()
	b, _, _, _ := board(t)

	b.Write(dramCached+0x40, bus.Word, 0x11223344)
	for i, want := range []uint32{0x11, 0x22, 0x33, 0x44} {
		if got := b.Read(dramCached+0x40+uint32(i), bus.Byte); got != want {
			t.Fatalf("byte %d of 0x11223344 is %#x, want %#x", i, got, want)
		}
	}
	if got := b.Read(dramUncached+0x40, bus.Half); got != 0x1122 {
		t.Fatalf("the top halfword is %#x, want 0x1122", got)
	}
}

// TC-1.3's other half: the flash region refuses writes, through either window, and the refusal is
// counted rather than silent - a flash that swallows a write is indistinguishable from one that
// took it, and the firmware's answer to that difference is to spin for ever.
func TestFlashRefusesWritesThroughBothWindows(t *testing.T) {
	t.Parallel()
	b, _, u202, _ := board(t)

	before := b.Read(flashU202+0x1000, bus.Word)
	if before == 0 {
		t.Fatal("the test image reads as zero at the address under test, so this proves nothing")
	}

	b.Write(flashU202+0x1000, bus.Word, 0xFFFFFFFF)
	if got := b.Read(flashU202+0x1000, bus.Word); got != before {
		t.Fatalf("a write through the reset window changed flash: %#x, was %#x", got, before)
	}
	b.Write(flashU202Mirr+0x1000, bus.Word, 0x00000000)
	if got := b.Read(flashU202Mirr+0x1000, bus.Word); got != before {
		t.Fatalf("a write through the cached mirror changed flash: %#x, was %#x", got, before)
	}
	if u202.Refused() != 2 {
		t.Fatalf("the part counted %d refused writes, want 2", u202.Refused())
	}
}

func TestFlashMirrorsReadTheSameBytes(t *testing.T) {
	t.Parallel()
	b, _, _, _ := board(t)

	for _, pair := range [][2]uint32{{flashU202, flashU202Mirr}, {flashU203, flashU203Mirr}} {
		for _, off := range []uint32{0, 4, 0x20000, memory.FlashSize - 4} {
			direct := b.Read(pair[0]+off, bus.Word)
			mirror := b.Read(pair[1]+off, bus.Word)
			if direct != mirror {
				t.Fatalf("%#08x reads %#x and its mirror %#08x reads %#x",
					pair[0]+off, direct, pair[1]+off, mirror)
			}
		}
	}
}

// The two chips are separate parts holding different bytes. When the predecessor modelled only
// one, the other answered zero, the content manager read thirty blocks of zeros and set about
// mirroring 1.5 MB into a chip that was not there.
func TestTheTwoFlashPartsAreSeparate(t *testing.T) {
	t.Parallel()
	b, _, _, _ := board(t)

	if a, c := b.Read(flashU202, bus.Word), b.Read(flashU203, bus.Word); a == c {
		t.Fatalf("both parts read %#x at offset 0, so this test cannot tell them apart", a)
	}
	if b.UnmappedTotals().Reads != 0 {
		t.Fatalf("a documented address went unmapped: %+v", b.Unmapped())
	}
}

// Flash is non-volatile. A reset that wiped it would leave the machine with no reset vector, and
// the oracle's own reset leaves both chips' arrays alone.
func TestResetLeavesFlashAloneAndZeroesDram(t *testing.T) {
	t.Parallel()
	b, _, u202, _ := board(t)

	vector := b.Read(flashU202, bus.Word)
	b.Write(dramCached+0x80, bus.Word, 0xC0FFEE)
	b.Write(flashU202, bus.Word, 0) // refused, but it bumps the count Reset clears

	b.Reset()

	if got := b.Read(flashU202, bus.Word); got != vector {
		t.Fatalf("Reset changed the reset vector: %#x, was %#x", got, vector)
	}
	if got := b.Read(dramCached+0x80, bus.Word); got != 0 {
		t.Fatalf("Reset left %#x in DRAM, want 0", got)
	}
	if u202.Refused() != 0 {
		t.Fatalf("Reset left %d refused writes counted, want 0", u202.Refused())
	}
}

// The dirty set is what keeps the checkpoint hash affordable: re-digesting 32 MB every thousand
// instructions would dominate the run (spike 002), so it re-digests only what changed.
func TestDirtyPagesTrackWritesAndClear(t *testing.T) {
	t.Parallel()
	ram := newRAM(t, 64*memory.DirtyPageLen)

	if n := ram.DirtyCount(); n != 0 {
		t.Fatalf("fresh DRAM has %d dirty pages, want 0", n)
	}

	ram.Write(0, bus.Word, 1)
	ram.Write(3*memory.DirtyPageLen+16, bus.Byte, 2)
	ram.Read(5*memory.DirtyPageLen, bus.Word) // a read must not dirty anything

	var pages []uint32
	ram.EachDirtyPage(func(p uint32, data []byte) {
		if uint32(len(data)) != memory.DirtyPageLen {
			t.Fatalf("page %d came back %d bytes, want %d", p, len(data), memory.DirtyPageLen)
		}
		pages = append(pages, p)
	})
	if len(pages) != 2 || pages[0] != 0 || pages[1] != 3 {
		t.Fatalf("dirty pages are %v, want [0 3] in address order", pages)
	}

	ram.ClearDirty()
	if n := ram.DirtyCount(); n != 0 {
		t.Fatalf("%d pages still dirty after ClearDirty", n)
	}

	// A write straddling a page boundary dirties both, or the digest misses half of it.
	ram.Write(memory.DirtyPageLen-2, bus.Word, 0x12345678)
	pages = pages[:0]
	ram.EachDirtyPage(func(p uint32, _ []byte) { pages = append(pages, p) })
	if len(pages) != 2 || pages[0] != 0 || pages[1] != 1 {
		t.Fatalf("a write across a page boundary dirtied %v, want [0 1]", pages)
	}

	ram.ClearDirty()
	ram.MarkAllDirty()
	if n := ram.DirtyCount(); n != 64 {
		t.Fatalf("MarkAllDirty marked %d of 64 pages", n)
	}
}

// An access running off the end of a device would otherwise index past the slice. It answers with
// zeroes beyond the end and counts itself, so the condition is visible rather than silent.
//
// The low end of memory is loaded with a recognisable pattern first, because "zero beyond the end"
// and "wrapped around to the bottom" give the same answer on a fresh device - which is how the
// first version of this test passed an implementation that wrapped.
func TestAnAccessRunningOffTheEndIsAnsweredAndCounted(t *testing.T) {
	t.Parallel()
	ram := newRAM(t, 2*memory.DirtyPageLen)
	top := uint32(2*memory.DirtyPageLen - 1)
	ram.Write(0, bus.Word, 0x11223344)

	ram.Write(top, bus.Byte, 0xAB)
	if got := ram.Read(top, bus.Word); got != 0xAB000000 {
		t.Fatalf("a word read at the last byte returned %#x, want 0xAB000000 - anything carrying "+
			"0x11, 0x22 or 0x33 means it wrapped to the bottom of memory", got)
	}
	if got := ram.Read(top+1, bus.Word); got != 0 {
		t.Fatalf("a word read entirely past the end returned %#x, want 0", got)
	}

	ram.Write(top, bus.Word, 0x55667788)
	if got := ram.Read(top, bus.Byte); got != 0x55 {
		t.Fatalf("the in-range byte of a straddling write is %#x, want 0x55", got)
	}
	if got := ram.Read(0, bus.Word); got != 0x11223344 {
		t.Fatalf("a straddling write wrapped and corrupted the bottom of memory: %#x", got)
	}

	// And the dirty set must not be told a page was written that was not.
	ram.ClearDirty()
	ram.Write(top+8, bus.Word, 0x99)
	if n := ram.DirtyCount(); n != 0 {
		t.Fatalf("a write entirely past the end dirtied %d pages, want 0", n)
	}
}

func TestNewRamRefusesASizeItCannotPage(t *testing.T) {
	t.Parallel()
	for _, size := range []uint32{0, 1, memory.DirtyPageLen - 1, memory.DirtyPageLen + 1} {
		if _, err := memory.NewRAM("dram", size); err == nil {
			t.Fatalf("NewRAM(%d) must be refused", size)
		}
	}
	if _, err := memory.NewRAM("", memory.DirtyPageLen); err == nil {
		t.Fatal("unnamed DRAM must be refused: the name is the snapshot key")
	}
	if _, err := memory.NewFlash("U202", nil); err == nil {
		t.Fatal("an empty flash image must be refused")
	}
}

// NewFlash copies its image, because the loader hands it a buffer it may reuse and a flash chip
// whose contents change underneath the machine reads as a firmware fault.
func TestNewFlashCopiesItsImage(t *testing.T) {
	t.Parallel()
	img := image(memory.DirtyPageLen, 0x11)
	f := newFlash(t, "U202", img)
	before := f.Read(0, bus.Word)

	for i := range img {
		img[i] = 0
	}
	if got := f.Read(0, bus.Word); got != before {
		t.Fatalf("the part changed when the caller's buffer did: %#x, was %#x", got, before)
	}
}

func TestRamHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return newRAM(t, 8*memory.DirtyPageLen) },
		Mutate: func(d bus.Device) {
			d.Write(0x10, bus.Word, 0xDEADBEEF)
			d.Write(3*memory.DirtyPageLen, bus.Half, 0xFFC0)
			d.Read(8*memory.DirtyPageLen-1, bus.Word) // straddles the end, so it is counted
		},
		Disturb: func(d bus.Device) {
			d.Write(0x10, bus.Word, 0x5A5A5A5A)
			d.Write(6*memory.DirtyPageLen, bus.Byte, 0x01)
			d.Read(8*memory.DirtyPageLen-2, bus.Word)
		},
		// The name is the device's identity and is established at construction; a snapshot that
		// could rename the device it restores into would be a snapshot that swapped two devices.
		Constant: []string{"name"},
	})
	if err != nil {
		t.Fatalf("DRAM must hold the contract every device holds: %v", err)
	}
}

func TestFlashHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return newFlash(t, "U202", image(memory.DirtyPageLen, 0xA5)) },
		Mutate: func(d bus.Device) {
			d.Write(0x10, bus.Word, 0xFFFFFFFF)
		},
		Disturb: func(d bus.Device) {
			d.Write(0x20, bus.Word, 0x00000000)
			d.Write(0x24, bus.Word, 0x00000000)
		},
		// The image is configuration in this phase, not state: the part is read-only and the
		// loader verifies it against firmware/MANIFEST.md before the machine starts. TASK-2.8
		// makes it writable, and this exemption must go when it does.
		Constant: []string{"name", "bytes"},
	})
	if err != nil {
		t.Fatalf("flash must hold the contract every device holds: %v", err)
	}
}

func TestRestoreRefusesASnapshotOfADifferentlySizedMachine(t *testing.T) {
	t.Parallel()
	small := newRAM(t, 4*memory.DirtyPageLen)
	state, err := small.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	big := newRAM(t, 8*memory.DirtyPageLen)
	err = big.Restore(state)
	if err == nil {
		t.Fatal("a snapshot of a machine with different DRAM must be refused")
	}
	if !strings.Contains(err.Error(), "DRAM") {
		t.Fatalf("the refusal must say what does not match, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

func TestRestoreRefusesAnotherDevicesSnapshot(t *testing.T) {
	t.Parallel()
	u203 := newFlash(t, "U203", image(memory.DirtyPageLen, 0x5A))
	state, err := u203.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	u202 := newFlash(t, "U202", image(memory.DirtyPageLen, 0xA5))
	if err := u202.Restore(state); err == nil {
		t.Fatal("restoring U203's snapshot into U202 must be refused")
	} else {
		t.Logf("caught: %v", err)
	}
}

// The whole board, snapshotted and restored through the bus, which is how a machine snapshot is
// actually taken.
func TestTheBoardRoundTripsThroughTheBus(t *testing.T) {
	t.Parallel()
	saved, _, _, _ := board(t)
	saved.Write(dramCached+0x105C44, bus.Word, 0x80081C58)
	saved.Write(flashU202, bus.Word, 0) // refused, and counted
	state, err := saved.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	restored, _, u202, _ := board(t)
	restored.Write(dramUncached+0x105C44, bus.Word, 0xBADF00D)
	if err := restored.Restore(state); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := restored.Read(dramCached+0x105C44, bus.Word); got != 0x80081C58 {
		t.Fatalf("DRAM restored %#x, want 0x80081C58", got)
	}
	if u202.Refused() != 1 {
		t.Fatalf("U202 restored %d refused writes, want 1", u202.Refused())
	}
}
