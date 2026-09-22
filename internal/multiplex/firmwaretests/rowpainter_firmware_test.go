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
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, settled := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				settled = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
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
