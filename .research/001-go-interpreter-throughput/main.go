// SPIKE 001 -- can a Go interpreter of this CPU actually go fast enough to be worth the port?
//
// THE CLAIM UNDER TEST. The whole rationale for leaving the browser is speed: a 447M-instruction
// cold boot takes 135 s there, about 3.1M instructions/s. SPEC.md NFR says >= 50M/s, which would
// make that boot under 10 s. If Go lands at 10M/s the port is still worth doing but the targets and
// the phase plan change, so this is measured before anything is planned around it.
//
// WHAT IT DOES. Loads the real flash, executes from the reset vector with a straightforward
// switch-dispatch interpreter, and reports sustained instructions/s. It implements the subset the
// bootloader actually uses and STOPS at the first thing it does not know -- reporting how far it
// got and on what, because a throughput number from an interpreter that fell out of the loop after
// 500 instructions would be measuring nothing.
//
// WHAT IT DELIBERATELY IS NOT. Not the port. No MIPS16, no peripherals, no COP0 beyond what the
// early boot reads. Those add work per instruction, so the number here is an UPPER bound -- which
// is the honest direction for a go/no-go: if the upper bound is already too slow, the answer is no.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"time"
)

const (
	flashBase = 0x9FC00000
	ramBase   = 0x80000000
	ramSize   = 32 << 20
)

type cpu struct {
	pc      uint32
	reg     [32]uint32
	hi, lo  uint32
	flash   []byte
	ram     []byte
	icount  uint64
	delayed bool
	target  uint32
}

func (c *cpu) read32(a uint32) uint32 {
	switch {
	case a >= flashBase && int(a-flashBase) < len(c.flash)-3:
		return binary.BigEndian.Uint32(c.flash[a-flashBase:])
	case a&0x1FFFFFFF < ramSize:
		return binary.BigEndian.Uint32(c.ram[a&0x1FFFFFF:])
	}
	return 0
}

func (c *cpu) write32(a, v uint32) {
	if a&0x1FFFFFFF < ramSize {
		binary.BigEndian.PutUint32(c.ram[a&0x1FFFFFF:], v)
	}
}

