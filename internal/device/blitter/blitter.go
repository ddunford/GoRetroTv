// Package blitter models the graphics command block at the media ASIC's 0x6000 window.
package blitter

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the blitter's uncached MMIO address.
	Base = 0xB0006000
	// Size is the blitter MMIO window size.
	Size = 0x100
)

// Blitter accepts fifteen command words, executes them when channel 12 delivers
// a command, and stores its output in the board's existing DRAM.
type Blitter struct {
	words   [15]uint32
	control uint32
	ram     *memory.RAM
}

// New binds the command engine to board DRAM. RAM is a configuration pointer;
// its bytes are captured by the separate DRAM device snapshot.
func New(ram *memory.RAM) *Blitter { return &Blitter{ram: ram} }

// Name is the snapshot key.
func (*Blitter) Name() string { return "blitter" }

// Read returns command words and control. The busy bit is clear because a DMA
// submission executes synchronously in this instruction-clock model.
func (b *Blitter) Read(off uint32, size bus.Size) uint32 {
	if size != bus.Word {
		return 0
	}
	switch {
	case off < 0x3c && off%4 == 0:
		return b.words[off/4]
	case off == 0x3c:
		return b.control
	default:
		return 0
	}
}

// Write receives a command word or control register write.
func (b *Blitter) Write(off uint32, size bus.Size, value uint32) {
	if size != bus.Word {
		return
	}
	switch {
	case off < 0x3c && off%4 == 0:
		b.words[off/4] = value
	case off == 0x3c:
		b.control = value
	}
}

// Reset clears the command registers without altering board DRAM.
func (b *Blitter) Reset() { b.words, b.control = [15]uint32{}, 0 }

// Snapshot captures every command register and the control word.
func (b *Blitter) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(b.Name(), 1)
	w.Words(b.words[:])
	w.Uint32(b.control)
	return w.Blob()
}

// Restore validates a full command image before changing register state.
func (b *Blitter) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("blitter: restore: %w", err)
	}
	if err := r.Expect(b.Name(), 1, 1); err != nil {
		return fmt.Errorf("blitter: restore: %w", err)
	}
	words, control := r.Words(), r.Uint32()
	if err := r.Done(); err != nil {
		return fmt.Errorf("blitter: restore: %w", err)
	}
	if len(words) != len(b.words) {
		return fmt.Errorf("blitter: restore: invalid word count")
	}
	copy(b.words[:], words)
	b.control = control
	return nil
}
