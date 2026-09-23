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
		stateToCode = 0x800AC534
		// loadTheState is 0x800ADDA4, `lw s0,12(v0)`. v0 there is the RESOLVED OBJECT, so the
		// state the grid's fate turns on lives at object+12 and its kind at object+0. Capturing
		// the pointer is what turns "the state is 4" into an address something can be watched at.
		loadTheState = 0x800ADDA4
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
		object    uint32
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
		if at == loadTheState && object == 0 {
			object = box.Machine.Core.State().GPR[2]
		}
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
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
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
	if object == 0 {
		t.Fatal("harness: the instruction that loads the state never executed, so no object was " +
			"captured and the dump below would be of address zero")
	}
	t.Logf("THE OBJECT IS AT %08X -- kind at +0, state at +12:", object)
	if off := object & 0x1fffffff; off+64 <= box.RAM.Size() {
		for row := uint32(0); row < 64; row += 16 {
			t.Logf("    +%02d  %08X %08X %08X %08X", row,
				box.RAM.Read(off+row, bus.Word), box.RAM.Read(off+row+4, bus.Word),
				box.RAM.Read(off+row+8, bus.Word), box.RAM.Read(off+row+12, bus.Word))
		}
	}
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

// WHAT WRITES THE STATE THE GRID'S WHOLE SCREEN TURNS ON.
//
// The object the ALL CHANNELS grid asks about lives at 0x802B2A54 and reads, in full:
//
//	+0  0000000E   kind 14, which is what selects the resolver's arm
//	+4  0000000B   the tag it copies into the caller's buffer
//	+8  00000300   the handle's pool, 0x3001-1 >> 12
//	+12 00000004   THE STATE -- 4, where the grid needs 6 or more
//	+16 802CEE14   a pointer whose +20 and +22 halfwords the resolver copies out
//	+24 00200020   network 32 and transport 32 -- THE SAME IDS the box's own NIT and SDT match
//	               units are programmed with (unit 1: 40/FE 00/FF 20/FF, unit 2: 42/FB 00/FF 20/FF)
//
// So this is the box's record of the transport our broadcast arrives on, and the grid gates on its
// state. One other object was seen in state 7 during the same draw, so 7 is reachable in this box —
// which means state 4 is a position in a progression rather than a dead end, and something moves
// things along it.
//
// **THIS WATCHES EVERY WRITE TO THAT ONE WORD, FROM BEFORE ACQUISITION BEGINS**, and reports the
// instruction that made each one and the value it wrote. Three outcomes, all worth having: a write
// that sets 4 and never runs again names the code that would have to run a second time; a sequence
// 1,2,3,4 that stops names how far the progression gets and where it stalls; and no writes at all
// means 4 was there before this run started and comes out of the restored snapshot.
//
// IT ASSERTS ITS OWN SUBJECT, and this one matters more than usual. The address is a HEAP address
// captured on a different run, and a probe watching an address the object has moved off reports a
// confident zero that reads exactly like "nothing writes it". So the object is checked at the end:
// kind must still be 14 at +0, or the run is void and says so.
//
// IT ONLY READS -- the watch observes writes the guest makes, and writes nothing itself.
func TestWhatWritesTheTransportStateTheGridGatesOn(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	// Captured on an earlier run of the trace above; re-verified at the end of this one.
	const (
		objectAt = 0x802B2A54
		kindAt   = 0
		stateAt2 = 12
		wantKind = 14
	)
	type write struct {
		pc    uint32
		value uint32
	}
	var writes []write
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write || a.Fetch {
			return
		}
		if a.Virtual&0x1fffffff == (objectAt+stateAt2)&0x1fffffff {
			writes = append(writes, write{pc: box.Machine.Core.State().PC &^ 1, value: a.Value})
		}
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	for i := 0; i < 160_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	_ = registered
	_ = want

	off := uint32(objectAt) & 0x1fffffff
	if kind := box.RAM.Read(off+kindAt, bus.Word); kind != wantKind {
		t.Fatalf("harness: %08X holds kind %d, not %d -- the object has moved since it was "+
			"captured, so this run watched somebody else's memory and its silence means nothing",
			uint32(objectAt), kind, wantKind)
	}
	state := box.RAM.Read(off+stateAt2, bus.Word)
	t.Logf("the transport object is still at %08X, kind %d, and its state is now %d",
		uint32(objectAt), wantKind, state)

	if len(writes) == 0 {
		t.Logf("VERDICT: NOTHING WROTE THE STATE in 160 million instructions of acquisition. It is "+
			"%d because the restored snapshot already had it at %d, and nothing the box has "+
			"received since moves it. Whatever advances a transport past this point either never "+
			"runs here or is waiting on something we do not send.", state, state)
		return
	}
	t.Logf("VERDICT: the state was written %d times:", len(writes))
	for i, w := range writes {
		if i >= 40 {
			t.Logf("    ... and %d more", len(writes)-40)
			break
		}
		t.Logf("    %08X wrote %d", w.pc, w.value)
	}
}