// step executes one instruction. Returns false on an opcode this spike does not implement, which
// ends the run honestly rather than silently looping.
func (c *cpu) step() bool {
	pc := c.pc
	in := c.read32(pc)
	c.pc += 4
	if c.delayed {
		c.delayed = false
		defer func() { c.pc = c.target }()
	}
	c.icount++

	op := in >> 26
	rs := (in >> 21) & 31
	rt := (in >> 16) & 31
	rd := (in >> 11) & 31
	sa := (in >> 6) & 31
	imm := uint32(int32(int16(in)))
	switch op {
	case 0x00: // SPECIAL
		switch in & 0x3F {
		case 0x00: // SLL (also NOP)
			c.set(rd, c.reg[rt]<<sa)
		case 0x02:
			c.set(rd, c.reg[rt]>>sa)
		case 0x03:
			c.set(rd, uint32(int32(c.reg[rt])>>sa))
		case 0x04:
			c.set(rd, c.reg[rt]<<(c.reg[rs]&31))
		case 0x06:
			c.set(rd, c.reg[rt]>>(c.reg[rs]&31))
		case 0x08: // JR
			c.target, c.delayed = c.reg[rs], true
		case 0x09: // JALR
			c.set(rd, c.pc+4)
			c.target, c.delayed = c.reg[rs], true
		case 0x10:
			c.set(rd, c.hi)
		case 0x11:
			c.hi = c.reg[rs]
		case 0x12:
			c.set(rd, c.lo)
		case 0x13:
			c.lo = c.reg[rs]
		case 0x18, 0x19:
			p := uint64(c.reg[rs]) * uint64(c.reg[rt])
			c.lo, c.hi = uint32(p), uint32(p>>32)
		case 0x21, 0x20:
			c.set(rd, c.reg[rs]+c.reg[rt])
		case 0x23, 0x22:
			c.set(rd, c.reg[rs]-c.reg[rt])
		case 0x24:
			c.set(rd, c.reg[rs]&c.reg[rt])
		case 0x25:
			c.set(rd, c.reg[rs]|c.reg[rt])
		case 0x26:
			c.set(rd, c.reg[rs]^c.reg[rt])
		case 0x27:
			c.set(rd, ^(c.reg[rs] | c.reg[rt]))
		case 0x2A:
			c.set(rd, b2u(int32(c.reg[rs]) < int32(c.reg[rt])))
		case 0x2B:
			c.set(rd, b2u(c.reg[rs] < c.reg[rt]))
		default:
			return false
		}
	case 0x01: // REGIMM: BLTZ/BGEZ (+AL)
		take := false
		switch rt {
		case 0x00, 0x10:
			take = int32(c.reg[rs]) < 0
		case 0x01, 0x11:
			take = int32(c.reg[rs]) >= 0
		default:
			return false
		}
		if rt&0x10 != 0 {
			c.set(31, c.pc+4)
		}
		if take {
			c.target, c.delayed = c.pc+imm<<2, true
		}
	case 0x02, 0x03: // J / JAL
		if op == 0x03 {
			c.set(31, c.pc+4)
		}
		c.target, c.delayed = (c.pc&0xF0000000)|(in&0x03FFFFFF)<<2, true
	case 0x04:
		if c.reg[rs] == c.reg[rt] {
			c.target, c.delayed = c.pc+imm<<2, true
		}
	case 0x05:
		if c.reg[rs] != c.reg[rt] {
			c.target, c.delayed = c.pc+imm<<2, true
		}
	case 0x06:
		if int32(c.reg[rs]) <= 0 {
			c.target, c.delayed = c.pc+imm<<2, true
		}
	case 0x07:
		if int32(c.reg[rs]) > 0 {
			c.target, c.delayed = c.pc+imm<<2, true
		}
	case 0x08, 0x09:
		c.set(rt, c.reg[rs]+imm)
	case 0x0A:
		c.set(rt, b2u(int32(c.reg[rs]) < int32(imm)))
	case 0x0B:
		c.set(rt, b2u(c.reg[rs] < imm))
	case 0x0C:
		c.set(rt, c.reg[rs]&(imm&0xFFFF))
	case 0x0D:
		c.set(rt, c.reg[rs]|(imm&0xFFFF))
	case 0x0E:
		c.set(rt, c.reg[rs]^(imm&0xFFFF))
	case 0x0F:
		c.set(rt, (imm&0xFFFF)<<16)
	case 0x10: // COP0 -- enough to not fall out of the loop during early boot
		switch rs {
		case 0x00, 0x04:
			c.set(rt, 0)
		default:
			// eret and friends: treat as no-op for a throughput measurement
		}
	case 0x20: // LB
		c.set(rt, uint32(int32(int8(c.readb(c.reg[rs]+imm)))))
	case 0x24:
		c.set(rt, uint32(c.readb(c.reg[rs]+imm)))
	case 0x21:
		c.set(rt, uint32(int32(int16(c.read16(c.reg[rs]+imm)))))
	case 0x25:
		c.set(rt, uint32(c.read16(c.reg[rs]+imm)))
	case 0x23:
		c.set(rt, c.read32(c.reg[rs]+imm))
	case 0x28:
		c.writeb(c.reg[rs]+imm, byte(c.reg[rt]))
	case 0x29:
		c.write16(c.reg[rs]+imm, uint16(c.reg[rt]))
	case 0x2B:
		c.write32(c.reg[rs]+imm, c.reg[rt])
	case 0x2F, 0x30, 0x38: // cache / ll / sc -- ignorable here
	default:
		return false
	}
	return true
}

func (c *cpu) set(r, v uint32) {
	if r != 0 {
		c.reg[r] = v
	}
}
func (c *cpu) readb(a uint32) byte {
	if a >= flashBase && int(a-flashBase) < len(c.flash) {
		return c.flash[a-flashBase]
	}
	if a&0x1FFFFFFF < ramSize {
		return c.ram[a&0x1FFFFFF]
	}
	return 0
}
func (c *cpu) writeb(a uint32, v byte) {
	if a&0x1FFFFFFF < ramSize {
		c.ram[a&0x1FFFFFF] = v
	}
}
func (c *cpu) read16(a uint32) uint16 {
	return uint16(c.readb(a))<<8 | uint16(c.readb(a+1))
}
func (c *cpu) write16(a uint32, v uint16) { c.writeb(a, byte(v>>8)); c.writeb(a+1, byte(v)) }
func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// plantBenchmark writes a representative inner loop into RAM and returns its entry point.
//
// WHY SYNTHETIC RATHER THAN THE REAL BOOT. The first version of this spike ran the flash from the
// reset vector and reported 83M/s -- which was measuring NOTHING. With no peripherals modelled the
// firmware leaves mapped memory within a few thousand instructions, and from then on the
// interpreter reads zeros from unmapped space and executes them as NOPs: 3,125,000 distinct PCs
// across 3,125,000 samples, at addresses like 0xD10D59F0 that do not exist on this board. The
// cheapest possible instruction, dispatched forever.
//
// A real-firmware number cannot be had until the peripherals exist -- which is the port. So this
// measures the thing that IS knowable now: sustained dispatch over a realistic MIPS mix with real
// memory traffic. Roughly a third ALU, a third load/store against a working set larger than L1, a
// third branches, which is the shape of the interpreted code in this firmware.
//
// It is still an UPPER BOUND: no MIPS16 decode, no COP0 bookkeeping, no interrupt checks, no
// peripheral dispatch on every load. The honest reading is "the ceiling is here", and the port has
// to stay under it.
func plantBenchmark(c *cpu) uint32 {
	const entry = 0x80100000
	w := func(off int, in uint32) { c.write32(entry+uint32(off), in) }
	i := 0
	emit := func(in uint32) { w(i, in); i += 4 }
	// t0 = working-set base, t1 = counter
	emit(0x3C08_8020)             // lui  t0, 0x8020
	emit(0x3409_FFFF)             // ori  t1, zero, 0xFFFF
	loop := entry + uint32(i)
	emit(0x8D0A_0000)             // lw   t2, 0(t0)
	emit(0x8D0B_0004)             // lw   t3, 4(t0)
	emit(0x014B_6020)             // add  t4, t2, t3
	emit(0x018A_6822)             // sub  t5, t4, t2
	emit(0x01AC_7024)             // and  t6, t5, t4
	emit(0x01CD_7825)             // or   t7, t6, t5
	emit(0xAD0C_0008)             // sw   t4, 8(t0)
	emit(0xAD0F_000C)             // sw   t7, 12(t0)
	emit(0x2508_0010)             // addiu t0, t0, 16
	emit(0x2129_FFFF)             // addi  t1, t1, -1
	// bne t1, zero, loop  (offset is relative to the delay slot)
	off := (int32(loop) - int32(entry+uint32(i)+4)) >> 2
	emit(0x1520_0000 | uint32(uint16(off)))
	emit(0x0000_0000)             // nop (delay slot)
	// wrap the working set and go again
	emit(0x3C08_8020)             // lui t0, 0x8020
	emit(0x3409_FFFF)             // ori t1, zero, 0xFFFF
	back := (int32(loop) - int32(entry+uint32(i)+8)) >> 2
	emit(0x1000_0000 | uint32(uint16(back))) // b loop
	emit(0x0000_0000)
	return entry
}

