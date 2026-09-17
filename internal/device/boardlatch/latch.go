// Package boardlatch models the measured readback word at B200A000.
package boardlatch

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the measured board latch address in the guest's uncached bus window.
	Base = 0xB200A000
	// Size is the one word whose readback behavior has been measured.
	Size = 4
)

// Latch stores the board word used by the firmware's set/clear read-modify-write paths.
// Its wider hardware identity is not established by the reference measurements.
type Latch struct{ value uint32 }

var _ bus.Device = (*Latch)(nil)

// New returns a reset board latch.
func New() *Latch { return &Latch{} }

// Name identifies this device in snapshots and bus diagnostics.
func (*Latch) Name() string { return "board-latch" }

func (l *Latch) Read(off uint32, size bus.Size) uint32 {
	if off == 0 && size == bus.Word {
		return l.value
	}
	return 0
}

func (l *Latch) Write(off uint32, size bus.Size, value uint32) {
	if off == 0 && size == bus.Word {
		l.value = value
	}
}

// Reset clears the stored board word.
func (l *Latch) Reset() { l.value = 0 }

// Snapshot saves the complete measured register state.
func (l *Latch) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(l.Name(), 1)
	w.Uint32(l.value)
	return w.Blob()
}

// Restore replaces the register state after validating the snapshot.
func (l *Latch) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("board latch: restore: %w", err)
	}
	if err := r.Expect(l.Name(), 1, 1); err != nil {
		return fmt.Errorf("board latch: restore: %w", err)
	}
	value := r.Uint32()
	if err := r.Done(); err != nil {
		return fmt.Errorf("board latch: restore: %w", err)
	}
	l.value = value
	return nil
}
