package demux

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
)

const (
	transportPacketSize = 188
	transportRAMOffset  = 0x3b0000
	maxTransportSection = 4096
)

// PushTransport consumes complete DVB transport packets moved by the media DMA.
// The ROM's self-test uses this path before the application section rings exist.
func (d *Demux) PushTransport(data []byte) error {
	if d.ram == nil {
		return fmt.Errorf("demux: transport RAM is not bound")
	}
	if d.control140&1 == 0 {
		return nil
	}
	for _, b := range data {
		if len(d.transportPacket) == 0 && b == 0 {
			continue // A zero-filled DMA descriptor flushes without a packet.
		}
		if len(d.transportPacket) == 0 && b != 0x47 {
			return fmt.Errorf("demux: transport sync byte %#x", b)
		}
		d.transportPacket = append(d.transportPacket, b)
		if len(d.transportPacket) != transportPacketSize {
			continue
		}
		if err := d.pushTransportPacket(d.transportPacket); err != nil {
			return err
		}
		d.transportPacket = nil
	}
	return nil
}

func (d *Demux) pushTransportPacket(packet []byte) error {
	if packet[1]&0x80 != 0 {
		return nil
	} // transport-error indicator
	pid := uint16(packet[1]&0x1f)<<8 | uint16(packet[2])
	control := (packet[3] >> 4) & 3
	if control&1 == 0 {
		return nil
	}
	pos := 4
	if control&2 != 0 {
		pos += 1 + int(packet[pos])
		if pos > transportPacketSize {
			return fmt.Errorf("demux: invalid adaptation length")
		}
	}
	for channel := uint8(0); channel < 2; channel++ {
		if !d.pidWritten[channel] || d.pidChannels[channel]&0x1fff != uint32(pid) || d.matchWords[channel][8]&(1<<channel) == 0 {
			continue
		}
		payload := packet[pos:]
		if packet[1]&0x40 != 0 {
			if len(payload) == 0 {
				return fmt.Errorf("demux: missing section pointer")
			}
			pointer := int(payload[0])
			payload = payload[1:]
			if pointer > len(payload) {
				return fmt.Errorf("demux: section pointer outside packet")
			}
			if err := d.continueTransport(channel, payload[:pointer]); err != nil {
				return err
			}
			if len(d.transportPart[channel]) != 0 {
				d.transportPart[channel] = nil
			}
			if err := d.startTransport(channel, payload[pointer:]); err != nil {
				return err
			}
		} else if err := d.continueTransport(channel, payload); err != nil {
			return err
		}
	}
	return nil
}

func (d *Demux) startTransport(channel uint8, payload []byte) error {
	for len(payload) > 0 && payload[0] != 0xff {
		if len(payload) < 3 {
			d.transportPart[channel] = append(d.transportPart[channel][:0], payload...)
			return nil
		}
		length := 3 + (int(payload[1]&0x0f) << 8) + int(payload[2])
		if length < 7 || length > maxTransportSection {
			return fmt.Errorf("demux: transport section length %d", length)
		}
		if len(payload) < length {
			d.transportPart[channel] = append(d.transportPart[channel][:0], payload...)
			return nil
		}
		if err := d.acceptTransportSection(channel, payload[:length]); err != nil {
			return err
		}
		payload = payload[length:]
	}
	return nil
}

func (d *Demux) continueTransport(channel uint8, payload []byte) error {
	part := d.transportPart[channel]
	if len(part) == 0 {
		return nil
	}
	part = append(part, payload...)
	if len(part) < 3 {
		d.transportPart[channel] = part
		return nil
	}
	length := 3 + (int(part[1]&0x0f) << 8) + int(part[2])
	if length < 7 || length > maxTransportSection {
		return fmt.Errorf("demux: transport section length %d", length)
	}
	if len(part) < length {
		d.transportPart[channel] = part
		return nil
	}
	d.transportPart[channel] = nil
	return d.acceptTransportSection(channel, part[:length])
}

func (d *Demux) acceptTransportSection(channel uint8, section []byte) error {
	if mpegCRC(section) != 0 {
		return nil
	}
	for i, offset := range [...]int{0, 3, 4} {
		word := d.matchWords[0][i]
		if channel == 1 {
			word >>= 16
		}
		match, mask := byte(word>>8), byte(word)
		if section[offset]&mask != match&mask {
			return nil
		}
	}
	start, end := d.indirect[uint32(channel)*4], d.indirect[uint32(channel)*4+3]
	ptr := d.writePointer[channel]
	if ptr < start || uint64(ptr)+uint64(len(section))+1 > uint64(end)+1 || uint64(ptr)+uint64(len(section))+1 > 0x1fffff || uint64(transportRAMOffset)+uint64(ptr)+uint64(len(section))+1 > uint64(d.ram.Size()) {
		return fmt.Errorf("demux: transport section exceeds configured ring")
	}
	for i, b := range section {
		d.ram.Write(transportRAMOffset+ptr+uint32(i), bus.Byte, uint32(b)) // #nosec G115 -- section length bounded above.
	}
	d.ram.Write(transportRAMOffset+ptr+uint32(len(section)), bus.Byte, uint32(channel)) // #nosec G115 -- length and channel bounded above.
	d.writePointer[channel] += uint32(len(section)) + 1                                 // #nosec G115 -- checked against ring limit above.
	return nil
}
