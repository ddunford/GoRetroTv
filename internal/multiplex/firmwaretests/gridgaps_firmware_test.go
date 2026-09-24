package firmwaretests_test

import (
	"fmt"
	"image"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT PUTS THE BLANK ROWS IN THE ALL CHANNELS GRID.
//
// The grid draws the demo's six channels with a BLANK ROW SLOT between four of the five adjacent
// pairs. Sky One and Sky Soap sit together; Sky Travel, Sky Movies, Sky Sports 1 and Sky News each
// get a gap above them. Measured off the row plates: the pitch is 32 pixels between the first pair
// and 64 -- exactly two slots -- between the rest.
//
// TWO EXPLANATIONS FIT THE SHIPPED DATA PERFECTLY AND IT IS HOLDING BOTH, which is the whole reason
// this probe exists rather than a patch:
//
//	genre groups      101 g3, 121 g3 | 251 g1 | 301 g6 | 401 g7 | 501 g5   -> 4 changes
//	channel jumps     101 ->121 is +20; +130, +50, +100, +100 after that   -> 4 big jumps
//
// Four of each, in the same four places. This project has lost a week to exactly this shape before
// -- a symptom that correlated with the variable already in hand, chased through that variable,
// when the answer was the one nobody was varying. So neither is believed until one of them moves
// the gaps.
//
// THE EXPERIMENT IS ONE VARIABLE AT A TIME, from the same fixture:
//
//	stock         the shipped schedule, which must show the gaps or there is nothing to explain
//	one genre     every channel's genre set to 3, CHANNEL NUMBERS UNTOUCHED
//	contiguous    channels renumbered 101..106, GENRES UNTOUCHED
//
// Whichever run flattens the grid names the cause. If both flatten it, they are not independent
// here and the probe says so rather than picking one. If neither does, both stories are wrong and
// the gaps come from something this probe has not thought of -- which is a finding too, and a more
// useful one than a fix built on a guess.
//
// IT ASSERTS ITS OWN SUBJECT: the stock run must reproduce the four gaps. A control that draws a
// flat grid means the fixture, the hour or the route changed under this probe, and every conclusion
// below would be about a screen nobody has looked at.
//
// IT ONLY READS the machine. The schedule it broadcasts is built in memory from the shipped one;
// nothing on disk is touched and nothing is poked into the guest.
func TestWhatPutsTheBlankRowsInTheGrid(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	run := func(t *testing.T, name string, adjust func(*multiplex.Listings), artefact string) []plate {
		t.Helper()
		guide := demoGuide(t)
		listings := guide.On(day)
		if len(listings.Services) < 3 {
			t.Fatalf("harness: the fixture has %d services, too few for a gap to exist between "+
				"any of them", len(listings.Services))
		}
		if adjust != nil {
			adjust(listings)
		}
		shape := make([]string, 0, len(listings.Services))
		for _, s := range listings.Services {
			shape = append(shape, fmt.Sprintf("%d/g%d", s.Channel, s.Genre))
		}
		t.Logf("%-12s %v", name, shape)

		dict := demoDictionary(t)
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			demoSchedule())
		if err != nil {
			t.Fatal(err)
		}
		want := programmesInTheBlock(t, guide, day)
		registered := 0
		if at := runUntil(t, box, transmitter, 120_000_000,
			registeringProgrammes(box, want, &registered)); at < 0 {
			t.Fatalf("harness: %s registered only %d of %d programmes, so a short grid would be "+
				"about a missed delivery rather than about its layout", name, registered, want)
		}
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

		pump := func() error { return transmitter.Pump(box.Machine.Retired) }
		press := azPressFunc(t, box, pump)
		menu := uint32(0)
		for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
			menu = press(keyBoxOffice, "box office", pressBudget)
		}
		if menu != boxOfficeMenu {
			t.Fatalf("harness: %s: box office drew %08X, not %08X", name, menu, uint32(boxOfficeMenu))
		}
		tab := uint32(0)
		for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
			tab = press(keyLeft, "left to the tv guide menu", pressBudget)
		}
		if tab != tvGuideMenuScreen {
			t.Fatalf("harness: %s: never reached the TV GUIDE menu; drew %08X", name, tab)
		}
		grid := uint32(0)
		for attempt := 1; attempt <= attempts && (grid == 0 || grid == tab); attempt++ {
			grid = press(keySelect, "select ALL CHANNELS", pressBudget)
		}
		if grid == 0 || grid == tab {
			t.Fatalf("harness: %s: SELECT never opened the grid from %08X", name, tab)
		}
		// The grid opens on "Searching for listings" and fills tens of millions of instructions
		// later, so a row count taken at the press is of a screen that has not drawn its rows.
		for i := 0; i < 60_000_000; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		if err := dumpScreen(t, box, artefact); err != nil {
			t.Fatal(err)
		}
		plates := channelRowStarts(t, box)
		t.Logf("%-12s %08X  %d rows at y=%v  pitches=%v  gaps=%d   .artifacts/%s",
			name, screenNow(t, box), len(plates), tops(plates), pitches(plates),
			countGaps(plates), artefact)
		return plates
	}

	stock := run(t, "stock", nil, "gridgaps-stock.png")
	if got := countGaps(stock); got == 0 {
		t.Fatalf("harness: the stock schedule drew a FLAT grid (rows at %v). This probe exists to "+
			"explain gaps that are not there, so the fixture, the hour or the route has changed "+
			"under it and nothing below would be about the screen this was written for", tops(stock))
	}

	oneGenre := run(t, "one genre", func(l *multiplex.Listings) {
		for i := range l.Services {
			l.Services[i].Genre = l.Services[0].Genre
		}
	}, "gridgaps-onegenre.png")

	contiguous := run(t, "contiguous", func(l *multiplex.Listings) {
		for i := range l.Services {
			l.Services[i].Channel = uint16(101 + i)
		}
	}, "gridgaps-contiguous.png")

	// AND THE RULE ITSELF, PREDICTED BEFORE IT IS MEASURED. If a blank slot appears wherever the
	// genre CHANGES between one drawn row and the next, then six channels sharing three genres in
	// adjacent pairs must show exactly TWO -- not none, and not four. A run that only ever flattens
	// the grid cannot tell "separates genre groups" from "does something when genres differ", and
	// the difference decides whether a real line-up would look like this.
	paired := run(t, "three pairs", func(l *multiplex.Listings) {
		byChannel := append([]multiplex.ListedService(nil), l.Services...)
		sort.Slice(byChannel, func(i, j int) bool { return byChannel[i].Channel < byChannel[j].Channel })
		genres := []byte{3, 3, 6, 6, 5, 5}
		for i := range byChannel {
			if i >= len(genres) {
				break
			}
			for j := range l.Services {
				if l.Services[j].Channel == byChannel[i].Channel {
					l.Services[j].Genre = genres[i]
				}
			}
		}
	}, "gridgaps-threepairs.png")
	t.Logf("three pairs: %d gaps, and the rule predicts 2 -- one at each genre change between "+
		"adjacent channels", countGaps(paired))

	genreFlattens := countGaps(oneGenre) < countGaps(stock)
	channelFlattens := countGaps(contiguous) < countGaps(stock)
	t.Logf("=== WHICH VARIABLE MOVED THE GAPS ===")
	t.Logf("    stock %d gaps | one genre %d gaps | contiguous channels %d gaps",
		countGaps(stock), countGaps(oneGenre), countGaps(contiguous))
	switch {
	case genreFlattens && !channelFlattens:
		t.Logf("THE GENRE GROUPS THE ROWS. Channel numbers are untouched in the run that flattened, "+
			"so the blank slot is a separator between genre groups and nothing to do with how far "+
			"apart the channel numbers are. %d gaps became %d.", countGaps(stock), countGaps(oneGenre))
	case channelFlattens && !genreFlattens:
		t.Logf("THE CHANNEL NUMBERS SPACE THE ROWS. Genres are untouched in the run that flattened, "+
			"so the grid is reserving a slot for the numbers it has no channel for. %d gaps became "+
			"%d.", countGaps(stock), countGaps(contiguous))
	case genreFlattens && channelFlattens:
		t.Logf("BOTH FLATTENED IT, so this fixture cannot separate them and neither is established. " +
			"Vary one channel rather than all of them before believing either.")
	default:
		t.Logf("NEITHER FLATTENED IT. Both stories are wrong and the gaps come from something else " +
			"-- read the three pictures side by side before proposing a third.")
	}
}

