package i2c

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/demod"
	"github.com/ddunford/goretrotv/internal/device/eeprom"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
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
	c := New(store, interrupts)
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
	c2 := New(store2, irq.New(nil))
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
	c := New(store, nil)
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

func TestDemodIndirectRegisterReadOverI2C(t *testing.T) {
	t.Parallel()
	mux := NewMux()
	mux.Write(0, bus.Word, 0)
	model := demod.New()
	c := New(eeprom.New(), nil)
	c.BindDemod(model)
	for _, data := range [][]uint32{{0x18, 0, 75}, {0x18, 1, 0}, {0x18, 3}} {
		c.Write(0, bus.Word, 0x9a)
		for _, b := range data {
			transferByte(t, c, b)
		}
		c.Write(0, bus.Word, 0x99)
	}
	c.Write(0, bus.Word, 0x9a)
	transferByte(t, c, 0x19)
	c.Write(0, bus.Word, 0xac)
	if got := c.Read(0x40, bus.Word); got != 0x17 {
		t.Fatalf("polled lock register 75 = %#x", got)
	}
	c.Write(0, bus.Word, 0xac)
	if got := c.Read(0x40, bus.Word); got != 0 {
		t.Fatalf("next BER register = %#x", got)
	}
}

func TestSnapshotResumesIndirectDemodReadWithExactChildState(t *testing.T) {
	t.Parallel()
	model := demod.New()
	c := New(eeprom.New(), nil)
	c.BindDemod(model)
	for _, data := range [][]uint32{{0x18, 0, 75}, {0x18, 1, 0}, {0x18, 3}} {
		c.Write(0, bus.Word, 0x9a)
		for _, b := range data {
			transferByte(t, c, b)
		}
		c.Write(0, bus.Word, 0x99)
	}
	c.Write(0, bus.Word, 0x9a)
	transferByte(t, c, 0x19)
	c.Write(0, bus.Word, 0xac)
	if got := c.Read(0x40, bus.Word); got != 0x17 {
		t.Fatalf("first indirect byte = %#x", got)
	}
	paused, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var directRegisters []uint16
	model.SetReadObserver(func(register uint16, _ uint8) { directRegisters = append(directRegisters, register) })
	directValues := make([]uint32, 0, 3)
	for range 3 {
		c.Write(0, bus.Word, 0xac)
		directValues = append(directValues, c.Read(0x40, bus.Word))
	}
	directState, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint32{0, 0, 2}; !slices.Equal(directValues, want) {
		t.Fatalf("direct values = %v, want %v", directValues, want)
	}
	if want := []uint16{76, 77, 78}; !slices.Equal(directRegisters, want) {
		t.Fatalf("direct registers = %v, want %v", directRegisters, want)
	}

	restoredModel := demod.New()
	restored := New(eeprom.New(), nil)
	restored.BindDemod(restoredModel)
	restoredModel.ShiftWrite(0xee) // a stale selector must be replaced
	if err := restored.Restore(paused); err != nil {
		t.Fatal(err)
	}
	if got, err := restored.Snapshot(); err != nil || !bytes.Equal(got, paused) {
		t.Fatalf("restored state differs from paused state: %v", err)
	}
	var resumedRegisters []uint16
	restoredModel.SetReadObserver(func(register uint16, _ uint8) { resumedRegisters = append(resumedRegisters, register) })
	resumedValues := make([]uint32, 0, 3)
	for range 3 {
		restored.Write(0, bus.Word, 0xac)
		resumedValues = append(resumedValues, restored.Read(0x40, bus.Word))
	}
	resumedState, err := restored.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resumedValues, directValues) || !slices.Equal(resumedRegisters, directRegisters) || !bytes.Equal(resumedState, directState) {
		t.Fatalf("run-on differs: reads %v/%v registers %v/%v state equal %t", resumedValues, directValues, resumedRegisters, directRegisters, bytes.Equal(resumedState, directState))
	}
}

