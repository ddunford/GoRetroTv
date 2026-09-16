package cpu_test

import "testing"

func TestInterruptWaitsUntilAfterBranchDelaySlot(t *testing.T) {
	t.Parallel()
	c, _ := machine(t, ri(4, 0, 0, 2), ri(9, 2, 2, 1), 0, 0)
	c.COP0[12] = 1 | 1<<10 // IE and IM2
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	c.Interrupt(2)
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.PC != codeBase+12 || c.GPR[2] != 1 || c.COP0[14] != 0 {
		t.Fatalf("interrupt entered the slot: PC=%#x slot=%d EPC=%#x", c.PC, c.GPR[2], c.COP0[14])
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.PC != 0x80000184 || c.COP0[14] != codeBase+12 || c.COP0[12]&2 == 0 || c.COP0[13]&(1<<10) == 0 {
		t.Fatalf("interrupt PC=%#x EPC=%#x Status=%#x Cause=%#x", c.PC, c.COP0[14], c.COP0[12], c.COP0[13])
	}
}

func TestInterruptRequiresEnableAndSelectsBEVVector(t *testing.T) {
	t.Parallel()
	c, _ := machine(t, 0, 0, 0)
	c.COP0[12] = 1 << 10 // IM2 set, IE clear
	c.Interrupt(2)
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.PC != codeBase+4 {
		t.Fatalf("IE-clear interrupt moved PC to %#x", c.PC)
	}
	c.COP0[12] = 1 | 1<<22 // IE and BEV, but IM2 clear
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.PC != codeBase+8 {
		t.Fatalf("IM2-clear interrupt moved PC to %#x", c.PC)
	}
	c.COP0[12] = 1 | 1<<10 | 1<<22 // IE, IM2, BEV
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.PC != 0xBFC00384 || c.COP0[14] != codeBase+8 {
		t.Fatalf("BEV interrupt PC=%#x EPC=%#x", c.PC, c.COP0[14])
	}
}

func TestTimerInterruptIsClearedOnlyByCompareWrite(t *testing.T) {
	t.Parallel()
	c, _ := machine(t, 0, 0)
	c.COP0[11] = 1
	c.COP0[12] = 1 | 1<<15 // IE and IM7
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if !c.TimerPending() || c.COP0[14] != codeBase || c.COP0[13]&(1<<15) == 0 {
		t.Fatalf("timer request/delivery Count=%d pending=%v EPC=%#x Cause=%#x",
			c.COP0[9], c.TimerPending(), c.COP0[14], c.COP0[13])
	}
}

func TestInterruptDoesNotNestWhileExceptionLevelIsSet(t *testing.T) {
	t.Parallel()
	for _, flag := range []struct {
		name string
		bit  uint32
	}{{"EXL", 2}, {"ERL", 4}} {
		t.Run(flag.name, func(t *testing.T) {
			t.Parallel()
			c, _ := machine(t, 0)
			c.COP0[12] = 1 | 1<<10 | flag.bit
			c.Interrupt(2)
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			if c.PC != codeBase+4 || c.COP0[14] != 0 {
				t.Fatalf("nested interrupt: PC=%#x EPC=%#x", c.PC, c.COP0[14])
			}
		})
	}
}
