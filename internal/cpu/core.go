// Package cpu executes the NEC VR4111's 32-bit instruction set against the board bus.
package cpu

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// Core owns the registers and current instruction address of one deterministic machine.
type Core struct {
	GPR [32]uint32
	PC  uint32
	HI  uint32
	LO  uint32

	bus *bus.Bus
}

// New starts execution at pc on the supplied bus.
func New(b *bus.Bus, pc uint32) *Core { return &Core{PC: pc, bus: b} }

// Step retires one instruction or returns a visible halt error without advancing PC.
func (c *Core) Step() error {
	word := c.bus.Read(c.PC, bus.Word)
	if word != 0 {
		return fmt.Errorf("cpu: unsupported instruction %s at %s", hexfmt.Word(word), hexfmt.Addr(c.PC))
	}
	c.PC += 4
	return nil
}
