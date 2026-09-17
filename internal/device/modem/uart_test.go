package modem

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/irq"
)

func TestATReplyAndControlLineTransition(t *testing.T) {
	t.Parallel()
	interrupts := irq.New(nil)
	interrupts.Write(0x40, bus.Word, IRQMask)
	u := New(interrupts)
	if got := u.Read(0x50, bus.Word); got != 0x60 {
		t.Fatalf("empty modem line status = %#x", got)
	}
	u.Write(0x10, bus.Word, 0x0b)
	u.Write(0x40, bus.Word, 3)
	if got := u.Read(0x60, bus.Word); got&0xb3 != 0xb3 {
		t.Fatalf("modem lines/deltas = %#x", got)
	}
	if got := u.Read(0x60, bus.Word) & 0x0b; got != 0 {
		t.Fatalf("delta bits did not clear on read: %#x", got)
	}
	u.Write(0x00, bus.Word, 'A')
	u.Write(0x00, bus.Word, 'T')
	if got := u.Read(0x50, bus.Word); got != 0x61 {
		t.Fatalf("reply line status = %#x", got)
	}
	if got := u.Read(0x20, bus.Word); got != 0x04 {
		t.Fatalf("RX interrupt identification = %#x", got)
	}
	for _, want := range []byte("\r\nOK\r\n") {
		if got := u.Read(0x00, bus.Word); got != uint32(want) {
			t.Fatalf("reply byte = %#x, want %#x", got, want)
		}
	}
	if got := u.Read(0x50, bus.Word); got != 0x60 {
		t.Fatalf("drained line status = %#x", got)
	}
}

func TestModemIRQWaitsForPumpAndRepeatsOncePerBoardTick(t *testing.T) {
	t.Parallel()
	var requests int
	interrupts := irq.New(func(uint8) { requests++ })
	u := New(interrupts)
	u.Write(0x10, bus.Word, 2) // transmitter empty
	if requests != 0 || interrupts.Read(0x30, bus.Word)&IRQMask != 0 {
		t.Fatal("UART write signalled before device pump")
	}
	u.Pump(5)
	u.Pump(5)
	if requests != 1 {
		t.Fatalf("same-tick requests = %d, want 1", requests)
	}
	u.Pump(6)
	if requests != 2 {
		t.Fatalf("new-tick requests = %d, want 2", requests)
	}
	u.Write(0x10, bus.Word, 0)
	u.Pump(6)
	if interrupts.Read(0x30, bus.Word)&IRQMask != 0 {
		t.Fatal("cleared UART cause left pending status")
	}
}

func TestUARTHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(nil) },
		Mutate: func(device bus.Device) {
			u := device.(*UART)
			u.Write(0x10, bus.Word, 0x0b)
			u.Write(0x40, bus.Word, 3)
			u.Write(0x00, bus.Word, 'A')
			u.Write(0x00, bus.Word, 'T')
			u.Write(0x30, bus.Word, 0x80)
			u.Write(0x00, bus.Word, 12)
			u.Write(0x10, bus.Word, 1)
			u.Write(0x30, bus.Word, 3)
			u.Pump(5)
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"interrupt"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
