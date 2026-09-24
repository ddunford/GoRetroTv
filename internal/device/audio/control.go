// Package audio models the byte-wide control block driven by the firmware's sound settings.
package audio

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the uncached address written by the firmware audio driver.
	Base = 0xB4080100
	// Size covers the measured byte registers at offsets 0, 1, 2, 3 and 6.
	Size = 0x10
)

// Control holds the last byte written to each register. The measured status read at +6 remains
// zero: making a peripheral read back writes without evidence has changed guest control flow in
// this firmware before.
type Control struct {
	registers [Size]byte
}

// New returns the power-on register state.
func New() *Control { return &Control{} }

// Name is the snapshot identity.
func (*Control) Name() string { return "audio-control" }

// Read returns the measured zero status. No other read value has been observed.
func (*Control) Read(_ uint32, _ bus.Size) uint32 { return 0 }

// Write records measured byte writes. Other widths have not been observed and remain inert.
func (c *Control) Write(off uint32, size bus.Size, value uint32) {
	if off < Size && size == bus.Byte {
		c.registers[off] = byte(value & 0xff)
	}
}

// Register returns the last byte written at off for audio-path integration and diagnostics.
func (c *Control) Register(off uint32) byte {
	if off >= Size {
		return 0
	}
	return c.registers[off]
}

// Reset restores the measured zero power-on state.
func (c *Control) Reset() { c.registers = [Size]byte{} }

// Snapshot captures every byte in the control window.
func (c *Control) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(c.Name(), 1)
	w.Bytes(c.registers[:])
	return w.Blob()
}

// Restore validates the complete blob before replacing device state.
func (c *Control) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("audio control: restore: %w", err)
	}
	if err := r.Expect(c.Name(), 1, 1); err != nil {
		return fmt.Errorf("audio control: restore: %w", err)
	}
	registers := r.Bytes()
	if err := r.Done(); err != nil {
		return fmt.Errorf("audio control: restore: %w", err)
	}
	if len(registers) != Size {
		return fmt.Errorf("audio control: restore: got %d register bytes, want %d", len(registers), Size)
	}
	copy(c.registers[:], registers)
	return nil
}
