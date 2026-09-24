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
