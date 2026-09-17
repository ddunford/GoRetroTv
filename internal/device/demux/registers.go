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
	demuxSnapshotVersion = 3
)

// Demux owns the status and enable state of four EMMA interrupt groups.
// Other registers answer zero until a measured read contract gives them meaning.
type Demux struct {
	name           string
	status         [4]uint32
	enable         [4]uint32
	selectedFilter uint32
	writePointer   [FilterCount]uint32
	pidChannels    [FilterCount]uint32
	pidWritten     [FilterCount]bool
	matchValue     uint32
	matchUnits     [16][16]MatchByte
	ram            *memory.RAM
	interrupt      *irq.Controller
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
		d.selectedFilter = (value >> 2) & (FilterCount - 1)
	case reg >= 0x14 && reg < 0x14+4*FilterCount:
		channel := (reg - 0x14) / 4
		d.pidChannels[channel] = value
		d.pidWritten[channel] = true
	case reg == 0x148:
		d.matchValue = value
	case reg == 0x144 && value&0xff00 == 0xc000:
		unit, byteIndex := value&0xf, (value>>4)&0xf
		d.matchUnits[unit][byteIndex] = MatchByte{Value: uint8((d.matchValue >> 8) & 0xff), Mask: uint8(d.matchValue & 0xff)} // #nosec G115 -- both fields are masked to eight bits
	}
}

// ArmedPIDs returns the PIDs in enabled, programmed channels. The channel index is a
// section filter index; the independent 16 match units do not select a PID channel.
func (d *Demux) ArmedPIDs() []uint16 {
	var pids []uint16
	for ch := range d.pidChannels {
		if !d.pidWritten[ch] || d.enable[2]&(1<<ch) == 0 {
			continue
		}
		pid := d.pidChannels[ch] & 0x1fff
		if pid != 0x1fff {
			pids = append(pids, uint16(pid)) // #nosec G115 -- a DVB PID is masked to 13 bits above
		}
	}
	return pids
}

// Match returns one independent unit's byte rule; it does not imply a PID channel.
func (d *Demux) Match(unit, byteIndex uint8) (MatchByte, bool) {
	if unit >= 16 || byteIndex >= 16 {
		return MatchByte{}, false
	}
	return d.matchUnits[unit][byteIndex], true
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
	d.selectedFilter = 0
	d.writePointer = [FilterCount]uint32{}
	d.pidChannels = [FilterCount]uint32{}
	d.pidWritten = [FilterCount]bool{}
	d.matchValue = 0
	d.matchUnits = [16][16]MatchByte{}
	d.updateLine()
}

// Snapshot captures every register bit that can affect subsequent firmware reads.
func (d *Demux) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(d.name, demuxSnapshotVersion)
	w.Words(d.status[:])
	w.Words(d.enable[:])
	w.Words([]uint32{d.selectedFilter})
	w.Words(d.writePointer[:])
	w.Words(d.pidChannels[:])
	for _, written := range d.pidWritten {
		w.Bool(written)
	}
	w.Words([]uint32{d.matchValue})
	for _, unit := range d.matchUnits {
		for _, entry := range unit {
			w.Bytes([]byte{entry.Value, entry.Mask})
		}
	}
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
	matchValue := r.Words()
	var matches [16][16]MatchByte
	for unit := range matches {
		for i := range matches[unit] {
			entry := r.Bytes()
			if len(entry) != 2 {
				return fmt.Errorf("demux: restore: invalid match byte")
			}
			matches[unit][i] = MatchByte{Value: entry[0], Mask: entry[1]}
		}
	}
	if err := r.Done(); err != nil {
		return fmt.Errorf("demux: restore: %w", err)
	}
	if len(status) != len(d.status) || len(enable) != len(d.enable) ||
		len(selected) != 1 || selected[0] >= FilterCount || len(pointers) != len(d.writePointer) ||
		len(channels) != len(d.pidChannels) || len(matchValue) != 1 {
		return fmt.Errorf("demux: restore: incompatible register count")
	}
	for _, pointer := range pointers {
		if pointer > 0x1fffff {
			return fmt.Errorf("demux: restore: invalid write pointer")
		}
	}
	copy(d.status[:], status)
	copy(d.enable[:], enable)
	d.selectedFilter = selected[0]
	copy(d.writePointer[:], pointers)
	copy(d.pidChannels[:], channels)
	d.pidWritten = written
	d.matchValue = matchValue[0]
	d.matchUnits = matches
	d.updateLine()
	return nil
}
