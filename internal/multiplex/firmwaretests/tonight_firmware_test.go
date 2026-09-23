package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// TOMORROW ARRIVES WHEN TONIGHT RUNS OUT.
//
// gort-k1z carries its own stop-gate and this is it: MEASURE BEFORE BUILDING. The reasoning is
// that the guide registers TWO notification slots -- the block its clock is in and the one after
// it -- and at 19:00 the second reads day+1 block 0, because the block after 18:00-24:00 belongs
// to tomorrow. This transmitter broadcasts the day its clock is in and nothing else, so that
// second subscription is never answered.
//
// WHAT THAT PREDICTS IS SPECIFIC, WHICH IS WHY IT CAN BE TESTED. At 23:15 Sky One is showing
// "Midnight Mass", which runs 23:00 to midnight and has NOTHING after it in the day's schedule --
// so the banner's NEXT line can only be filled from tomorrow. If it is empty, the subscription
// really is unanswered and the task is worth building. If the box fills it anyway, it has its own
// route to the next day and the task is not.
//
// THE GRID IS READ AT THE SAME HOUR for the same reason: its three columns at 23:15 are 23:00,
// 23:30 and 00:00, and that third column is tomorrow whatever the banner does.
//
// MEASURED BEFORE THE FIX, WHICH IS WHY THIS PROBE EXISTS IN THIS SHAPE. With today only on air:
//
//	the banner   NOW Midnight Mass / "Further schedule information is not available"
//	the grid     columns 11.00pm / 11.30pm / 12.00am, every midnight cell EMPTY
//
// The transmitter now answers the next day's subscription too, and the grid is the proof: the
// midnight column fills for EXACTLY the channels whose schedule has something there. Sky News runs
// "Sky News Overnight" from 00:00 and Sky Movies runs "Heat"; the other four have nothing before
// 06:00 and their midnight cells stay empty, which is the correct answer rather than a failure.
//
// THE BANNER IS READ ON A CHANNEL THAT HAS A NEXT PROGRAMME, and that is not a detail. The box
// wakes tuned to Sky One, whose schedule genuinely ends at midnight -- so its banner says "further
// schedule information is not available" both before and after this fix, and a probe that read
// only that surface would report no change from a change that plainly works.
//
// IT ASSERTS ITS OWN SUBJECT: the evening block must be fully registered, or an empty midnight
// column would be about a missed delivery rather than about the next day.
//
// IT ONLY READS.
func TestWhatIsOnNextWhenTonightRunsOut(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	// 23:15 is inside Midnight Mass, which is the day's last programme on Sky One. Far enough in
	// that the banner is unambiguous, and still in the 18:00-24:00 block the guide subscribed to.
	night := time.Date(1998, 12, 24, 23, 15, 0, 0, time.UTC)

	listings := guide.On(night)
	var onAir, after string
	for _, s := range listings.Services {
		if s.Name != "Sky One" {
			continue
		}
		for _, p := range s.Programmes {
			switch {
			case p.Start == "23:00":
				onAir = p.Title
			case p.Start > "23:00":
				after = p.Title
			}
		}
	}
	if onAir == "" {
		t.Fatalf("harness: the schedule has nothing starting at 23:00 on Sky One, so this probe "+
			"is not measuring the case it was written for. Programmes: %d",
			len(listings.Services))
	}
	t.Logf("at 23:15 Sky One is showing %q, and the day's schedule has %q after it",
		onAir, after)
	if after != "" {
		t.Fatalf("harness: %q follows it TODAY, so the banner's NEXT can be filled without "+
			"tomorrow and this run would say nothing about the next-day subscription", after)
	}

	box := restoredBox(t)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: night},
		demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	want := programmesInTheBlock(t, guide, night)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered for the 18:00-24:00 block, so the "+
			"box has less than the evening and an empty NEXT would not be about tomorrow",
			registered, want)
	}
	t.Logf("%d of %d programmes registered for the evening block", registered, want)
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := azPressFunc(t, box, pump)

	// THE GRID FIRST, because its third column at this hour is tomorrow and it names every channel
	// at once.
	menu := uint32(0)
	for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
		menu = press(keyBoxOffice, "box office", pressBudget)
	}
	if menu != boxOfficeMenu {
		t.Fatalf("harness: box office drew %08X, not %08X", menu, uint32(boxOfficeMenu))
	}
	tab := uint32(0)
	for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
		tab = press(keyLeft, fmt.Sprintf("left to the tv guide menu (%d)", attempt), pressBudget)
	}
	if tab != tvGuideMenuScreen {
		t.Fatalf("harness: never reached the TV GUIDE menu; drew %08X", tab)
	}
	grid := uint32(0)
	for attempt := 1; attempt <= attempts && (grid == 0 || grid == tab); attempt++ {
		grid = press(0x01, fmt.Sprintf("ALL CHANNELS (%d)", attempt), pressBudget)
	}
	if grid == 0 || grid == tab {
		t.Fatalf("harness: ALL CHANNELS never opened from the TV GUIDE menu (%08X)", tab)
	}
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "tonight-grid.png"); err != nil {
		t.Fatal(err)
	}
	filled := screenNow(t, box)
	// The grid with NOTHING after midnight, measured before the next day went on air. It is named
	// rather than described because "the grid changed" is not a finding and "it is no longer the
	// screen that had an empty midnight column" is.
	const midnightEmpty = 0xFF5F7884
	t.Logf("the grid settled on %08X -- READ .artifacts/tonight-grid.png: its columns are 23:00, "+
		"23:30 and 00:00, and the last of those is tomorrow", filled)
	if filled == midnightEmpty {
		t.Fatalf("the grid is still %08X, the screen measured with today alone on air -- every "+
			"midnight cell empty. The next day's sections are not reaching the box", filled)
	}

	// AND THE BANNER, ON A CHANNEL THAT HAS SOMETHING AFTER MIDNIGHT. Sky Movies is the fourth row
	// and runs "Heat" from 00:00; Sky One, which the box wakes tuned to, genuinely has nothing
	// until 06:00 and would say so however well this worked.
	for i := 0; i < 3; i++ {
		press(keyDown, fmt.Sprintf("down to Sky Movies (%d of 3)", i+1), 20_000_000)
	}
	viewing := uint32(0)
	for attempt := 1; attempt <= attempts && (viewing == 0 || viewing == filled); attempt++ {
		viewing = press(keySelect, fmt.Sprintf("select to view (%d)", attempt), pressBudget)
	}
	if viewing == 0 || viewing == filled {
		t.Fatalf("harness: SELECT never left the grid (%08X), so no channel was tuned and there "+
			"is no banner to read", filled)
	}
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "tonight-banner.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the tuned channel's banner drew %08X -- READ .artifacts/tonight-banner.png: it must "+
		"name the channel the picture shows, and its NEXT line is what tomorrow bought",
		screenNow(t, box))
}
