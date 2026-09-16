// Package cpu executes the NEC VR4111's 32-bit instruction set against the board bus.
package cpu

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// Core owns the registers and current instruction address of one deterministic machine.
type Core struct {
	GPR  [32]uint32
	PC   uint32
	HI   uint32
	LO   uint32
	ISA  bool
	COP0 [32]uint32

	delayed        branch
	reserved       uint32
	hasReservation bool
	timerPending   bool

	bus *bus.Bus
}

type branch struct {
	target uint32
	isa    bool
	armed  bool
}

// New starts execution at pc on the supplied bus.
func New(b *bus.Bus, pc uint32) *Core { return &Core{PC: pc, bus: b} }

// HasPendingBranch reports the interval in which a checkpoint or interrupt would see an
// incomplete machine: the branch has retired, but its delay slot has not.
func (c *Core) HasPendingBranch() bool { return c.delayed.armed }

// TimerPending reports the Count/Compare request until software writes Compare.
func (c *Core) TimerPending() bool { return c.timerPending }

// Step retires one instruction or returns a visible halt error without advancing PC.
func (c *Core) Step() error {
	if c.ISA {
		return fmt.Errorf("cpu: MIPS16 instruction at %s before MIPS16 decoder is available", hexfmt.Addr(c.PC))
	}
	if c.PC&3 != 0 {
		return fmt.Errorf("cpu: unaligned MIPS32 PC %s", hexfmt.Addr(c.PC))
	}
	word := c.bus.Read(c.PC, bus.Word)
	c.COP0[9]++
	if c.COP0[11] != 0 && c.COP0[9] == c.COP0[11] {
		c.timerPending = true
	}
	effect, err := c.execute32(word)
	if err != nil {
		return fmt.Errorf("cpu: %s at %s: %w", hexfmt.Word(word), hexfmt.Addr(c.PC), err)
	}
	switch {
	case c.delayed.armed:
		if effect.armed {
			return fmt.Errorf("cpu: branch in delay slot at %s", hexfmt.Addr(c.PC))
		}
		c.PC, c.ISA = c.delayed.target, c.delayed.isa
		c.delayed = branch{}
	case effect.armed:
		c.delayed = effect
		c.PC += 4
	default:
		c.PC += 4
		if effect.target == skipSlot {
			c.PC += 4
		}
	}
	c.GPR[0] = 0
	return nil
}

const skipSlot = ^uint32(0)
