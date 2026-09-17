package demux

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
)

func TestEnableSetsAndStatusClears(t *testing.T) {
	t.Parallel()
	d := New()
	d.Write(0xD8, bus.Word, 1<<22)
	d.Write(0xD8, bus.Word, 1<<23)
	if got := d.Read(0xD8, bus.Word); got != (1<<22)|(1<<23) {
		t.Fatalf("enable %#08x, want both independently armed filters", got)
	}
	if got := d.Read(0xB8, bus.Word); got != 0 {
		t.Fatalf("status %#08x before delivery", got)
	}
	d.complete(22)
	d.complete(23)
	if got := d.Read(0xB8, bus.Word); got != (1<<22)|(1<<23) {
		t.Fatalf("status %#08x after two completions", got)
	}
	d.Write(0xB8, bus.Word, ^uint32(1<<22))
	if got := d.Read(0xB8, bus.Word); got != 1<<23 {
		t.Fatalf("complement acknowledgement left status %#08x", got)
	}
	if got := d.Read(0xD8, bus.Word); got != (1<<22)|(1<<23) {
		t.Fatalf("acknowledgement changed enable to %#08x", got)
	}
	d.Write(0x00, bus.Word, 1)
	if got := d.Read(0xD8, bus.Word) | d.Read(0xB8, bus.Word); got != 0 {
		t.Fatalf("block reset left enable/status %#08x", got)
	}
}

func TestDemuxRegistersCarryAllStateInSnapshot(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New() },
		Mutate: func(device bus.Device) {
			d := device.(*Demux)
			d.Write(0xD8, bus.Word, 0x00400000)
			d.complete(22)
			d.writePointer[22] = 0x45c24
			d.Write(0x124, bus.Word, 0x4000|(22<<2))
		},
		Disturb: func(device bus.Device) {
			d := device.(*Demux)
			d.Write(0xD8, bus.Word, 0x00800000)
			d.complete(23)
			d.writePointer[22] = 0x45c30
			d.Write(0x124, bus.Word, 0x4000|(23<<2))
		},
		Constant: []string{"name"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLISRPointerHandshake(t *testing.T) {
	t.Parallel()
	d := New()
	d.writePointer[22] = 0x45c24
	d.writePointer[23] = 0x46c32
	for _, tc := range []struct {
		filter uint32
		want   uint32
	}{
		{22, 0x45c24},
		{23, 0x46c32},
	} {
		d.Write(0x124, bus.Word, 0x4000|(tc.filter<<2))
		cleared := false
		for spin := 0; spin < 8; spin++ {
			if d.Read(0x124, bus.Word)&0x4000 == 0 {
				cleared = true
				break
			}
		}
		if !cleared {
			t.Fatalf("filter %d LISR would hang waiting for command busy bit", tc.filter)
		}
		if got := d.Read(0x128, bus.Word); got != tc.want {
			t.Fatalf("filter %d pointer = %#x, want %#x", tc.filter, got, tc.want)
		}
	}
	d.Reset()
	if got := d.Read(0x128, bus.Word); got != 0 {
		t.Fatalf("reset pointer = %#x", got)
	}
}
