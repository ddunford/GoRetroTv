package cpu_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
)

func TestMIPS16InstructionFamilies(t *testing.T) {
	cases := []struct {
		name       string
		half       uint32
		s0, s1, sp uint32
		reg        uint32
		want       uint32
	}{
		{"ADDIUSP", 0x0004, 0, 0, codeBase + 0x100, 16, codeBase + 0x110},
		{"ADDIUPC", 0x0804, 0, 0, 0, 16, codeBase + 0x50},
		{"RRI addiu", 0x4000 | 1<<5 | 3, 5, 0, 0, 17, 8},
		{"ADDIU8 signed", 0x4800 | 0xfe, 5, 0, 0, 16, 3},
		{"SLTI writes T", 0x5000 | 5, 3, 0, 0, 24, 1},
		{"SLTIU unsigned", 0x5800 | 5, 0xffffffff, 0, 0, 24, 0},
		{"LI", 0x6800 | 0xaa, 0, 0, 0, 16, 0xaa},
		{"CMPI writes T", 0x7000 | 5, 5, 0, 0, 24, 0},
		{"immediate SLL", 0x3000 | 1<<5 | 3<<2, 0, 5, 0, 16, 40},
		{"RR AND", 0xe800 | 1<<5 | 12, 0x33, 0x0f, 0, 16, 3},
		{"RRR ADDU", 0xe000 | 1<<5 | 2<<2 | 1, 3, 5, 0, 2, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, b := machine(t)
			c.PC, c.ISA = codeBase+0x40, true
			c.GPR[16], c.GPR[17], c.GPR[29] = tc.s0, tc.s1, tc.sp
			b.Write(c.PC, bus.Half, tc.half)
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			if got := c.GPR[tc.reg]; got != tc.want {
				t.Fatalf("register %d=%#x, want %#x", tc.reg, got, tc.want)
			}
		})
	}
}

func TestMIPS16MemoryFamilies(t *testing.T) {
	cases := []struct {
		name  string
		half  uint32
		value uint32
		want  uint32
		store bool
	}{
		{"LB sign extends", 0x8000 | 1<<5 | 1, 0x80, 0xffffff80, false},
		{"LHU zero extends", 0xa800 | 1<<5 | 1, 0x80ff, 0x80ff, false},
		{"LW", 0x9800 | 1<<5 | 1, 0x81234567, 0x81234567, false},
		{"SB", 0xc000 | 1<<5 | 1, 0x1234, 0x34, true},
		{"SH", 0xc800 | 1<<5 | 1, 0x123456, 0x3456, true},
		{"SW", 0xd800 | 1<<5 | 1, 0x12345678, 0x12345678, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, b := machine(t)
			c.PC, c.ISA = codeBase+0x40, true
			c.GPR[16] = codeBase + 0x100
			b.Write(c.PC, bus.Half, tc.half)
			width := bus.Byte
			off := uint32(1)
			if tc.name == "LHU zero extends" || tc.name == "SH" {
				width = bus.Half
				off = 2
			}
			if tc.name == "LW" || tc.name == "SW" {
				width = bus.Word
				off = 4
			}
			if tc.store {
				c.GPR[17] = tc.value
			} else {
				b.Write(codeBase+0x100+off, width, tc.value)
			}
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			got := c.GPR[17]
			if tc.store {
				got = b.Read(codeBase+0x100+off, width)
			}
			if got != tc.want {
				t.Fatalf("result=%#x, want %#x", got, tc.want)
			}
		})
	}
}
