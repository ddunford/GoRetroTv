package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT HOLDS THE NUMBER OF ROWS A LIST SCREEN IS ABOUT TO DRAW?
//
// The ALL CHANNELS grid draws no rows, and the open question is the one gort-qxl.2 states: does the
// row loop RUN and draw nothing, or never run at all? Those are different faults with the same
// blank screen and they want opposite fixes, and no amount of watching the grid alone separates
// them -- a loop over zero channels and a missing loop both read nothing.
//
// **BUT THIS BOX DRAWS TWO OTHER LIST SCREENS, AND THEY HAVE DIFFERENT LENGTHS.** The BOX OFFICE
// menu has six entries and the TV GUIDE menu has ten. So the row count is not a thing to be guessed
// at: it is a value that must read SIX while one of them draws and TEN while the other does, and an
// address that does both is the row count by behaviour rather than by assumption. Then the grid can
// simply be asked.
//
// THAT IS THE WHOLE INSTRUMENT, and its control is built in. A candidate must track the count
// across two screens; an address that merely happens to hold 6 somewhere is excluded by having to
// hold 10 somewhere else, and vice versa. With 500,000 reads per draw, "held the right number
// twice" is a far stronger filter than any single screen could give.
//
// IT ONLY READS, and every screen it visits is pinned by a hash verified against a dumped picture,
// because three instruments in this project have measured the wrong screen while reporting
// confidently about this one.
func TestWhatHoldsTheRowCountOnAListScreen(t *testing.T) {
	const boxOfficeMenu = 0xFE8D1CCC // six entries, verified by eye
	const tvGuideMenu = 0xDDBC18E9   // ten entries, verified by eye
	const allChannels = 0x42DBD889   // the grid, no rows, verified by eye
	const boxOfficeRows, tvGuideRows = 6, 10

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

	// seen is, per draw, the distinct values each DRAM address returned. Capped per address so one
	// busy counter cannot dominate the map.
	type valueSet map[uint32]map[uint32]bool
	record := valueSet{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Fetch || a.Write || a.Virtual < 0x80000000 || a.Virtual >= 0xA0000000 {
			return
		}
		at := a.Virtual & 0x1fffffff
		vs := record[at]
		if vs == nil {
			vs = map[uint32]bool{}
			record[at] = vs
		}
		if len(vs) < 8 {
			vs[a.Value] = true
		}
	}}

	press := func(raw uint8, name string, budget int) (uint32, valueSet) {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		record = valueSet{}
		watching = true
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
		watching = false
		t.Logf("%-30s drew %08X over %d addresses", name, settled, len(record))
		return settled, record
	}

	// Box office first -- six entries.
	screen, sixes := press(keyBoxOffice, "box office (six entries)", 80_000_000)
	for attempt := 1; attempt <= 4 && screen != boxOfficeMenu; attempt++ {
		screen, sixes = press(keyBoxOffice, "box office, again", 80_000_000)
	}
	if screen != boxOfficeMenu {
		t.Fatalf("harness: box office settled on %08X, not the six-entry menu %08X", screen,
			uint32(boxOfficeMenu))
	}
	// Then the TV GUIDE menu -- ten entries.
	var tens valueSet
	for attempt := 1; attempt <= 6 && screen != tvGuideMenu; attempt++ {
		runUntil(t, box, transmitter, 8_000_000, func(int) bool { return false })
		screen, tens = press(keyLeft, "left to the tv guide menu (ten entries)", 80_000_000)
	}
	if screen != tvGuideMenu {
		t.Fatalf("harness: never reached the ten-entry TV GUIDE menu (%08X); settled on %08X",
			uint32(tvGuideMenu), screen)
	}
	// Then the grid.
	var gridReads valueSet
	for attempt := 1; attempt <= 6 && screen != allChannels; attempt++ {
		runUntil(t, box, transmitter, 8_000_000, func(int) bool { return false })
		screen, gridReads = press(keySelect,
			fmt.Sprintf("select ALL CHANNELS (try %d)", attempt), 60_000_000)
	}
	if screen != allChannels {
		t.Fatalf("harness: never reached ALL CHANNELS (%08X); settled on %08X", uint32(allChannels),
			screen)
	}

	// THE CANDIDATES, ON A LADDER OF STRICTNESS RATHER THAN ONE RULE. The strictest version --
	// saw six on the six-entry menu, ten on the ten-entry one, and neither value on the other
	// screen -- returned nothing at all, and a single empty answer does not say whether the idea
	// is wrong or the rule was. A counter that counts UP to six passes through values that also
	// appear elsewhere, so "never held the other number" excludes the very thing being looked for.
	// The looser rungs are reported beside it so the reader can see which assumption did the work.
	sawBoth := func(strict bool) []uint32 {
		var out []uint32
		for at, vs := range sixes {
			if !vs[boxOfficeRows] {
				continue
			}
			other := tens[at]
			if other == nil || !other[tvGuideRows] {
				continue
			}
			if strict && (vs[tvGuideRows] || other[boxOfficeRows]) {
				continue
			}
			out = append(out, at)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}
	strictly, loosely := sawBoth(true), sawBoth(false)
	t.Logf("addresses that read %d on the six-entry menu and %d on the ten-entry one: %d strictly "+
		"(neither number on the other screen), %d loosely", boxOfficeRows, tvGuideRows,
		len(strictly), len(loosely))

	candidates := strictly
	if len(candidates) == 0 {
		candidates = loosely
	}
	if len(candidates) == 0 {
		t.Log("VERDICT: NO address held the row count on either rung. Either these menus do not " +
			"share the machinery the grid uses, or the count never reaches DRAM at all -- a value " +
			"kept in a register for the life of a loop is invisible to a read watch. Neither is a " +
			"finding about the grid, and saying so is the result.")
		return
	}
	var zero, missing int
	for i, at := range candidates {
		vs := gridReads[at]
		switch {
		case vs == nil:
			missing++
		case vs[0]:
			zero++
		}
		if i < 24 {
			t.Logf("    %08X  six-entry %s  ten-entry %s  GRID %s", 0x80000000|at,
				describeValues(sixes[at]), describeValues(tens[at]), describeValues(vs))
		}
	}
	if len(candidates) > 24 {
		t.Logf("    ... and %d more", len(candidates)-24)
	}
	t.Logf("VERDICT: of %d row-count candidates, the grid read %d as ZERO and never read %d at "+
		"all. A count of zero says the loop RAN over an empty list; never reading it says the loop "+
		"was never reached. That is gort-qxl.2's question and this is the shape of its answer.",
		len(candidates), zero, missing)
}

// describeValues renders the values an address returned during one draw.
func describeValues(vs map[uint32]bool) string {
	if vs == nil {
		return "NOTHING -- not read at all during the grid draw"
	}
	out := make([]uint32, 0, len(vs))
	for v := range vs {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return fmt.Sprintf("%v", out)
}
