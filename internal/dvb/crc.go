// Package dvb holds wire-format primitives shared by DVB section producers and consumers.
package dvb

// MPEGCRC32 computes the non-reflected MPEG-2 CRC with polynomial 0x04C11DB7,
// all-one initial state and no final inversion.
func MPEGCRC32(data []byte) uint32 {
	crc := ^uint32(0)
	for _, b := range data {
		crc ^= uint32(b) << 24
		for bit := 0; bit < 8; bit++ {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
