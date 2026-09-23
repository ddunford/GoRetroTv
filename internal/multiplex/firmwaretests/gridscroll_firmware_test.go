package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// keyYellow is the handset's yellow key, which the grid's footer offers as "+24 Hours".
const keyYellow = 0x6F

// DOES THE GRID SHOW TOMORROW WHEN YOU SCROLL TO IT, OR ONLY WHEN IT IS ALREADY IN VIEW?
//
// gort-qbn.tomorrowhour was filed as "tomorrow reaches the guide at 23:15 but not at 21:21", and
// that framing is WRONG -- the transmitter is identical at both hours. Measured, at 19:00, 21:21
// and 23:15 alike: the box arms tomorrow's listings PID 0x34, and the transmitter reports
// unaddressed=0, titles=3, derived=3. Tomorrow goes out at the failing hour exactly as it does at
// the passing one, so nothing about the broadcast distinguishes them.
//
// WHAT ACTUALLY DIFFERS IS HOW THE MIDNIGHT COLUMN IS REACHED. At 23:15 the grid opens on 23:00,
// 23:30, 00:00 and the last column is tomorrow WITHOUT ANYONE TOUCHING THE HANDSET -- which is the
// only case gort-k1z ever proved. At 21:21 it opens on today's evening and a viewer must press the
// yellow key to scroll forward, and that is the case that came back empty on the live box:
//
//	after one yellow press   11.30pm  12.00am  12.30am
//	                         Sky One "Jerry Spri.." at 11.30pm -- today's 23:00 programme, correct
//	                         every channel's 12.00am and 12.30am cell "..no listings available"
//	                         including Sky News (Sky News Overnight 00:00) and Sky Movies (Heat 00:00)
//
// So the question is not the hour. It is whether a SCROLLED column draws the day the box holds.
//
// THE EXPERIMENT PUTS THE SCROLL ON BOTH SIDES OF A KNOWN-GOOD CASE. At 23:15 the midnight column
// is read before any key is pressed, which must be populated or gort-k1z's gate has regressed and
// nothing else here means anything; then the same grid is scrolled and read again. At 21:21 only
// the scrolled reading exists, because midnight cannot be in view at that hour.
//
//	23:15 unscrolled   the control, and the one case already proved
//	23:15 scrolled     if this empties, the scroll is the fault and the hour never mattered
//	21:21 scrolled     the live failure, reproduced or not
//
// IT ASSERTS ITS OWN SUBJECT TWICE: the grid must open at both hours, and the 23:15 unscrolled
// reading must differ from the measured empty-midnight screen. Without those two a pair of
// identical hashes would say the scroll changed nothing when in fact nothing was ever drawn.
//
// EVERY READING IS PHOTOGRAPHED, because "the hash changed" is not a finding about a television
// listing and only the picture says which cells carry a programme.
//
// IT ONLY READS.
func TestWhetherTheGridDrawsTomorrowWhenScrolledTo(t *testing.T) {
	// The screen measured with today alone on air, before gort-k1z. A scrolled grid that lands on
	// it has genuinely lost the next day rather than merely looking different.
	const midnightEmpty = 0xFF5F7884

	open := func(t *testing.T, at time.Time, artefact string) (uint32, func(string, string) uint32) {
		t.Helper()
		guide := demoGuide(t)
		dict := demoDictionary(t)
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: at},
			demoSchedule())
		if err != nil {
			t.Fatal(err)
		}
		want := programmesInTheBlock(t, guide, at)
		registered := 0
		if got := runUntil(t, box, transmitter, 120_000_000,
			registeringProgrammes(box, want, &registered)); got < 0 {
			t.Fatalf("harness: only %d of %d programmes registered at %s, so an empty column would "+
				"be about a missed delivery rather than about scrolling",
				registered, want, at.Format("15:04"))
		}
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

		pump := func() error { return transmitter.Pump(box.Machine.Retired) }
		press := azPressFunc(t, box, pump)
		menu := uint32(0)
		for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
			menu = press(keyBoxOffice, "box office", pressBudget)
		}
		if menu != boxOfficeMenu {
			t.Fatalf("harness: %s: box office drew %08X", at.Format("15:04"), menu)
		}
		tab := uint32(0)
		for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
			tab = press(keyLeft, "left to the tv guide menu", pressBudget)
		}
		if tab != tvGuideMenuScreen {
			t.Fatalf("harness: %s: never reached the TV GUIDE menu; drew %08X", at.Format("15:04"), tab)
		}
		grid := uint32(0)
		for attempt := 1; attempt <= attempts && (grid == 0 || grid == tab); attempt++ {
			grid = press(keySelect, "select ALL CHANNELS", pressBudget)
		}
		if grid == 0 || grid == tab {
			t.Fatalf("harness: %s: SELECT never opened the grid from %08X", at.Format("15:04"), tab)
		}
		settle := func() {
			for i := 0; i < 60_000_000; i++ {
				if err := pump(); err != nil {
					t.Fatal(err)
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
		}
		settle()
		if err := dumpScreen(t, box, artefact); err != nil {
			t.Fatal(err)
		}
		opened := screenNow(t, box)
		t.Logf("%s opened the grid on %08X   .artifacts/%s", at.Format("15:04"), opened, artefact)

		// scroll presses the yellow key and lets the grid redraw, returning what it settled on.
		scroll := func(label, shot string) uint32 {
			drew := press(keyYellow, label, pressBudget)
			settle()
			if err := dumpScreen(t, box, shot); err != nil {
				t.Fatal(err)
			}
			drew = screenNow(t, box)
			t.Logf("    %-28s %08X   .artifacts/%s", label, drew, shot)
			return drew
		}
		return opened, scroll
	}

	late := time.Date(1998, 12, 24, 23, 15, 0, 0, time.UTC)
	lateOpen, lateScroll := open(t, late, "gridscroll-2315-open.png")
	if lateOpen == midnightEmpty {
		t.Fatalf("harness: at 23:15 the grid opened on %08X, the screen measured with today alone "+
			"on air -- gort-k1z's gate has regressed and there is no working case to scroll from",
			lateOpen)
	}
	lateScrolled := lateScroll("yellow, +24 Hours", "gridscroll-2315-scrolled.png")

	// AND AT 21:21 IT TAKES MORE THAN ONE PRESS TO REACH MIDNIGHT, which is why a single scroll
	// there proves nothing: one press lands on 10.30pm-11.30pm, still comfortably inside today.
	// The live sequence was two, so this walks forward until the box either shows a column past
	// midnight or refuses to go further, photographing every step.
	early := time.Date(1998, 12, 24, 21, 21, 0, 0, time.UTC)
	_, earlyScroll := open(t, early, "gridscroll-2121-open.png")
	earlyScrolled := uint32(0)
	for step := 1; step <= 4; step++ {
		next := earlyScroll(fmt.Sprintf("yellow, press %d", step),
			fmt.Sprintf("gridscroll-2121-scroll%d.png", step))
		if next == earlyScrolled {
			t.Logf("    press %d changed nothing, so the grid has stopped scrolling here", step)
			break
		}
		earlyScrolled = next
	}

	t.Logf("=== WHAT THE SCROLL DID ===")
	t.Logf("    23:15 opened %08X -> scrolled %08X", lateOpen, lateScrolled)
	t.Logf("    21:21 scrolled %08X", earlyScrolled)
	t.Log("READ THE FOUR PICTURES. The question each one answers is whether a cell past midnight " +
		"carries Sky News Overnight and Heat -- the two programmes the schedule puts there -- or " +
		"says no listings are available. A hash cannot tell those apart and neither can this test.")
	if lateScrolled == lateOpen {
		t.Errorf("the yellow key changed nothing at 23:15 (%08X both times), so this run never "+
			"scrolled and says nothing about scrolling. Check the key: the footer offers +24 Hours "+
			"on yellow, and %#02x is what this probe sends", lateOpen, keyYellow)
	}
}
