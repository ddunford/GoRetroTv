package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DO THE SIX ROWS REACH THE FRAMEBUFFER AT ALL?
//
// The OSD does not walk a display list of objects — it scans out a BITMAP, whose address the
// firmware programs into the display-list descriptor as Field0. So anything that appears on this
// screen got there by being written into that bitmap, and a row that is never written into it is a
// row that was computed and thrown away.
//
// **IT WATCHES THE BITMAP, NOT THE BLITTER, AND THAT IS A CORRECTION.** The first version of this
// probe counted CPU writes to the blitter's MMIO window at 0xB0006000 and measured ZERO during a
// draw that visibly puts a header, a date, a clock and half-hour columns on screen. Its own subject
// assertion caught it: a screen that demonstrably draws cannot have drawn through a window nothing
// wrote to. Whatever this firmware uses to paint, it is not that register file, and counting it
// would have reported "the rows are never drawn" about every row on the screen. The framebuffer is
// the one surface every drawn pixel must pass through whatever paints it.
//
// The grid computes six rows and shows none. Two things that look alike from the outside are still
// possible, and they want different work:
//
//   - the rows never reach the blitter — the body computes and throws the result away, or stops
//     before it draws, and the fix is upstream of drawing entirely;
//   - the rows ARE blitted and render as nothing — into the wrong bitmap, off-screen, or in a
//     colour that is the background, and the fix is in the drawing.
//
// **THE CONTROL IS THE SAME SCREEN WITH THE TRANSPORT NOT READY**, and that is what makes this
// worth a run rather than a number with nothing to compare it to. Pressed after a fixed settle the
// transport is still at state 4, the row loop runs ONCE and leaves; waited for, it is at 6 or more
// and the loop runs all six channels. Same box, same route, same screen, and the only difference is
// the thing already known to change the loop. So the blits attributable to the six rows are exactly
// the difference between the two counts:
//
//	same count   -> six iterations of four thousand instructions produce NO DRAWING
//	higher count -> the rows are drawn and something about the drawing is wrong
//
// IT ASSERTS ITS OWN SUBJECT: both runs must land on the pinned empty grid (they do — that is the
// whole puzzle), and the blitter must be written at all, or a zero difference is the instrument
// rather than the screen. The grid draws a header, a date, a clock and half-hour columns, so a run
// that sees no blits at all has measured nothing.
//
// IT ONLY READS.
func TestWhetherTheGridsSixRowsReachTheBlitter(t *testing.T) {
	ready := measureGridBlits(t, true)
	notReady := measureGridBlits(t, false)

	total := func(m map[uint32]int) int {
		n := 0
		for _, c := range m {
			n += c
		}
		return n
	}
	t.Logf("=== DRAM writes during the ALL CHANNELS draw ===")
	t.Logf("  transport NOT ready (loop runs once): %d writes over %d pages",
		total(notReady), len(notReady))
	t.Logf("  transport ready     (loop runs six):  %d writes over %d pages",
		total(ready), len(ready))
	if total(ready) == 0 && total(notReady) == 0 {
		t.Fatal("harness: the guest wrote to DRAM not once during either draw, which cannot be " +
			"true of a box that changed its screen. The watch is wrong and its zeros describe the " +
			"instrument")
	}

	// THE DRAWING SURFACE, NAMED BY THE CONTROL RATHER THAN GUESSED AT. Drawing the ten-entry TV
	// GUIDE menu -- ten rows of text, right there in its artefact -- writes about eighty-five
	// thousand times across 0x8055C000..0x8055FFFF, four contiguous pages, and nothing else in the
	// box is written remotely like that. That is where a row lands. (The display-list descriptor
	// names a different bitmap at physical page 0x584 which the CPU never writes, so the box
	// paints off-screen and the picture is presented some other way.)
	surface := 0
	for page := uint32(0x55c); page <= 0x55f; page++ {
		t.Logf("  drawing surface page %08X: ready %d, control %d",
			0x80000000|(page<<12), ready[page], notReady[page])
		surface += ready[page] - notReady[page]
	}
	if ready[0x55c]+notReady[0x55c] == 0 {
		t.Fatal("harness: neither draw wrote the drawing surface at all, yet the grid visibly " +
			"paints a header, a date, a clock and half-hour columns. The surface named by the " +
			"control is not the one this screen uses and the comparison below means nothing")
	}
	switch {
	case surface == 0:
		t.Logf("VERDICT: THE SIX ROWS WRITE NOT ONE EXTRA PIXEL. The drawing surface takes exactly " +
			"as many writes when the body runs six times as when it runs once, so the rows are " +
			"computed and NEVER DRAWN. The second refusal is between computing a row and drawing " +
			"it, and no amount of looking at bitmaps or colours will find it.")
	case surface > 0:
		t.Logf("VERDICT: the six rows write %d MORE times to the drawing surface, so they ARE "+
			"drawn and render as nothing -- off-screen, clipped, or in the background colour. "+
			"That is a question about the drawing and not about the data.", surface)
	default:
		t.Logf("VERDICT: the ready run wrote the drawing surface %d times FEWER, which neither "+
			"reading predicts and wants explaining first.", -surface)
	}

	// THE PAGES THE SIX ROWS COST. Same screen, same route; the only difference is the transport
	// state that is already known to turn one loop iteration into six. A page written more in the
	// ready run is a page the rows touched.
	pages := map[uint32]bool{}
	for p := range ready {
		pages[p] = true
	}
	for p := range notReady {
		pages[p] = true
	}
	type delta struct {
		page             uint32
		readyN, controlN int
	}
	var moved []delta
	for p := range pages {
		if ready[p] != notReady[p] {
			moved = append(moved, delta{page: p, readyN: ready[p], controlN: notReady[p]})
		}
	}
	sort.Slice(moved, func(a, b int) bool {
		return moved[a].readyN-moved[a].controlN > moved[b].readyN-moved[b].controlN
	})
	if len(moved) == 0 {
		t.Logf("VERDICT: NOT ONE PAGE OF DRAM IS WRITTEN DIFFERENTLY. Six iterations of the row " +
			"body, four thousand instructions each, leave the same memory in the same state as the " +
			"run where the body executes once. Whatever the body computes, it neither draws it nor " +
			"stores it -- and that makes the second refusal something inside the iteration that " +
			"discards its own work.")
		return
	}
	t.Logf("VERDICT: %d pages are written differently. The rows DO produce something; these are "+
		"where it goes, biggest gain first:", len(moved))
	for i, d := range moved {
		if i >= 20 {
			t.Logf("    ... and %d more", len(moved)-20)
			break
		}
		t.Logf("    page %08X  ready %d  control %d  (%+d)",
			0x80000000|(d.page<<12), d.readyN, d.controlN, d.readyN-d.controlN)
	}
}

