package dvb

import (
	"bytes"
	"testing"
)

func TestPacketizeSectionCarriesPointerContinuationAndStuffing(t *testing.T) {
	t.Parallel()
	section := make([]byte, 300)
	for i := range section {
		section[i] = byte(i)
	}
	packets := PacketizeSection(0x123, section, 14)
	if len(packets) != 2*188 {
		t.Fatalf("packetized length = %d", len(packets))
	}
	if got := packets[:5]; !bytes.Equal(got, []byte{0x47, 0x41, 0x23, 0x1e, 0}) {
		t.Fatalf("first header = % X", got)
	}
	if got := packets[188:192]; !bytes.Equal(got, []byte{0x47, 0x01, 0x23, 0x1f}) {
		t.Fatalf("continuation header = % X", got)
	}
	reassembled := append(append([]byte(nil), packets[5:188]...), packets[192:192+117]...)
	if !bytes.Equal(reassembled, section) {
		t.Fatal("packet payload does not reassemble to the section")
	}
	for _, b := range packets[192+117:] {
		if b != 0xff {
			t.Fatalf("stuffing byte = %#x", b)
		}
	}
}

func TestPacketizePESCarriesStartContinuityAndAdaptationStuffing(t *testing.T) {
	t.Parallel()
	pes := make([]byte, 300)
	for i := range pes {
		pes[i] = byte(i)
	}
	packets := PacketizePES(0x101, pes, 15)
	if len(packets) != 2*188 {
		t.Fatalf("packetized length = %d", len(packets))
	}
	if got := packets[:4]; !bytes.Equal(got, []byte{0x47, 0x41, 0x01, 0x1f}) {
		t.Fatalf("first header = % X", got)
	}
	if got := packets[188:193]; !bytes.Equal(got, []byte{0x47, 0x01, 0x01, 0x30, 67}) {
		t.Fatalf("final header/adaptation length = % X", got)
	}
	if packets[193] != 0 {
		t.Fatalf("adaptation flags = %#02x", packets[193])
	}
	reassembled := append(append([]byte(nil), packets[4:188]...), packets[188+72:]...)
	if !bytes.Equal(reassembled, pes) {
		t.Fatal("packet payload does not reassemble to the PES")
	}
}
