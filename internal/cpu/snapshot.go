package cpu

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const snapshotVersion = 1

// Snapshot encodes all CPU state, including a pending branch and interrupt requests.
func (c *Core) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter("cpu", snapshotVersion)
	w.Words(c.GPR[:])
	w.Uint32(c.PC)
	w.Uint32(c.HI)
	w.Uint32(c.LO)
	w.Bool(c.ISA)
	w.Words(c.COP0[:])
	w.Uint32(c.delayed.target)
	w.Uint32(c.delayed.from)
	w.Bool(c.delayed.isa)
	w.Bool(c.delayed.armed)
	w.Bool(c.delayed.immediate)
	w.Uint32(c.reserved)
	w.Bool(c.hasReservation)
	w.Bool(c.timerPending)
	w.Uint8(c.pendingLines)
	return w.Blob()
}

// Restore validates the complete blob before replacing state; the attached bus is retained.
func (c *Core) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("cpu: restore: %w", err)
	}
	if err := r.Expect("cpu", snapshotVersion, snapshotVersion); err != nil {
		return fmt.Errorf("cpu: restore: %w", err)
	}
	gpr := r.Words()
	pc, hi, lo, isa := r.Uint32(), r.Uint32(), r.Uint32(), r.Bool()
	cop0 := r.Words()
	delayed := branch{target: r.Uint32(), from: r.Uint32(), isa: r.Bool(), armed: r.Bool(), immediate: r.Bool()}
	reserved, hasReservation := r.Uint32(), r.Bool()
	timerPending, pendingLines := r.Bool(), r.Uint8()
	if err := r.Done(); err != nil {
		return fmt.Errorf("cpu: restore: %w", err)
	}
	if len(gpr) != 32 || len(cop0) != 32 || gpr[0] != 0 || pc&1 != 0 || (!isa && pc&3 != 0) ||
		(delayed.armed && (delayed.immediate || delayed.target&1 != 0 || (!delayed.isa && delayed.target&3 != 0))) ||
		(delayed.armed && (delayed.from&1 != 0 || (!isa && delayed.from&3 != 0))) ||
		(!delayed.armed && (delayed.immediate || delayed.target != 0 || delayed.from != 0)) {
		return fmt.Errorf("cpu: restore: incompatible register state")
	}
	copy(c.GPR[:], gpr)
	c.PC, c.HI, c.LO, c.ISA = pc, hi, lo, isa
	copy(c.COP0[:], cop0)
	c.delayed = delayed
	c.reserved = reserved
	c.hasReservation = hasReservation
	c.timerPending, c.pendingLines = timerPending, pendingLines
	return nil
}
