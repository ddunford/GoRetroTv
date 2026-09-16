package cpu_test

import "testing"

func TestBranchDelaySlotForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name              string
		word              uint32
		rs, rt            uint32
		firstPC, secondPC uint32
		value, link       uint32
		isa               bool
	}{
		{"beq taken", ri(4, 1, 2, 2), 3, 3, 4, 12, 1, 0, false},
		{"bne not taken still executes slot", ri(5, 1, 2, 2), 3, 3, 4, 8, 1, 0, false},
		{"blez taken", ri(6, 1, 0, 2), 0xffffffff, 0, 4, 12, 1, 0, false},
		{"bgtz taken", ri(7, 1, 0, 2), 1, 0, 4, 12, 1, 0, false},
		{"bltz taken", ri(1, 1, 0, 2), 0xffffffff, 0, 4, 12, 1, 0, false},
		{"bgezal links", ri(1, 1, 17, 2), 0, 0, 4, 12, 1, codeBase + 8, false},
		{"beql not taken nullifies slot", ri(20, 1, 2, 2), 1, 2, 8, 12, 10, 0, false},
		{"bnel taken", ri(21, 1, 2, 2), 1, 2, 4, 12, 1, 0, false},
		{"blezl not taken nullifies slot", ri(22, 1, 0, 2), 1, 0, 8, 12, 10, 0, false},
		{"bgtzl not taken nullifies slot", ri(23, 1, 0, 2), 0, 0, 8, 12, 10, 0, false},
		{"bltzall not taken nullifies slot", ri(1, 1, 18, 2), 1, 0, 8, 12, 10, codeBase + 8, false},
		{"j", 2<<26 | 3, 0, 0, 4, 12, 1, 0, false},
		{"jal", 3<<26 | 3, 0, 0, 4, 12, 1, codeBase + 8, false},
		{"jalx switches after slot", 29<<26 | 3, 0, 0, 4, 12, 1, codeBase + 8, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := machine(t, tc.word, ri(9, 4, 4, 1), ri(9, 4, 4, 10), 0)
			c.GPR[1], c.GPR[2] = tc.rs, tc.rt
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			if c.PC != codeBase+tc.firstPC {
				t.Fatalf("after branch PC = %#x, want %#x", c.PC, codeBase+tc.firstPC)
			}
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			if c.PC != codeBase+tc.secondPC || c.GPR[4] != tc.value || c.GPR[31] != tc.link || c.ISA != tc.isa {
				t.Fatalf("after slot PC=%#x value=%d link=%#x isa=%v; want %#x %d %#x %v",
					c.PC, c.GPR[4], c.GPR[31], c.ISA, codeBase+tc.secondPC, tc.value, tc.link, tc.isa)
			}
		})
	}
}

func TestPendingBranchIsVisibleToCheckpointDriver(t *testing.T) {
	t.Parallel()
	c, _ := machine(t, ri(4, 0, 0, 2), 0, 0, 0)
	if c.HasPendingBranch() {
		t.Fatal("reset core has a pending branch")
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if !c.HasPendingBranch() {
		t.Fatal("branch target must remain pending until its slot retires")
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.HasPendingBranch() {
		t.Fatal("slot retired but branch is still pending")
	}
}