// plate is one drawn channel-name block: where it starts and how tall it is.
//
// THE HEIGHT IS CARRIED BECAUSE THE PITCH ALONE CANNOT BE READ. The first version of this probe
// took the row unit to be the smallest pitch on the screen, which is right only when at least one
// pair is drawn together -- and on a grid where EVERY pair is separated, the smallest pitch IS the
// gapped one and the instrument reports a flat grid. It did, on the contiguous-channels run, and
// the raw pitches are the only reason it was caught. A plate's own height is the row unit whether
// or not any two rows touch.
type plate struct{ top, height int }

// channelRowStarts reports each channel-name plate down the grid's left column.
//
// It reads the plates rather than the text because a plate is a solid block of one colour the full
// height of its row, which is exactly what a blank slot is the absence of. The background is taken
// as the most common colour in the scanned column rather than sampled from a corner: a corner is a
// guess about layout, and the modal colour is a measurement of this screen.
func channelRowStarts(t *testing.T, box *board.Runtime) []plate {
	t.Helper()
	picture, err := box.Compose()
	if err != nil {
		t.Fatal(err)
	}
	// x sits inside the name plate and clear of the grid's left margin; the y range covers the
	// rows and excludes the header band and the footer.
	const x, top, bottom = 70, 140, 480
	bounds := picture.Bounds()
	if bounds.Dx() <= x || bounds.Dy() <= bottom {
		t.Fatalf("harness: the composed picture is %dx%d, too small for the grid column this "+
			"reads at x=%d, y=%d..%d", bounds.Dx(), bounds.Dy(), x, top, bottom)
	}
	tally := map[color]int{}
	column := make([]color, 0, bottom-top)
	for y := top; y < bottom; y++ {
		c := colorAt(picture, x, y)
		column = append(column, c)
		tally[c]++
	}
	background, most := color{}, 0
	for c, n := range tally {
		if n > most {
			background, most = c, n
		}
	}
	var plates []plate
	inPlate := false
	runStart := 0
	for i, c := range column {
		switch {
		case c != background && !inPlate:
			inPlate, runStart = true, top+i
		case c == background && inPlate:
			inPlate = false
			// A plate is the full height of a row; anything shorter is text bleeding into the
			// column or an edge, and counting it would invent rows.
			if h := top + i - runStart; h >= 10 {
				plates = append(plates, plate{top: runStart, height: h})
			}
		}
	}
	if h := top + len(column) - runStart; inPlate && h >= 10 {
		plates = append(plates, plate{top: runStart, height: h})
	}
	sort.Slice(plates, func(i, j int) bool { return plates[i].top < plates[j].top })
	return plates
}

// tops is the plates' y positions, for a log line that a reader can line up against a picture.
func tops(plates []plate) []int {
	out := make([]int, 0, len(plates))
	for _, p := range plates {
		out = append(out, p.top)
	}
	return out
}

type color struct{ r, g, b, a uint32 }

func colorAt(picture image.Image, x, y int) color {
	r, g, b, a := picture.At(x, y).RGBA()
	return color{r, g, b, a}
}

// pitches is the distance between one row's top and the next's.
func pitches(plates []plate) []int {
	var out []int
	for i := 1; i < len(plates); i++ {
		out = append(out, plates[i].top-plates[i-1].top)
	}
	return out
}

// countGaps reports how many pairs of drawn rows have an empty slot between them.
//
// THE UNIT IS THE PLATE, not the smallest pitch -- see the note on plate. A pitch half again taller
// than a drawn row has something the height of a row between the two, and that is a blank slot.
func countGaps(plates []plate) int {
	if len(plates) < 2 {
		return 0
	}
	tallest := 0
	for _, p := range plates {
		if p.height > tallest {
			tallest = p.height
		}
	}
	gaps := 0
	for _, p := range pitches(plates) {
		if p > tallest+tallest/2 {
			gaps++
		}
	}
	return gaps
}
