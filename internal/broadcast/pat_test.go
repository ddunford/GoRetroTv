package broadcast_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// A PAT IS CHECKED AGAINST THE SPECIFICATION IT CLAIMS, field by field, because it is the one
// table in this transmitter that was built from a document rather than measured off the box. A
// section that is merely well-formed to its own builder proves nothing.
func TestThePATSaysWhereEachProgrammesMapTableIs(t *testing.T) {
	t.Parallel()
	section, err := broadcast.PAT(0x0020, 3, []broadcast.Programme{
		{Number: 1, MapPID: 0x0100},
		{Number: 2, MapPID: 0x01ff},
	})
	if err != nil {
		t.Fatal(err)
	}
	if section[0] != 0x00 {
		t.Errorf("table id is %#02x, want 0x00", section[0])
	}
	if section[1]&0xb0 != 0xb0 {
		t.Errorf("syntax indicator and reserved bits are %#02x, want the top nibble 0xb", section[1])
	}
	if length := int(section[1]&0x0f)<<8 | int(section[2]); length != len(section)-3 {
		t.Errorf("section_length says %d, but %d bytes follow it", length, len(section)-3)
	}
	if id := uint16(section[3])<<8 | uint16(section[4]); id != 0x0020 {
		t.Errorf("transport_stream_id is %#04x, want 0x0020", id)
	}
	if v := (section[5] >> 1) & 0x1f; v != 3 {
		t.Errorf("version is %d, want 3", v)
	}
	if section[5]&1 != 1 {
		t.Error("current_next_indicator is not set, so the box would hold this table as the NEXT one")
	}
	// Two four-byte entries between the eight-byte header and the four-byte CRC.
	if body := len(section) - 8 - 4; body != 8 {
		t.Fatalf("the programme loop is %d bytes, want 8 for two programmes", body)
	}
	for i, want := range []struct{ number, pid uint16 }{{1, 0x0100}, {2, 0x01ff}} {
		at := 8 + i*4
		number := uint16(section[at])<<8 | uint16(section[at+1])
		pid := uint16(section[at+2])<<8 | uint16(section[at+3])
		if number != want.number {
			t.Errorf("programme %d has number %d, want %d", i, number, want.number)
		}
		if pid&0xe000 != 0xe000 {
			t.Errorf("programme %d leaves the three reserved bits clear (%#04x)", i, pid)
		}
		if pid&0x1fff != want.pid {
			t.Errorf("programme %d maps to PID %#04x, want %#04x", i, pid&0x1fff, want.pid)
		}
	}
	// THE CRC IS THE WHOLE POINT OF A SECTION: the hardware checks it, and a table that fails it
	// is discarded before any firmware sees it -- which would look exactly like "the box ignored
	// our PAT".
	if crc := dvb.MPEGCRC32(section[:len(section)-4]); crc != uint32(section[len(section)-4])<<24|
		uint32(section[len(section)-3])<<16|uint32(section[len(section)-2])<<8|
		uint32(section[len(section)-1]) {
		t.Error("the CRC does not cover the section, so the demux would drop it")
	}
	if _, err := broadcast.PAT(0x0020, 3, nil); err == nil {
		t.Error("a PAT with no programmes was accepted, and it announces nothing")
	}
	if _, err := broadcast.PAT(0x0020, 3, []broadcast.Programme{{Number: 0, MapPID: 0x100}}); err == nil {
		t.Error("programme number 0 was accepted, and it is reserved for the network PID")
	}
}
