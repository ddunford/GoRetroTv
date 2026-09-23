package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE GRID WITH EVERY CHANNEL AT LINE-UP KIND 2, AND NOTHING POKED.
//
// The chain is now complete and every link is measured:
//
//	the row record's first halfword must be 2 or the grid draws "..no listings available";
//	it is COPIED from an object, which is copied from a six-byte-per-channel table;
//	that table's type field is map(byte at s0+12), built at 0x800BF96C;
//	the map is a plain lookup at 0x800BFA44 -- 1->1, 2->2, 3->4, 4->8, 5->16, 6->512;
//	and s0+12 is THE LINE-UP ENTRY'S KIND BYTE, measured landing there on all six channels.
//
// So the grid draws a programme for a channel whose line-up kind is 2, and this port has sent 1 in
// every feed it has ever transmitted.
//
// AND TYPE 2 IS NOT "THIS CHANNEL HAS LISTINGS". Forcing it drew the text from the 0xB2 descriptor,
// not from the title store -- so it means "the row carries its own content", which is exactly what
// the 0xB2 supplies. That reading is what makes kind 2 plausible rather than a value picked because
// the map said 2.
//
// THE TENSION IS REAL AND IS NOT HIDDEN. A separate sweep measured that ONLY kind 1 stores
// programmes in the title store: with kinds 1..8 across six channels, exactly the kind-1 channel's
// six programmes registered and the others stored nothing. So this run is expected to register
// NOTHING, and if the grid draws anyway it is drawing from the descriptor -- which is the claim.
// What that costs the now-and-next banner and the A-Z screen, which do read the title store, is a
// separate question this probe does not answer and must not be read as answering.
//
// IT IS ENTIRELY IN THE SIGNAL. No memory is written; the only change is one byte per channel in
// the BAT's line-up entry.
func TestTheGridWithEveryChannelAtKindTwo(t *testing.T) {
	const stateWriteAt = 0x800CB138
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	for i := range listings.Services {
		listings.Services[i].Kind = 2
	}
	t.Logf("every one of the %d channels is transmitted with line-up kind 2", len(listings.Services))

	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	// A FIXED WARM-UP, NOT A WAIT FOR REGISTRATION. Waiting for programmes that are never going to
	// arrive burns the whole budget, so the first key lands a hundred and forty million
	// instructions in instead of eighty -- and a press that late is swallowed, which reads exactly
	// like a broken route. The probes that navigate successfully press at about eighty million.
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	runUntil(t, box, transmitter, 60_000_000, registeringProgrammes(box, want, &registered))
	t.Logf("%d of %d programmes registered -- expected to be none, since only kind 1 stores",
		registered, want)
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
	settled := openAllChannels(t, press, ".artifacts/kindtwo-grid.png", true)
	final := settled
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	if err := dumpScreen(t, box, "kindtwo-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid settled on %08X and finished on %08X", settled, final)
	if len(states) == 0 {
		t.Fatalf("harness: no row type was stored while the grid drew, so this says nothing " +
			"about kind 2. Read .artifacts/kindtwo-grid.png")
	}
	sort.Slice(order, func(a, b int) bool { return order[a] < order[b] })
	twos := 0
	t.Logf("=== the type each row record was given ===")
	for _, at := range order {
		note := ""
		if states[at] == 2 {
			note = "   <- 2: the programme draw"
			twos++
		}
		t.Logf("    record %08X  type %d%s", at, states[at], note)
	}
	if twos == 0 {
		t.Errorf("every row is still type 1 with kind 2 on the wire, so the map's 2->2 does not " +
			"reach the row and the chain is wrong somewhere between the line-up entry and the " +
			"table at 0x800BF96C")
		return
	}
	t.Logf("%d of %d rows are TYPE 2, from the broadcast alone with nothing poked. READ "+
		".artifacts/kindtwo-grid.png -- only the picture says whether they drew programmes",
		twos, len(order))
}
