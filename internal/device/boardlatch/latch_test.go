package boardlatch_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/boardlatch"
)

func TestLatchHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return boardlatch.New() },
		Mutate: func(device bus.Device) {
			device.Write(0, bus.Word, 0x15)
		},
		Disturb: func(device bus.Device) { device.Reset() },
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetClearAndSnapshot(t *testing.T) {
	l := boardlatch.New()
	if got := l.Read(0, bus.Word); got != 0 {
		t.Fatalf("power-on word = %#x, want zero", got)
	}
	l.Write(0, bus.Word, 0x15)
	l.Write(0, bus.Word, l.Read(0, bus.Word)&^0x4)
	if got := l.Read(0, bus.Word); got != 0x11 {
		t.Fatalf("read-modify-write value = %#x, want 0x11", got)
	}
	blob, err := l.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	l.Reset()
	if err := l.Restore(blob); err != nil {
		t.Fatal(err)
	}
	if got := l.Read(0, bus.Word); got != 0x11 {
		t.Fatalf("restored value = %#x, want 0x11", got)
	}
	if err := l.Restore(blob[:len(blob)-1]); err == nil {
		t.Fatal("truncated snapshot was accepted")
	}
	if got := l.Read(0, bus.Word); got != 0x11 {
		t.Fatalf("failed restore changed live value to %#x", got)
	}
}
