package firmwaretests_test

import (
	"fmt"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
)

// THE ONE ROUTE TO ALL PROGRAMMES A-Z, PINNED AT EVERY STAGE.
//
// Reaching this screen is box office, two LEFTs, nine DOWNs and two SELECTs, and counting those
// presses does not work. A press that lands while a menu is painting is swallowed, so the first
// version counted screen CHANGES instead -- and how many changes a press produces depends on how
// busy the box is. Shortening the delivery that ran before the walk was enough to move the
// highlight: one run opened PERSONAL PLANNER and drew "There are no programmes in your Personal
// Planner", which is exactly what an index that failed would look like. Another reached a menu
// nobody has named.
//
// So nothing here counts anything. Every stage is driven to a HASH THAT HAS BEEN SEEN, and the
// walk refuses to continue from a screen it cannot name -- the same rule, and for the same reason,
// as the ALL CHANNELS route in route_test.go, which exists because that grid was measured wrong
// five times.
//
// EVERY HASH HERE IS A FINISHED SCREEN, which is the other half of why the older pins failed.
// 0xDDBC18E9 -- what route_test.go pins as this menu -- is the SAME menu MID-PAINT, with its tab
// icon still sheared; 0x43779DC8, which that file names "tvGuideMenuRedrawn" and reads as evidence
// that a SELECT did not land, is the finished article. The pictures say so. A route that waits for
// the mid-paint frame presses SELECT into a screen that is still drawing, which is exactly when a
// press is swallowed, and then reads the finished menu as a failure. Every press here goes through
// pressAndLetItFinish for that reason, and with it the whole walk runs with no swallowed press at
// all.
//
// THE DESTINATION IS THE ONE THING NOT PINNED, and deliberately. ALL PROGRAMMES A-Z opens EMPTY
// when no index has been delivered and opens ALREADY FILLED when one resolved, so its hash is the
// thing under test; pinning it cost a run. The screen BEFORE it is pinned instead -- the A-Z
// category menu, eight fixed entries the box draws from its own resources, identical on every run
// whatever was delivered -- and ALL PROGRAMMES is its highlighted first entry, so one SELECT from
// there lands where we mean. The artefact is always dumped, because the picture is the only proof.
const (
	azBoxOffice    = 0xFE8D1CCC // the box office menu, FINISHED
	azTVGuideMenu  = 0x43779DC8 // the ten-entry TV GUIDE menu, ALL CHANNELS highlighted, FINISHED
	azHighlighted  = 0xF77F97B8 // the same menu with A-Z LISTINGS highlighted
	azCategoryMenu = 0x63270B3D // ALL PROGRAMMES / ENTERTAINMENT / MOVIES / ... eight entries

	// keyDown is the handset's DOWN. It is proved rather than assumed: lessons.md records that
	// only four codes here were ever tested against a screen, and this is one of the four -- it
	// moves the TV GUIDE menu's highlight, which is visible in every route above.
	keyDown = 0x59
)

