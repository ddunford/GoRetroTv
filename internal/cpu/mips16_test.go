package cpu_test

import (
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
)

func TestMIPS16CompareTAndImmediateBranch(t *testing.T) {
	c, b := machine(t)
	c.PC, c.ISA = codeBase+0x40, true
	c.GPR[16] = 5
	b.Write(c.PC, bus.Half, 0x7005)   // cmpi s0,5
	b.Write(c.PC+2, bus.Half, 0x6001) // bteqz +2; skips next halfword without a slot
	b.Write(c.PC+4, bus.Half, 0x6809) // li s0,9 (skipped)
	b.Write(c.PC+6, bus.Half, 0x6807) // li s0,7
	for i := 0; i < 3; i++ {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.GPR[24] != 0 || c.GPR[16] != 7 || c.PC != codeBase+0x48 {
		t.Fatalf("T=%#x s0=%#x PC=%#x", c.GPR[24], c.GPR[16], c.PC)
	}
}

func TestMIPS16MoveT8IsImplicitCondition(t *testing.T) {
	c, b := machine(t)
	c.PC, c.ISA = codeBase+0x40, true
	c.GPR[16] = 0x1234
	b.Write(c.PC, bus.Half, 0x6500|3<<3) // move t8,s0 (MOV32R)
	b.Write(c.PC+2, bus.Half, 0x6101)    // btnez +2
	b.Write(c.PC+4, bus.Half, 0x6809)
	b.Write(c.PC+6, bus.Half, 0x6807)
	for i := 0; i < 3; i++ {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.GPR[24] != 0x1234 || c.GPR[16] != 7 {
		t.Fatalf("T=%#x s0=%#x", c.GPR[24], c.GPR[16])
	}
}

func TestMIPS16JALXReturnsToMIPS32(t *testing.T) {
	c, b := machine(t)
	c.PC, c.ISA = codeBase+0x40, true
	b.Write(c.PC, bus.Half, 0x1c00)   // JALX index follows
	b.Write(c.PC+2, bus.Half, 4)      // target codeBase+0x10
	b.Write(c.PC+4, bus.Half, 0x6803) // delay slot: li s0,3
	b.Write(codeBase+0x10, bus.Word, ri(9, 0, 2, 42))
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if !c.HasPendingBranch() || c.PC != codeBase+0x44 || c.GPR[31] != codeBase+0x47 {
		t.Fatalf("after JALX PC=%#x ra=%#x", c.PC, c.GPR[31])
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.ISA || c.PC != codeBase+0x10 || c.GPR[16] != 3 {
		t.Fatalf("after slot PC=%#x ISA=%v s0=%d", c.PC, c.ISA, c.GPR[16])
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.GPR[2] != 42 {
		t.Fatalf("MIPS32 target result=%d", c.GPR[2])
	}
}

func TestMIPS32JALXEntersMIPS16AfterDelaySlot(t *testing.T) {
	c, b := machine(t, 29<<26|0x10, ri(9, 0, 3, 9)) // jump to codeBase+0x40
	b.Write(codeBase+0x40, bus.Half, 0x6807)        // li s0,7
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.ISA || c.PC != codeBase+4 {
		t.Fatalf("MIPS32 slot not reached: PC=%#x ISA=%v", c.PC, c.ISA)
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if !c.ISA || c.PC != codeBase+0x40 || c.GPR[3] != 9 {
		t.Fatalf("JALX transfer PC=%#x ISA=%v slot=%d", c.PC, c.ISA, c.GPR[3])
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.GPR[16] != 7 {
		t.Fatalf("MIPS16 target result=%d", c.GPR[16])
	}
}

func TestMIPS16ExtendedImmediateAndOriginalASE(t *testing.T) {
	c, b := machine(t)
	c.PC, c.ISA = codeBase+0x40, true
	b.Write(c.PC, bus.Half, 0xf000|1) // EXTEND; high immediate bits
	b.Write(c.PC+2, bus.Half, 0x6803) // LI s0,0x803
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.GPR[16] != 0x803 || c.PC != codeBase+0x44 {
		t.Fatalf("extended LI=%#x PC=%#x", c.GPR[16], c.PC)
	}
	b.Write(c.PC, bus.Half, 0x6400) // SAVE/RESTORE (MIPS16e)
	if err := c.Step(); err == nil || !strings.Contains(err.Error(), "SAVE/RESTORE") {
		t.Fatalf("SAVE/RESTORE error=%v", err)
	}
}

func TestMIPS16VariableShiftOperandOrder(t *testing.T) {
	c, b := machine(t)
	c.PC, c.ISA = codeBase+0x40, true
	c.GPR[16], c.GPR[17] = 3, 5
	b.Write(c.PC, bus.Half, 0xe800|1<<5|4) // sllv s0,s1: s1 = s1 << s0
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.GPR[17] != 40 || c.GPR[16] != 3 {
		t.Fatalf("shift value=%d amount=%d", c.GPR[17], c.GPR[16])
	}
}
