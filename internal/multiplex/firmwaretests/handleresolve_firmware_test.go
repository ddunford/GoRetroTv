package firmwaretests_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHERE THE 2 COMES FROM.
//
// The ALL CHANNELS grid enters its row loop knowing there are six channels, runs the body once for
// index 0, calls 0x800ADD08 with a handle taken from 0x80164AB4, and leaves:
//
//	800a4bac  jalr  a2                call 0x800ADD08
//	800a4bb2  cmpi  v1,2
//	800a4bb4  bteqz 0x800a4d8d        result == 2 -> LEAVE the loop
//
// 0x800ADD08 is a handle resolver rather than a listings lookup -- it zeroes twelve bytes of the
// caller's buffer, splits the sixteen-bit handle into a pool index `(id-1)>>12` and an element
// `(id-1)&0xFFF`, and bounds-checks both against a pool descriptor whose +8 is the element size,
// +12 the base and +16 the count. Every failure path in the part already read returns ZERO, so the
// 2 comes from somewhere further in.
//
// **READING THE REST OF IT WOULD BE A GUESS AND THIS PROJECT HAS PAID FOR THOSE.** The function has
// several hundred instructions and at least four exits; picking the one that "looks like" the
// answer is exactly how a plausible wrong finding gets recorded. So this TRACES IT: every
// instruction the box actually fetches from the moment the grid enters the resolver, in order,
// until it returns. A few hundred addresses, read against the disassembly, say which branch was
// taken at every fork -- and the last one before the return is where the 2 is made.
//
// IT ALSO TRACES THE HANDLE IT WAS GIVEN, because a resolver that is handed a handle of zero and
// one that is handed a good handle it cannot find are different faults with different fixes, and
// the trace alone does not distinguish them. a0 at entry is the handle.
//
// IT ASSERTS ITS OWN SUBJECT: the resolver must be entered at all, or the trace is empty for a
// reason that has nothing to do with the grid.
//
// IT ONLY READS.
func TestWhatTheHandleResolverDoesForTheGrid(t *testing.T) {
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

	const (
		resolverEntry = 0x800ADD08
		// stateToCode is 0x800AC534, a pure jump-table mapping of an object's INTERNAL STATE to
		// the code the caller sees: state 2 -> 1, 3 -> 0, 4 -> 2, 5 -> 2, 6 -> 3, 7 -> 4. a0 at
		// entry is the state itself, which is the number this probe actually wants -- the code
		// collapses 4 and 5 into one answer and they are different states.
		stateToCode  = 0x800AC534
		loopHead     = 0x800A4B60
		afterTheCall = 0x800A4BB0 // where the resolver's result lands in v1
		traceLimit   = 800
	)

	var (
		watching  bool
		tracing   bool
		done      bool
		handle    uint32
		buffer    uint32
		trace     []uint32
		entries   int
		resultAt  = map[uint32]int{}
		stateAt   = map[uint32]int{}
		callSites = map[uint32]int{}
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		at := a.Virtual &^ 1
		if at == stateToCode {
			stateAt[box.Machine.Core.State().GPR[4]]++
		}
		if at == resolverEntry {
			entries++
			st := box.Machine.Core.State()
			callSites[st.GPR[31]&^1]++
			if !done {
				// a0 is the handle and a1 the caller's twelve-byte buffer, read at entry before
				// the prologue has moved anything.
				handle, buffer = st.GPR[4]&0xffff, st.GPR[5]
				tracing, trace = true, trace[:0]
			}
		}
		if tracing {
			trace = append(trace, at)
			if len(trace) >= traceLimit {
				tracing, done = false, true
			}
		}
		if at == afterTheCall && tracing {
			// The result is in v0 the instant control returns to the caller.
			resultAt[box.Machine.Core.State().GPR[2]]++
			tracing, done = false, true
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, settled := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				settled = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	grid := openAllChannels(t, func(raw uint8, name string, budget int) uint32 {
		if raw == keySelect {
			watching, tracing, done, entries = true, false, false, 0
			trace = trace[:0]
			for k := range resultAt {
				delete(resultAt, k)
			}
			for k := range callSites {
				delete(callSites, k)
			}
			for k := range stateAt {
				delete(stateAt, k)
			}
		}
		drew := press(raw, name, budget)
		watching = false
		return drew
	}, ".artifacts/handle-resolve-all-channels.png", false)
	if err := dumpScreen(t, box, "handle-resolve-all-channels.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid settled on %08X", grid)

	if entries == 0 {
		t.Fatalf("harness: the grid draw never entered the resolver at %08X at all, so this trace "+
			"is empty for a reason that has nothing to do with the loop -- either the route did not "+
			"reach the grid or the address is wrong", uint32(resolverEntry))
	}
	t.Logf("the resolver was entered %d times during the draw", entries)
	for site, n := range callSites {
		t.Logf("    called from %08X, %d times", site, n)
	}
	t.Logf("HANDLE %#06x, caller's buffer %08X", handle, buffer)
	if handle == 0 {
		t.Logf("  -- and a handle of ZERO is a different fault from a handle that cannot be found: " +
			"nothing had filled it in, so the question moves back to whatever writes 0x80164AB4")
	}
	for v, n := range stateAt {
		t.Logf("INTERNAL STATE %d, %d times -- mapped to a code by 0x800AC534", v, n)
	}
	for v, n := range resultAt {
		t.Logf("RESULT %d, %d times -- the caller compares this against 2", int32(v), n) // #nosec G115
	}

	// The trace, in order and in the disassembly's own formatting so it can be read straight down
	// beside the listing.
	t.Logf("the first call fetched %d instructions:", len(trace))
	var line []string
	for i, at := range trace {
		line = append(line, hexPC(at))
		if len(line) == 8 || i == len(trace)-1 {
			t.Logf("    %s", strings.Join(line, " "))
			line = line[:0]
		}
	}
}
