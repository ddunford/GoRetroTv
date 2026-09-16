package cpu_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/memory"
)

const codeBase = 0x80000000

func machine(t *testing.T, words ...uint32) (*cpu.Core, *bus.Bus) {
	t.Helper()
	b := bus.New()
	r, err := memory.NewRAM("dram", memory.DirtyPageLen)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Attach(codeBase, memory.DirtyPageLen, r); err != nil {
		t.Fatal(err)
	}
	for i, word := range words {
		b.Write(codeBase+uint32(i*4), bus.Word, word)
	}
	return cpu.New(b, codeBase), b
}

func ri(op, rs, rt, imm uint32) uint32 {
	return op<<26 | rs<<21 | rt<<16 | imm&0xffff
}

func rr(rs, rt, rd, sa, fn uint32) uint32 {
	return rs<<21 | rt<<16 | rd<<11 | sa<<6 | fn
}

func TestMIPS32ArithmeticAndShiftFamilies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		word   uint32
		rs, rt uint32
		want   uint32
	}{
		{"addiu sign extends", ri(9, 1, 3, 0xfffe), 5, 0, 3},
		{"addu wraps", rr(1, 2, 3, 0, 33), 0xffffffff, 2, 1},
		{"sltu unsigned", rr(1, 2, 3, 0, 43), 0xffffffff, 2, 0},
		{"variable shift uses rs amount", rr(1, 2, 3, 0, 4), 3, 5, 40},
		{"arithmetic right shift", rr(0, 2, 3, 2, 3), 0, 0xfffffff0, 0xfffffffc},
		{"lui", ri(15, 0, 3, 0xabcd), 0, 0, 0xabcd0000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := machine(t, tc.word)
			c.GPR[1], c.GPR[2] = tc.rs, tc.rt
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			if got := c.GPR[3]; got != tc.want {
				t.Fatalf("register 3 = %#x, want %#x", got, tc.want)
			}
		})
	}
}

func TestMIPS32BigEndianUnalignedQuartet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		op      uint32
		off     uint32
		initial uint32
		word    uint32
		want    uint32
		store   bool
	}{
		{"lwl first byte", 34, 0, 0xaabbccdd, 0x11223344, 0xaabbccdd, false},
		{"lwl second byte", 34, 1, 0x11223344, 0xaabbccdd, 0xbbccdd44, false},
		{"lwr third byte", 38, 2, 0x11223344, 0xaabbccdd, 0x11aabbcc, false},
		{"lwr last byte", 38, 3, 0x11223344, 0xaabbccdd, 0xaabbccdd, false},
		{"swl second byte", 42, 1, 0x11223344, 0xaabbccdd, 0x11aabbcc, true},
		{"swr third byte", 46, 2, 0x11223344, 0xaabbccdd, 0xbbccdd44, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, b := machine(t, ri(tc.op, 1, 2, tc.off))
			c.GPR[1] = codeBase + 0x100
			c.GPR[2] = tc.word
			b.Write(codeBase+0x100, bus.Word, tc.initial)
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			got := c.GPR[2]
			if tc.store {
				got = b.Read(codeBase+0x100, bus.Word)
			}
			if got != tc.want {
				t.Fatalf("got %#x, want %#x", got, tc.want)
			}
		})
	}
}

func TestMIPS32BadInstructionHaltsWithoutAdvancing(t *testing.T) {
	t.Parallel()
	c, _ := machine(t, 0xfc000000)
	if err := c.Step(); err == nil {
		t.Fatal("unknown opcode must halt visibly")
	}
	if c.PC != codeBase {
		t.Fatalf("PC advanced to %#x after bad instruction", c.PC)
	}
}
