package cpu

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
)

func (c *Core) set(i, v uint32) {
	if i != 0 {
		c.GPR[i] = v
	}
}

func (c *Core) execute32(w uint32) (branch, error) {
	op, rs, rt, rd := w>>26, (w>>21)&31, (w>>16)&31, (w>>11)&31
	sa, fn, imm := (w>>6)&31, w&63, w&0xffff
	simm := signExtend16(imm)
	next := c.PC + 4
	r := &c.GPR
	taken := func(target uint32, isa bool) branch {
		return branch{target: target, isa: isa, armed: true}
	}
	branchTo := func(ok bool) branch {
		if ok {
			return taken(next+(simm<<2), false)
		}
		return branch{}
	}
	switch op {
	case 0:
		switch fn {
		case 0:
			c.set(rd, r[rt]<<sa)
		case 2:
			c.set(rd, r[rt]>>sa)
		case 3:
			c.set(rd, bits32(signed32(r[rt])>>sa))
		case 4:
			c.set(rd, r[rt]<<(r[rs]&31))
		case 6:
			c.set(rd, r[rt]>>(r[rs]&31))
		case 7:
			c.set(rd, bits32(signed32(r[rt])>>(r[rs]&31)))
		case 8:
			return taken(r[rs]&^1, r[rs]&1 != 0), nil
		case 9:
			c.set(rd, next+4)
			return taken(r[rs]&^1, r[rs]&1 != 0), nil
		case 15: // SYNC orders memory operations; the single-threaded bus is already ordered.
		case 16:
			c.set(rd, c.HI)
		case 17:
			c.HI = r[rs]
		case 18:
			c.set(rd, c.LO)
		case 19:
			c.LO = r[rs]
		case 24:
			p := int64(signed32(r[rs])) * int64(signed32(r[rt]))
			c.LO, c.HI = low32(bits64(p)), low32(bits64(p)>>32)
		case 25:
			p := uint64(r[rs]) * uint64(r[rt])
			c.LO, c.HI = low32(p), low32(p>>32)
		case 26:
			if r[rt] != 0 {
				a, b := int64(signed32(r[rs])), int64(signed32(r[rt]))
				c.LO, c.HI = low32(bits64(a/b)), low32(bits64(a%b))
			}
		case 27:
			if r[rt] != 0 {
				c.LO, c.HI = r[rs]/r[rt], r[rs]%r[rt]
			}
		case 32, 33:
			c.set(rd, r[rs]+r[rt])
		case 34, 35:
			c.set(rd, r[rs]-r[rt])
		case 36:
			c.set(rd, r[rs]&r[rt])
		case 37:
			c.set(rd, r[rs]|r[rt])
		case 38:
			c.set(rd, r[rs]^r[rt])
		case 39:
			c.set(rd, ^(r[rs] | r[rt]))
		case 42:
			c.set(rd, boolWord(signed32(r[rs]) < signed32(r[rt])))
		case 43:
			c.set(rd, boolWord(r[rs] < r[rt]))
		default:
			return branch{}, fmt.Errorf("unsupported SPECIAL function %d", fn)
		}
	case 1:
		var ok, link, likely bool
		switch rt {
		case 0, 2, 16, 18:
			ok = signed32(r[rs]) < 0
		case 1, 3, 17, 19:
			ok = signed32(r[rs]) >= 0
		default:
			return branch{}, fmt.Errorf("unsupported REGIMM variant %d", rt)
		}
		link = rt >= 16
		likely = rt == 2 || rt == 3 || rt == 18 || rt == 19
		if link {
			c.set(31, next+4)
		}
		if !ok && likely {
			return branch{target: skipSlot}, nil
		}
		return branchTo(ok), nil
	case 2, 3, 29:
		if op != 2 {
			c.set(31, next+4)
		}
		return taken((next&0xf0000000)|((w&0x03ffffff)<<2), op == 29), nil
	case 4:
		return branchTo(r[rs] == r[rt]), nil
	case 5:
		return branchTo(r[rs] != r[rt]), nil
	case 6:
		return branchTo(signed32(r[rs]) <= 0), nil
	case 7:
		return branchTo(signed32(r[rs]) > 0), nil
	case 8, 9:
		c.set(rt, r[rs]+simm)
	case 10:
		c.set(rt, boolWord(signed32(r[rs]) < signed32(simm)))
	case 11:
		c.set(rt, boolWord(r[rs] < simm))
	case 12:
		c.set(rt, r[rs]&imm)
	case 13:
		c.set(rt, r[rs]|imm)
	case 14:
		c.set(rt, r[rs]^imm)
	case 15:
		c.set(rt, imm<<16)
	case 20, 21, 22, 23:
		var ok bool
		switch op {
		case 20:
			ok = r[rs] == r[rt]
		case 21:
			ok = r[rs] != r[rt]
		case 22:
			ok = signed32(r[rs]) <= 0
		case 23:
			ok = signed32(r[rs]) > 0
		}
		if !ok {
			return branch{target: skipSlot}, nil
		}
		return branchTo(true), nil
	case 16:
		switch rs {
		case 0:
			c.set(rt, c.COP0[rd])
		case 4:
			c.COP0[rd] = r[rt]
			if rd == 11 {
				c.timerPending = false
			}
		case 16:
			if w != 0x42000018 {
				return branch{}, fmt.Errorf("unsupported COP0 operation %d", rs)
			}
			status := c.COP0[12]
			ret := c.COP0[14]
			if status&4 != 0 {
				ret = c.COP0[30]
				c.COP0[12] = status &^ 4
			} else {
				c.COP0[12] = status &^ 2
			}
			return branch{target: ret &^ 1, isa: ret&1 != 0, immediate: true}, nil
		default:
			return branch{}, fmt.Errorf("unsupported COP0 operation %d", rs)
		}
	case 32, 33, 35, 36, 37, 40, 41, 43, 48, 56:
		ea := r[rs] + simm
		switch op {
		case 32:
			c.set(rt, signExtend8(c.bus.Read(ea, bus.Byte)))
		case 33:
			c.set(rt, signExtend16(c.bus.Read(ea, bus.Half)))
		case 35:
			c.set(rt, c.bus.Read(ea, bus.Word))
		case 36:
			c.set(rt, c.bus.Read(ea, bus.Byte))
		case 37:
			c.set(rt, c.bus.Read(ea, bus.Half))
		case 40:
			c.bus.Write(ea, bus.Byte, r[rt])
			c.hasReservation = false
		case 41:
			c.bus.Write(ea, bus.Half, r[rt])
			c.hasReservation = false
		case 43:
			c.bus.Write(ea, bus.Word, r[rt])
			c.hasReservation = false
		case 48:
			c.set(rt, c.bus.Read(ea, bus.Word))
			c.reserved, c.hasReservation = ea, true
		case 56:
			if c.hasReservation && c.reserved == ea {
				c.bus.Write(ea, bus.Word, r[rt])
				c.set(rt, 1)
			} else {
				c.set(rt, 0)
			}
			c.hasReservation = false
		}
	case 34, 38, 42, 46:
		c.unaligned(op, rt, r[rs]+simm)
	case 47: // CACHE has no effect on this uncached memory model.
	default:
		return branch{}, fmt.Errorf("unsupported opcode %d", op)
	}
	return branch{}, nil
}

