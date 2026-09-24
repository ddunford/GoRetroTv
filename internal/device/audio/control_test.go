package audio

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
)

func TestControlHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	if err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New() },
		Mutate: func(device bus.Device) {
			device.Write(0, bus.Byte, 0xe7)
			device.Write(6, bus.Byte, 0x10)
		},
		Disturb: func(device bus.Device) { device.Reset() },
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMeasuredVolumeSequenceKeepsStatusZero(t *testing.T) {
	c := New()
	sequence := []struct {
		off   uint32
		value uint32
	}{{6, 0x10}, {3, 0x8235}, {2, 0x82}, {1, 0x0F}, {0, 0xE7}}
	for _, step := range sequence {
		c.Write(step.off, bus.Byte, step.value)
	}
	if got := c.Read(6, bus.Byte); got != 0 {
		t.Fatalf("status read %#x, want measured zero", got)
	}
	for _, step := range sequence {
		if got := c.Register(step.off); got != byte(step.value) {
			t.Errorf("register +%d = %#x, want %#x", step.off, got, byte(step.value))
		}
	}
}

func TestSnapshotRestoreAndReset(t *testing.T) {
	c := New()
	c.Write(0, bus.Byte, 0xE7)
	c.Write(6, bus.Byte, 0x10)
	blob, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	c.Reset()
	if c.Register(0) != 0 || c.Register(6) != 0 {
		t.Fatal("reset retained audio control state")
	}
	if err := c.Restore(blob); err != nil {
		t.Fatal(err)
	}
	if c.Register(0) != 0xE7 || c.Register(6) != 0x10 {
		t.Fatalf("restored registers +0=%#x +6=%#x", c.Register(0), c.Register(6))
	}
}
