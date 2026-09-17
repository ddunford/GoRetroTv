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
	l.Pump(PowerupInstructions - 1)
	if l.Read(0x20, bus.Word) != 0 {
		t.Fatal("reader answered before power-up")
	}
	l.Pump(PowerupInstructions)
	if l.Read(0x20, bus.Word) != 1 || l.Read(0x10, bus.Word) != uint32(want[0]) {
		t.Fatal("paced reply did not enter receive register")
	}
	remaining := len(l.reply)
	l.Write(0x20, bus.Word, 0)
	l.Pump(PowerupInstructions + TrafficInstructions)
	if len(l.reply) != remaining {
		t.Fatal("reply advanced before guest transmit receipt")
	}
	l.Write(0x10, bus.Word, 0)
	l.Pump(PowerupInstructions + 2*TrafficInstructions)
	if len(l.reply) != remaining-1 {
		t.Fatal("receipt failed to release next reply byte")
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