func boolWord(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}

// These casts reproduce the ISA's two's-complement interpretation and truncation.
func signed32(v uint32) int32 { return int32(v) } // #nosec G115 -- same 32 register bits

func bits32(v int32) uint32 { return uint32(v) } // #nosec G115 -- same 32 register bits

func bits64(v int64) uint64 { return uint64(v) } // #nosec G115 -- same 64 product bits

func low32(v uint64) uint32 { return uint32(v) } // #nosec G115 -- the low machine word

func signExtend16(v uint32) uint32 {
	return uint32(int32(int16(v))) // #nosec G115 -- the low immediate halfword is signed
}

func signExtend8(v uint32) uint32 {
	return uint32(int32(int8(v))) // #nosec G115 -- the low loaded byte is signed
}

func (c *Core) unaligned(op, rt, ea uint32) {
	n, addr := ea&3, ea&^uint32(3)
	word := c.bus.Read(addr, bus.Word)
	old := c.GPR[rt]
	switch op {
	case 34: // LWL, big-endian high bytes from n through the end of the aligned word.
		keep := uint32(0)
		if n != 0 {
			keep = (uint32(1) << (8 * n)) - 1
		}
		c.set(rt, word<<(8*n)|old&keep)
	case 38: // LWR, big-endian low bytes from the start through n.
		mask := uint32(0xffffffff)
		if n != 3 {
			mask = (uint32(1) << (8 * (n + 1))) - 1
		}
		c.set(rt, old&^mask|(word>>(8*(3-n)))&mask)
	case 42: // SWL preserves the bytes before n.
		keep := uint32(0)
		if n != 0 {
			keep = 0xffffffff << (8 * (4 - n))
		}
		c.bus.Write(addr, bus.Word, word&keep|old>>(8*n))
		c.hasReservation = false
	case 46: // SWR preserves the bytes after n.
		keep := uint32(0)
		if n != 3 {
			keep = (uint32(1) << (8 * (3 - n))) - 1
		}
		c.bus.Write(addr, bus.Word, word&keep|old<<(8*(3-n)))
		c.hasReservation = false
	}
}
