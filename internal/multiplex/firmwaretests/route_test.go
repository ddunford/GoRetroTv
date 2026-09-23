package firmwaretests_test

import (
	"fmt"
	"testing"
)

// THE ONE ROUTE TO THE ALL CHANNELS GRID, BECAUSE EVERY COPY OF IT HAS BEEN WRONG.
//
// Reaching this screen is a handful of handset presses and it has now produced a wrong measurement
// six times on this project. The first four accepted the BOX OFFICE menu as the tv guide tab,
// because a LEFT from box office draws a screen that is different from the one before it and a
// probe checking only "the screen changed" takes that as arrival; three instruments, one of them
// committed, reported findings about "ALL CHANNELS" from a screenshot that says MOVIES BY START
// TIME.
//
//	"Different from the screen before it" is not "the screen I asked for", and a menu has more
//	than one hash.
//
// THE FIFTH AND SIXTH WERE THE SAME FAULT WEARING THE SETTLE TRAP'S HAT, and they are why this
// file's constants changed on 2026-09-23. A press that stops at the settle detector returns a menu
// caught HALF-PAINTED, so every pin here was a picture of a screen mid-draw:
//
//	0x1CBD8D51 -> 0xFE8D1CCC   the six-entry BOX OFFICE menu
//	0xDDBC18E9 -> 0x43779DC8   the ten-entry TV GUIDE menu, ALL CHANNELS highlighted
//
// The pair 0xDDBC18E9/0x43779DC8 was read as two different states -- this file called the second
// "tvGuideMenuRedrawn" and treated it as proof that a SELECT had failed to land -- and a claim that
// they were one screen was made and withdrawn on the same day, because SELECT appeared to behave
// differently from each. It did, and the reason was WHEN in the paint the press landed rather than
// WHICH screen it landed on. Once every press lets the paint finish (pressAndLetItFinish), the two
// hashes stop being reachable as separate states and the route becomes shorter than it was written:
//
//	box office -> LEFT -> SELECT, photographed at each step 2026-09-23
//
// ONE LEFT, not two. The second LEFT this file used to send was compensating for the first press
// being read mid-paint; with the tail it walks straight past the menu it wanted and on to the next
// tab. Walking four presses proved it: box office, the TV GUIDE menu, a third screen, the SERVICES
// menu, and SELECT there opened SERVICES' own HELP INFORMATION page.
//
// So the route lives here once, pinned at BOTH ends, and every probe calls it instead of writing
// its own presses:
//
//   - the tv guide tab must hash to tvGuideMenuScreen, or the route failed early;
//   - the grid must hash to one of the two frames ALL CHANNELS settles on -- searching, or
//     filled -- or the route failed late.
//
// A pinned destination and a screen that might legitimately CHANGE are in tension, and the tension
// is resolved by saying which is which rather than by loosening the gate: a probe that expects to
// move the grid passes wantChange, and then any settled screen that is neither the menu nor the tab
// is accepted AND the artefact is the proof. A probe that is only trying to observe the grid as it
// stands leaves it false and gets the pin. Loosening the gate for everyone is how the fifth wrong
// measurement happened.
//
// IT ALWAYS DUMPS THE SCREENSHOT. Every one of the six was caught by opening the picture and by
// nothing else -- not by the hash, not by the read counts, not by the verdict text, all of which
// were internally consistent and wrong.
const (
	// All three verified by eye against .artifacts/route-*.png on 2026-09-23, from presses that
	// let the paint finish.
	boxOfficeMenu     = 0xFE8D1CCC // six entries, MOVIES BY START TIME highlighted
	tvGuideMenuScreen = 0x43779DC8 // ten entries, ALL CHANNELS highlighted
	// THE GRID HAS TWO SETTLED FRAMES AND BOTH ARE ARRIVAL. It opens on "Searching for listings"
	// and fills a few tens of millions of instructions later, so which one a press returns depends
	// on that press's budget -- a probe with a generous one sails through the searching frame and
	// settles on the filled grid. Naming both is still a pin; naming one made four probes fail at
	// a screen they had correctly reached.
	//
	// A probe that wants the FILLED grid must not assume it got it: run a tail and photograph the
	// result. The fill is the thing most of this phase is measuring, so its hash is exactly what
	// must be allowed to move.
	allChannelsOpening = 0x144CF59D // "Searching for listings / Please wait"
	allChannelsFilled  = 0x71A6DFE8 // six channels, programmes, continuation arrows
)

// atAllChannels reports whether a settled screen is the ALL CHANNELS grid, at either of the two
// frames it holds still on.
func atAllChannels(screen uint32) bool {
	return screen == allChannelsOpening || screen == allChannelsFilled
}

