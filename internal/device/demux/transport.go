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

// ScheduleTransport replaces the pending transport with complete packets which enter the demux at
// an instruction-counted cadence. The input is copied so its caller cannot mutate future state.
func (d *Demux) ScheduleTransport(data []byte, first, period uint64) error {
	if d.programmeScheduled {
		return fmt.Errorf("demux: programme transport is already scheduled")
	}
	if period == 0 {
		return fmt.Errorf("demux: programme transport period is zero")
	}
	if len(data) == 0 || len(data)%transportPacketSize != 0 || len(data) > maxProgrammeTransport {
		return fmt.Errorf("demux: programme transport has invalid length %d", len(data))
	}
	for at := 0; at < len(data); at += transportPacketSize {
		if data[at] != 0x47 {
			return fmt.Errorf("demux: programme transport packet %d has sync byte %#02x",
				at/transportPacketSize, data[at])
		}
	}
	d.scheduledTransport = append(d.scheduledTransport[:0], data...)
	d.scheduledCursor = 0
	d.nextProgrammeAt, d.programmePeriod = first, period
	d.programmeScheduled = true
	return nil
}

// TransportPending reports whether an instruction-counted programme burst is still entering the
// device. It lets the edge producer apply backpressure without replacing packets already in flight.
func (d *Demux) TransportPending() bool { return d.programmeScheduled }

// Pump admits every packet whose instruction-count deadline has passed. The board invokes this
// from the same deterministic clock pump as the hardware timer and serial links.
func (d *Demux) Pump(now uint64) error {
	for d.programmeScheduled && now >= d.nextProgrammeAt {
		at := int(d.scheduledCursor)
		if err := d.PushTransport(d.scheduledTransport[at : at+transportPacketSize]); err != nil {
			return err
		}
		d.scheduledCursor += transportPacketSize
		d.nextProgrammeAt += d.programmePeriod
		if int(d.scheduledCursor) == len(d.scheduledTransport) {
			d.programmeScheduled = false
		}
	}
	return nil
}

// PushTransport consumes complete DVB transport packets entering the demux. The ROM's self-test
// feeds this input through DMA channel 5 before the application section rings exist; the tuned
// application instead programs the demux's dedicated video/audio PID inputs and does not arm a
// new general DMA channel for them.
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
	videoPID, audioPID, programmeReady := d.ProgrammePIDs()
	// The host decoder consumes a transport stream, not the receiver's internal elementary-stream
	// bus, so retain the PAT and the announced PMT alongside the two PIDs the guest selected. They
	// are admitted only after both decoder inputs are valid; they cannot create a selection.
	const programmeMapPID = 0x100
	if programmeReady && (pid == 0 || pid == programmeMapPID || pid == videoPID || pid == audioPID) {
		if len(d.programmeTransport)+len(packet) > maxProgrammeTransport {
			return fmt.Errorf("demux: programme transport queue exceeds %d bytes", maxProgrammeTransport)
		}
		d.programmeTransport = append(d.programmeTransport, packet...)
		return nil
	}
	for channel := uint8(0); channel < FilterCount; channel++ {
		if !d.pidWritten[channel] || d.pidChannels[channel]&0x1fff != uint32(pid) {
			continue
		}
		applicationRoute := d.enable[2]&(1<<channel) != 0
		romRoute := channel < 2 && d.matchWords[channel][8]&(1<<channel) != 0
		if !applicationRoute && !romRoute {
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

// TakeProgrammeTransport drains transport packets admitted by the two decoder PIDs the guest
// programmed at +0x94/+0x98, plus PAT/PMT metadata needed to describe those streams to the host
// transport decoder. No general DMA channel is implied: the measured tuned firmware arms none for
// media, so this is the dedicated decoder input boundary and nothing more.
func (d *Demux) TakeProgrammeTransport() []byte {
	packets := append([]byte(nil), d.programmeTransport...)
	d.programmeTransport = nil
	return packets
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
	if err := validateSection(section); err != nil {
		return nil //nolint:nilerr // hardware silently drops malformed transport sections
	}
	if channel < 2 && d.enable[2]&(1<<channel) == 0 &&
		d.matchWords[channel][8]&(1<<channel) != 0 {
		return d.acceptROMTransportSection(channel, section)
	}
	routed := false
	tableHasRule := false
	for unit := uint8(0); unit < 16; unit++ {
		table, ok := d.Match(unit, 0)
		if ok && table.Mask != 0 && section[0]&table.Mask == table.Value&table.Mask {
			tableHasRule = true
		}
		if d.matchWords[unit][9]&(uint32(1)<<channel) == 0 {
			continue
		}
		routed = true
		if d.sectionMatches(unit, section) {
			return d.pushFilter(channel, section)
		}
	}
	if routed {
		// Some application channels are deliberately PID-only. The fixed PMT channel is the
		// measured example: the guest opens PID 0x0100 but programs no table-0x02 match unit.
		// Bits in unrelated units' byte-nine words overlap the channel bitmap and used to make
		// that look routed through the BAT unit, dropping every PMT against table 0x4A. When no
		// unit in the machine describes this table at all, there is no table rule to apply and
		// the enabled PID channel admits it. A table for which the guest did program a rule is
		// still dropped when none of its routed rules match.
		if tableHasRule {
			return nil
		}
		return d.pushFilter(channel, section)
	}
	return d.pushFilter(channel, section)
}

// acceptROMTransportSection implements the boot ROM's two temporary PSI channels. The ROM binds
// unit 0 to channel 0 and unit 1 to channel 1 in match word 8, but does not use the application's
// channel-enable register or section rings. It instead programs four indirect words per channel
// and polls the write pointer in the shared transport RAM.
func (d *Demux) acceptROMTransportSection(channel uint8, section []byte) error {
	for i, offset := range [...]int{0, 3, 4} {
		word := d.matchWords[0][i]
		if channel == 1 {
			word >>= 16
		}
		match, mask := byte((word>>8)&0xff), byte(word&0xff)
		if section[offset]&mask != match&mask {
			return nil
		}
	}
	start, end := d.indirect[uint32(channel)*4], d.indirect[uint32(channel)*4+3]
	ptr := d.writePointer[channel]
	if ptr < start || uint64(ptr)+uint64(len(section))+1 > uint64(end)+1 ||
		uint64(ptr)+uint64(len(section))+1 > 0x1fffff ||
		uint64(transportRAMOffset)+uint64(ptr)+uint64(len(section))+1 > uint64(d.ram.Size()) {
		return fmt.Errorf("demux: ROM transport section exceeds configured ring")
	}
	for i, b := range section {
		d.ram.Write(transportRAMOffset+ptr+uint32(i), bus.Byte, uint32(b)) // #nosec G115 -- bounded above.
	}
	d.ram.Write(transportRAMOffset+ptr+uint32(len(section)), bus.Byte, uint32(channel)) // #nosec G115 -- bounded above.
	d.writePointer[channel] += uint32(len(section)) + 1                                 // #nosec G115 -- checked above.
	return nil
}
