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
		// THE MENU'S SECOND HASH -- AND IT IS THE SAME SCREEN, WHICH IS WHY SELECT KEPT NOT
		// LANDING. Corrected 2026-09-22, by picture.
		//
		// 0xDDBC18E9 above is this menu MID-PAINT, with its tab icon still sheared. 0x43779DC8 is
		// the same menu FINISHED. Neither is a redraw and neither is a failure: the settle
		// detector calls a screen done after four identical samples 65,536 instructions apart --
		// about a quarter of a million instructions of stillness -- and a menu painting under a
		// busy carousel holds a HALF-DRAWN frame still for longer than that.
		//
		// So a route that waits for 0xDDBC18E9 presses SELECT into a screen that is still drawing,
		// which is exactly when a press is swallowed, and then sees 0x43779DC8 and concludes the
		// select did not land. It did not, and the route is why. The fix is to let the paint finish
		// before reading the screen -- pressAndLetItFinish in rununtil_test.go, which runs a tail
		// after the settle; with it the whole box-office-to-A-Z walk runs with no swallowed press
		// at all. **This route should be moved onto it**, and its two pins re-derived as finished
		// frames at the same time; it is left alone here only because doing that blind would
		// re-pin the grid route on hashes nobody has looked at.
		tvGuideMenuRedrawn = 0x43779DC8
	)
	// STOP AT EITHER HASH, BECAUSE THEY ARE THE SAME SCREEN. The loop used to run until it saw
	// the mid-paint frame specifically, and would press straight past the finished menu looking
	// for it -- so the moment the carousel got busy enough that the paint completed inside the
	// press budget, this route walked off the tv guide tab entirely and reported it unreachable.
	// Which of the two frames a run happens to catch is a property of how loaded the box is, and
	// no route should depend on that.
	arrived := func(s uint32) bool { return s == tvGuideMenuScreen || s == tvGuideMenuRedrawn }
	menu := press(keyBoxOffice, "box office", 80_000_000)
	tab := menu
	seen := []uint32{menu}
	for attempt := 1; attempt <= 6 && !arrived(tab); attempt++ {
		tab = press(keyLeft, "left to the tv guide tab", 80_000_000)
		seen = append(seen, tab)
	}
	if arrived(tab) {
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

// THE SAME ROUTE FOR EXPERIMENTS THAT CHANGE THE BROADCAST, AND IT IS DELIBERATELY WEAKER.
//
// `openAllChannels` pins the tv guide tab to 0xDDBC18E9 and that pin has earned its place -- it has
// caught the wrong screen twice since it was written. But it assumes the menu is a CONSTANT, and it
// is not: the menu lists channels, so changing what the broadcast says about them changes the menu
// too. Sweeping the line-up flags moved it to 0xE84F6057, 0x65BA675D and 0x20DB8CF2 among others,
// and the route refused to continue -- correctly, because a screen it cannot name is a screen it
// must not measure.
//
// So an experiment that changes the broadcast gets this instead, and the difference is stated
// rather than hidden: **it trusts the press count where the pinned route trusts a hash.** Two LEFTs
// from the box office menu reach the tv guide tab, and select opens ALL CHANNELS. Every screen on
// the way is logged and the destination is dumped, because with no hash to check against, THE
// PICTURE IS THE ONLY PROOF -- and this project has measured the wrong screen five times.
//
// Use the pinned route wherever the broadcast is the ordinary one. Use this only where it cannot
// be, and read the artefact.
func openAllChannelsUnpinned(t *testing.T, press pressFunc, artefact string) uint32 {
	t.Helper()
	const boxOfficeMenu = 0x1CBD8D51 // this one does NOT depend on the channel list
	menu := press(keyBoxOffice, "box office", 80_000_000)
	if menu != boxOfficeMenu {
		t.Fatalf("harness: box office drew %08X, not %08X -- the route starts from a screen it "+
			"cannot name, and everything after it would be guesswork", menu, uint32(boxOfficeMenu))
	}
	var seen []uint32
	for i := 1; i <= 2; i++ {
		seen = append(seen, press(keyLeft, fmt.Sprintf("left %d of 2 to the tv guide tab", i), 80_000_000))
	}
	tab := seen[len(seen)-1]
	if tab == 0 || tab == menu {
		t.Fatalf("harness: two LEFTs from box office drew %08X, so the tab was not reached", tab)
	}
	grid := uint32(0)
	for attempt := 1; attempt <= 6; attempt++ {
		grid = press(keySelect, fmt.Sprintf("select ALL CHANNELS (try %d)", attempt), 60_000_000)
		if grid != 0 && grid != tab && grid != menu {
			t.Logf("the grid settled on %08X -- READ %s, there is no hash to check it against",
				grid, artefact)
			return grid
		}
	}
	t.Fatalf("harness: select never left the tv guide tab (%08X); screens seen: %08X", tab, seen)
	return grid
}
