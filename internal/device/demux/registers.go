package demux

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// MMIOBase is the EMMA demux register window on the board's uncached bus.
	MMIOBase = 0xB000A000
	// MMIOSize is the demux register window attached to the physical bus.
	MMIOSize             = 0x1000
	demuxSnapshotVersion = 5
)

// Demux owns the status and enable state of four EMMA interrupt groups.
// Other registers answer zero until a measured read contract gives them meaning.
type Demux struct {
	name             string
	status           [4]uint32
	enable           [4]uint32
	selectedFilter   uint32
	selectedIndirect uint8
	writePointer     [FilterCount]uint32
	pidChannels      [FilterCount]uint32
	pidWritten       [FilterCount]bool
	matchValue       uint32
	matchUnit        uint8
	matchIndex       uint8
	matchWords       [16][16]uint32
	control140       uint32
	indirectData     uint32
	indirect         [FilterCount]uint32
	transportPart    [FilterCount][]byte
	transportPacket  []byte
	ram              *memory.RAM
	interrupt        *irq.Controller
}

// MatchByte is one byte of a section match unit's value and mask.
type MatchByte struct {
	Value uint8
	Mask  uint8
}

// New returns the reset state of the EMMA transport demux.
func New() *Demux { return &Demux{name: "emma-demux"} }

// Name is the stable key for machine snapshots.
func (d *Demux) Name() string { return d.name }

// Read answers only measured status and enable registers. Recording a write elsewhere does not
// justify making that address read back: echoing +0x124 stopped the predecessor's RTOS boot.
func (d *Demux) Read(off uint32, size bus.Size) uint32 {
	reg := off &^ 3
	var word uint32
	switch {
	case reg >= 0xB0 && reg <= 0xBC:
		word = d.status[(reg-0xB0)/4]
	case reg >= 0xD0 && reg <= 0xDC:
		word = d.enable[(reg-0xD0)/4]
	case reg == 0x128:
		word = d.writePointer[d.selectedFilter] & 0x1fffff
	default:
		return 0
	}
	shift := laneShift(off, size)
	return word >> shift & widthMask(size)
}

// Write gives the filter enables write-one-to-set semantics and the completion status
// write-zero-to-clear semantics. A block reset is the one operation that drops enables.
func (d *Demux) Write(off uint32, size bus.Size, value uint32) {
	reg := off &^ 3
	mask := widthMask(size) << laneShift(off, size)
	bits := (value << laneShift(off, size)) & mask
	switch {
	case reg >= 0xB0 && reg <= 0xBC:
		i := (reg - 0xB0) / 4
		d.status[i] &= ^mask | bits
		d.updateLine()
	case reg >= 0xD0 && reg <= 0xDC:
		i := (reg - 0xD0) / 4
		d.enable[i] |= bits
		d.updateLine()
	case reg == 0 && value&1 != 0:
		d.Reset()
	case reg == 0x124:
		d.selectedIndirect = uint8(value & 0x7f) // #nosec G115 -- seven-bit address.
		d.selectedFilter = uint32(d.selectedIndirect>>2) & (FilterCount - 1)
		if value&0xC000 == 0xC000 {
			if d.selectedIndirect < FilterCount {
				d.indirect[d.selectedIndirect] = d.indirectData
			}
			if d.selectedIndirect%4 == 0 {
				d.writePointer[d.selectedFilter] = d.indirectData & 0x1fffff
			}
		}
	case reg == 0x128:
		d.indirectData = value
	case reg == 0x140:
		d.control140 = value
	case reg >= 0x14 && reg < 0x14+4*FilterCount:
		channel := (reg - 0x14) / 4
		d.pidChannels[channel] = value
		d.pidWritten[channel] = true
	case reg == 0x148:
		d.matchValue = value
	case reg == 0x144 && value&0x4000 != 0:
		d.matchUnit, d.matchIndex = uint8(value&0xf), uint8((value>>4)&0xf) // #nosec G115 -- masked to nibbles.
		if value&0x8000 != 0 {
			d.matchWords[d.matchUnit][d.matchIndex] = d.matchValue
		}
	}
}

// channelPID reads a channel register, and says whether that channel is watching anything.
//
// A CHANNEL REGISTER OF ZERO IS A CLEARED CHANNEL, NOT A CHANNEL WATCHING PID 0, and telling those
// two apart is not cosmetic: PID 0 is where a programme association table lives, so a cleared
// channel read as "armed for PID 0" says the box is asking for a PAT when it is asking for nothing.
// That reading sent this project as far as building a PAT, pushing it and measuring that the box
// ignored it -- the box ignored it because it had never asked.
//
// The evidence is the shape of the registers the guest actually writes. Every channel it programs
// carries 0x14000 above the thirteen-bit PID:
//
//	channel 19 -> 00014034    channel 22 -> 00014014
//	channel 20 -> 00014033    channel 23 -> 00014011
//	channel 24 -> 00014010    and the one that read as PID 0 -> 00000000
//
// So a channel with nothing above the PID field has not been programmed, whatever its low bits say.
// What the 0x14000 MEANS is not established here and is not guessed at; only that its absence,
// together with a zero PID, is a register nobody has written a filter into.
func channelPID(word uint32) (uint16, bool) {
	pid := word & 0x1fff
	if pid == 0x1fff || word>>13 == 0 {
		return 0, false
	}
	return uint16(pid), true // #nosec G115 -- masked to thirteen bits above
}

