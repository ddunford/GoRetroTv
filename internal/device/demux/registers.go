package demux

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// MMIOBase is the EMMA demux register window on the board's uncached bus.
	MMIOBase = 0xB000A000
	// MMIOSize is the demux register window attached to the physical bus.
	MMIOSize             = 0x1000
	demuxSnapshotVersion = 1
)

// Demux owns the status and enable state of four EMMA interrupt groups.
// Other registers answer zero until a measured read contract gives them meaning.
type Demux struct {
	name   string
	status [4]uint32
	enable [4]uint32
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
	case reg >= 0xD0 && reg <= 0xDC:
		i := (reg - 0xD0) / 4
		d.enable[i] |= bits
	case reg == 0 && value&1 != 0:
		d.Reset()
	}
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

func (d *Demux) complete(filter uint8) { d.status[2] |= 1 << filter }

// Reset clears all device state after a block reset or machine reset.
func (d *Demux) Reset() { d.status, d.enable = [4]uint32{}, [4]uint32{} }

// Snapshot captures every register bit that can affect subsequent firmware reads.
func (d *Demux) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(d.name, demuxSnapshotVersion)
	w.Words(d.status[:])
	w.Words(d.enable[:])
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
	if err := r.Done(); err != nil {
		return fmt.Errorf("demux: restore: %w", err)
	}
	if len(status) != len(d.status) || len(enable) != len(d.enable) {
		return fmt.Errorf("demux: restore: incompatible register count")
	}
	copy(d.status[:], status)
	copy(d.enable[:], enable)
	return nil
}
