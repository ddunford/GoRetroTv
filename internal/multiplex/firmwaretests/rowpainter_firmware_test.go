package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT PAINTS A ROW, AND WHETHER THE GRID EVER CALLS IT.
//
// The drawing surface is known: the box paints off-screen into 0x8055C000..0x8055FFFF, four
// contiguous pages, found by histogramming DRAM writes while a screen that demonstrably draws rows
// was drawing them. And the grid's six computed rows write NOT ONE BYTE of it -- 42062, 38914,
// 26893 and 20192 writes, identical to the digit whether the row body runs once or six times.
//
// So the rows stop somewhere between being computed and being painted, and this finds where by
// asking which INSTRUCTIONS do the painting:
//
//	the ten-entry TV GUIDE menu  -> ten rows of text plus its furniture
//	the ALL CHANNELS grid        -> its furniture and nothing else
//
// Both screens paint a header, a background and text, so the instructions they SHARE are the
// furniture and the general text machinery. **The instructions the menu writes the surface from and
// the grid never does are the row painter**, and that is a set this measurement produces rather
// than a function somebody picks out of a listing.
//
// Each write is recorded with its caller as well, because the painter will be a shared text or
// rectangle routine whose own address names nothing -- the same lesson as memmove, where the
// instruction that read the channel index turned out to be a library leaf reached through a thunk
// table and the useful address was two hops up.
//
// IT ASSERTS ITS OWN SUBJECT: both screens are pinned by hash AND dumped, because a hash cannot
// tell ALL CHANNELS from the menu with its highlight moved and this project has measured the wrong
// screen five times; and both must write the surface at all, or the surface named by the control is
// not the one that screen uses.
//
// **The assertion used to be that the MENU writes the surface more, and that was wrong** -- ten
// rows of text sounds like more drawing than none, and it is not. The grid wrote 128,061 times from
// 24 instructions against the menu's 84,903 from 18, because it paints a full-screen background, a
// header, an in-world date, a clock and a half-hour time axis. Volume was never the signal here.
// WHICH INSTRUCTIONS is.
//
// IT ONLY READS.
func TestWhatPaintsARowAndWhetherTheGridCallsIt(t *testing.T) {
	menu := paintersFor(t, false)
	grid := paintersFor(t, true)

	t.Logf("=== instructions that write the drawing surface ===")
	t.Logf("  the ten-row TV GUIDE menu: %d writes from %d instructions",
		totalWrites(menu), len(menu))
	t.Logf("  the ALL CHANNELS grid:     %d writes from %d instructions",
		totalWrites(grid), len(grid))
	// THE SUBJECT ASSERTION IS THAT BOTH SCREENS PAINT, NOT THAT ONE PAINTS MORE -- and that is a
	// correction. This probe first demanded the ten-row menu write the surface MORE than the empty
	// grid, on the reasoning that ten rows of text is more drawing than none. It is not: the grid
	// wrote 128,061 times from 24 instructions against the menu's 84,903 from 18. The grid paints
	// a full-screen background, a header, an in-world date, a clock and a half-hour time axis, and
	// all of that is far more pixels than ten short lines of text. Volume was never the signal
	// here; WHICH INSTRUCTIONS is.
	if totalWrites(menu) == 0 || totalWrites(grid) == 0 {
		t.Fatalf("harness: one of the two screens wrote the drawing surface not at all (menu %d, "+
			"grid %d), so the surface named by the control is not the one that screen uses and "+
			"every difference below is an artefact of watching the wrong memory",
			totalWrites(menu), totalWrites(grid))
	}

	// THE ROW PAINTER: what the menu paints from and the grid never does.
	type painter struct {
		at painterSite
		n  int
	}
	var only []painter
	for site, n := range menu {
		if _, shared := grid[site]; !shared {
			only = append(only, painter{at: site, n: n})
		}
	}
	sort.Slice(only, func(a, b int) bool { return only[a].n > only[b].n })
	if len(only) == 0 {
		t.Logf("VERDICT: every instruction that paints the menu ALSO paints the grid. The two " +
			"screens use exactly the same painting code and the grid simply calls it fewer times, " +
			"so the row painter is not a separate routine and the difference is in how often it " +
			"is reached, not in whether it is reached at all.")
		return
	}
	t.Logf("VERDICT: %d (instruction, caller) pairs paint the MENU and never the grid. These are "+
		"the row painter, busiest first:", len(only))
	for i, p := range only {
		if i >= 25 {
			t.Logf("    ... and %d more", len(only)-25)
			break
		}
		t.Logf("    writes at %08X  called from %08X  %d writes", p.at.pc, p.at.ra, p.n)
	}

	// And the other direction, which is worth a line because it is the control on the control: an
	// instruction the GRID paints from and the menu does not is furniture unique to the grid, and
	// a long list there would mean the two screens share less machinery than this comparison
	// assumes.
	unique := 0
	for site := range grid {
		if _, shared := menu[site]; !shared {
			unique++
		}
	}
	t.Logf("(for contrast, %d pairs paint the grid and never the menu -- the grid's own furniture)",
		unique)
	shared := 0
	for site := range menu {
		if _, both := grid[site]; both {
			shared++
		}
	}
	t.Logf("(%d pairs paint both, which is the shared text and rectangle machinery)", shared)
}

