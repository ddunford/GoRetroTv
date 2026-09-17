// Package irq models the board interrupt pending and enable registers.
package irq

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the board interrupt controller's MMIO window.
	Base = 0xB0000000
	// Size is the board interrupt controller's MMIO window length.
	Size = 0x100
	// DemuxMask selects dispatch row zero's demux handler.
	DemuxMask = 0x01000000
)

// Controller multiplexes peripheral pending bits onto CPU IP2.
type Controller struct {
	pending uint32
	enable  uint32
	raise   func(uint8)
}

// New creates the controller; raise is the CPU's Interrupt method.
func New(raise func(uint8)) *Controller { return &Controller{raise: raise} }

// Name is the snapshot key.
func (*Controller) Name() string { return "board-irq" }

// Read exposes only the measured pending and enable registers.
func (c *Controller) Read(off uint32, size bus.Size) uint32 {
	if size != bus.Word {
		return 0
	}
	switch off {
	case 0x30:
		return c.pending
	case 0x40:
		return c.enable
	default:
		return 0
	}
}

// Write stores the firmware's enable bitmap. Pending bits are device-owned.
func (c *Controller) Write(off uint32, size bus.Size, value uint32) {
	if size == bus.Word && off == 0x40 {
		before := c.pending & c.enable
		c.enable = value
		if c.pending&c.enable&^before != 0 {
			c.signal()
		}
	}
}

// SetLine sets or clears one device's pending bit and signals an enabled rising edge.
func (c *Controller) SetLine(mask uint32, asserted bool) {
	before := c.pending & c.enable
	if asserted {
		c.pending |= mask
	} else {
		c.pending &^= mask
	}
	if c.pending&c.enable&^before != 0 {
		c.signal()
	}
}

func (c *Controller) signal() {
	if c.raise != nil {
		c.raise(2)
	}
}

// Reset clears all interrupt controller state.
func (c *Controller) Reset() { c.pending, c.enable = 0, 0 }

// Snapshot captures pending and enable bits.
func (c *Controller) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(c.Name(), 1)
	w.Words([]uint32{c.pending, c.enable})
	return w.Blob()
}

// Restore validates the full blob before changing state.
func (c *Controller) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("irq: restore: %w", err)
	}
	if err := r.Expect(c.Name(), 1, 1); err != nil {
		return fmt.Errorf("irq: restore: %w", err)
	}
	words := r.Words()
	if err := r.Done(); err != nil {
		return fmt.Errorf("irq: restore: %w", err)
	}
	if len(words) != 2 {
		return fmt.Errorf("irq: restore: invalid register count")
	}
	c.pending, c.enable = words[0], words[1]
	return nil
}
