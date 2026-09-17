package csi

// SetAckPolicy selects the command codes the front-panel micro answers.
// The measured boot policy answers card status (0x52) and heartbeat (0x18).
func (l *Link) SetAckPolicy(codes []uint8) {
	l.ack = [4]uint64{}
	for _, code := range codes {
		l.ack[code/64] |= 1 << (code % 64)
	}
}

// AckAll is a diagnostic policy that acknowledges every command.
func (l *Link) AckAll() { l.ack = [4]uint64{^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0)} }

func (l *Link) accepts(code uint8) bool { return l.ack[code/64]&(1<<(code%64)) != 0 }

// accept de-frames one guest transmit byte. It is called only while the link
// is enabled; a zero outside an escaped payload ends a frame.
func (l *Link) accept(b byte) {
	if l.escaped {
		l.escaped = false
		if len(l.frame) < 32 {
			l.frame = append(l.frame, b)
		}
		return
	}
	if b == 0x1b {
		l.escaped = true
		return
	}
	if b != 0 {
		if len(l.frame) < 32 {
			l.frame = append(l.frame, b)
		}
		return
	}
	frame := l.frame
	l.frame = nil
	if len(frame) < 3 || len(frame) != int(frame[0])+1 || !l.accepts(frame[2]) {
		return
	}
	seq, code := frame[1], frame[2]
	payload := []byte{2, seq, code}
	switch code {
	case 0x52:
		payload = []byte{3, seq, code, 2}
	case 0x10:
		payload = []byte{8, seq, code, 0, 0, 0, 0, 0, 0}
	case 0x44:
		payload = []byte{4, seq, code, 1, 0xff}
	}
	wire := Encode(payload)
	if len(wire) <= MaxQueuedBytes-len(l.reply) {
		l.reply = append(l.reply, wire...)
	}
}
