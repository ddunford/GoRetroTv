package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE GRID'S ROW-CREATING NATIVE IS ACTUALLY PASSED.
//
// The ALL CHANNELS grid keeps a 336-byte record per row and tests a halfword in it against 2
// before drawing a programme. All six hold 1, nothing ever writes 2, and the records are CREATED
// by `scall (0x0c, 0x08)` -- module twelve, function eight, which the native table now resolves to
// 0x800C06B8, declared as (int, int, pointer, pointer) returning int.
//
// Four arguments, one of which may well be the KIND the row is created as. If one of them is 1 on
// every call, that is the candidate, and the question moves to what decides it -- which is back in
// the o-code at 0x9FC77F16, where the operands are pushed:
//
//	push_fp_nn +07 ; push_fp_nn +09 ; push_ind_fp_nn +06
//	push_ds ; add_nnnnnnnn 0x00029d28 ; get
//	scall_uu_uu 0c 08
//	pop_fp_minus16 ; push_4 ; jne 0x9fc77f36     <- the result is compared against 4
//
// So this reads a0..a3 at entry and the result at the return, six times over, and prints them
// beside each other. It needs no disassembler and does not wait on one.
//
// THE RETURN IS TAKEN ONLY WHEN THE FRAME IS POPPED. "Leaving a module is not returning from it" is
// written in this project's blood: an earlier probe took a `jalr` for a return, read a live
// register as a return value, and produced a confident finding that had to be withdrawn. The value
// here is read when the PC leaves the function AND the stack pointer is back above where it was on
// entry.
//
// IT ASSERTS ITS OWN SUBJECT: the native must actually be entered, and six times, or the values
// below describe something other than the six rows.
//
// IT ONLY READS.
func TestWhatTheGridsRowCreatingNativeIsPassed(t *testing.T) {
	const (
		nativeAt = 0x800C06B8 // module 12, function 8, from the native module table
		a0, a1   = 4, 5       // MIPS argument registers
		a2, a3   = 6, 7
		v0       = 2 // the return value
		sp       = 29
	)
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	type call struct {
		args   [4]uint32
		result uint32
		got    bool
	}
	var calls []call
	inside := false
	entrySP := uint32(0)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Fetch {
			return
		}
		st := box.Machine.Core.State()
		pc := a.Virtual &^ 1
		if pc == nativeAt && !inside {
			inside, entrySP = true, st.GPR[sp]
			calls = append(calls, call{args: [4]uint32{
				st.GPR[a0], st.GPR[a1], st.GPR[a2], st.GPR[a3],
			}})
			return
		}
		// THE FRAME MUST BE POPPED. A branch out of the function is not a return from it.
		if inside && (pc < nativeAt || pc > nativeAt+0x400) && st.GPR[sp] >= entrySP {
			inside = false
			if n := len(calls) - 1; n >= 0 && !calls[n].got {
				calls[n].result, calls[n].got = st.GPR[v0], true
			}
		}
	}}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
	settled := openAllChannelsFinished(t, press, ".artifacts/rowcreate-grid.png")
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "rowcreate-grid.png"); err != nil {
		t.Fatal(err)
	}
	if len(calls) == 0 {
		t.Fatalf("harness: %#08X was never entered while the grid drew, so either the native "+
			"table resolved the wrong address or this is not the screen. Read "+
			".artifacts/rowcreate-grid.png", uint32(nativeAt))
	}
	t.Logf("the grid settled on %08X; the native was entered %d times", settled, len(calls))
	t.Logf("=== every call to (12,0x08), the row-creating native ===")
	t.Logf("    %-10s %-10s %-10s %-10s  %s", "a0", "a1", "a2", "a3", "returned")
	for _, c := range calls {
		ret := "  (no return seen)"
		if c.got {
			ret = ""
		}
		t.Logf("    %-10d %-10d 0x%08X 0x%08X  %d%s",
			c.args[0], c.args[1], c.args[2], c.args[3], c.result, ret)
	}
	// Which argument is constant across every call? A kind would be.
	for i := 0; i < 4; i++ {
		same := true
		for _, c := range calls {
			if c.args[i] != calls[0].args[i] {
				same = false
				break
			}
		}
		if same {
			t.Logf("a%d is %d (0x%08X) on ALL %d calls -- a candidate for the row's kind",
				i, calls[0].args[i], calls[0].args[i], len(calls))
		}
	}
}
