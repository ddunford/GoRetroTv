package csi

import (
	"bytes"
	"fmt"
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
	link.Pump(0, PowerupTicks)
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
		link.Pump(at-1, 0)
		if link.Read(0x20, bus.Word) != 0 {
			t.Fatal("byte arrived before its instruction deadline")
		}
		link.Pump(at, 0)
		if link.Read(0x20, bus.Word) != 0 {
			t.Fatal("handset byte arrived before guest transmit receipt")
		}
		link.Write(0x10, bus.Word, 0)
		link.Pump(at, 0)
		if link.Read(0x20, bus.Word) != 1 || interrupts.Read(0x30, bus.Word) != IRQMask {
			t.Fatal("ready byte failed to raise board IRQ")
		}
		got = append(got, byte(link.Read(0x10, bus.Word)))
		link.Pump(at+1, 0)
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
	transmitted := link.Transmitted()
	if len(transmitted) != len(want)+1 || transmitted[len(transmitted)-1] != 0xa5 {
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
			l.lastByteAt, l.now, l.powerAtTick, l.timerTicks = 2100, 2500, 120, 37
			l.cardLive, l.boxBusy, l.txSeen, l.escaped = true, true, true, true
			l.frame = []byte{3, 1, 0x52}
			l.inFrame, l.sendingReply, l.outEscaped = true, true, true
		},
		Disturb: func(device bus.Device) { device.Reset() },
		// ack is the MODEL'S configuration, not the machine's state: it says what the peripheral
		// answers, the way the baud rate says how fast it answers, and Restore deliberately takes
		// it from the build rather than from the blob. Listing it here is this harness's own way
		// of saying a field is not state, and it is the honest alternative to snapshotting
		// something the product must never read back out of a file.
		Constant: []string{"interrupt", "ack"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A CARD REPLY MUST NOT SPLICE ITSELF INTO A HANDSET FRAME, WHEREVER IN IT THE REPLY FALLS.
//
// Both travel on this one wire and both are framed the same way -- bytes until a zero terminator --
// so a frame interrupted half way through is not a delayed frame, it is TWO CORRUPT ONES. The
// guest's de-framer has no way to tell that the bytes it is accumulating stopped being the same
// message.
//
// THIS WAS REAL TWICE, in the same place, for two different reasons, and the second is why this
// test sweeps instead of picking a moment. First, delivery took the reply queue first
// unconditionally, so any reply generated mid-frame cut in; that went unnoticed for as long as the
// modelled card answered only two command codes and therefore almost never had anything to say,
// and widening the policy to the two codes the box sends on every key press made it common.
//
// The fix tracked "a frame is under way" as "the last byte presented was not zero" -- and A KEY
// FRAME CONTAINS A ZERO. Encode escapes it as `1b 00`, so at its fifth byte the guard fell open
// again and a waiting reply spliced in exactly there:
//
//	05 80 02 1b 00 | 02 2b 18 00 | 05 a0 00
//
// That is the lost first press (gort-4sx.firstkey), measured on the real firmware: a box idle long
// enough to have exchanged a heartbeat has a reply pending when the viewer's key arrives, so the
// first press of every visit was destroyed and the second worked.
//
// A TEST THAT PICKED ONE MOMENT MISSED IT, and did so while looking like coverage: the original
// released its command on the third delivered byte, the reply landed after the escape had passed,
// and the case that mattered was never exercised. So the moment is now the variable -- the reply is
// released at every byte position across the frame, and the frame must survive all of them.
func TestACardReplyDoesNotSpliceItselfIntoAHandsetFrame(t *testing.T) {
	t.Parallel()
	// 0x7d is the Sky key from the record. Its payload carries a zero in the fourth byte, so the
	// encoded frame is 05 80 02 1b 00 07 d0 00 -- eight bytes with the escape in the middle.
	key := Encode([]byte{5, 0x80, 2, 0, 7, 0xd0})
	if !bytes.Contains(key, []byte{0x1b, 0}) {
		t.Fatalf("harness: the frame under test %x carries no escaped zero, so this sweep cannot "+
			"reach the case it exists for", key)
	}

	// A command the policy answers, so a reply really is generated. THE ARGUMENT IS 0x08 AND NOT
	// ZERO: a zero byte IS the terminator on this wire unless it is escaped, so `03 01 41 00` ends
	// the frame one byte early, fails its own length check and is dropped -- no reply, nothing
	// spliced, and a sweep that passes while proving nothing. `03 06 41 08` is one of the real ones
	// out of the record's boot capture.
	command := []byte{3, 1, 0x41, 8, 0}

	for release := 0; release <= len(key); release++ {
		t.Run(fmt.Sprintf("reply_released_at_byte_%d", release), func(t *testing.T) {
			t.Parallel()
			link := New(nil)
			link.Write(0, bus.Word, 0x80)
			link.Write(0x30, bus.Word, 1)
			link.Pump(0, PowerupTicks)
			if err := link.Key(0x7d, 0); err != nil {
				t.Fatal(err)
			}

			pending := command
			var got []byte
			at := uint64(0)
			for step := 0; step < 200 && len(got) < len(key)+len(command)+8; step++ {
				at += TrafficInstructions
				link.Pump(at, 0)
				// The guest clocks this link by writing one byte for each it takes, so the
				// command goes out at the pace the frame comes in -- which is what puts the
				// reply's arrival at a chosen byte of the frame rather than at a chosen instant.
				var out byte
				if step >= release && len(pending) > 0 {
					out, pending = pending[0], pending[1:]
				}
				link.Write(0x10, bus.Word, uint32(out))
				link.Pump(at, 0)
				if link.Read(0x20, bus.Word) != 1 {
					continue
				}
				got = append(got, byte(link.Read(0x10, bus.Word)))
				link.Write(0x20, bus.Word, 0)
			}
			if len(got) < len(key) {
				t.Fatalf("harness: only %d bytes came back, too few to hold the %d-byte handset "+
					"frame at all -- the link is not delivering and this proves nothing",
					len(got), len(key))
			}
			if !bytes.Contains(got, key) {
				t.Fatalf("the handset frame %x does not appear contiguously in what the guest "+
					"received:\n  %x\na reply cut into it at byte %d, so the guest de-frames two "+
					"corrupt messages instead of a key and an acknowledgement", key, got, release)
			}
		})
	}
}