func TestSnapshotResumesEEPROMPageWriteWithChipContents(t *testing.T) {
	t.Parallel()
	store := eeprom.New()
	c := New(store, nil)
	c.Write(0, bus.Word, 0x9a)
	for _, b := range []uint32{0xa0, 0x04, 0x00, 0x11} {
		transferByte(t, c, b)
	}
	paused, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	transferByte(t, c, 0x22)
	end, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	otherStore := eeprom.New()
	otherStore.Write(0x400, bus.Byte, 0x99)
	restored := New(otherStore, nil)
	if err := restored.Restore(paused); err != nil {
		t.Fatal(err)
	}
	if got := otherStore.Read(0x400, bus.Byte); got != 0x11 {
		t.Fatalf("restored EEPROM byte = %#x, want 0x11", got)
	}
	transferByte(t, restored, 0x22)
	resumed, err := restored.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resumed, end) {
		t.Fatal("EEPROM page write resumed to a different controller/chip state")
	}
}

func TestRestoreRefusesOldSnapshotWithoutChildState(t *testing.T) {
	t.Parallel()
	w := snapcodec.NewWriter("i2c-master", 1)
	w.Words([]uint32{0x99, 0, 0, 0, 0})
	w.Bool(false)
	w.Bool(false)
	w.Bool(false)
	w.Uint8(0)
	w.Uint16(0)
	w.Uint8(0)
	w.String("")
	old, err := w.Blob()
	if err != nil {
		t.Fatal(err)
	}
	c := New(eeprom.New(), nil)
	c.BindDemod(demod.New())
	before, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(old); err == nil || !strings.Contains(err.Error(), "omits bound") {
		t.Fatalf("old incomplete snapshot error = %v", err)
	}
	after, err := c.Snapshot()
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("refused restore changed state: %v", err)
	}
}

func TestRestoreRejectsCorruptChildWithoutChangingEitherSlave(t *testing.T) {
	t.Parallel()
	for _, damaged := range []string{"EEPROM", "demod"} {
		t.Run(damaged, func(t *testing.T) {
			t.Parallel()
			store, model := eeprom.New(), demod.New()
			store.Write(0x400, bus.Byte, 0x66)
			model.ShiftWrite(3)
			model.ShiftWrite(0x88)
			c := New(store, nil)
			c.BindDemod(model)
			before, err := c.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			otherStore, otherModel := eeprom.New(), demod.New()
			otherStore.Write(0x400, bus.Byte, 0x11)
			otherModel.ShiftWrite(0)
			otherModel.ShiftWrite(75)
			storeState, err := otherStore.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			demodState, err := otherModel.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if damaged == "EEPROM" {
				storeState = []byte{1, 2, 3}
			} else {
				demodState = []byte{1, 2, 3}
			}
			w := snapcodec.NewWriter("i2c-master", 2)
			w.Words([]uint32{0xac, 4, 2, 0x56, 1})
			w.Bool(true)
			w.Bool(true)
			w.Bool(true)
			w.Uint8(0x18)
			w.Uint16(75)
			w.Uint8(2)
			w.String("changed")
			w.Bool(true)
			w.Bytes(storeState)
			w.Bool(true)
			w.Bytes(demodState)
			broken, err := w.Blob()
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Restore(broken); err == nil || !strings.Contains(err.Error(), damaged) {
				t.Fatalf("corrupt %s error = %v", damaged, err)
			}
			after, err := c.Snapshot()
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("corrupt %s changed controller or slave state: %v", damaged, err)
			}
		})
	}
}

func TestKnownSlaveAcknowledgesWithChannelLatchDisconnected(t *testing.T) {
	t.Parallel()
	c := New(eeprom.New(), nil)
	c.Write(0, bus.Word, 0x9a)
	transferByte(t, c, 0xca)
	transferByte(t, c, 0x42)
}

func TestControllerHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(eeprom.New(), nil) },
		Mutate: func(device bus.Device) {
			c := device.(*Controller)
			c.control, c.status, c.clock, c.data, c.enable = 0xac, 4, 2, 0x56, 1
			c.startArmed, c.active, c.dirty = true, true, true
			c.slave, c.pointer, c.addressBytes, c.fault = 0xa0, 0x1234, 2, "test"
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"store.image", "interrupt", "imagePath", "demod"},
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
