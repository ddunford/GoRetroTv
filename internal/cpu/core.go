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
	pendingLines   uint8

	bus *bus.Bus
}

type branch struct {
	target    uint32
	isa       bool
	armed     bool
	immediate bool
}

// New starts execution at pc on the supplied bus.
func New(b *bus.Bus, pc uint32) *Core { return &Core{PC: pc, bus: b} }

// HasPendingBranch reports the interval in which a checkpoint or interrupt would see an
// incomplete machine: the branch has retired, but its delay slot has not.
func (c *Core) HasPendingBranch() bool { return c.delayed.armed }

// TimerPending reports the Count/Compare request until software writes Compare.
func (c *Core) TimerPending() bool { return c.timerPending }

// Interrupt raises one external IP line. A device clears its physical line by acknowledging it;
// this core consumes the requested edge only when Status allows delivery.
func (c *Core) Interrupt(ip uint8) {
	if ip > 7 {
		panic("cpu: interrupt line must be in 0..7")
	}
	c.pendingLines |= 1 << ip
}

// Step retires one instruction or returns a visible halt error without advancing PC.
func (c *Core) Step() error {
	c.COP0[9]++
	if c.COP0[11] != 0 && c.COP0[9] == c.COP0[11] {
		c.timerPending = true
	}
	if !c.delayed.armed {
		c.serviceInterrupt()
	}
	if c.ISA {
		return fmt.Errorf("cpu: MIPS16 instruction at %s before MIPS16 decoder is available", hexfmt.Addr(c.PC))
	}
	if c.PC&3 != 0 {
		return fmt.Errorf("cpu: unaligned MIPS32 PC %s", hexfmt.Addr(c.PC))
	}
	word := c.bus.Read(c.PC, bus.Word)
	effect, err := c.execute32(word)
	if err != nil {
		return fmt.Errorf("cpu: %s at %s: %w", hexfmt.Word(word), hexfmt.Addr(c.PC), err)
	}
	switch {
	case c.delayed.armed:
		if effect.armed || effect.immediate {
			return fmt.Errorf("cpu: branch in delay slot at %s", hexfmt.Addr(c.PC))
		}
		c.PC, c.ISA = c.delayed.target, c.delayed.isa
		c.delayed = branch{}
	case effect.armed:
		c.delayed = effect
		c.PC += 4
	case effect.immediate:
		c.PC, c.ISA = effect.target, effect.isa
	default:
		c.PC += 4
		if effect.target == skipSlot {
			c.PC += 4
		}
	}
	c.GPR[0] = 0
	return nil
}

func (c *Core) serviceInterrupt() bool {
	ip := uint32(c.pendingLines) << 8
	if c.timerPending {
		ip |= 1 << 15
	}
	if ip == 0 {
		return false
	}
	status := c.COP0[12]
	if status&1 == 0 || status&6 != 0 || status&ip&0xff00 == 0 {
		return false
	}
	c.COP0[13] = c.COP0[13]&^uint32(0xff7c) | ip
	c.COP0[14] = c.PC
	if c.ISA {
		c.COP0[14] |= 1
	}
	c.COP0[12] = status | 2
	if status&(1<<22) != 0 {
		c.PC = 0xBFC00380
	} else {
		c.PC = 0x80000180
	}
	c.ISA = false
	c.pendingLines = 0
	c.hasReservation = false
	return true
}

const skipSlot = ^uint32(0)
