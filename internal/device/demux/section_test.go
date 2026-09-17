package demux

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
)

func TestSectionRingAndRecordGeometry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		filter uint8
		ring   uint32
		record uint32
	}{
		{0, 0xA07A0000, 0x80142D34},
		{1, 0xA07A3000, 0x80142D48},
		{22, 0xA07E2000, 0x80142EEC},
		{31, 0xA07FD000, 0x80142FA0},
	} {
		if got := RingBase(tc.filter); got != tc.ring {
			t.Errorf("filter %d ring = %#08x, want %#08x", tc.filter, got, tc.ring)
		}
		if got := RecordBase(tc.filter); got != tc.record {
			t.Errorf("filter %d record = %#08x, want %#08x", tc.filter, got, tc.record)
		}
	}
}

func TestSectionRingsAreBusMemoryAndSurviveSnapshot(t *testing.T) {
	t.Parallel()
	b := bus.New()
	r, err := AttachSectionRAM(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range []uint8{0, 22, 31} {
		at := RingBase(filter) + RingSize - 1
		b.Write(at, bus.Byte, uint32(filter+1))
		if got := b.Read(at, bus.Byte); got != uint32(filter+1) {
			t.Errorf("filter %d final byte = %#x", filter, got)
		}
	}
	// KSEG0 and KSEG1 see the same physical section RAM, including the last byte.
	if got := b.Read(0x807FFFFF, bus.Byte); got != 32 {
		t.Fatalf("cached alias reads %#x, want final byte of filter 31", got)
	}
	state, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	r.Reset()
	if got := b.Read(RingBase(22)+RingSize-1, bus.Byte); got != 0 {
		t.Fatalf("reset left byte %#x", got)
	}
	if err := r.Restore(state); err != nil {
		t.Fatal(err)
	}
	if got := b.Read(RingBase(22)+RingSize-1, bus.Byte); got != 23 {
		t.Fatalf("snapshot restored byte %#x, want 23", got)
	}
}
