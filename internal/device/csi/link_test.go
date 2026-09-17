package csi

import (
	"bytes"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
	"github.com/ddunford/goretrotv/internal/device/irq"
)

func TestHandsetFrameIsPacedThroughReceiveRegister(t *testing.T) {
	t.Parallel()
	interrupts := irq.New(nil)
	interrupts.Write(0x40, bus.Word, IRQMask)
	link := New(interrupts)
	link.Write(0, bus.Word, 0x80)
	link.Write(0x30, bus.Word, 1)
	if err := link.Key(0x7d, 0); err != nil {
		t.Fatal(err)
	}
	want := []byte{5, 0x80, 2, 0, 7, 0xd0}
	want = Encode(want)
	if len(want) != 8 || want[3] != 0x1b || want[4] != 0 {
		t.Fatalf("wrong escaped key frame: %x", want)
	}
	got := make([]byte, 0, len(want))
	for index := range want {
		at := uint64(index+1) * TrafficInstructions
		link.Pump(at - 1)
		if link.Read(0x20, bus.Word) != 0 {
			t.Fatal("byte arrived before its instruction deadline")
		}
		link.Pump(at)
		if link.Read(0x20, bus.Word) != 1 || interrupts.Read(0x30, bus.Word) != IRQMask {
			t.Fatal("ready byte failed to raise board IRQ")
		}
		got = append(got, byte(link.Read(0x10, bus.Word)))
		link.Pump(at + 1)
		if link.Pending() != len(want)-index-1 {
			t.Fatal("unacknowledged receive register was overwritten")
		}
		link.Write(0x20, bus.Word, 0)
		if interrupts.Read(0x30, bus.Word) != 0 {
			t.Fatal("acknowledge failed to clear board IRQ")
		}
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("wire = %x, want %x", got, want)
	}
	link.Write(0x10, bus.Word, 0xa5)
	if !bytes.Equal(link.Transmitted(), []byte{0xa5}) {
		t.Fatal("transmitted byte not recorded")
	}
}

func TestLinkRejectsUnboundedQueue(t *testing.T) {
	t.Parallel()
	l := New(nil)
	if err := l.Queue(make([]byte, MaxQueuedBytes)); err != nil {
		t.Fatal(err)
	}
	if err := l.Key(0x80, 0); err == nil {
		t.Fatal("overflowing key was accepted")
	}
	if l.Pending() != MaxQueuedBytes {
		t.Fatal("rejected key changed queue")
	}
}

func TestLinkHoldsTheDeviceContract(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return New(nil) },
		Mutate: func(device bus.Device) {
			l := device.(*Link)
			l.control, l.interruptEnable, l.data, l.ready = 0x80, 1, 0x7d, true
			l.queue, l.reply, l.transmitted = []byte{1, 2}, []byte{4}, []byte{3}
			l.lastByteAt, l.now, l.powerAt = 2100, 2500, 2400000
			l.cardLive, l.boxBusy, l.txSeen, l.escaped = true, true, true, true
			l.frame = []byte{3, 1, 0x52}
			l.AckAll()
		},
		Disturb:  func(device bus.Device) { device.Reset() },
		Constant: []string{"interrupt"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
