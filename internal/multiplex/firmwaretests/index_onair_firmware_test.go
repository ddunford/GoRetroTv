package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// A-Z LISTINGS OFF THE BROADCAST ITSELF, WITH NOTHING PUSHED BY HAND.
//
// Every earlier probe of table 0xC1 pushed its sections directly at the demux, which proves the box
// accepts them and proves nothing at all about the product: a viewer opening the demo is fed by the
// carousel, and a carousel that does not carry the index leaves A-Z LISTINGS saying "Searching for
// listings" for ever no matter what a test can do by hand.
//
// So this one presses keys and pushes NOTHING. The transmitter builds the index from the same
// schedule file the titles come from, stamps each letter with the initial of the programmes under
// it, and the carousel repeats it -- which is the only thing that can work, because the parser
// hands its array to whatever already sits in the list-head slot and a viewer chooses their own
// moment to open the screen.
//
// THE ARRIVAL SCREEN IS PINNED. The nine DOWNs to A-Z LISTINGS count screen CHANGES, and how many
// changes a press produces depends on how busy the box is -- shortening an earlier delivery moved
// the highlight and opened PERSONAL PLANNER instead, which drew a perfectly plausible "There are
// no programmes in your Personal Planner" and would have been read as the index failing.
func TestAtoZListingsFillsFromTheCarouselAlone(t *testing.T) {
	const headsPool = 0x800C51A0
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoScheduleWithIndex())
	if err != nil {
		t.Fatal(err)
	}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	// LET A WHOLE ALPHABET GO ROUND BEFORE WALKING. ALL PROGRAMMES A-Z is not a per-letter page:
	// it is every programme merged and sorted, so a list head filled for 'A' alone leaves it still
	// searching. One wave carries one letter at IndexPeriod, so a cycle of the nineteen letters
	// this schedule uses takes about forty million instructions.
	runUntil(t, box, transmitter, 80_000_000, func(int) bool { return false })

	// DID THE CAROUSEL ACTUALLY CARRY IT? If not, everything below measures the transmitter.
	if counts := transmitter.Counts(); counts.Index == 0 {
		t.Fatalf("harness: the carousel has sent %d title waves and NOT ONE index wave, so the "+
			"screen below would be empty for want of a broadcast rather than for want of a "+
			"feature", counts.Titles)
	}
	word := func(virtual uint32) uint32 { return box.RAM.Read(virtual&0x1fffffff, bus.Word) }
	heads := word(headsPool)
	if heads&0xf0000000 == 0 || heads&3 != 0 {
		t.Fatalf("harness: the list-head table pointer is %08X, not a guest address", heads)
	}
	filled := 0
	for letter := byte('A'); letter <= 'Z'; letter++ {
		if word(heads+uint32(letter-0x40)*4) != 0 {
			filled++
		}
	}
	t.Logf("the carousel has sent %d index waves; %d of 26 letter slots hold a head",
		transmitter.Counts().Index, filled)
	// EVERY LETTER, NOT JUST THE ONES WITH PROGRAMMES. This is the whole finding: a six-channel
	// schedule leaves seven letters empty, and while those heads were null the screen drew
	// "Searching for listings" for ever with the other nineteen filled. A letter with nothing on
	// it and a letter the box has never heard of are different states, and it waits for the
	// second to become the first.
	if filled != 26 {
		t.Fatalf("only %d of 26 letter list heads are filled. The screen waits for ALL of them: "+
			"with nineteen filled and seven null it searched for ever, and filling the empty "+
			"letters with zero-record sections is what made it draw", filled)
	}

	press := azPressFunc(t, box, func() error { return transmitter.Pump(box.Machine.Retired) })

	inner := openAllProgrammesAtoZ(t, press, ".artifacts/onair-az-listings.png")

	final := inner
	for i := 0; i < 60_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	if err := dumpScreen(t, box, "onair-az-listings.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("ALL PROGRAMMES A-Z opened on %08X and finished on %08X, fed only by the carousel",
		inner, final)
	if final == inner {
		t.Errorf("the screen never left the one it opened on (%08X), which is the "+
			"'Searching for listings' state -- the carousel carried %d index waves and filled "+
			"%d letter slots, so the sections arrived and the screen still drew nothing",
			inner, transmitter.Counts().Index, filled)
	}
	t.Logf("READ .artifacts/onair-az-listings.png -- only the picture says it is programmes")
}