// painterSite is one (instruction, caller) pair that wrote the drawing surface.
type painterSite struct{ pc, ra uint32 }

func totalWrites(m map[painterSite]int) int {
	n := 0
	for _, c := range m {
		n += c
	}
	return n
}

// paintersFor walks to one of the two screens and records every instruction that writes the
// drawing surface while it draws.
func paintersFor(t *testing.T, wantGrid bool) map[painterSite]int {
	t.Helper()
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		wantKind    = 14
		readyFrom   = 6
		// The four contiguous pages the control named. Physical, because that is what the histogram
		// that found them bucketed on.
		surfaceLo = 0x55c
		surfaceHi = 0x55f
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })
	if wantGrid {
		// The grid is only worth measuring with its first gate open, or its row body runs once and
		// this compares the menu's ten rows against one iteration rather than against six.
		if kind := box.RAM.Read(off, bus.Word); kind != wantKind {
			t.Fatalf("harness: %08X holds kind %d, not %d -- the transport object has moved",
				uint32(transportAt), kind, wantKind)
		}
		if reached := runUntil(t, box, transmitter, 400_000_000,
			func(int) bool { return state() >= readyFrom }); reached < 0 {
			t.Fatalf("harness: the transport never reached state %d; it is still %d",
				readyFrom, state())
		}
		t.Logf("the transport is ready at state %d", state())
	}

	sites, watching := map[painterSite]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Write || a.Fetch {
			return
		}
		if page := (a.Virtual & 0x1fffffff) >> 12; page < surfaceLo || page > surfaceHi {
			return
		}
		st := box.Machine.Core.State()
		sites[painterSite{pc: st.PC &^ 1, ra: st.GPR[31] &^ 1}]++
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		sites, watching = map[painterSite]int{}, true
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	if wantGrid {
		grid := openAllChannels(t, press, ".artifacts/row-painter-grid.png", false)
		if err := dumpScreen(t, box, "row-painter-grid.png"); err != nil {
			t.Fatal(err)
		}
		t.Logf("the grid drew %08X, transport state %d", grid, state())
		return sites
	}

	const tvGuideMenu = 0xDDBC18E9 // verified by eye: ten rows of text
	first := press(keyBoxOffice, "box office", 80_000_000)
	tab := first
	for attempt := 1; attempt <= 6 && tab != tvGuideMenu; attempt++ {
		tab = press(keyLeft, "left to the tv guide tab (ten rows)", 80_000_000)
	}
	if tab != tvGuideMenu {
		t.Fatalf("harness: never reached the ten-row tv guide menu (%08X); the painters below "+
			"would belong to whatever was drawn instead", uint32(tvGuideMenu))
	}
	if err := dumpScreen(t, box, "row-painter-menu.png"); err != nil {
		t.Fatal(err)
	}
	return sites
}

