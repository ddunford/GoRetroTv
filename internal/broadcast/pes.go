package broadcast

import "fmt"

// PES builds one packetized elementary-stream packet with a presentation timestamp. Payload is
// already encoded elementary-stream data; this function supplies only the H.222.0 PES envelope.
func PES(streamID byte, pts90k uint64, payload []byte) ([]byte, error) {
	if pts90k >= 1<<33 {
		return nil, fmt.Errorf("broadcast: PES timestamp %d exceeds 33 bits", pts90k)
	}
	if streamID < 0xc0 || streamID > 0xef {
		return nil, fmt.Errorf("broadcast: PES stream id %#02x is not MPEG audio or video", streamID)
	}
	// packet_length counts everything after its own two-byte field. A zero length is permitted for
	// video PES packets; audio packets must retain their explicit boundary.
	packetLength := 3 + 5 + len(payload)
	if packetLength > 0xffff && streamID < 0xe0 {
		return nil, fmt.Errorf("broadcast: audio PES payload is too large (%d bytes)", len(payload))
	}
	length := packetLength
	if length > 0xffff {
		length = 0
	}
	pes := []byte{0x00, 0x00, 0x01, streamID, byte(length >> 8), byte(length), 0x80, 0x80, 0x05}
	pes = append(pes,
		0x21|byte((pts90k>>29)&0x0e),
		byte((pts90k>>22)&0xff),
		0x01|byte((pts90k>>14)&0xfe),
		byte((pts90k>>7)&0xff),
		0x01|byte((pts90k<<1)&0xfe),
	)
	return append(pes, payload...), nil
}
