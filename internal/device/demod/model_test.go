package demod

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
)

func TestLockedResponsesAndMicrocodeUpload(t *testing.T) {
	t.Parallel()
	m := New()
	for _, tc := range []struct {
		register uint16
		want     uint8
	}{
		{11, 0x3f}, {34, 3}, {64, 0}, {69, 0}, {74, 0x80}, {75, 0x17}, {78, 2}, {92, 0x80},
	} {
		if got := m.Read(uint32(tc.register), bus.Byte); got != uint32(tc.want) {
			t.Fatalf("register %d = %#x, want %#x", tc.register, got, tc.want)
		}
	}
	if !m.Locked() {
		t.Fatal("virtual front-end lost lock")
	}
	m.Start()
	m.ShiftWrite(0)
	m.ShiftWrite(0)
	m.Start()
	m.ShiftWrite(1)
	m.ShiftWrite(0)
	m.Start()
	m.ShiftWrite(5)
	for i := 0; i < 1024; i++ {
		m.ShiftWrite(uint8(i))
	} // #nosec G115 -- microcode fixture intentionally cycles bytes
	if value, ok := m.Written(5, 0); !ok || value != 0 {
		t.Fatalf("first microcode byte = %#x, %t", value, ok)
	}
	if value, ok := m.Written(5, 1023); !ok || value != 0xff {
		t.Fatalf("last microcode byte = %#x, %t", value, ok)
	}
	if got := m.Read(75, bus.Byte); got != 0x17 {
		t.Fatalf("upload changed lock readback to %#x", got)
	}
}

func TestModelHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New() },
		Mutate: func(device bus.Device) {
			m := device.(*Model)
			m.Start()
			m.ShiftWrite(0)
			m.ShiftWrite(75)
			m.Start()
			m.ShiftWrite(3)
			m.ShiftWrite(0x77)
			m.ShiftRead()
		},
		Disturb: func(device bus.Device) { device.Reset() },
	})
	if err != nil {
		t.Fatal(err)
	}
}
