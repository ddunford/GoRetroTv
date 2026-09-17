package smartcard

import (
	"bytes"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/irq"
)

func TestBootFrameCompletesSixInterruptsAndRepliesNoCard(t *testing.T) {
	t.Parallel()
	interrupts := irq.New(nil)
	interrupts.Write(0x40, bus.Word, IRQMask)
	p := New(interrupts)
	p.Write(0x70, bus.Word, 2)
	request := []byte{0x60, 0, 1, 0x11, 4, 0x74}
	for i, b := range request {
		p.Write(0x50, bus.Word, uint32(b))
		for range ByteInstructions/16 - 1 {
			p.Pump(16)
		}
		if p.Read(0x60, bus.Word) != 0 {
			t.Fatalf("byte %d completed early", i)
		}
		p.Pump(16)
		if p.Read(0x60, bus.Word)&2 == 0 || interrupts.Read(0x30, bus.Word) != IRQMask {
			t.Fatalf("byte %d did not interrupt", i)
		}
		p.Write(0x60, bus.Word, 0)
	}
	p.Write(0x70, bus.Word, 4)
	want := []byte{0xe0, 0, 1, 0x11, 0xc0, 0x30}
	got := make([]byte, 0, len(want))
	for i := range want {
		gap := ReplyGapInstructions
		if i == 0 {
			gap *= 2
		}
		for range gap / 16 {
			p.Pump(16)
		}
		if p.Read(0x60, bus.Word)&4 != 0 {
			t.Fatalf("reply byte %d appeared when delay reached zero", i)
		}
		p.Pump(16)
		if p.Read(0x60, bus.Word)&4 == 0 {
			t.Fatalf("reply byte %d was not ready", i)
		}
		got = append(got, byte(p.Read(0x53, bus.Byte)))
		p.Pump(16)
		if len(p.reply) != len(want)-i-1 {
			t.Fatal("receive register overwritten before acknowledge")
		}
		p.Write(0x60, bus.Word, 0)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("no-card reply = %x, want %x", got, want)
	}
}

func TestPortHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(nil) },
		Mutate: func(device bus.Device) {
			p := device.(*Port)
			p.control, p.pair30, p.pair40, p.status, p.enable = 0xc0, 0x42, 1, 2, 7
			p.rxByte, p.frame, p.reply = 0xe0, []byte{0x60, 0}, []byte{0xe0, 0}
			p.txDelay, p.rxDelay = 200, 300
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"interrupt"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
