package cpu_test

import "testing"

func mtc0(rt, rd uint32) uint32 { return 16<<26 | 4<<21 | rt<<16 | rd<<11 }
func mfc0(rt, rd uint32) uint32 { return 16<<26 | rt<<16 | rd<<11 }

func TestCOP0RegistersSurviveWritesAndReads(t *testing.T) {
	t.Parallel()
	for _, rd := range []uint32{8, 12, 13, 14, 16, 30} {
		t.Run(map[uint32]string{8: "BadVAddr", 12: "Status", 13: "Cause", 14: "EPC", 16: "Config", 30: "ErrorEPC"}[rd], func(t *testing.T) {
			t.Parallel()
			c, _ := machine(t, mtc0(2, rd), mfc0(3, rd))
			c.GPR[2] = 0x80081c58
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			if err := c.Step(); err != nil {
				t.Fatal(err)
			}
			if c.GPR[3] != 0x80081c58 {
				t.Fatalf("COP0 register %d returned %#x, want 0x80081C58", rd, c.GPR[3])
			}
		})
	}
}

func TestCOP0CompareRequestsTimerAndWriteClearsIt(t *testing.T) {
	t.Parallel()
	c, _ := machine(t, 0, 0, 0, mtc0(2, 11), 0)
	c.COP0[11] = 3
	for i := 0; i < 3; i++ {
		if err := c.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if c.COP0[9] != 3 || !c.TimerPending() {
		t.Fatalf("Count=%d timer=%v; want Count 3 and pending", c.COP0[9], c.TimerPending())
	}
	c.GPR[2] = 5
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.COP0[11] != 5 || c.TimerPending() {
		t.Fatalf("writing Compare left Compare=%d timer=%v", c.COP0[11], c.TimerPending())
	}
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if c.COP0[9] != 5 || !c.TimerPending() {
		t.Fatalf("timer did not request again at the new comparison: Count=%d timer=%v", c.COP0[9], c.TimerPending())
	}
}
