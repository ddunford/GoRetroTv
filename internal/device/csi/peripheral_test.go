package csi

import (
	"bytes"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
)

func sendFrame(link *Link, payload []byte) {
	for _, b := range Encode(payload) {
		link.Write(0x10, bus.Word, uint32(b))
	}
}

func TestPeripheralAcknowledgesOnlyMeasuredCommands(t *testing.T) {
	t.Parallel()
	l := New(nil)
	l.Write(0, bus.Word, 0x80)
	l.Write(0x30, bus.Word, 1)
	sendFrame(l, []byte{3, 1, 0x11, 7})
	if len(l.reply) != 0 {
		t.Fatal("default policy answered an unmeasured command")
	}
	sendFrame(l, []byte{3, 9, 0x52, 0})
	want := Encode([]byte{3, 9, 0x52, 2})
	if !bytes.Equal(l.reply, want) {
		t.Fatalf("card status response %x, want %x", l.reply, want)
	}
	l.Pump(PowerupTicks*20000-1, PowerupTicks-1)
	if l.Read(0x20, bus.Word) != 0 {
		t.Fatal("reader answered before power-up")
	}
	l.Pump(PowerupTicks*20000, PowerupTicks)
	if l.Read(0x20, bus.Word) != 1 || l.Read(0x10, bus.Word) != uint32(want[0]) {
		t.Fatal("paced reply did not enter receive register")
	}
	remaining := len(l.reply)
	l.Write(0x20, bus.Word, 0)
	l.Pump(PowerupTicks*20000+TrafficInstructions, PowerupTicks)
	if len(l.reply) != remaining {
		t.Fatal("reply advanced before guest transmit receipt")
	}
	l.Write(0x10, bus.Word, 0)
	l.Pump(PowerupTicks*20000+2*TrafficInstructions, PowerupTicks)
	if len(l.reply) != remaining-1 {
		t.Fatal("receipt failed to release next reply byte")
	}
}

func TestReaderPowerupUsesBoardTicksAndRearmsOnEnableWrite(t *testing.T) {
	t.Parallel()
	l := New(nil)
	l.Pump(1000000, 10)
	l.Write(0, bus.Word, 0x80)
	l.Write(0x30, bus.Word, 1)
	l.Pump(5000000, 129)
	if l.cardLive {
		t.Fatal("reader powered up before 120 timer ticks")
	}
	l.Pump(5000001, 130)
	if !l.cardLive {
		t.Fatal("reader did not power up at tick 130")
	}
	l.Write(0x30, bus.Word, 1)
	if l.cardLive {
		t.Fatal("enable write did not restart reader power-up")
	}
	l.Pump(6000000, 249)
	if l.cardLive {
		t.Fatal("reader restarted before 120 more ticks")
	}
	l.Pump(6000001, 250)
	if !l.cardLive {
		t.Fatal("reader did not restart at tick 250")
	}
}

func TestAcknowledgeAllChangesTheReplyPolicy(t *testing.T) {
	t.Parallel()
	l := New(nil)
	l.Write(0, bus.Word, 0x80)
	l.Write(0x30, bus.Word, 1)
	l.AckAll()
	sendFrame(l, []byte{3, 2, 0x11, 7})
	if want := Encode([]byte{2, 2, 0x11}); !bytes.Equal(l.reply, want) {
		t.Fatalf("all-policy echo = %x, want %x", l.reply, want)
	}
	l.reply = nil
	sendFrame(l, []byte{3, 3, 0x10, 0})
	if want := Encode([]byte{8, 3, 0x10, 0, 0, 0, 0, 0, 0}); !bytes.Equal(l.reply, want) {
		t.Fatalf("version response = %x, want %x", l.reply, want)
	}
}