// ArmedFilter is one armed section channel: which of the thirty-two it is, and the PID it watches.
type ArmedFilter struct {
	// Filter is the section channel index, 0..31. IT IS NOT A MATCH-UNIT INDEX: there are 32
	// channels and 16 units, Match refuses anything from 16 up, and passing a channel number to it
	// returns "not set" for every byte -- which reads as "this filter accepts any table" and is
	// how a run of six channels was reported as six unfiltered ones.
	Filter uint8
	// PID is the thirteen-bit PID that filter is watching.
	PID uint16
	// Word is the whole value the guest wrote to that channel's register. Only the low thirteen
	// bits are the PID; what the rest carries is not established, and it is exposed rather than
	// masked away because the binding between a channel and a match unit has to be written down
	// SOMEWHERE and this register is where the guest says everything else about the channel.
	Word uint32
}

// ArmedFilters answers which channel holds which PID, which ArmedPIDs alone cannot.
//
// The pairing is the whole point. A PID being open says a channel exists; only the filter index
// says which match RULES apply to it, and those are what decide whether a section arriving on that
// PID would reach the firmware on real hardware. Asking "is PID 0 armed" and concluding "so a
// programme association table can reach the box" skips exactly that step -- the PID was open and
// every rule on its filter was looking for a different table.
func (d *Demux) ArmedFilters() []ArmedFilter {
	var out []ArmedFilter
	for ch := range d.pidChannels {
		if !d.pidWritten[ch] || d.enable[2]&(1<<ch) == 0 {
			continue
		}
		pid, ok := channelPID(d.pidChannels[ch])
		if !ok {
			continue
		}
		out = append(out, ArmedFilter{
			Filter: uint8(ch), // #nosec G115 -- bounded by FilterCount
			PID:    pid,
			Word:   d.pidChannels[ch],
		})
	}
	return out
}

// TransportEnabled reports whether the guest has switched on the 188-byte transport packet path.
//
// It is here because PushTransport SILENTLY DOES NOTHING when it is off -- the ROM's self-test uses
// that path before the application's section rings exist, so a no-op is the correct behaviour and
// an error would be wrong. The cost is that a caller feeding a transport stream at a box that has
// not enabled it gets no signal at all that its packets went nowhere, which is exactly the kind of
// confident silence this project keeps having to unpick.
func (d *Demux) TransportEnabled() bool { return d.control140&1 != 0 }

// ArmedPIDs returns the PIDs in enabled, programmed channels. The channel index is a
// section filter index; the independent 16 match units do not select a PID channel.
func (d *Demux) ArmedPIDs() []uint16 {
	var pids []uint16
	for ch := range d.pidChannels {
		if !d.pidWritten[ch] || d.enable[2]&(1<<ch) == 0 {
			continue
		}
		if pid, ok := channelPID(d.pidChannels[ch]); ok {
			pids = append(pids, pid)
		}
	}
	return pids
}

// Match returns one independent unit's byte rule; it does not imply a PID channel.
//
// THE UNIT NUMBER SELECTS WHICH HALF OF THE WORD CARRIES THE RULE. Units 0..7
// write it in the low halfword and units 8..15 in the high one — sixteen units
// packed two to a word. Reading the low half for every unit made every high
// unit read back as 00/00, which is not "the guest did not program it": it
// looks exactly like a filter that was never set, and that is how the box's
// own listings subscription stayed invisible. Measured 2026-09-20 by logging
// the guest's writes to +0x148/+0x144 through the bus observer: unit 2 wrote
// 42ff42fb and means the SDT filter 42/fb in the low half, while unit 8 wrote
// a3fe0000, 0bff0000, b8ff0000, c6ff0000, 7eff0000 and means a complete title
// filter — table 0xA3 mask 0xFE, extension 0x0BB8, MJD 0xC67E — in the high
// one.
func (d *Demux) Match(unit, byteIndex uint8) (MatchByte, bool) {
	if unit >= 16 || byteIndex >= 16 {
		return MatchByte{}, false
	}
	word := d.matchWords[unit][byteIndex]
	if unit >= 8 {
		word >>= 16
	}
	return MatchByte{Value: uint8((word >> 8) & 0xff), Mask: uint8(word & 0xff)}, true // #nosec G115 -- masked to bytes.
}

func widthMask(size bus.Size) uint32 {
	switch size {
	case bus.Byte:
		return 0xff
	case bus.Half:
		return 0xffff
	case bus.Word:
		return ^uint32(0)
	}
	return 0
}

func laneShift(off uint32, size bus.Size) uint32 {
	switch size {
	case bus.Byte:
		return (3 - (off & 3)) * 8
	case bus.Half:
		return (2 - (off & 2)) * 8
	case bus.Word:
		return 0
	}
	return 0
}

