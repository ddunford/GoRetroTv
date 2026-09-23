package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHY THIRTY-ONE PROBES STOPPED REACHING THE TV GUIDE TAB THE DAY THE 0xB2 SHIPPED.
//
// Bisected, not guessed: TestWhoChoosesNoListingsAvailable passes at 7ed8ca2 and fails at 07981cc
// "Broadcast the 0xB2 descriptor", and every failure wears the same sentence --
//
//	left 1 of 2 to the tv guide tab  drew FE8D1CCC
//	left 2 of 2 to the tv guide tab  drew 00000000
//
// -- which pressAndLetItFinish returns when the framebuffer never produced FOUR consecutive
// identical samples that differed from the screen it started on. Across the suite it appeared 65
// times, and zero times in either run before that commit.
//
// TWO VERY DIFFERENT THINGS PRODUCE THAT ZERO and they want opposite work:
//
//   - THE PRESS DID NOT LAND. The box is wedged or swallowing input, the screen never changes, and
//     the 0xB2 broke the firmware. That is a product bug and the descriptor would have to come out.
//   - THE PRESS LANDED AND THE SCREEN WILL NOT HOLD STILL. The menu is drawing, and something on it
//     keeps changing faster than the settle detector's quarter of a million instructions of
//     stillness. Then the instrument is what broke, the picture is fine, and the fix is in the
//     harness rather than the broadcast.
//
// A hash of 00000000 cannot tell them apart and neither can a screenshot taken at one instant, so
// this samples the WHOLE budget and reports the shape of what it sees: how many distinct pictures,
// the longest run of consecutive identical samples, and whether the churn survives cropping the
// animated tab strip away. It dumps the last picture, because on this project the picture has
// caught five wrong readings that the numbers agreed with.
//
// IT ASSERTS ITS OWN SUBJECT: if the screen never changes at all after the press, that is the first
// case and it says so in those words rather than reporting a count.
//
// IT ONLY READS.
func TestWhyTheTVGuideTabStoppedSettling(t *testing.T) {
	const sample = 65_536
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
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		drew := pressAndLetItFinish(t, box, pump, raw, budget)
		t.Logf("%-34s drew %08X", name, drew)
		return drew
	}
	press(keyBoxOffice, "box office", 80_000_000)
	press(keyLeft, "left 1 of 2 to the tv guide tab", 80_000_000)

	// The second LEFT is the one that returns zero. Send it by hand and then WATCH, instead of
	// asking the settle detector for a verdict it has already been shown to get wrong here.
	before := screenNow(t, box)
	beforeBody := screenBodyNow(t, box)
	if err := box.CSI.Key(keyLeft, 0); err != nil {
		t.Fatal(err)
	}
	type run struct {
		hash  uint32
		count int
	}
	var whole, body []run
	add := func(rs *[]run, h uint32) {
		if n := len(*rs); n > 0 && (*rs)[n-1].hash == h {
			(*rs)[n-1].count++
			return
		}
		*rs = append(*rs, run{hash: h, count: 1})
	}
	const budget = 80_000_000
	for i := 0; i < budget; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
		if i%sample != 0 {
			continue
		}
		add(&whole, screenNow(t, box))
		add(&body, screenBodyNow(t, box))
	}
	if err := dumpScreen(t, box, "menusettle-tab.png"); err != nil {
		t.Fatal(err)
	}

	summarise := func(name string, rs []run, start uint32) (distinct, longest int) {
		seen := map[uint32]bool{}
		for _, r := range rs {
			seen[r.hash] = true
			if r.count > longest {
				longest = r.count
			}
		}
		distinct = len(seen)
		t.Logf("%-10s %d samples in %d runs, %d distinct pictures, longest still run %d samples "+
			"(%d instructions); started on %08X", name, func() int {
			n := 0
			for _, r := range rs {
				n += r.count
			}
			return n
		}(), len(rs), distinct, longest, longest*sample, start)
		return distinct, longest
	}
	wholeDistinct, wholeLongest := summarise("whole", whole, before)
	bodyDistinct, bodyLongest := summarise("body", body, beforeBody)
	for i, r := range whole {
		if i >= 12 {
			t.Logf("    ... %d more runs", len(whole)-i)
			break
		}
		t.Logf("    whole run %2d  %08X x%d", i, r.hash, r.count)
	}

	if wholeDistinct == 1 && whole[0].hash == before {
		t.Fatalf("THE PRESS DID NOT LAND: %d million instructions after LEFT the box is still "+
			"showing %08X and has drawn nothing else. That is the product, not the harness -- the "+
			"0xB2 descriptor would have to come out. Read .artifacts/menusettle-tab.png",
			budget/1_000_000, before)
	}
	const settleNeeds = 4 // what pressAndLetItFinish asks for
	t.Logf("MEASURED 2026-09-23: the press LANDED -- the box drew %d distinct pictures after it. "+
		"The whole-frame hash held still for at most %d consecutive samples against the %d "+
		"pressAndLetItFinish requires, so the settle detector never fires and reports 00000000 for "+
		"a screen that is drawing perfectly well.", wholeDistinct, wholeLongest, settleNeeds)
	if bodyLongest > wholeLongest {
		t.Logf("CROPPING THE TAB STRIP FIXES IT: below row %d the picture held still for %d "+
			"consecutive samples across %d distinct bodies, so the churn is in the animated band "+
			"above the menu and screenBodyNow is the hash this route wants.",
			screenBodyTop, bodyLongest, bodyDistinct)
	} else {
		t.Logf("CROPPING THE TAB STRIP DOES NOT FIX IT: the body churned as hard as the whole frame "+
			"(%d still samples across %d distinct bodies), so what keeps changing is the menu's own "+
			"CONTENT and not the icon band. Read .artifacts/menusettle-tab.png before choosing a "+
			"fix -- a longer budget would not help either.", bodyLongest, bodyDistinct)
	}
}
