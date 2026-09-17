package eeprom

import (
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
)

func TestPersistsAcrossNewMachine(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "digibox.nvram")
	first := New()
	if got := first.Read(0x400, bus.Byte); got != 0xff {
		t.Fatalf("factory blank = %#x", got)
	}
	first.Write(0x400, bus.Byte, 1)
	first.Write(0x401, bus.Byte, 1)
	if err := first.Persist(path); err != nil {
		t.Fatal(err)
	}
	second := New()
	if err := second.Load(path); err != nil {
		t.Fatal(err)
	}
	if got := second.Read(0x400, bus.Byte); got != 1 || second.Read(0x401, bus.Byte) != 1 {
		t.Fatalf("NVRAM restart bytes = %#x, %#x", got, second.Read(0x401, bus.Byte))
	}
	second.Write(0x400, bus.Byte, 2)
	if err := second.Load(path); err != nil {
		t.Fatal(err)
	}
	if got := second.Read(0x400, bus.Byte); got != 1 {
		t.Fatalf("reload = %#x", got)
	}
}

func TestStoreHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New:     func() bus.Device { return New() },
		Mutate:  func(device bus.Device) { device.Write(0x1400, bus.Byte, 0x12) },
		Disturb: func(device bus.Device) { device.Write(0x1400, bus.Byte, 0x34) },
	})
	if err != nil {
		t.Fatal(err)
	}
}
