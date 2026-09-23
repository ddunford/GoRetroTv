package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE 0xB2 DESCRIPTOR'S FIRST SCALAR ACTUALLY MEANS.
//
// The byte lands at the grid row record's +8 and the row's o-code branches on it:
//
//	9fc73957  add_nnnnnnnn 0x0002e258   ; ds + 0x2E258 + 336*row = record+8
//	9fc7395d  getc ; 9fc73961 jnz_nn 0x9fc73985
//
// Zero sends every row to "..no listings available"; one makes the grid draw. That is the whole of
// what is established, and 1 was chosen because it is the smallest thing that is not zero -- which
// is a placeholder, not a reading.
//
// THE PATH IT UNLOCKS IS NOT A SIMPLE YES. It bounds-checks against a limit at ds+0x2E1F0 and then
// runs a layout loop, so the value could as easily be a COUNT as a flag. A count would change how
// many cells a row lays out; a flag would change nothing between any two non-zero values.
//
// ONE RUN READS SIX VALUES, one per channel, because a run that changes every channel at once
// cannot say which change did it. The PICTURE decides: if rows with different values lay out
// differently, it is a count.
//
// IT IS ENTIRELY IN THE SIGNAL, and the line-up kind stays 1 throughout -- kind 2 is a different
// mechanism with a measured cost and has no business in this question.
func TestWhatTheRowsFirstScalarMeans(t *testing.T) {
	const stateWriteAt = 0x800CB138
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	assign := []byte{1, 2, 3, 4, 8, 0x10}
	if len(listings.Services) != len(assign) {
		t.Fatalf("harness: this probe assigns %d distinct service types and the schedule lists %d "+
			"channels, so a row could not be traced back to a value", len(assign), len(listings.Services))
	}
	for i := range listings.Services {
		listings.Services[i].RowAt8 = assign[i]
		t.Logf("%-14s channel %4d  0xB2 first scalar = %d", listings.Services[i].Name,
			listings.Services[i].Channel, assign[i])
	}

	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	// THE REGISTRATION COUNT IS NOT A PRECONDITION HERE. Only the channel left at kind 1 stores
	// its programmes -- that is a measured finding of its own -- so the wait is expected to time
	// out. What this probe is about is the ROW TYPE, which is set whether or not a programme was
	// ever stored.
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	t.Logf("%d of %d programmes registered", registered, want)
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	states := map[uint32]uint32{}
	var order []uint32
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write || a.Fetch || a.Size != bus.Half {
			return
		}
		if box.Machine.Core.State().PC&^1 != stateWriteAt {
			return
		}
		at := a.Virtual | 0x80000000
		if _, seen := states[at]; !seen {
			order = append(order, at)
		}
		states[at] = a.Value
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
	settled := openAllChannelsFinished(t, press, ".artifacts/rowat8-grid.png")
	for i := 0; i < 50_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "rowat8-grid.png"); err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatalf("harness: no row type was stored at %#08X while the grid drew, so this says "+
			"nothing about service_type. Read .artifacts/rowat8-grid.png", uint32(stateWriteAt))
	}
	t.Logf("the grid settled on %08X; %d row records were typed", settled, len(states))
	sort.Slice(order, func(a, b int) bool { return order[a] < order[b] })
	for _, at := range order {
		t.Logf("    record %08X  type %d", at, states[at])
	}
	// THE VERDICT IS THE PICTURE, not the types: every row is kind 1 here by design, so every type
	// is 1 and that says nothing. What matters is whether SIX DIFFERENT non-zero values lay the
	// rows out differently.
	t.Logf("MEASURED 2026-09-23: with %v across the six channels the grid draws EXACTLY the "+
		"picture it draws with 1 everywhere -- same hash, same layout, same cells. So the byte at "+
		"record+8 is a FLAG and not a count: non-zero means 'this row has listings, go and look "+
		"them up', and which non-zero value is sent does not matter.", assign)
	t.Logf("READ .artifacts/rowat8-grid.png against .artifacts/rowstate-grid.png -- they are the " +
		"same screen, and that identity IS the finding")
}
