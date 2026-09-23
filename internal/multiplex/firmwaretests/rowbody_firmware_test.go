package firmwaretests_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE SECOND REFUSAL: WHAT THE ROW BODY DOES NOW THAT IT RUNS SIX TIMES.
//
// The grid's first gate is open. Its row loop used to run once and leave, because the transport
// record it asks about was still in state 4 and 0x800AC534 maps 4 to the code 2 that means "go
// away". Waiting for the transport to reach 6 -- which it does about fifty-six million instructions
// after the last programme registers -- changes that completely: the loop now runs indices 0 to 5
// against a limit of 6, and the resolver answers code 4 every time.
//
// **And the screen is still byte for byte 42DBD889.** So something inside the body refuses as well,
// and the body is where the answer now is:
//
//	800a4bb2  cmpi  v1,2
//	800a4bb4  bteqz 0x800a4d8d        code == 2 -> leave (this no longer happens)
//	800a4bb8  slti  v1,2
//	800a4bba  btnez 0x800a4dad        code <  2 -> leave
//	800a4bbe  li    v1,7              <- THE BODY, which now executes, six times
//
// This traces ONE COMPLETE ITERATION: every instruction the box fetches from the first time it
// reaches 0x800A4BBE until it arrives back at the loop head at 0x800A4B60. Read against the
// listing, that names every branch the body took and every function it called, and the fork where a
// row stops becoming a row is somewhere in it.
//
// **TRACING BEATS READING HERE AND THE REASON IS SPECIFIC.** The exit from this same loop was found
// by tracing after reading the function had produced a plausible wrong answer: the literal pool
// said the resolver returns 0xFFFFFFFB and the box returned 2, because the arm that actually ran
// was not the arm that looked like the exit. A trace has no opinion.
//
// IT ASSERTS ITS OWN SUBJECT twice over: the transport must reach the ready state, or this measures
// the old one-iteration behaviour and calls it the body; and the body must be entered, or the trace
// is empty for a reason that has nothing to do with rows.
//
// IT ONLY READS.
func TestWhatTheGridsRowBodyDoesWithTheTransportReady(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		wantKind    = 14
		readyFrom   = 6

		loopHead   = 0x800A4B60
		bodyStart  = 0x800A4BBE
		traceLimit = 4000
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if kind := box.RAM.Read(off, bus.Word); kind != wantKind {
		t.Fatalf("harness: %08X holds kind %d, not %d -- the transport object has moved",
			uint32(transportAt), kind, wantKind)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d, so the row body "+
			"would not run and this trace would be of the old behaviour", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d", state())

	var (
		watching bool
		tracing  bool
		done     bool
		trace    []uint32
		bodies   int
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		at := a.Virtual &^ 1
		if at == bodyStart {
			bodies++
			if !done && !tracing {
				tracing, trace = true, trace[:0]
			}
		}
		if !tracing {
			return
		}
		trace = append(trace, at)
		// One complete iteration ends where the next one is decided.
		if at == loopHead && len(trace) > 1 {
			tracing, done = false, true
			return
		}
		if len(trace) >= traceLimit {
			tracing, done = false, true
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching, tracing, done, bodies = true, false, false, 0
			trace = trace[:0]
		}
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	grid := openAllChannels(t, press, ".artifacts/row-body-all-channels.png", true)
	if err := dumpScreen(t, box, "row-body-all-channels.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid drew %08X with the transport at state %d", grid, state())

	if bodies == 0 {
		t.Fatalf("harness: the row body at %08X never executed even with the transport ready, so "+
			"this trace is empty for a reason that has nothing to do with drawing a row",
			uint32(bodyStart))
	}
	t.Logf("the row body executed %d times; the first iteration fetched %d instructions:",
		bodies, len(trace))

	// Printed in order, eight to a line, so it can be read straight down beside the listing. The
	// SECOND pass below collapses it to the distinct functions it visited, which is what actually
	// gets read first -- a run of consecutive addresses is one basic block and says nothing.
	var line []string
	for i, at := range trace {
		line = append(line, hexPC(at))
		if len(line) == 8 || i == len(trace)-1 {
			t.Logf("    %s", strings.Join(line, " "))
			line = line[:0]
		}
	}

	t.Logf("the iteration entered these regions, in order (a jump of more than 64 bytes is a call " +
		"or a branch worth reading):")
	prev := uint32(0)
	for _, at := range trace {
		if prev != 0 && (at > prev+64 || prev > at+64) {
			t.Logf("    %s -> %s", hexPC(prev), hexPC(at))
		}
		prev = at
	}
}

// DOES IT FILL IF YOU WAIT? The body is not bailing out.
//
// With the transport ready the row body runs six times and does REAL WORK each time -- the first
// iteration alone fetches more than four thousand instructions, through the same subsystem at
// 0x800D0F00..0x800D3BC0 the resolver walks, and out into 0x800AC400..0x800AF100. That is not a
// loop giving up; that is a loop building six rows.
//
// And the screen is byte for byte the empty grid. Two readings survive that, and they want
// different work: the rows are computed and DRAW NOTHING, or they are computed and DRAWN LATER than
// anybody has looked. Every measurement of this screen on this project, including all of today's,
// has stopped at a settle -- four samples 65,536 instructions apart with no change -- and a settle
// is a statement about the last quarter of a million instructions, not about the screen.
//
// That is the same mistake as the transport state wearing different clothes: waiting for a NUMBER
// instead of for the BOX. So this presses select and then keeps the box running for two hundred
// million instructions, sampling as it goes, and reports every distinct screen it sees.
//
// IT ASSERTS ITS OWN SUBJECT: the transport must be ready and the grid must be the pinned empty
// screen at the start, or a change afterwards would not be a change in the grid.
func TestWhetherTheGridFillsIfYouKeepWatching(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		wantKind    = 14
		readyFrom   = 6
		emptyGrid   = 0x42DBD889
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if kind := box.RAM.Read(off, bus.Word); kind != wantKind {
		t.Fatalf("harness: %08X holds kind %d, not %d -- the transport object has moved",
			uint32(transportAt), kind, wantKind)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d", state())

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		settled := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	grid := openAllChannels(t, press, ".artifacts/grid-kept-watching.png", true)
	if grid != emptyGrid {
		t.Fatalf("harness: the grid opened on %08X and not the pinned empty screen %08X, so a "+
			"change while watching would not be a change in the grid", grid, uint32(emptyGrid))
	}
	if err := dumpScreen(t, box, "grid-kept-watching-at-open.png"); err != nil {
		t.Fatal(err)
	}

	// KEEP WATCHING. Two hundred million instructions is roughly three and a half times the gap
	// between the last programme registering and the transport becoming ready, so it is a wait on
	// the same scale as the one that mattered rather than a round number.
	seen := []uint32{grid}
	last := grid
	for i := 0; i < 200_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 != 0 {
			continue
		}
		if now := screenNow(t, box); now != last {
			t.Logf("  at +%3dM instructions the screen became %08X", i/1_000_000, now)
			seen = append(seen, now)
			last = now
		}
	}
	if err := dumpScreen(t, box, "grid-kept-watching.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the transport is state %d and the screen is %08X after watching", state(), last)

	if len(seen) == 1 {
		t.Logf("VERDICT: the grid NEVER CHANGED in 200 million instructions after it opened. The "+
			"rows are computed and draw nothing; waiting is not the answer and the six iterations "+
			"produce no pixels. Screen throughout: %08X.", grid)
		return
	}
	t.Logf("VERDICT: THE SCREEN CHANGED %d times after the grid opened, ending on %08X. Every "+
		"measurement of this screen on this project stopped at a settle and would have missed "+
		"this. READ .artifacts/grid-kept-watching.png.", len(seen)-1, last)
}