func (d *Demux) complete(filter uint8) {
	d.status[2] |= 1 << filter
	d.updateLine()
}

// BindIRQ connects the demux completion line to dispatch row zero of the board controller.
func (d *Demux) BindIRQ(controller *irq.Controller) {
	d.interrupt = controller
	d.updateLine()
}

func (d *Demux) updateLine() {
	if d.interrupt == nil {
		return
	}
	var active uint32
	for i := range d.status {
		active |= d.status[i] & d.enable[i]
	}
	d.interrupt.SetLine(irq.DemuxMask, active != 0)
}

// Reset clears all device state after a block reset or machine reset.
func (d *Demux) Reset() {
	d.status, d.enable = [4]uint32{}, [4]uint32{}
	d.selectedFilter, d.selectedIndirect = 0, 0
	d.writePointer = [FilterCount]uint32{}
	d.pidChannels = [FilterCount]uint32{}
	d.pidWritten = [FilterCount]bool{}
	d.matchValue = 0
	d.matchUnit, d.matchIndex = 0, 0
	d.matchWords = [16][16]uint32{}
	d.control140, d.indirectData = 0, 0
	d.indirect = [FilterCount]uint32{}
	d.transportPart = [FilterCount][]byte{}
	d.transportPacket = nil
	d.updateLine()
}

// Snapshot captures every register bit that can affect subsequent firmware reads.
func (d *Demux) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(d.name, demuxSnapshotVersion)
	w.Words(d.status[:])
	w.Words(d.enable[:])
	w.Words([]uint32{d.selectedFilter, uint32(d.selectedIndirect)})
	w.Words(d.writePointer[:])
	w.Words(d.pidChannels[:])
	for _, written := range d.pidWritten {
		w.Bool(written)
	}
	w.Words([]uint32{d.matchValue, uint32(d.matchUnit), uint32(d.matchIndex)})
	for _, unit := range d.matchWords {
		w.Words(unit[:])
	}
	w.Words([]uint32{d.control140, d.indirectData})
	w.Words(d.indirect[:])
	for _, part := range d.transportPart {
		w.Bytes(part)
	}
	w.Bytes(d.transportPacket)
	return w.Blob()
}

// Restore replaces register state only after the full versioned blob has been validated.
func (d *Demux) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("demux: restore: %w", err)
	}
	if err := r.Expect(d.name, demuxSnapshotVersion, demuxSnapshotVersion); err != nil {
		return fmt.Errorf("demux: restore: %w", err)
	}
	status, enable := r.Words(), r.Words()
	selected := r.Words()
	pointers := r.Words()
	channels := r.Words()
	var written [FilterCount]bool
	for i := range written {
		written[i] = r.Bool()
	}
	matchState := r.Words()
	var matches [16][16]uint32
	for unit := range matches {
		entry := r.Words()
		if len(entry) != len(matches[unit]) {
			return fmt.Errorf("demux: restore: invalid match unit")
		}
		copy(matches[unit][:], entry)
	}
	transportState, indirect := r.Words(), r.Words()
	var parts [FilterCount][]byte
	for i := range parts {
		parts[i] = r.Bytes()
		if len(parts[i]) == 0 {
			parts[i] = nil
		}
	}
	packet := r.Bytes()
	if len(packet) == 0 {
		packet = nil
	}
	if err := r.Done(); err != nil {
		return fmt.Errorf("demux: restore: %w", err)
	}
	if len(status) != len(d.status) || len(enable) != len(d.enable) ||
		len(selected) != 2 || selected[0] >= FilterCount || selected[1] > 0x7f || selected[0] != selected[1]>>2 || len(pointers) != len(d.writePointer) ||
		len(channels) != len(d.pidChannels) || len(matchState) != 3 || matchState[1] >= 16 || matchState[2] >= 16 || len(transportState) != 2 || len(indirect) != FilterCount {
		return fmt.Errorf("demux: restore: incompatible register count")
	}
	for _, part := range parts {
		if len(part) > 4096 {
			return fmt.Errorf("demux: restore: oversized transport section")
		}
	}
	if len(packet) >= transportPacketSize || len(packet) > 0 && packet[0] != 0x47 {
		return fmt.Errorf("demux: restore: invalid transport packet")
	}
	for _, pointer := range pointers {
		if pointer > 0x1fffff {
			return fmt.Errorf("demux: restore: invalid write pointer")
		}
	}
	copy(d.status[:], status)
	copy(d.enable[:], enable)
	d.selectedFilter, d.selectedIndirect = selected[0], uint8(selected[1]) // #nosec G115 -- range checked above.
	copy(d.writePointer[:], pointers)
	copy(d.pidChannels[:], channels)
	d.pidWritten = written
	d.matchValue = matchState[0]
	d.matchUnit, d.matchIndex = uint8(matchState[1]), uint8(matchState[2]) // #nosec G115 -- range checked above.
	d.matchWords = matches
	d.control140, d.indirectData = transportState[0], transportState[1]
	copy(d.indirect[:], indirect)
	d.transportPart = parts
	d.transportPacket = packet
	d.updateLine()
	return nil
}
