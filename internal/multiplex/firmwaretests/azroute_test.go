package firmwaretests_test

import (
	"fmt"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
)

// THE ONE ROUTE TO ALL PROGRAMMES A-Z, PINNED AT EVERY STAGE.
//
// Reaching this screen is a handful of presses and counting them does not work. A press that lands
// while a menu is painting is swallowed, so the first version counted screen CHANGES instead --
// and how many changes a press produces depends on how busy the box is. Shortening the delivery
// that ran before the walk was enough to move the highlight: one run opened PERSONAL PLANNER and
// drew "There are no programmes in your Personal Planner", which is exactly what an index that
// failed would look like. Another reached a menu nobody has named.
//
// So nothing here counts anything. Every stage is driven to a HASH THAT HAS BEEN SEEN, and the
// walk refuses to continue from a screen it cannot name -- the same rule, and for the same reason,
// as the ALL CHANNELS route in route_test.go, which exists because that grid was measured wrong
// six times. The two files share the screens they both pass through: the box office menu and the
// ten-entry TV GUIDE menu are named once, in route_test.go, and this walk turns off at A-Z
// LISTINGS where that route carries on to ALL CHANNELS.
//
// THE DESTINATION IS THE ONE THING NOT PINNED, and deliberately. ALL PROGRAMMES A-Z opens EMPTY
// when no index has been delivered and opens ALREADY FILLED when one resolved, so its hash is the
// thing under test; pinning it cost a run. The screen BEFORE it is pinned instead -- the A-Z
// category menu, eight fixed entries the box draws from its own resources, identical on every run
// whatever was delivered -- and ALL PROGRAMMES is its highlighted first entry, so one SELECT from
// there lands where we mean. The artefact is always dumped, because the picture is the only proof.
const (
	azHighlighted  = 0xF77F97B8 // the TV GUIDE menu with A-Z LISTINGS highlighted
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

	menu := note(press(keyBoxOffice, "box office", pressBudget))
	tab := menu
	for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
		if drew := press(keyLeft, fmt.Sprintf("left to the tv guide tab (%d)", attempt), pressBudget); drew != 0 {
			tab = note(drew)
		}
	}
	if tab != tvGuideMenuScreen {
		t.Fatalf("harness: never reached the TV GUIDE menu (%08X); screens seen: %08X",
			uint32(tvGuideMenuScreen), seen)
	}

	// DOWN UNTIL A-Z LISTINGS IS HIGHLIGHTED, not down nine times. A swallowed press costs an
	// iteration and nothing else, where a miscounted one changes which screen is measured.
	row := tab
	for attempt := 1; attempt <= 30 && row != azHighlighted; attempt++ {
		// A SMALLER CAP THAN THE REST OF THE WALK, on purpose: a DOWN moves a highlight and
		// nothing else, so it settles in a fraction of what a menu change needs -- and this loop
		// runs up to thirty times, so a swallowed press burning the full budget thirty times over
		// would dominate the probe.
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
	for attempt := 1; attempt <= attempts && categories == 0; attempt++ {
		categories = press(keySelect, fmt.Sprintf("select A-Z LISTINGS (%d)", attempt), pressBudget)
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
	for attempt := 1; attempt <= attempts && inner == 0; attempt++ {
		inner = press(keySelect, fmt.Sprintf("select ALL PROGRAMMES (%d)", attempt), pressBudget)
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
