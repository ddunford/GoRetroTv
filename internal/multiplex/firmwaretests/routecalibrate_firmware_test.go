package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE ROUTE TO ALL CHANNELS, RE-MEASURED ONCE THE PRESSES STOPPED READING MID-PAINT.
//
// route_test.go pins three hashes it calls the box office menu, the tv guide tab and the tab
// redrawn. All three were measured by a press that stopped at the settle detector, which this
// package has now proved returns a HALF-PAINTED screen under a busy carousel -- so all three are
// pictures of a menu caught partway through drawing, and they moved the moment the 0xB2 descriptor
// gave the box more to paint.
//
// Re-pinning them from the same kind of reading would buy one more week. This walks the route with
// the press that lets the paint finish, DUMPS A PICTURE AT EVERY STEP, and reports what each key
// actually reaches, so the pins can be written from what the screens are rather than from what the
// sequence implies they ought to be. On this project the picture has caught five wrong readings
// that the numbers agreed with, which is why there is one per press and not one at the end.
//
// IT ASSERTS ITS OWN SUBJECT: if a press draws nothing, the walk stops and says so, because a route
// measured through a swallowed press is how the wrong screen gets pinned in the first place.
//
// IT ONLY READS.
func TestWhatTheRouteToAllChannelsActuallyReaches(t *testing.T) {
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

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	step := 0
	walk := func(raw uint8, name string) uint32 {
		t.Helper()
		step++
		drew := pressAndLetItFinish(t, box, pump, raw, 80_000_000)
		shot := fmt.Sprintf("route-%d-%s.png", step, name)
		if err := dumpScreen(t, box, shot); err != nil {
			t.Fatal(err)
		}
		if drew == 0 {
			t.Fatalf("step %d (%s) drew nothing in 80 million instructions even with the paint "+
				"tail, so the walk stopped here. Read .artifacts/%s", step, name, shot)
		}
		t.Logf("step %d  %-22s -> %08X   .artifacts/%s", step, name, drew, shot)
		return drew
	}

	// MEASURED 2026-09-23, and it is one LEFT and not two. Walking box office -> LEFT -> LEFT ->
	// LEFT -> SELECT photographed the BOX OFFICE menu, the ten-entry TV GUIDE menu, a third screen,
	// the SERVICES menu and finally SERVICES' own HELP INFORMATION page. So the TV GUIDE menu is
	// ONE LEFT from box office; route_test.go's second LEFT was compensating for the first press
	// being read mid-paint, and with the paint tail it walks straight past the menu it wanted.
	seen := make([]uint32, 0, 4)
	seen = append(seen, walk(keyBoxOffice, "boxoffice"), walk(keyLeft, "left1"))
	seen = append(seen, walk(keySelect, "select"))
	// THE GRID OPENS ON "SEARCHING FOR LISTINGS" and fills afterwards, so the screen a press
	// returns is not the screen a probe wants to measure. Give it time and photograph both.
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "route-4-grid-filled.png"); err != nil {
		t.Fatal(err)
	}
	filled := screenNow(t, box)
	t.Logf("sixty million instructions later the grid is %08X   .artifacts/route-4-grid-filled.png",
		filled)
	seen = append(seen, filled)
	t.Logf("the walk reached %08X", seen)
	t.Log("READ THE THREE PNGs IN ORDER -- the pins in route_test.go are whichever of these hashes " +
		"the pictures show to be the box office menu, the tv guide menu and the ALL CHANNELS grid")
}