// DOES ANYTHING READ THE LIST THE ROW BODY BUILDS?
//
// The grid's MIPS row loop is the database side: it runs six times with the transport ready, writes
// 737 more words than the one-iteration control, and never enters the drawing module at 0x8009xxxx
// once. So it assembles something and the interpreted o-code screen is supposed to draw from it.
//
// That makes one question decisive, and it is a question no amount of disassembly answers because
// the consumer is o-code rather than MIPS:
//
//	are the words the row body writes ever READ again while the same screen draws?
//
// A consumer names itself -- the instructions that read those addresses after the loop finishes are
// whatever picks the list up, and in an interpreter they will be the interpreter's own fetch and
// load sites, which is itself the answer: the o-code ran and looked. **Nothing reading them at all
// is the stronger result**, because then the list is built for a reader that never comes, and the
// fault is in whatever should have told the screen there was something to draw.
//
// IT SEPARATES THE TWO PHASES BY THE LOOP ITSELF rather than by an instruction count: writes are
// collected only while the loop is running, and reads are counted only after it has finished, so a
// word the body writes and immediately re-reads inside its own iteration cannot be mistaken for a
// consumer.
//
// IT ASSERTS ITS OWN SUBJECT: the loop must run its six iterations (or the transport gate is shut
// and this measures the old behaviour), and the body must write something (or there is no list and
// the zero below is trivially true).
//
// IT ONLY READS.
func TestWhetherAnythingReadsTheListTheRowBodyBuilds(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		wantKind    = 14
		readyFrom   = 6
		loopHead    = 0x800A4B60
		bodyStart   = 0x800A4BBE
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if kind := box.RAM.Read(off, bus.Word); kind != wantKind {
		t.Fatalf("harness: %08X holds kind %d, not %d -- the transport object has moved",
			uint32(transportAt), kind, wantKind)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d", state())

	var (
		watching   bool
		inLoop     bool
		finished   bool
		iterations int
		built      = map[uint32]bool{}
		readers    = map[painterSite]int{}
		readWords  int
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching {
			return
		}
		if a.Fetch {
			switch a.Virtual &^ 1 {
			case bodyStart:
				inLoop = true
			case loopHead:
				iterations++
				// The loop head is reached once per iteration plus once to decide to stop; the
				// body having run at least once and the head coming round again after the last
				// body is where the list is complete.
				if inLoop && iterations > 6 {
					inLoop, finished = false, true
				}
			}
			return
		}
		at := a.Virtual & 0x1fffffff
		if a.Write {
			if inLoop {
				built[at] = true
			}
			return
		}
		if finished && built[at] {
			readWords++
			st := box.Machine.Core.State()
			readers[painterSite{pc: st.PC &^ 1, ra: st.GPR[31] &^ 1}]++
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching, inLoop, finished, iterations = true, false, false, 0
			built, readers, readWords = map[uint32]bool{}, map[painterSite]int{}, 0
		}
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	grid := openAllChannels(t, press, ".artifacts/list-consumer.png", false)
	if err := dumpScreen(t, box, "list-consumer.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid drew %08X, transport state %d", grid, state())

	if iterations < 7 {
		t.Fatalf("harness: the row loop head was reached %d times, so it did not run its six "+
			"iterations and this measured the old one-iteration behaviour", iterations)
	}
	if len(built) == 0 {
		t.Fatal("harness: the row body wrote nothing at all, so there is no list and the count " +
			"below is trivially zero")
	}
	t.Logf("the six iterations wrote %d distinct words; after the loop finished they were read "+
		"%d times from %d (instruction, caller) pairs", len(built), readWords, len(readers))

	if readWords == 0 {
		t.Logf("VERDICT: NOTHING READS IT. The row body writes %d words and not one of them is "+
			"read again while the screen draws. The list is built for a reader that never comes, "+
			"so the fault is in whatever should tell the screen there is something to draw -- not "+
			"in the data, which is now complete, and not in the drawing, which never hears about "+
			"it.", len(built))
		return
	}
	type reader struct {
		at painterSite
		n  int
	}
	var sorted []reader
	for site, n := range readers {
		sorted = append(sorted, reader{at: site, n: n})
	}
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].n > sorted[b].n })
	t.Logf("VERDICT: the list IS read after it is built. These are its consumers, busiest first; " +
		"an interpreter's own load sites here mean the o-code ran and looked:")
	for i, r := range sorted {
		if i >= 20 {
			t.Logf("    ... and %d more", len(sorted)-20)
			break
		}
		t.Logf("    read at %08X  called from %08X  %d reads", r.at.pc, r.at.ra, r.n)
	}
}