func openAllChannels(t *testing.T, press pressFunc, artefact string, wantChange bool) uint32 {
	t.Helper()
	seen := []uint32{}
	menu := uint32(0)
	for attempt := 1; attempt <= 4 && menu != boxOfficeMenu; attempt++ {
		menu = press(keyBoxOffice, "box office", 80_000_000)
		seen = append(seen, menu)
	}
	if menu != boxOfficeMenu {
		t.Fatalf("harness: box office drew %08X, not %08X -- the route starts from a screen it "+
			"cannot name, and everything after it would be guesswork. Screens seen: %08X",
			menu, uint32(boxOfficeMenu), seen)
	}
	tab := uint32(0)
	for attempt := 1; attempt <= 4 && tab != tvGuideMenuScreen; attempt++ {
		tab = press(keyLeft, "left to the tv guide tab", 80_000_000)
		seen = append(seen, tab)
	}
	if tab != tvGuideMenuScreen {
		t.Fatalf("harness: never reached the tv guide tab (%08X). Screens seen: %08X. The box "+
			"office menu has been accepted as this tab four times on this project",
			uint32(tvGuideMenuScreen), seen)
	}

	grid := uint32(0)
	for attempt := 1; attempt <= 4; attempt++ {
		grid = press(keySelect, fmt.Sprintf("select ALL CHANNELS (try %d)", attempt), 60_000_000)
		seen = append(seen, grid)
		if atAllChannels(grid) {
			return grid
		}
		if wantChange && grid != 0 && grid != tvGuideMenuScreen && grid != boxOfficeMenu {
			t.Logf("the grid settled on %08X, which is neither frame this project has measured "+
				"ALL CHANNELS on (%08X searching, %08X filled) -- READ %s AND SEE WHAT IT DRAWS",
				grid, uint32(allChannelsOpening), uint32(allChannelsFilled), artefact)
			return grid
		}
	}
	t.Fatalf("harness: select never opened the ALL CHANNELS grid (%08X searching or %08X "+
		"filled). Screens seen: %08X. "+
		"A redraw of the menu is a DIFFERENT hash from the menu, so 'not the tab' is not arrival; "+
		"open %s before believing anything else about this run",
		uint32(allChannelsOpening), uint32(allChannelsFilled), seen, artefact)
	return grid
}

// THE SAME ROUTE FOR EXPERIMENTS THAT CHANGE THE BROADCAST, AND IT IS DELIBERATELY WEAKER.
//
// openAllChannels pins the tv guide tab and that pin has earned its place -- it has caught the
// wrong screen twice since it was written. But it assumes the menu is a CONSTANT, and it is not:
// the guide's screens list channels, so changing what the broadcast says about them changes what
// they draw. Sweeping the line-up flags moved the menu to 0xE84F6057, 0x65BA675D and 0x20DB8CF2
// among others, and the route refused to continue -- correctly, because a screen it cannot name is
// a screen it must not measure.
//
// So an experiment that changes the broadcast gets this instead, and the difference is stated
// rather than hidden: **the tv guide tab is not pinned, only the box office menu is.** That menu
// lists Sky's movie categories and does not depend on the channel line-up at all, so it stays a
// real check on the route's first step whatever the experiment is doing; from there one LEFT
// reaches the tv guide menu and SELECT opens ALL CHANNELS. Every screen on the way is logged and
// the destination is dumped, because with no hash to check the destination against, THE PICTURE IS
// THE ONLY PROOF -- and this project has measured the wrong screen six times.
//
// Use the pinned route wherever the broadcast is the ordinary one. Use this only where it cannot
// be, and read the artefact.
func openAllChannelsUnpinned(t *testing.T, press pressFunc, artefact string) uint32 {
	t.Helper()
	menu := uint32(0)
	seen := []uint32{}
	for attempt := 1; attempt <= 4 && menu != boxOfficeMenu; attempt++ {
		menu = press(keyBoxOffice, "box office", 80_000_000)
		seen = append(seen, menu)
	}
	if menu != boxOfficeMenu {
		t.Fatalf("harness: box office drew %08X, not %08X -- the route starts from a screen it "+
			"cannot name, and everything after it would be guesswork. Screens seen: %08X",
			menu, uint32(boxOfficeMenu), seen)
	}
	tab := press(keyLeft, "left to the tv guide tab", 80_000_000)
	seen = append(seen, tab)
	if tab == 0 || tab == menu {
		t.Fatalf("harness: LEFT from box office drew %08X, so the tab was not reached. Screens "+
			"seen: %08X", tab, seen)
	}
	grid := uint32(0)
	for attempt := 1; attempt <= 4; attempt++ {
		grid = press(keySelect, fmt.Sprintf("select ALL CHANNELS (try %d)", attempt), 60_000_000)
		seen = append(seen, grid)
		if grid != 0 && grid != tab && grid != menu {
			t.Logf("the grid settled on %08X -- READ %s, there is no hash to check it against",
				grid, artefact)
			return grid
		}
	}
	t.Fatalf("harness: select never left the tv guide tab (%08X); screens seen: %08X", tab, seen)
	return grid
}
