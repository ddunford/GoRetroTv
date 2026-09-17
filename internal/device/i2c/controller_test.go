package i2c

import (
	"path/filepath"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/eeprom"
	"github.com/ddunford/goretrotv/internal/device/irq"
)

func transferByte(t *testing.T, c *Controller, value uint32) {
	t.Helper()
	c.Write(0x40, bus.Word, value)
	if c.Read(0x10, bus.Word) != 4 {
		t.Fatalf("byte %#x was not acknowledged", value)
	}
	c.Write(0x50, bus.Word, 0)
}

func TestEEPROMTransactionsPersistAcrossControllerRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nvram.img")
	store, mux, interrupts := eeprom.New(), NewMux(), irq.New(nil)
	c := New(store, mux, interrupts)
	if err := c.BindImage(path); err != nil {
		t.Fatal(err)
	}
	interrupts.Write(0x40, bus.Word, IRQMask)
	mux.Write(0, bus.Word, 3<<4)
	c.Write(0x60, bus.Word, 1)
	c.Write(0, bus.Word, 0x9a)
	if interrupts.Read(0x30, bus.Word) != 0 {
		t.Fatal("START control raised interrupt before address byte")
	}
	c.Write(0x40, bus.Word, 0xa0)
	if c.Read(0x10, bus.Word) != 4 || interrupts.Read(0x30, bus.Word) != IRQMask {
		t.Fatal("address completion did not acknowledge and interrupt")
	}
	c.Write(0x50, bus.Word, 0)
	if interrupts.Read(0x30, bus.Word) != 0 {
		t.Fatal("interrupt acknowledgement failed")
	}
	transferByte(t, c, 0x04)
	transferByte(t, c, 0x00)
	transferByte(t, c, 0x01)
	transferByte(t, c, 0x01)
	c.Write(0, bus.Word, 0x99)
	if err := c.Fault(); err != nil {
		t.Fatal(err)
	}
	store2, mux2 := eeprom.New(), NewMux()
	c2 := New(store2, mux2, irq.New(nil))
	if err := c2.BindImage(path); err != nil {
		t.Fatal(err)
	}
	mux2.Write(0, bus.Word, 3<<4)
	c2.Write(0, bus.Word, 0x9a)
	transferByte(t, c2, 0xa0)
	transferByte(t, c2, 0x04)
	transferByte(t, c2, 0x00)
	c2.Write(0, bus.Word, 0x9a)
	transferByte(t, c2, 0xa1)
	c2.Write(0, bus.Word, 0xac)
	if got := c2.Read(0x40, bus.Word); got != 1 {
		t.Fatalf("first NVRAM byte after restart = %#x", got)
	}
	c2.Write(0, bus.Word, 0xac)
	if got := c2.Read(0x40, bus.Word); got != 1 {
		t.Fatalf("second NVRAM byte after restart = %#x", got)
	}
}

func TestPageWriteWrapsWithin64Bytes(t *testing.T) {
	t.Parallel()
	store, mux := eeprom.New(), NewMux()
	c := New(store, mux, nil)
	mux.Write(0, bus.Word, 3<<4)
	c.Write(0, bus.Word, 0x9a)
	transferByte(t, c, 0xa0)
	transferByte(t, c, 0)
	transferByte(t, c, 63)
	transferByte(t, c, 0xab)
	transferByte(t, c, 0xcd)
	if store.Read(63, bus.Byte) != 0xab || store.Read(0, bus.Byte) != 0xcd || store.Read(64, bus.Byte) != 0xff {
		t.Fatal("page write crossed into next page")
	}
}

func TestControllerHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(eeprom.New(), NewMux(), nil) },
		Mutate: func(device bus.Device) {
			c := device.(*Controller)
			c.control, c.status, c.clock, c.data, c.enable = 0xac, 4, 2, 0x56, 1
			c.startArmed, c.active, c.dirty = true, true, true
			c.slave, c.pointer, c.addressBytes, c.fault = 0xa0, 0x1234, 2, "test"
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"store.image", "mux.value", "interrupt", "imagePath"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMuxHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New:     func() bus.Device { return NewMux() },
		Mutate:  func(device bus.Device) { device.Write(0, bus.Word, 3<<4) },
		Disturb: func(device bus.Device) { device.Reset() },
	})
	if err != nil {
		t.Fatal(err)
	}
}
