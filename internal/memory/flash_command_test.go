package memory_test

import (
	"bytes"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
)

func unlockFlash(f interface {
	Write(uint32, bus.Size, uint32)
}) {
	f.Write(0xaaa, bus.Half, 0xaa)
	f.Write(0x554, bus.Half, 0x55)
}

func TestFlashAutoselectPerPart(t *testing.T) {
	a := newFlash(t, "U202", bytes.Repeat([]byte{0xff}, 0x20000))
	b := newFlash(t, "U203", bytes.Repeat([]byte{0xff}, 0x20000))
	unlockFlash(a)
	a.Write(0xaaa, bus.Half, 0x90)
	if got := a.Read(0, bus.Half); got != 4 {
		t.Fatalf("manufacturer=%#x", got)
	}
	if got := a.Read(2, bus.Half); got != 0x2249 {
		t.Fatalf("device=%#x", got)
	}
	if got := a.Read(0, bus.Word); got != 0x00042249 {
		t.Fatalf("word IDs=%#x", got)
	}
	if got := b.Read(0, bus.Half); got != 0xffff {
		t.Fatalf("other chip entered autoselect: %#x", got)
	}
	a.Write(0, bus.Half, 0xf0)
	if got := a.Read(0, bus.Half); got != 0xffff {
		t.Fatalf("exit read=%#x", got)
	}
}

func TestFlashProgramAndBottomBootSectorErase(t *testing.T) {
	f := newFlash(t, "U202", bytes.Repeat([]byte{0xff}, 0x20000))
	unlockFlash(f)
	f.Write(0xaaa, bus.Half, 0xa0)
	f.Write(0x4ffe, bus.Half, 0x1234)
	if got := f.Read(0x4ffe, bus.Half); got != 0x1234 {
		t.Fatalf("program=%#x", got)
	}
	unlockFlash(f)
	f.Write(0xaaa, bus.Half, 0xa0)
	f.Write(0x4ffe, bus.Half, 0xff0f)
	if got := f.Read(0x4ffe, bus.Half); got != 0x1204 {
		t.Fatalf("program must only clear bits: %#x", got)
	}
	unlockFlash(f)
	f.Write(0xaaa, bus.Half, 0x80)
	unlockFlash(f)
	f.Write(0x5000, bus.Half, 0x30)
	if got := f.Read(0x4ffe, bus.Half); got != 0xffff {
		t.Fatalf("sector erase=%#x", got)
	}
}

func TestFlashSnapshotRestoresUnlockPositionAndContents(t *testing.T) {
	f := newFlash(t, "U202", bytes.Repeat([]byte{0xff}, 0x20000))
	f.Write(0xaaa, bus.Half, 0xaa) // one command into unlock
	state, err := f.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	f.Write(0x554, bus.Half, 0x55)
	f.Write(0xaaa, bus.Half, 0xa0)
	f.Write(0x100, bus.Byte, 0x34)
	if got := f.Read(0x100, bus.Byte); got != 0x34 {
		t.Fatalf("first program=%#x", got)
	}
	if err := f.Restore(state); err != nil {
		t.Fatal(err)
	}
	if got := f.Read(0x100, bus.Byte); got != 0xff {
		t.Fatalf("contents not restored: %#x", got)
	}
	f.Write(0x554, bus.Half, 0x55)
	f.Write(0xaaa, bus.Half, 0xa0)
	f.Write(0x100, bus.Byte, 0x56)
	if got := f.Read(0x100, bus.Byte); got != 0x56 {
		t.Fatalf("unlock position not restored: %#x", got)
	}
}
