package broadcast

import (
	"bytes"
	"testing"
)

func TestPESCarriesStreamTimestampAndPayload(t *testing.T) {
	t.Parallel()
	payload := []byte{0x00, 0x00, 0x01, 0xb3, 0x16, 0x01}
	pes, err := PES(0xe0, 90_000, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pes[:9], []byte{0, 0, 1, 0xe0, 0, 14, 0x80, 0x80, 5}) {
		t.Fatalf("PES header = % X", pes[:9])
	}
	if !bytes.Equal(pes[9:14], []byte{0x21, 0x00, 0x05, 0xbf, 0x21}) {
		t.Fatalf("PTS = % X", pes[9:14])
	}
	if !bytes.Equal(pes[14:], payload) {
		t.Fatalf("payload = % X", pes[14:])
	}
}

func TestPESRejectsInvalidFields(t *testing.T) {
	t.Parallel()
	if _, err := PES(0xbd, 0, nil); err == nil {
		t.Fatal("accepted a private stream id as MPEG audio/video")
	}
	if _, err := PES(0xe0, 1<<33, nil); err == nil {
		t.Fatal("accepted a timestamp wider than the PES field")
	}
	if _, err := PES(0xc0, 0, make([]byte, 0x1_0000)); err == nil {
		t.Fatal("accepted an oversized audio PES packet")
	}
}