// AND THE EXPERIMENT THAT FOLLOWS FROM IT: WAIT FOR THE TRANSPORT, THEN OPEN THE GRID.
//
// The state the grid gates on is not stuck. Watching every write to it across a long acquisition
// shows it MOVING:
//
//	800AC876 wrote 4       something early sets it to 4
//	800AA8F8 wrote 6       and something later sets it to 6
//	800ADE4A wrote 7       and the resolver ITSELF promotes 6 to 7 when it is next asked
//	                       (800ade42 cmpi s0,6 / 800ade46 lw v0,12(sp) / li s0,7 / sw s0,12(v0))
//
// State 6 maps to code 3 and state 7 to code 4, and the grid draws a row whenever the code is three
// or more. So the screen is not waiting on a field we never send — **it is waiting on a transport
// that had not finished when the probe pressed select.** Every measurement of this screen, on this
// project, going back to the first, opened it after a fixed settle and read the state as 4.
//
// This one waits for the box instead of for a number of instructions: it runs until the transport
// object says 6 or more, asserting rather than hoping, and only then walks to ALL CHANNELS.
//
// **THE ARTEFACT IS THE RESULT.** A hash cannot tell rows from no rows, and this project has filed
// the wrong screen five times; the route is asked for wantChange because a run that succeeds is
// precisely a run whose hash nobody has seen before.
func TestWhetherTheGridDrawsOnceTheTransportIsReady(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		objectAt  = 0x802B2A54
		stateOff  = 12
		wantKind  = 14
		readyFrom = 6 // the first state whose code is three or more
	)
	off := uint32(objectAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if kind := box.RAM.Read(off, bus.Word); kind != wantKind {
		t.Fatalf("harness: %08X holds kind %d, not %d -- the transport object has moved and this "+
			"run would wait on somebody else's memory for ever", uint32(objectAt), kind, wantKind)
	}
	t.Logf("programmes registered; the transport state is %d", state())

	// WAIT FOR THE BOX, NOT FOR A NUMBER. Every previous measurement of this screen used a fixed
	// settle and caught the transport at 4.
	reached := runUntil(t, box, transmitter, 400_000_000, func(int) bool { return state() >= readyFrom })
	if reached < 0 {
		t.Fatalf("harness: the transport never reached state %d in 400 million instructions; it is "+
			"still %d, so this run has nothing to say about a ready transport", readyFrom, state())
	}
	t.Logf("the transport reached state %d after %d further instructions", state(), reached)

	// DID ANYTHING MOVE? The transport being ready is only interesting if the loop that gates on it
	// now behaves differently, so the same two entry points are counted as the screen draws.
	iterations, details := 0, 0
	seenBounds := map[bound]int{}
	codes := map[uint32]int{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		switch a.Virtual &^ 1 {
		case 0x800A4B60:
			iterations++
			st := box.Machine.Core.State()
			seenBounds[bound{index: int32(st.GPR[17]), limit: int32(st.GPR[3])}]++ // #nosec G115
		case 0x800A45C4:
			details++
		case 0x800A4BB0:
			codes[box.Machine.Core.State().GPR[2]]++
		}
	}}
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			iterations, details, watching = 0, 0, true
			seenBounds, codes = map[bound]int{}, map[uint32]int{}
		}
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	grid := openAllChannels(t, press, ".artifacts/grid-with-transport-ready.png", true)
	t.Logf("with the transport ready: %d row-loop iterations, %d channel-detail calls",
		iterations, details)
	for b, n := range seenBounds {
		t.Logf("    loop head: index %d, limit %d, %d times", b.index, b.limit, n)
	}
	for c, n := range codes {
		t.Logf("    the resolver answered code %d, %d times", int32(c), n) // #nosec G115
	}
	if err := dumpScreen(t, box, "grid-with-transport-ready.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the transport state at the draw was %d and the grid drew %08X", state(), grid)
	if grid == 0x42DBD889 {
		t.Logf("VERDICT: the grid is STILL the empty screen even with the transport at %d, so the "+
			"transport state is necessary and not sufficient, and something else refuses as well.",
			state())
		return
	}
	t.Logf("VERDICT: THE GRID DREW A SCREEN NOBODY HAS SEEN ON THIS PROJECT (%08X) with the "+
		"transport at state %d. READ .artifacts/grid-with-transport-ready.png -- rows or no rows "+
		"is a question for the picture and not for this hash.", grid, state())
}