// openAllProgrammesAtoZ walks to ALL PROGRAMMES A-Z and returns the screen it opened on.
func openAllProgrammesAtoZ(t *testing.T, press pressFunc, artefact string) uint32 {
	t.Helper()
	seen := []uint32{}
	note := func(s uint32) uint32 { seen = append(seen, s); return s }

	menu := note(press(keyBoxOffice, "box office", 80_000_000))
	tab := menu
	for attempt := 1; attempt <= 6 && tab != azTVGuideMenu; attempt++ {
		if drew := press(keyLeft, fmt.Sprintf("left to the tv guide tab (%d)", attempt), 80_000_000); drew != 0 {
			tab = note(drew)
		}
	}
	if tab != azTVGuideMenu {
		t.Fatalf("harness: never reached the TV GUIDE menu (%08X); screens seen: %08X",
			uint32(azTVGuideMenu), seen)
	}

	// DOWN UNTIL A-Z LISTINGS IS HIGHLIGHTED, not down nine times. A swallowed press costs an
	// iteration and nothing else, where a miscounted one changes which screen is measured.
	row := tab
	for attempt := 1; attempt <= 30 && row != azHighlighted; attempt++ {
		if moved := press(keyDown, fmt.Sprintf("down to A-Z LISTINGS (%d)", attempt), 20_000_000); moved != 0 {
			row = note(moved)
		}
	}
	if row != azHighlighted {
		t.Fatalf("harness: A-Z LISTINGS was never highlighted (%08X) in thirty DOWN presses; "+
			"screens seen: %08X", uint32(azHighlighted), seen)
	}

	// ONE SELECT EACH, RETRIED ONLY WHEN SWALLOWED -- not "press until the hash matches".
	//
	// The source screen is pinned and picture-verified, so ONE select from it lands on the A-Z
	// category menu by construction. Pinning the DESTINATION as well looked safer and was not: the
	// category menu's hash is not stable under a live carousel, so the loop kept pressing SELECT
	// and dived two screens deeper than it meant to, reporting four hashes nobody has seen. A
	// press that is swallowed returns zero and costs a retry; a press that lands must be trusted,
	// because the thing that guarantees it is the pin behind it.
	categories := uint32(0)
	for attempt := 1; attempt <= 6 && categories == 0; attempt++ {
		categories = press(keySelect, fmt.Sprintf("select A-Z LISTINGS (%d)", attempt), 80_000_000)
	}
	if categories == 0 || categories == azHighlighted {
		t.Fatalf("harness: SELECT never left the TV GUIDE menu; screens seen: %08X", seen)
	}
	if categories != azCategoryMenu {
		t.Logf("the A-Z category menu drew %08X rather than the %08X seen before -- it lists eight "+
			"fixed entries, so this is a repaint rather than a different screen, but READ %s",
			categories, uint32(azCategoryMenu), artefact)
	}
	note(categories)

	inner := uint32(0)
	for attempt := 1; attempt <= 6 && inner == 0; attempt++ {
		inner = press(keySelect, fmt.Sprintf("select ALL PROGRAMMES (%d)", attempt), 80_000_000)
	}
	if inner == 0 || inner == categories {
		t.Fatalf("harness: SELECT never left the A-Z category menu (%08X); screens seen: %08X",
			categories, seen)
	}
	t.Logf("ALL PROGRAMMES A-Z opened on %08X -- READ %s, it is the only thing that says what it "+
		"drew", inner, artefact)
	return inner
}

// azPressFunc is the press every walk in this package should use: settle, then LET THE PAINT
// FINISH. See pressAndLetItFinish for what that fixes and what it cost to find.
func azPressFunc(t *testing.T, box *board.Runtime, pump func() error) pressFunc {
	return func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		drew := pressAndLetItFinish(t, box, pump, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
}

// openAllChannelsFinished is the route to the ALL CHANNELS grid, pinned on a FINISHED frame.
//
// route_test.go's version pins the TV GUIDE menu to 0xDDBC18E9, which is that menu MID-PAINT, and
// pins the box office menu to a hash a finished paint does not produce either. Both were taken with
// the settle detector, which calls a screen done after about a quarter of a million instructions of
// stillness -- less than a menu painting under a busy carousel holds a half-drawn frame. That is
// why SELECT "kept not landing" there: the route pressed into a screen that was still drawing.
//
// This one pins the single screen that has been verified by picture -- the ten-entry TV GUIDE menu,
// finished, with ALL CHANNELS highlighted as entry 1 -- and takes ONE select from it. The
// destination is deliberately not pinned: the grid filling is the thing under test, so its hash is
// exactly what must be allowed to change. The artefact is the proof, as it has been for all five
// wrong measurements this project has made of this screen.
func openAllChannelsFinished(t *testing.T, press pressFunc, artefact string) uint32 {
	t.Helper()
	var seen []uint32
	note := func(s uint32) uint32 { seen = append(seen, s); return s }

	if drew := press(keyBoxOffice, "box office", 80_000_000); drew != 0 {
		note(drew)
	}
	tab := uint32(0)
	for attempt := 1; attempt <= 8 && tab != azTVGuideMenu; attempt++ {
		if drew := press(keyLeft, fmt.Sprintf("left to the tv guide tab (%d)", attempt), 80_000_000); drew != 0 {
			tab = note(drew)
		}
	}
	if tab != azTVGuideMenu {
		t.Fatalf("harness: never reached the finished TV GUIDE menu (%08X); screens seen: %08X. "+
			"Every hash here is a FINISHED frame -- if the menu has genuinely changed, re-derive "+
			"it with TestDumpEveryScreenOnTheAtoZWalk and look at the pictures",
			uint32(azTVGuideMenu), seen)
	}
	grid := uint32(0)
	for attempt := 1; attempt <= 6 && grid == 0; attempt++ {
		grid = press(keySelect, fmt.Sprintf("select ALL CHANNELS (%d)", attempt), 80_000_000)
	}
	if grid == 0 || grid == tab {
		t.Fatalf("harness: SELECT never left the TV GUIDE menu (%08X); screens seen: %08X", tab, seen)
	}
	t.Logf("ALL CHANNELS opened on %08X -- READ %s, it is the only thing that says what it drew",
		grid, artefact)
	return grid
}
