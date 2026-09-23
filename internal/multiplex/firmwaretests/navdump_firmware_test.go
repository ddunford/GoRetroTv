package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DUMP EVERY SCREEN ON THE WALK, SO THE ROUTE'S PINS CAN BE RE-DERIVED BY EYE.
//
// The routes in this package pin each stage to a hash that has been SEEN, which is what stops a
// probe measuring the wrong screen -- it has happened five times here. The cost is that a pin is
// only valid for the broadcast it was taken under: the TV GUIDE menu lists what the broadcast says
// about the channels, so changing the schedule changes the menu, and the route then correctly
// refuses to continue rather than quietly measuring something else.
//
// This is the tool for that situation. It presses through the walk and writes a PNG per screen,
// named for the press that produced it, so the new hashes can be read off beside the pictures that
// justify them. It asserts nothing: it is an eye, not a gate.
func TestDumpEveryScreenOnTheAtoZWalk(t *testing.T) {
	if testing.Short() {
		t.Skip("a real box and twenty screenshots")
	}
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

	shot := 0
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		settled := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		// LET IT ACTUALLY FINISH. Four identical samples is ~260k instructions of stillness, and a
		// menu painting under a busier carousel holds a HALF-DRAWN frame still for longer than
		// that -- the TV GUIDE menu was being read mid-paint, with its tab icon still sheared,
		// which gave a hash nobody had seen and a screen whose focus was not where it looked.
		for i := 0; i < 10_000_000; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		if after := screenNow(t, box); after != settled && settled != 0 {
			t.Logf("    (%08X was mid-paint; it finished on %08X)", settled, after)
			settled = after
		}
		if settled == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		shot++
		file := fmt.Sprintf("navdump-%02d-%08X.png", shot, settled)
		if err := dumpScreen(t, box, file); err != nil {
			t.Fatal(err)
		}
		t.Logf("%2d  %-38s %08X -> %08X   %s", shot, name, before, settled, file)
		return settled
	}

	// THE TV GUIDE MENU, BY HASH. Three LEFTs reach it under the current broadcast and four
	// overshoot to SERVICES -- whose menu also responds to DOWN, so a route that counts presses
	// walks the wrong menu and says nothing about it.
	const tvGuideMenu = 0x43779DC8
	press(keyBoxOffice, "box office", 80_000_000)
	tab := uint32(0)
	for attempt := 1; attempt <= 6 && tab != tvGuideMenu; attempt++ {
		if drew := press(keyLeft, fmt.Sprintf("left %d", attempt), 80_000_000); drew != 0 {
			tab = drew
		}
	}
	if tab != tvGuideMenu {
		t.Fatalf("harness: never reached the TV GUIDE menu (%08X) -- read the dumps above and "+
			"re-derive it", uint32(tvGuideMenu))
	}
	// DOWN IS RETRIED, NOT COUNTED: a press that lands while the menu is painting is swallowed.
	// Eight moves from entry 1 reaches entry 9, A-Z LISTINGS.
	moved := 0
	for attempt := 1; attempt <= 40 && moved < 8; attempt++ {
		if press(keyDown, fmt.Sprintf("down (attempt %d, moved %d)", attempt, moved), 20_000_000) != 0 {
			moved++
		}
	}
	if moved < 8 {
		t.Fatalf("harness: the highlight moved only %d of 8 entries", moved)
	}
	for attempt := 1; attempt <= 6; attempt++ {
		if press(keySelect, fmt.Sprintf("select into the category menu (%d)", attempt), 80_000_000) != 0 {
			break
		}
	}
	for attempt := 1; attempt <= 6; attempt++ {
		if press(keySelect, fmt.Sprintf("select into the list (%d)", attempt), 80_000_000) != 0 {
			break
		}
	}
	t.Log("every screen above is written to .artifacts/navdump-NN-HASH.png; read them and pin the " +
		"ones the routes need")
}
