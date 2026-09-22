package firmwaretests_test

import (
	"fmt"
	"testing"
)

// THE ONE ROUTE TO THE ALL CHANNELS GRID, BECAUSE EVERY COPY OF IT HAS BEEN WRONG.
//
// Reaching this screen is four handset presses and it has now produced a wrong measurement five
// times on this project. The first four accepted the BOX OFFICE menu as the tv guide tab, because a
// LEFT from box office draws a screen that is different from the one before it and a probe checking
// only "the screen changed" takes that as arrival; three instruments, one of them committed,
// reported findings about "ALL CHANNELS" from a screenshot that says MOVIES BY START TIME.
//
// The fifth is this file's reason for existing, and it defeated the guard the first four taught us.
// The probe pinned the tv guide tab by hash, reached it correctly, pressed select, got a screen
// that was NOT the tab's hash, and measured it: 0x43779DC8, reported as the grid, with a clean
// "the grid never asks the channel database for a channel -- 0 calls against the banner's 5". The
// artefact says it is **the ten-entry TV GUIDE MENU**. Select had not landed at all and the menu
// had simply redrawn, and a redraw of the same screen is a different hash.
//
//	"Different from the screen before it" is not "the screen I asked for", and the menu has more
//	than one hash.
//
// So the route lives here once, pinned at BOTH ends, and every probe calls it instead of writing
// its own four presses:
//
//   - the tv guide tab must hash to tvGuideMenuScreen, or the route failed early;
//   - the grid must hash to allChannelsEmpty, the screen this project has measured on every
//     successful run, or the route failed late.
//
// A pinned destination and a screen that might legitimately CHANGE are in tension, and the tension
// is resolved by saying which is which rather than by loosening the gate: a probe that expects to
// move the grid passes wantChange, and then any settled screen that is neither the menu nor the tab
// is accepted AND the artefact is the proof. A probe that is only trying to observe the grid as it
// stands leaves it false and gets the pin. Loosening the gate for everyone is how the fifth wrong
// measurement happened.
//
// IT ALWAYS DUMPS THE SCREENSHOT. Every one of the five was caught by opening the picture and by
// nothing else -- not by the hash, not by the read counts, not by the verdict text, all of which
// were internally consistent and wrong.
func openAllChannels(t *testing.T, press pressFunc, artefact string, wantChange bool) uint32 {
	t.Helper()
	const (
		// Both verified by eye against their artefacts rather than inferred from a change.
		tvGuideMenuScreen = 0xDDBC18E9 // the ten-entry TV GUIDE menu, ALL CHANNELS highlighted
		allChannelsEmpty  = 0x42DBD889 // "ALL CHANNELS / Today 7.00pm 7.30pm 8.00pm", no rows
		// THE MENU'S SECOND HASH. A select that does not land leaves the ten-entry menu on screen
		// and it settles on this instead of on tvGuideMenuScreen. Verified by eye TWICE, from two
		// different probes' artefacts, both of which had reported it as the grid. It is listed
		// here because "not the tab" is the check that let it through, and a screen known not to
		// be arrival should be named rather than re-derived by whoever opens the next PNG.
		tvGuideMenuRedrawn = 0x43779DC8
	)
	menu := press(keyBoxOffice, "box office", 80_000_000)
	tab := menu
	seen := []uint32{menu}
	for attempt := 1; attempt <= 6 && tab != tvGuideMenuScreen; attempt++ {
		tab = press(keyLeft, "left to the tv guide tab", 80_000_000)
		seen = append(seen, tab)
	}
	if tab == tvGuideMenuRedrawn {
		tab = tvGuideMenuScreen
	}
	if tab != tvGuideMenuScreen {
		t.Fatalf("harness: never reached the tv guide tab (%08X). Screens seen: %08X. The box "+
			"office menu has been accepted as this tab four times on this project",
			uint32(tvGuideMenuScreen), seen)
	}

	grid := uint32(0)
	for attempt := 1; attempt <= 6; attempt++ {
		grid = press(keySelect, fmt.Sprintf("select ALL CHANNELS (try %d)", attempt), 60_000_000)
		seen = append(seen, grid)
		if grid == allChannelsEmpty {
			return grid
		}
		if wantChange && grid != 0 && grid != tvGuideMenuScreen &&
			grid != tvGuideMenuRedrawn && grid != menu {
			t.Logf("the grid settled on %08X, which is not the screen this project has measured "+
				"every time before (%08X) -- READ %s AND SEE WHAT IT DRAWS",
				grid, uint32(allChannelsEmpty), artefact)
			return grid
		}
	}
	t.Fatalf("harness: select never opened the ALL CHANNELS grid (%08X). Screens seen: %08X. "+
		"A redraw of the menu is a DIFFERENT hash from the menu, so 'not the tab' is not arrival; "+
		"open %s before believing anything else about this run",
		uint32(allChannelsEmpty), seen, artefact)
	return grid
}
