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
		},
		Disturb: func(device bus.Device) {
			d := device.(*Demux)
			d.Write(0xD8, bus.Word, 0x00800000)
			d.complete(23)
		},
		Constant: []string{"name"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