// IS 0x800A4B60 THE GRID'S LOOP, OR EVERY SCREEN'S?
//
// A whole chain of findings now rests on one identification: that the bounds-checked loop at
// 0x800A4B60 is the ALL CHANNELS grid's row loop. The evidence for it is a correlation -- open the
// grid with the transport at 4 and the loop runs once; open it with the transport ready and it runs
// six times against a limit of six, which is exactly the number of channels. That is a good
// correlation and it is not an identification, and this project has already spent a week on one
// correlation that was real and incidental.
//
// **The cheap control is the screen next door.** The ten-entry TV GUIDE menu draws ten rows of text
// and is reached on the way to the grid, so it costs nothing extra to measure. If the loop runs
// while the MENU draws too, it is a general channel enumeration that some other part of the box
// does on any screen change, and everything built on "the grid's row loop" needs re-reading. If it
// does not, the identification holds.
//
// The now-and-next banner is already a second control from an earlier run: it reached the loop head
// ZERO times while drawing a real programme on a real channel.
//
// IT ASSERTS ITS OWN SUBJECT: the menu must be the pinned ten-row screen, or the count belongs to
// something else.
func TestWhetherTheRowLoopIsTheGridsOrEveryScreens(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
		loopHead    = 0x800A4B60
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	// The transport is made ready FIRST, so the menu is measured under exactly the conditions that
	// let the loop run six times for the grid. Measuring the menu with the gate shut would prove
	// nothing: the loop would be short for both screens and for the same reason.
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d, so the loop is free to run for either screen", state())

	hits, watching := 0, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if watching && a.Fetch && a.Virtual&^1 == loopHead {
			hits++
		}
	}}
	press := func(raw uint8, name string, budget int) (uint32, int) {
		t.Helper()
		hits, watching = 0, true
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X, loop head reached %d times", name, settled, hits)
		return settled, hits
	}

	const tvGuideMenu = 0xDDBC18E9 // verified by eye: ten rows of text
	screen, boxOfficeHits := press(keyBoxOffice, "box office (six rows)", 80_000_000)
	menuHits := boxOfficeHits
	for attempt := 1; attempt <= 6 && screen != tvGuideMenu; attempt++ {
		screen, menuHits = press(keyLeft, "left to the tv guide tab (ten rows)", 80_000_000)
	}
	if screen != tvGuideMenu {
		t.Fatalf("harness: never reached the ten-row tv guide menu (%08X); the count belongs to "+
			"whatever was drawn instead", uint32(tvGuideMenu))
	}
	if err := dumpScreen(t, box, "row-loop-control-menu.png"); err != nil {
		t.Fatal(err)
	}

	t.Logf("=== the loop head at %08X, with the transport ready throughout ===", uint32(loopHead))
	t.Logf("  drawing the BOX OFFICE menu (six rows): %d", boxOfficeHits)
	t.Logf("  drawing the TV GUIDE menu (ten rows):   %d", menuHits)
	t.Logf("  drawing the now-and-next banner:        0 (measured earlier)")
	switch {
	case menuHits == 0 && boxOfficeHits == 0:
		t.Logf("VERDICT: NEITHER MENU TOUCHES IT. Two screens that draw rows of text -- six and ten " +
			"of them -- reach this loop zero times, and so does the banner. It runs for the ALL " +
			"CHANNELS grid and for nothing else, so the identification holds and everything built " +
			"on it stands.")
	default:
		t.Logf("VERDICT: A MENU REACHES IT TOO (%d box office, %d tv guide). This is not the grid's "+
			"row loop, it is a channel enumeration some other part of the box runs on a screen "+
			"change, and the correlation with the grid was incidental. Every finding that calls "+
			"it the grid's row loop needs re-reading.", boxOfficeHits, menuHits)
	}
}
