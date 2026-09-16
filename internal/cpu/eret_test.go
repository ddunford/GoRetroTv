package cpu_test

import "testing"

const eret = 0x42000018

func TestERETRestoresTheSelectedPCAndISAModeWithoutASlot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		status        uint32
		epc, errorEPC uint32
		wantPC        uint32
		wantISA       bool
		wantStatus    uint32
	}{
		{"ordinary exception returns through EPC", 0x2, codeBase + 0x100, codeBase + 0x200 | 1, codeBase + 0x100, false, 0},
		{"EPC bit zero selects MIPS16", 0x2, codeBase + 0x100 | 1, codeBase + 0x200, codeBase + 0x100, true, 0},
		{"error exception selects ErrorEPC", 0x6, codeBase + 0x100, codeBase + 0x200 | 1, codeBase + 0x200, true, 0x2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := machine(t,
				mtc0(2, 12), mtc0(3, 14), mtc0(4, 30), eret,
				ri(9, 5, 5, 1), // would run only if ERET incorrectly had a delay slot
			)
			c.GPR[2], c.GPR[3], c.GPR[4] = tc.status, tc.epc, tc.errorEPC
			for i := 0; i < 4; i++ {
				if err := c.Step(); err != nil {
					t.Fatalf("instruction %d: %v", i, err)
				}
			}
			if c.PC != tc.wantPC || c.ISA != tc.wantISA || c.COP0[12] != tc.wantStatus || c.GPR[5] != 0 {
				t.Fatalf("PC=%#x ISA=%v Status=%#x slot-value=%d; want %#x %v %#x 0",
					c.PC, c.ISA, c.COP0[12], c.GPR[5], tc.wantPC, tc.wantISA, tc.wantStatus)
			}
		})
	}
}
