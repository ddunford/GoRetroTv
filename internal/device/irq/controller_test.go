package irq

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
)

func TestPendingAndEnableSignalIP2(t *testing.T) {
	t.Parallel()
	var lines []uint8
	c := New(func(ip uint8) { lines = append(lines, ip) })
	c.SetLine(DemuxMask, true)
	if len(lines) != 0 {
		t.Fatal("masked pending interrupt reached CPU")
	}
	c.Write(0x40, bus.Word, DemuxMask)
	if len(lines) != 1 || lines[0] != 2 || c.Read(0x30, bus.Word) != DemuxMask {
		t.Fatalf("enabled pending demux line did not raise IP2: lines=%v pending=%#x", lines, c.Read(0x30, bus.Word))
	}
	c.SetLine(DemuxMask, false)
	if got := c.Read(0x30, bus.Word); got != 0 {
		t.Fatalf("acknowledged pending bit = %#x", got)
	}
	c.SetLine(DemuxMask, true)
	if len(lines) != 2 || lines[1] != 2 {
		t.Fatalf("second delivery did not raise IP2: %v", lines)
	}
}

func TestControllerHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(nil) },
		Mutate: func(device bus.Device) {
			c := device.(*Controller)
			c.Write(0x40, bus.Word, DemuxMask)
			c.SetLine(DemuxMask, true)
		},
		Disturb: func(device bus.Device) {
			c := device.(*Controller)
			c.Reset()
		},
		Constant: []string{"raise"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