// measureGridBlits opens ALL CHANNELS on a fresh box and counts the blitter register writes the
// draw performs, either after waiting for the transport or after the fixed settle every previous
// measurement of this screen used.
func measureGridBlits(t *testing.T, waitForTransport bool) map[uint32]int {
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
		emptyGrid   = 0x42DBD889
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
	if waitForTransport {
		if reached := runUntil(t, box, transmitter, 400_000_000,
			func(int) bool { return state() >= readyFrom }); reached < 0 {
			t.Fatalf("harness: the transport never reached state %d; it is still %d",
				readyFrom, state())
		}
	} else {
		// The settle every previous measurement of this screen used, which catches the transport
		// at 4 and is exactly the control this comparison needs.
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })
	}
	t.Logf("transport state before the walk: %d", state())

	// WHERE THE WRITES GO, FOUND RATHER THAN ASSUMED. Two earlier versions of this probe watched
	// a region picked in advance -- the blitter's MMIO window, then the bitmap the display-list
	// descriptor names -- and both measured ZERO during a draw that visibly puts text on screen.
	// A histogram by page has no such assumption in it and says where the drawing really lands.
	pages, watching := map[uint32]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Write || a.Fetch {
			return
		}
		// Bucketed on the PHYSICAL offset so a write through KSEG0 and one through the uncached
		// KSEG1 alias land in the same page.
		pages[(a.Virtual&0x1fffffff)>>12]++
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			pages, watching = map[uint32]int{}, true
		}
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	name := "grid-blits-settled.png"
	if waitForTransport {
		name = "grid-blits-transport-ready.png"
	}
	grid := openAllChannels(t, press, ".artifacts/"+name, false)
	if err := dumpScreen(t, box, name); err != nil {
		t.Fatal(err)
	}
	if grid != emptyGrid {
		t.Fatalf("harness: the grid drew %08X and not the pinned empty screen %08X, so the two "+
			"halves of this comparison are not of the same screen", grid, uint32(emptyGrid))
	}
	n := 0
	for _, c := range pages {
		n += c
	}
	t.Logf("transport state at the draw: %d, %d DRAM writes over %d pages", state(), n, len(pages))
	return pages
}

