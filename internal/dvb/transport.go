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

// PacketizePES places one complete PES packet on an MPEG-2 transport PID. Unlike a section, a PES
// begins directly after the transport header: payload_unit_start_indicator marks its start but
// there is no pointer_field. The final packet uses an adaptation field for stuffing so 0xFF bytes
// cannot be mistaken for elementary-stream payload.
func PacketizePES(pid uint16, pes []byte, continuity byte) []byte {
	remaining := pes
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
		n := min(len(remaining), 184)
		if n < 184 {
			packet[3] |= 0x20
			adaptationLength := 183 - n
			packet[4] = byte(adaptationLength)
			if adaptationLength > 0 {
				packet[5] = 0 // no adaptation flags; the rest is stuffing
			}
			at = 5 + adaptationLength
		}
		copy(packet[at:], remaining[:n])
		remaining = remaining[n:]
		out = append(out, packet...)
		first = false
	}
	return out
}
