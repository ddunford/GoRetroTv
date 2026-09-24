package dvb

// PacketizeSection places one complete DVB section on the MPEG-2 transport wire. The first packet
// carries pointer_field zero; later packets continue the same section, and unused payload bytes are
// the specified 0xFF stuffing value.
func PacketizeSection(pid uint16, section []byte, continuity byte) []byte {
	remaining := section
	var out []byte
	first := true
	for first || len(remaining) > 0 {
		packet := make([]byte, 188)
		for i := range packet {
			packet[i] = 0xff
		}
		packet[0] = 0x47
		packet[1] = byte(pid >> 8 & 0x1f)
		if first {
			packet[1] |= 0x40
		}
		packet[2] = byte(pid & 0xff)
		packet[3] = 0x10 | continuity&0x0f
		continuity = (continuity + 1) & 0x0f
		at := 4
		if first {
			packet[at] = 0
			at++
		}
		n := min(len(remaining), len(packet)-at)
		copy(packet[at:], remaining[:n])
		remaining = remaining[n:]
		out = append(out, packet...)
		first = false
	}
	return out
}