// WHERE DOES ANY PIXEL COME FROM? The drawing surface, found before it is reasoned about.
//
// Two assumptions about how this box paints have now been measured and both were wrong. The
// blitter's MMIO window at 0xB0006000 is written ZERO times during a draw that visibly puts a
// header, a date, a clock and half-hour columns on screen. The bitmap the display-list descriptor
// names -- 720x576 at 8bpp, physical 0x00584048 -- is written ZERO times as well. The CPU does not
// paint through either, and a probe that keeps picking a surface in advance keeps measuring zero
// and calling it a finding about the firmware.
//
// So this stops choosing and asks the box. It histograms every DRAM write by page during the draw
// of a screen that DEMONSTRABLY DRAWS ROWS -- the ten-entry TV GUIDE menu, the screen the grid is
// reached through and one whose rows are right there in its artefact -- and prints the busiest
// pages. Whatever surface ten rows of text land on will be at the top of that list, and once it is
// named the grid can be asked the one question this whole line of work needs: does it write there
// too?
//
// IT ASSERTS ITS OWN SUBJECT: the menu must be the pinned screen, verified by eye, or the pages
// belong to something else.
//
// IT ONLY READS.
func TestWhereADrawnRowActuallyLands(t *testing.T) {
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

	pages, watching := map[uint32]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Write || a.Fetch {
			return
		}
		pages[(a.Virtual&0x1fffffff)>>12]++
	}}
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		pages, watching = map[uint32]int{}, true
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	const tvGuideMenu = 0xDDBC18E9 // verified by eye: ten rows of text
	menu := press(keyBoxOffice, "box office", 80_000_000)
	tab := menu
	for attempt := 1; attempt <= 6 && tab != tvGuideMenu; attempt++ {
		tab = press(keyLeft, "left to the tv guide tab (ten rows)", 80_000_000)
	}
	if tab != tvGuideMenu {
		t.Fatalf("harness: never reached the ten-row tv guide menu (%08X); the pages below would "+
			"belong to whatever was drawn instead", uint32(tvGuideMenu))
	}
	if err := dumpScreen(t, box, "where-a-row-lands.png"); err != nil {
		t.Fatal(err)
	}

	total := 0
	for _, c := range pages {
		total += c
	}
	if total == 0 {
		t.Fatal("harness: the guest wrote to DRAM not once while drawing ten rows of text, which " +
			"cannot be true. The watch is wrong")
	}
	busiest := make([]uint32, 0, len(pages))
	for p := range pages {
		busiest = append(busiest, p)
	}
	sort.Slice(busiest, func(a, b int) bool { return pages[busiest[a]] > pages[busiest[b]] })
	t.Logf("drawing ten rows of text cost %d DRAM writes over %d pages; the busiest:",
		total, len(pages))
	for i, p := range busiest {
		if i >= 15 {
			break
		}
		t.Logf("    page %08X  %d writes", 0x80000000|(p<<12), pages[p])
	}

	desc, ok, err := box.Display.Descriptor()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Logf("the display-list descriptor points at %08X (%dx%d, %d bpp), physical page %05X -- "+
			"compare it against the list above", desc.Field0, desc.Width, desc.Height, desc.Depth,
			(desc.Field0&0x1fffffff)>>12)
	}
}
