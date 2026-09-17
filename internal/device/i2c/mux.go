// Package i2c models the board's byte-at-a-time I²C master and channel latch.
package i2c

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the I²C controller's uncached MMIO address.
	Base = 0xB2006000
	// Size is the controller's register window.
	Size = 0x100
	// MuxBase selects one of the four physical I²C branches.
	MuxBase = 0xB4180000
	// MuxSize is the channel latch's mapped window.
	MuxSize = 4
	// IRQMask selects the I²C ISR in the board interrupt controller.
	IRQMask = 0x00200000
)

// Mux is the four-channel I²C branch latch at B4180000.
type Mux struct{ value uint32 }

// NewMux returns the disconnected power-on latch.
func NewMux() *Mux { return &Mux{value: 8} }

// Name is the snapshot identity.
func (*Mux) Name() string { return "i2c-mux" }

// Read returns the last latch value.
func (m *Mux) Read(off uint32, size bus.Size) uint32 {
	if off == 0 && size == bus.Word {
		return m.value
	}
	return 0
}

// Write selects a branch or disables the bus with bit 3.
func (m *Mux) Write(off uint32, size bus.Size, value uint32) {
	if off == 0 && size == bus.Word {
		m.value = value
	}
}

// Channel reports the active branch, or false when bit 3 disconnects it.
func (m *Mux) Channel() (uint8, bool) {
	// #nosec G115 -- the mask restricts the result to two bits.
	return uint8((m.value >> 4) & 3), m.value&8 == 0
}

// Reset disconnects the four branches.
func (m *Mux) Reset() { m.value = 8 }

// Snapshot captures the branch latch.
func (m *Mux) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(m.Name(), 1)
	w.Uint32(m.value)
	return w.Blob()
}

// Restore replaces the latch after validating its snapshot.
func (m *Mux) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("i2c mux: restore: %w", err)
	}
	if err := r.Expect(m.Name(), 1, 1); err != nil {
		return fmt.Errorf("i2c mux: restore: %w", err)
	}
	value := r.Uint32()
	if err := r.Done(); err != nil {
		return fmt.Errorf("i2c mux: restore: %w", err)
	}
	m.value = value
	return nil
}
