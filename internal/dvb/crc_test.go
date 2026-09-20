package dvb

import "testing"

func TestMPEGCRC32(t *testing.T) {
	t.Parallel()
	if got := MPEGCRC32([]byte("123456789")); got != 0x0376e6e7 {
		t.Fatalf("MPEG-2 CRC check vector = %#08x", got)
	}
	// A section with its big-endian checksum appended has a zero residue.
	section := make([]byte, 0, 12)
	section = append(section, 0x70, 0x70, 0x05, 0xc6, 0x7e, 0x12, 0, 0)
	crc := MPEGCRC32(section)
	section = append(section, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	if got := MPEGCRC32(section); got != 0 {
		t.Fatalf("whole-section CRC residue = %#08x", got)
	}
	section[4] ^= 1
	if got := MPEGCRC32(section); got == 0 {
		t.Fatal("corruption retained zero CRC residue")
	}
}
