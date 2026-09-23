package firmwaretests_test

import (
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// The ALL CHANNELS grid draws a header, a date, a clock and correctly spaced
// half-hour columns, and no channel rows -- measured three times, most recently
// with all four six-hour blocks on air and the columns spanning the 18:00 block
// boundary, so it is not the listings.
//
// The record establishes that the grid KNOWS how many channels there are:
// DS+0x02E20C holds 12, written by executed code at 0x9FC75F2F and read at
// 0x9FC75F7B, 0x9FC762FD and 0x9FC77D41. The last of those compares it and
// picks a layout size, so it is chrome; the other two are where a row loop
// would start.
//
// THE QUESTION THIS ANSWERS IS WHICH FAULT IT IS, because a loop that runs
// twelve times and draws nothing and a loop that never runs produce the same
// blank screen and want opposite fixes. So this counts, per guest PC, how many
// times the firmware executes each address in the window around those reads
// while the grid is being drawn -- and it reports any PC that runs exactly
// twelve times, which is what a row loop over twelve channels looks like from
// outside.
//
// It asserts its own subject rather than reporting a zero: if the route never
// reaches the grid, or the window is never executed, that is a harness failure
// and it says so.
const (
	gridWindowLo = 0x80000000
	gridWindowHi = 0xA0000000

	pcChannelCountWrite = 0x9FC75F2E // 0x9FC75F2F with the MIPS16 bit masked off
	pcChannelCountReadA = 0x9FC75F7A
	pcChannelCountReadB = 0x9FC762FC
	pcLayoutSelector    = 0x9FC77D40

	keyBoxOffice = 0x7D
	keyLeft      = 0x5A
	keySelect    = 0x5C

	gridChannels = 12
)

func TestTheGridsRowLoopEitherRunsOrItDoesNot(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict,
		multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	// Listings on air first. The grid is being asked about with a full store
	// behind it, because a grid measured on an empty box answers a different
	// question -- and that is the question three earlier measurements of this
	// screen accidentally answered.
	// A BOX REGISTERS ONLY THE BLOCK IT IS LISTENING TO, so the whole day is not
	// the expectation -- asking for 67 here is how this instrument failed first
	// time, and the record already says so.
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of the %d programmes in this block registered, so the grid would be measured on a box without listings",
			registered, want)
	}
	t.Logf("listings on air: %d programmes registered for the %02d:00 block", registered, day.Hour())

	press := func(raw uint8, name string) uint32 {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		hash := drawnScreen(t, box, transmitter, 80_000_000, before)
		t.Logf("%-22s drew %08X", name, hash)
		return hash
	}
	// LET IT GO QUIET BEFORE THE FIRST PRESS. The record is explicit that a key
	// sent the instant the box finishes something does nothing at all, and this
	// instrument pressed straight after acquisition -- surviving only while the
	// box happened to be idle by then. Moving the NIT onto its own PID shifted
	// the timing by a hair and this test started failing on its FIRST press,
	// which read as a transmitter regression and was this.
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	// THE ROUTE MUST PROVE WHICH SCREEN IT IS ON, AND "THE HASH CHANGED" DOES
	// NOT. One LEFT from box office draws 0xFE8D1CCC, which is STILL THE BOX
	// OFFICE MENU -- the wedge guard names it as a constant for exactly this
	// reason -- so a check that only asks whether the screen moved accepts it as
	// the tv guide tab, selects box office's first entry and measures MOVIES BY
	// START TIME for the rest of the run. This instrument did precisely that,
	// and its own artefact said so; nothing else caught it, through a PC census,
	// a twelve-iteration analysis and three sets of notes.
	//
	// Both hashes below were verified against the dumped pictures by eye.
	const tvGuideMenu = tvGuideMenuScreen // the ten-entry TV GUIDE menu, ALL CHANNELS highlighted
	// 0x42DBD889 -- ALL CHANNELS with no rows at all -- was this pin until the 0xB2 descriptor
	// made the grid draw. That screen cannot be reached any more, so the pin is the pair the route
	// names: searching, and filled.
	menu := press(keyBoxOffice, "box office")
	tab := menu
	for attempt := 1; attempt <= 6 && tab != tvGuideMenu; attempt++ {
		tab = press(keyLeft, "left to tv guide tab")
	}
	if tab != tvGuideMenu {
		t.Fatalf("harness: never reached the TV GUIDE menu (%08X); the screen settled on %08X, and "+
			"a screen that merely differs from box office is what has been measured by mistake "+
			"before", uint32(tvGuideMenu), tab)
	}

	// SELECT IS LOST IF IT ARRIVES WHILE THE MENU IS STILL PAINTING -- the record
	// says so for 0x5C specifically, and the first run of this instrument proved
	// it again: it reported a clean census of the wrong screen, because the press
	// was swallowed and the "grid draw" it measured was the tab finishing its own
	// paint. So the harness RETRIES and then PROVES it left the menu, rather than
	// trusting one press.
	hits := make(map[uint32]int, 4096)
	region := make(map[uint32]int, 16)
	grid := uint32(0)
	for attempt := 1; attempt <= 6 && grid == 0; attempt++ {
		// Let the screen go completely quiet first.
		runUntil(t, box, transmitter, 8_000_000, func(int) bool { return false })
		for k := range hits {
			delete(hits, k)
		}
		for k := range region {
			delete(region, k)
		}
		before := screenNow(t, box)
		settled := pressAndLetItFinishWatching(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) },
			board.StepHooks{}, keySelect, 60_000_000, func(int) {
				pc := box.Machine.Core.State().PC &^ 1
				region[pc>>24]++
				if pc >= gridWindowLo && pc < gridWindowHi {
					hits[pc]++
				}
			})
		t.Logf("select attempt %d: screen %08X -> %08X", attempt, before, settled)
		// NOT "different from the menu" -- ALL CHANNELS by its own hash. A select that merely
		// moves the highlight also produces a screen different from the menu, and that is
		// indistinguishable here from the grid opening.
		if atAllChannels(settled) {
			grid = settled
		}
	}
	if grid == 0 {
		if err := dumpScreen(t, box, "grid-select-swallowed.png"); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: six select presses never left the tv guide menu, so nothing here is a measurement of the grid")
	}
	t.Logf("%-22s drew %08X", "ALL CHANNELS", grid)
	if err := dumpScreen(t, box, "grid-all-channels.png"); err != nil {
		t.Fatal(err)
	}

	buckets := make([]uint32, 0, len(region))
	for top := range region {
		buckets = append(buckets, top)
	}
	sort.Slice(buckets, func(a, b int) bool { return region[buckets[a]] > region[buckets[b]] })
	for _, top := range buckets {
		t.Logf("region %02X______ executed %d instructions", top, region[top])
	}
	if len(hits) == 0 {
		t.Logf("NOT ONE instruction executed in %08X..%08X during the grid draw", gridWindowLo, gridWindowHi)
	}

	for _, known := range []struct {
		pc   uint32
		what string
	}{
		{pcChannelCountWrite, "channel count write"},
		{pcChannelCountReadA, "channel count read A (row loop candidate)"},
		{pcChannelCountReadB, "channel count read B (row loop candidate)"},
		{pcLayoutSelector, "layout selector (chrome)"},
	} {
		t.Logf("%08X  %-44s executed %d times", known.pc, known.what, hits[known.pc])
	}

	// A row loop over twelve channels leaves twelve-hit PCs behind. Report them
	// all, and the hottest addresses either way, because "no twelves" is the
	// finding when the loop never runs.
	var twelves []uint32
	for pc, n := range hits {
		if n == gridChannels {
			twelves = append(twelves, pc)
		}
	}
	sort.Slice(twelves, func(a, b int) bool { return twelves[a] < twelves[b] })
	t.Logf("PCs executed exactly %d times (row-loop shape): %d", gridChannels, len(twelves))

	// A COUNT WITH NO CONTROL IS NOT EVIDENCE. If twelve-hit PCs are just what a
	// six-million-instruction trace looks like, its neighbours will be as
	// populous. A real loop over twelve channels makes twelve a SPIKE.
	spread := make(map[int]int, 32)
	for _, n := range hits {
		if n <= 24 {
			spread[n]++
		}
	}
	for n := 8; n <= 18; n++ {
		marker := ""
		if n == gridChannels {
			marker = "   <- channels in the line-up"
		}
		t.Logf("    PCs executed exactly %2d times: %5d%s", n, spread[n], marker)
	}

	// Contiguous runs are what a loop BODY looks like; scattered singletons are
	// what coincidence looks like. Report the longest runs of consecutive
	// twelve-hit addresses.
	type run struct{ lo, hi uint32 }
	var runs []run
	for i := 0; i < len(twelves); i++ {
		j := i
		for j+1 < len(twelves) && twelves[j+1]-twelves[j] <= 4 {
			j++
		}
		if j > i {
			runs = append(runs, run{twelves[i], twelves[j]})
		}
		i = j
	}
	sort.Slice(runs, func(a, b int) bool { return runs[a].hi-runs[a].lo > runs[b].hi-runs[b].lo })
	t.Logf("contiguous twelve-hit address runs: %d", len(runs))
	for i, r := range runs {
		if i >= 12 {
			break
		}
		t.Logf("    %08X..%08X  (%d bytes)", r.lo, r.hi, r.hi-r.lo+2)
	}

	type row struct {
		pc uint32
		n  int
	}
	hottest := make([]row, 0, len(hits))
	for pc, n := range hits {
		hottest = append(hottest, row{pc, n})
	}
	sort.Slice(hottest, func(a, b int) bool {
		if hottest[a].n != hottest[b].n {
			return hottest[a].n > hottest[b].n
		}
		return hottest[a].pc < hottest[b].pc
	})
	t.Logf("distinct PCs executed in the window: %d", len(hottest))
	for i, r := range hottest {
		if i >= 25 {
			break
		}
		t.Logf("    %08X  %d", r.pc, r.n)
	}
}

// dumpScreen writes what the box is showing to .artifacts/, which is gitignored,
// so a measurement of a SCREEN can be looked at rather than described.
func dumpScreen(t *testing.T, box *board.Runtime, name string) error {
	t.Helper()
	picture, err := box.Compose()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join("..", "..", "..", ".artifacts"), 0o750); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join("..", "..", "..", ".artifacts", name)) // #nosec G304 -- fixed test artefact name
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	t.Logf("wrote .artifacts/%s", name)
	return png.Encode(file, picture)
}
