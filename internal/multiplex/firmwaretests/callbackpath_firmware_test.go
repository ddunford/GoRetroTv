package firmwaretests_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH EXIT THE GRID'S ROW CALLBACK TAKES, AND WHAT IT ASKED FOR FIRST.
//
// Ghidra decompiles the callback and its opening is a gate:
//
//	if (*(short *)(*(int *)(param_2 + 8) + 4) != 0x10) {
//	  iVar1 = (*DAT_800cba38)(param_3, 0x5f, &local_9c, ...);   a query by TAG 0x5F
//	  if ((iVar1 != 3) && (iVar1 != 4))  return DAT_800cba7c;   -2, "abandon this row"
//	  if (local_9c == 0)                 return DAT_800cbd70;   -1
//	  iVar1 = (*DAT_800cba38)(local_8c, 0xb2, &local_10, ...);  a query by TAG 0xB2
//	  if ((iVar1 != 3) && (iVar1 != 4))  return DAT_800cba7c;   -2
//
// The constants are read off the literal pool: `DAT_800cba7c` is 0xFFFFFFFE (-2), `DAT_800cbd70` is
// 0xFFFFFFFF (-1), and the query is `0x800AD608`. **The callback answers -1 on all six calls**, and
// the only -1 in the part read so far is the `local_9c == 0` arm — the one where the tag query
// SUCCEEDS and comes back empty.
//
// **That is an inference and this project does not ship those.** The function is long and its later
// half has not been read; another -1 further down would look identical from outside. Twice today a
// reading taken from a listing was wrong in exactly this way — the resolver's literal pool said
// -5 and the box returned 2, and a probe took a `jalr` for a return. So this TRACES one complete
// invocation, from the callback's first instruction until control is back at its caller, and prints
// the path. A trace has no opinion about which arm looked likely.
//
// It also captures the ARGUMENTS of every tag query the callback makes — the tag in a1 and the
// answer written back — because "which tag did it ask for and what did it get" is the thing that
// turns a branch into a fix.
//
// IT ASSERTS ITS OWN SUBJECT: the callback must be entered and must return, or the trace is of
// nothing.
//
// IT ONLY READS.
func TestWhichExitTheRowCallbackTakes(t *testing.T) {
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
		readyFrom   = 6

		rowCallback = 0x800CB7B8
		tagQuery    = 0x800AD608 // DAT_800cba38, the (object, tag, out, ...) lookup
		traceLimit  = 6000
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d", state())

	type query struct {
		tag, out uint32
	}
	var (
		watching bool
		tracing  bool
		done     bool
		returnTo uint32
		trace    []uint32
		queries  []query
		answer   uint32
		entered  int
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		at := a.Virtual &^ 1
		if at == rowCallback {
			entered++
			if !done {
				st := box.Machine.Core.State()
				returnTo = st.GPR[31] &^ 1
				tracing, trace, queries = true, trace[:0], queries[:0]
			}
			return
		}
		if !tracing {
			return
		}
		// Every tag lookup the callback makes: a1 is the tag, a2 the address it writes the answer
		// to. Recorded on the way in; the answer is read when the trace ends.
		if at == tagQuery {
			st := box.Machine.Core.State()
			queries = append(queries, query{tag: st.GPR[5], out: st.GPR[6]})
		}
		trace = append(trace, at)
		if at == returnTo {
			answer = box.Machine.Core.State().GPR[2]
			tracing, done = false, true
			return
		}
		if len(trace) >= traceLimit {
			tracing, done = false, true
		}
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching, tracing, done, entered = true, false, false, 0
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	grid := openAllChannels(t, press, ".artifacts/callback-path-grid.png", false)
	if err := dumpScreen(t, box, "callback-path-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid drew %08X; the callback was entered %d times", grid, entered)
	if entered == 0 || len(trace) == 0 {
		t.Fatal("harness: the row callback was never entered or never traced, so there is no path " +
			"below and its silence is the instrument")
	}
	if !done {
		t.Fatalf("harness: the callback never returned to %08X within %d instructions, so the exit "+
			"below is a cap rather than a return", returnTo, traceLimit)
	}

	t.Logf("the first invocation ran %d instructions and answered %08X (%d)",
		len(trace), answer, int32(answer)) // #nosec G115
	t.Logf("=== the tag lookups it made, in order ===")
	if len(queries) == 0 {
		t.Logf("    NONE -- it returned before asking for anything, so the gate is the type check " +
			"at the top (*(short *)(*(int *)(param_2+8)+4) != 0x10) and not a missing descriptor.")
	}
	for i, q := range queries {
		out := uint32(0)
		if q.out >= 0x80000000 && (q.out&0x1fffffff)+4 <= box.RAM.Size() {
			out = box.RAM.Read(q.out&0x1fffffff, bus.Word)
		}
		t.Logf("    %d: tag %#04x -> wrote %08X at %08X", i+1, q.tag, out, q.out)
	}

	t.Logf("=== the path, in order (read it against the decompilation) ===")
	var line []string
	for i, at := range trace {
		line = append(line, hexPC(at))
		if len(line) == 10 || i == len(trace)-1 {
			t.Logf("    %s", strings.Join(line, " "))
			line = line[:0]
		}
	}
	t.Logf("=== the distinct addresses it touched inside the callback ===")
	inside := map[uint32]int{}
	for _, at := range trace {
		if at >= rowCallback && at < rowCallback+0x600 {
			inside[at]++
		}
	}
	keys := make([]uint32, 0, len(inside))
	for at := range inside {
		keys = append(keys, at)
	}
	sort.Slice(keys, func(a, b int) bool { return keys[a] < keys[b] })
	for _, at := range keys {
		t.Logf("    %08X  %d", at, inside[at])
	}
}

// IS TAG 0x5F EVER NON-ZERO IN THIS BOX?
//
// The grid's row callback makes exactly one lookup — tag `0x5F` on the object it was handed — gets
// **zero**, and returns -1 without drawing. Traced, not inferred: 580 instructions, one query, and
// it never reaches the `0xB2` lookup below it.
//
// That is a precise finding and it raises exactly one question before anything is built on it:
// **is that attribute ever non-zero here, on anything?** Two answers, and they point opposite ways.
//
//   - If the working banner asks for `0x5F` and gets a real value, the attribute CAN be set, the
//     grid is asking about an object that lacks it, and the work is to find what sets it.
//   - If NOTHING in this box ever has a non-zero `0x5F`, then nothing we have ever broadcast
//     populates it, and the work is in the signal after all — which would be the first evidence
//     pointing back that way since the store measurement.
//
// So this censuses every call to the lookup at `0x800AD608` on both screens: which tags are asked
// for, how often, and how often the answer is non-zero. `0x5F` is a DVB descriptor tag — the
// private data specifier, which this project's BAT does transmit — but whether the firmware means
// the same thing by the number is exactly what must not be assumed.
//
// IT ONLY READS.
func TestWhetherTagLookupsEverAnswerNonZero(t *testing.T) {
	for _, screen := range []struct {
		name   string
		banner bool
	}{{name: "the now-and-next banner (it works)", banner: true},
		{name: "the ALL CHANNELS grid", banner: false}} {
		t.Run(screen.name, func(t *testing.T) { censusTagLookups(t, screen.banner) })
	}
}

func censusTagLookups(t *testing.T, wantBanner bool) {
	t.Helper()
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
		readyFrom   = 6
		tagQuery    = 0x800AD608
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	type tally struct{ asked, nonZero int }
	tags := map[uint32]*tally{}
	pending := map[uint32]uint32{} // out address -> tag, read back once the call has returned
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		if a.Virtual&^1 != tagQuery {
			return
		}
		st := box.Machine.Core.State()
		tag, out := st.GPR[5], st.GPR[6]
		e := tags[tag]
		if e == nil {
			e = &tally{}
			tags[tag] = e
		}
		e.asked++
		pending[out] = tag
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect || raw == keySky {
			tags, pending, watching = map[uint32]*tally{}, map[uint32]uint32{}, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	if wantBanner {
		if drew := press(keySky, "sky (banner)", 60_000_000); drew == 0 {
			t.Fatal("harness: the banner drew nothing")
		}
	} else {
		grid := openAllChannels(t, press, ".artifacts/tag-census-grid.png", false)
		t.Logf("the grid drew %08X", grid)
	}

	// The answers are read where the calls asked them to be written. A value still zero after the
	// screen has finished is a lookup that came back empty.
	for out, tag := range pending {
		if out < 0x80000000 || (out&0x1fffffff)+4 > box.RAM.Size() {
			continue
		}
		if box.RAM.Read(out&0x1fffffff, bus.Word) != 0 {
			tags[tag].nonZero++
		}
	}
	if len(tags) == 0 {
		t.Fatalf("harness: the lookup at %08X was never called while this screen drew, so its "+
			"census is empty for a reason that has nothing to do with the screen", uint32(tagQuery))
	}
	keys := make([]uint32, 0, len(tags))
	for tag := range tags {
		keys = append(keys, tag)
	}
	sort.Slice(keys, func(a, b int) bool { return tags[keys[a]].asked > tags[keys[b]].asked })
	t.Logf("=== tag lookups while this screen drew ===")
	for _, tag := range keys {
		note := ""
		if tag == 0x5f {
			note = "   <- the tag the grid's row callback gives up on"
		}
		t.Logf("    tag %#04x  asked %4d  answered non-zero at %d of its out-addresses%s",
			tag, tags[tag].asked, tags[tag].nonZero, note)
	}
}
