package cpu

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
)

var m16Register = [8]uint32{16, 17, 2, 3, 4, 5, 6, 7}

func sx16(v uint32, bits uint) uint32 { return bits32(signed32(v<<(32-bits)) >> (32 - bits)) }

// execute16 implements the original MIPS16 instruction subset used by this firmware.
// MIPS16e SAVE/RESTORE and the 64-bit beta encodings are reserved on the VR4111.
func (c *Core) execute16() (branch, uint32, error) {
	pc := c.PC
	h := c.bus.Fetch(pc, bus.Half) & 0xffff
	length := uint32(2)
	ext := uint32(0)
	extended := h>>11 == 0x1e
	if extended {
		ext = h & 0x7ff
		h = c.bus.Fetch(pc+2, bus.Half) & 0xffff
		length = 4
	}
	op, rx, ry := h>>11, (h>>8)&7, (h>>5)&7
	x, y := m16Register[rx], m16Register[ry]
	next := pc + length
	imm16 := (ext&31)<<11 | ((ext>>5)&63)<<5 | h&31
	imm15 := (ext&15)<<11 | ((ext>>4)&127)<<4 | h&15
	imm8 := h & 255
	if extended {
		imm8 = imm16
	}
	pcBase := pc
	if c.delayed.armed {
		pcBase = c.delayed.from
	}
	bad := func(reason string) (branch, uint32, error) {
		return branch{}, length, fmt.Errorf("%s (0x%04X)", reason, h)
	}
	noext := func() bool { return !extended }
	constfield := func() bool { return !extended || h&0xe0 == 0 }
	instant := func(target uint32) branch { return branch{target: target, isa: true, immediate: true} }
	jump := func(target uint32, isa bool) branch { return branch{target: target, isa: isa, armed: true} }
	memoff := func(scale uint) uint32 {
		if extended {
			return sx16(imm16, 16)
		}
		return (h & 31) << scale
	}
	switch op {
	case 0, 1: // ADDIUSP, ADDIUPC
		if !constfield() {
			return bad("invalid extended constant field")
		}
		imm := imm8 << 2
		if extended {
			imm = sx16(imm16, 16)
		}
		base := c.GPR[29]
		if op == 1 {
			base = pcBase &^ 3
		}
		c.set(x, base+imm)
	case 2: // B, immediate transfer
		if extended && h&0x7e0 != 0 {
			return bad("invalid extended B")
		}
		off := sx16(h&0x7ff, 11) << 1
		if extended {
			off = sx16(imm16, 16) << 1
		}
		return instant(next + off), length, nil
	case 3: // JAL and JALX, second halfword follows the first
		if !noext() {
			return bad("extended JAL")
		}
		second := c.bus.Read(pc+2, bus.Half) & 0xffff
		idx := (h&31)<<21 | ((h>>5)&31)<<16 | second
		target := ((pc + 4) & 0xf0000000) | (idx << 2)
		c.set(31, (pc+6)|1)
		return jump(target, h&(1<<10) == 0), 4, nil
	case 4, 5: // BEQZ, BNEZ; no delay slot
		if !constfield() {
			return bad("invalid extended branch")
		}
		off := sx16(h&255, 8) << 1
		if extended {
			off = sx16(imm16, 16) << 1
		}
		if (c.GPR[x] == 0) == (op == 4) {
			return instant(next + off), length, nil
		}
	case 6: // immediate shifts
		fn := h & 3
		if fn == 1 {
			return bad("64-bit shift")
		}
		sa := (h >> 2) & 7
		if sa == 0 {
			sa = 8
		}
		if extended {
			if h&0x1c != 0 || ext&63 != 0 {
				return bad("invalid extended shift")
			}
			sa = (ext >> 6) & 31
		}
		v := c.GPR[y]
		switch fn {
		case 0:
			v <<= sa
		case 2:
			v >>= sa
		case 3:
			v = bits32(signed32(v) >> sa)
		}
		c.set(x, v)
	case 8: // RRI-A
		if h&16 != 0 {
			return bad("64-bit addiu")
		}
		imm := sx16(h&15, 4)
		if extended {
			imm = sx16(imm15, 15)
		}
		c.set(y, c.GPR[x]+imm)
	case 9: // ADDIU8
		if !constfield() {
			return bad("invalid extended addiu")
		}
		imm := sx16(h&255, 8)
		if extended {
			imm = sx16(imm16, 16)
		}
		c.set(x, c.GPR[x]+imm)
	case 10, 11: // SLTI, SLTIU write implicit T
		if !constfield() {
			return bad("invalid extended slti")
		}
		imm := h & 255
		if extended {
			imm = sx16(imm16, 16)
		}
		if op == 10 {
			c.GPR[24] = boolWord(signed32(c.GPR[x]) < signed32(imm))
		} else {
			c.GPR[24] = boolWord(c.GPR[x] < imm)
		}
	case 12: // I8
		fn := (h >> 8) & 7
		switch fn {
		case 0, 1: // T branches
			if !constfield() {
				return bad("invalid extended T branch")
			}
			off := sx16(h&255, 8) << 1
			if extended {
				off = sx16(imm16, 16) << 1
			}
			if (c.GPR[24] == 0) == (fn == 0) {
				return instant(next + off), length, nil
			}
		case 2, 3: // SWRASP, ADJSP
			if !constfield() {
				return bad("invalid extended I8")
			}
			if fn == 2 {
				off := imm8 << 2
				if extended {
					off = sx16(imm16, 16)
				}
				c.bus.Write(c.GPR[29]+off, bus.Word, c.GPR[31])
			} else {
				imm := sx16(h&255, 8) << 3
				if extended {
					imm = sx16(imm16, 16)
				}
				c.GPR[29] += imm
			}
		case 4:
			return bad("SAVE/RESTORE is not in original MIPS16")
		case 5: // MOV32R
			if extended {
				return bad("extended MOV32R")
			}
			fld := (h >> 3) & 31
			reg := (fld&3)<<3 | ((fld >> 2) & 7)
			c.set(reg, c.GPR[m16Register[h&7]])
		case 7: // MOVR32
			if extended {
				return bad("extended MOVR32")
			}
			c.set(y, c.GPR[h&31])
		default:
			return bad("reserved I8 function")
		}
	case 13, 14: // LI and CMPI
		if !constfield() {
			return bad("invalid extended immediate")
		}
		imm := h & 255
		if extended {
			imm = imm16
		}
		if op == 13 {
			c.set(x, imm)
		} else {
			c.GPR[24] = c.GPR[x] ^ imm
		}
	case 0x10, 0x11, 0x13, 0x14, 0x15, 0x18, 0x19, 0x1b: // register memory
		off := memoff(0)
		var size bus.Size
		var signed, write bool
		switch op {
		case 0x10:
			size = bus.Byte
			signed = true
		case 0x11:
			size = bus.Half
			if !extended {
				off = (h & 31) << 1
			}
			signed = true
		case 0x13:
			size = bus.Word
			if !extended {
				off = (h & 31) << 2
			}
		case 0x14:
			size = bus.Byte
		case 0x15:
			size = bus.Half
			if !extended {
				off = (h & 31) << 1
			}
		case 0x18:
			size = bus.Byte
			write = true
		case 0x19:
			size = bus.Half
			write = true
			if !extended {
				off = (h & 31) << 1
			}
		case 0x1b:
			size = bus.Word
			write = true
			if !extended {
				off = (h & 31) << 2
			}
		}
		addr := c.GPR[x] + off
		if write {
			c.bus.Write(addr, size, c.GPR[y])
		} else {
			v := c.bus.Read(addr, size)
			if signed {
				if size == bus.Byte {
					v = sx16(v, 8)
				} else {
					v = sx16(v, 16)
				}
			}
			c.set(y, v)
		}
	case 0x12, 0x1a, 0x16: // LWSP, SWSP, LWPC
		if !constfield() {
			return bad("invalid extended word access")
		}
		off := imm8 << 2
		if extended {
			off = sx16(imm16, 16)
		}
		base := c.GPR[29]
		if op == 0x16 {
			base = pcBase &^ 3
		}
		if op == 0x1a {
			c.bus.Write(base+off, bus.Word, c.GPR[x])
		} else {
			c.set(x, c.bus.Read(base+off, bus.Word))
		}
	case 0x1c: // RRR
		if extended {
			return bad("extended RRR")
		}
		z := m16Register[(h>>2)&7]
		switch h & 3 {
		case 1:
			c.set(z, c.GPR[x]+c.GPR[y])
		case 3:
			c.set(z, c.GPR[x]-c.GPR[y])
		default:
			return bad("64-bit RRR")
		}
	case 0x1d: // RR
		if extended {
			return bad("extended RR")
		}
		fn := h & 31
		switch fn {
		case 0: // JR, JALR, and compact forms
			src := x
			if ry == 1 || ry == 5 {
				if rx != 0 {
					return bad("invalid JR RA")
				}
				src = 31
			}
			v := c.GPR[src]
			if ry == 2 || ry == 6 {
				link := pc + 4
				if ry == 6 {
					link = pc + 2
				}
				c.set(31, link|1)
			}
			if ry <= 2 {
				return jump(v&^1, v&1 != 0), length, nil
			}
			if ry >= 4 && ry <= 6 {
				return branch{target: v &^ 1, isa: v&1 != 0, immediate: true}, length, nil
			}
			return bad("reserved JR selector")
		case 2:
			c.GPR[24] = boolWord(signed32(c.GPR[x]) < signed32(c.GPR[y]))
		case 3:
			c.GPR[24] = boolWord(c.GPR[x] < c.GPR[y])
		case 4:
			c.set(y, c.GPR[y]<<(c.GPR[x]&31))
		case 6:
			c.set(y, c.GPR[y]>>(c.GPR[x]&31))
		case 7:
			c.set(y, bits32(signed32(c.GPR[y])>>(c.GPR[x]&31)))
		case 10:
			c.GPR[24] = c.GPR[x] ^ c.GPR[y]
		case 11:
			c.set(x, -c.GPR[y])
		case 12:
			c.set(x, c.GPR[x]&c.GPR[y])
		case 13:
			c.set(x, c.GPR[x]|c.GPR[y])
		case 14:
			c.set(x, c.GPR[x]^c.GPR[y])
		case 15:
			c.set(x, ^c.GPR[y])
		case 16, 18:
			if ry != 0 {
				return bad("invalid MFHI/MFLO")
			}
			if fn == 16 {
				c.set(x, c.HI)
			} else {
				c.set(x, c.LO)
			}
		case 17: // CNVT
			v := c.GPR[x]
			switch ry {
			case 0:
				v &= 255
			case 1:
				v &= 65535
			case 4:
				v = sx16(v, 8)
			case 5:
				v = sx16(v, 16)
			default:
				return bad("reserved CNVT")
			}
			c.set(x, v)
		case 24, 25:
			p := uint64(c.GPR[x]) * uint64(c.GPR[y])
			if fn == 24 {
				p = bits64(int64(signed32(c.GPR[x])) * int64(signed32(c.GPR[y])))
			}
			c.LO = low32(p)
			c.HI = low32(p >> 32)
		case 26, 27:
			a, b := c.GPR[x], c.GPR[y]
			if b != 0 {
				if fn == 26 {
					c.LO = bits32(signed32(a) / signed32(b))
					c.HI = bits32(signed32(a) % signed32(b))
				} else {
					c.LO = a / b
					c.HI = a % b
				}
			}
		default:
			return bad("reserved RR function")
		}
	default:
		return bad("reserved MIPS16 opcode")
	}
	return branch{}, length, nil
}
