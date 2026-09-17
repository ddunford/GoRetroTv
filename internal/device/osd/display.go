package osd

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// Display stores the two measured OSD display-list roots.
type Display struct {
	roots [2]uint32
	ram   *memory.RAM
}

// NewDisplay returns an unprogrammed OSD display-list controller.
func NewDisplay() *Display { return &Display{} }

// BindRAM connects display-list and CLUT reads to the board's existing DRAM.
func (d *Display) BindRAM(ram *memory.RAM) error {
	if ram == nil || ram.Size() < memory.DRAMSize {
		return fmt.Errorf("osd: display needs full board DRAM")
	}
	d.ram = ram
	return nil
}

// Name is the snapshot key.
func (*Display) Name() string { return "osd-display" }

// Read returns the roots for the firmware's read-modify-write sequence.
func (d *Display) Read(off uint32, size bus.Size) uint32 {
	if size == bus.Word && (off == 0x200 || off == 0x204) {
		return d.roots[(off-0x200)/4]
	}
	return 0
}

// Write sets a display-list root.
func (d *Display) Write(off uint32, size bus.Size, value uint32) {
	if size == bus.Word && (off == 0x200 || off == 0x204) {
		d.roots[(off-0x200)/4] = value
	}
}

// Reset removes both display-list roots, leaving no programmed plane.
func (d *Display) Reset() { d.roots = [2]uint32{} }

// Snapshot captures both roots.
func (d *Display) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(d.Name(), 1)
	w.Words(d.roots[:])
	return w.Blob()
}

// Restore validates the complete display register blob before changing roots.
func (d *Display) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("osd: display restore: %w", err)
	}
	if err := r.Expect(d.Name(), 1, 1); err != nil {
		return fmt.Errorf("osd: display restore: %w", err)
	}
	roots := r.Words()
	if err := r.Done(); err != nil {
		return fmt.Errorf("osd: display restore: %w", err)
	}
	if len(roots) != len(d.roots) {
		return fmt.Errorf("osd: display restore: invalid root count")
	}
	copy(d.roots[:], roots)
	return nil
}
