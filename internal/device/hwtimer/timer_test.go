package hwtimer

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/irq"
)

func TestTimerHoldsTheDeviceContract(t *testing.T) {
	if err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(nil) },
		Mutate: func(device bus.Device) {
			timer := device.(*Timer)
			for off := uint32(0); off < Size; off += 4 {
				if off != 0xE0 {
					timer.Write(off, bus.Word, off+1)
				}
			}
			timer.Pump(PeriodInstructions)
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"interrupt"},
	}); err != nil {
		t.Fatal(err)
	}

	interrupts := irq.New(nil)
	interrupts.Write(0x40, bus.Word, IRQMask)
	timer := New(interrupts)
	var device bus.Device = timer
	if device.Name() == "" {
		t.Fatal("timer has no snapshot identity")
	}
	device.Write(0x00, bus.Word, 1)
	timer.Pump(PeriodInstructions - 1)
	if got := interrupts.Read(0x30, bus.Word); got != 0 {
		t.Fatalf("early interrupt: %#x", got)
	}
	timer.Pump(PeriodInstructions)
	if got := timer.Ticks(); got != 1 {
		t.Fatalf("board ticks = %d, want 1", got)
	}
	if got := device.Read(0xD0, bus.Word); got != 1 {
		t.Fatalf("timer status: %#x", got)
	}
	if got := interrupts.Read(0x30, bus.Word); got != IRQMask {
		t.Fatalf("board interrupt: %#x", got)
	}
	blob, err := device.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	device.Write(0xE0, bus.Word, 1)
	if got := interrupts.Read(0x30, bus.Word); got != 0 {
		t.Fatalf("acknowledge left interrupt: %#x", got)
	}
	if err := device.Restore(blob); err != nil {
		t.Fatal(err)
	}
	if got := interrupts.Read(0x30, bus.Word); got != IRQMask {
		t.Fatalf("restore lost interrupt: %#x", got)
	}
	if got := timer.Ticks(); got != 1 {
		t.Fatalf("restore lost timer tick: %d", got)
	}
	device.Reset()
	if got := interrupts.Read(0x30, bus.Word); got != 0 {
		t.Fatalf("reset left interrupt: %#x", got)
	}
	timer.Pump(PeriodInstructions * 2)
	if got := device.Read(0xD0, bus.Word); got != 0 {
		t.Fatalf("unarmed timer fired: %#x", got)
	}
}

func TestTimerRepulsesIP2WhileStatusIsPending(t *testing.T) {
	t.Parallel()
	var requests int
	controller := irq.New(func(uint8) { requests++ })
	timer := New(controller)
	timer.Write(0, bus.Word, 1)
	timer.Pump(PeriodInstructions)
	timer.Pump(2 * PeriodInstructions)
	if requests != 2 || timer.Ticks() != 2 || timer.Read(0xD0, bus.Word) != 1 {
		t.Fatalf("requests=%d ticks=%d status=%#x", requests, timer.Ticks(), timer.Read(0xD0, bus.Word))
	}
}