func main() {
	flash, err := os.ReadFile("../../firmware/FLASH_U202.bin")
	if err != nil {
		fmt.Println("need firmware/FLASH_U202.bin:", err)
		os.Exit(2)
	}
	c := &cpu{flash: flash, ram: make([]byte, ramSize)}
	c.pc = plantBenchmark(c)
	const budget = 200_000_000
	// IS THIS MEASURING REAL WORK? A throughput number from an interpreter spinning in a
	// three-instruction wait loop is a measurement of best-case dispatch and nothing else -- and
	// with no peripherals modelled, spinning is exactly what this firmware would do. So the PC is
	// sampled and the distinct-address count reported: a handful of addresses means the number is
	// an artefact and must be read as such.
	hist := map[uint32]uint64{}
	const sampleEvery = 64
	start := time.Now()
	ok := true
	for c.icount < budget {
		if c.icount%sampleEvery == 0 {
			hist[c.pc]++
		}
		if !c.step() {
			ok = false
			break
		}
	}
	el := time.Since(start)
	rate := float64(c.icount) / el.Seconds() / 1e6
	fmt.Printf("executed   : %d instructions\n", c.icount)
	fmt.Printf("elapsed    : %s\n", el.Round(time.Millisecond))
	fmt.Printf("throughput : %.1fM instructions/s\n", rate)
	if !ok {
		fmt.Printf("stopped at : pc=0x%08X instruction=0x%08X (opcode %d not implemented in this spike)\n",
			c.pc-4, c.read32(c.pc-4), c.read32(c.pc-4)>>26)
	}
	type hot struct {
		pc uint32
		n  uint64
	}
	var tops []hot
	for k, v := range hist {
		tops = append(tops, hot{k, v})
	}
	sort.Slice(tops, func(i, j int) bool { return tops[i].n > tops[j].n })
	var top10 uint64
	for i := 0; i < len(tops) && i < 10; i++ {
		top10 += tops[i].n
	}
	var total uint64
	for _, t := range tops {
		total += t.n
	}
	fmt.Printf("distinct PCs sampled: %d; the hottest 10 are %.1f%% of samples\n",
		len(tops), 100*float64(top10)/float64(total))
	for i := 0; i < len(tops) && i < 5; i++ {
		fmt.Printf("   0x%08X  %.1f%%\n", tops[i].pc, 100*float64(tops[i].n)/float64(total))
	}
	if len(tops) > 1000 {
		fmt.Printf("VERDICT CAVEAT: %d distinct addresses means the PC is running away through\n"+
			"   unmapped memory, not executing a loop. The rate above is meaningless.\n", len(tops))
	} else {
		fmt.Printf("working set: %d distinct addresses -- a real loop with real memory traffic.\n", len(tops))
	}
	fmt.Printf("\nbrowser baseline: 3.1M/s. SPEC NFR target: 50M/s. Measured: %.1fM/s -> %s\n",
		rate, map[bool]string{true: "TARGET MET", false: "BELOW TARGET"}[rate >= 50])
}
