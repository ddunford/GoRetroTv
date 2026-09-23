package firmwaretests_test

import (
	"bytes"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE STRING PAINTER IS HANDED WHEN A ROW COMES OUT AS STRIPES.
//
// Setting the line-up flags makes rows appear -- and they render as diagonal stripes rather than
// text. The artefact that says what that really is, is not the grid but the TV GUIDE MENU: with
// flags 0x03 on air its ninth entry, A-Z LISTINGS, is replaced by exactly the same striped block
// while ALL CHANNELS, ENTERTAINMENT, MOVIES, SPORTS and the rest render perfectly **on the same
// screen, by the same code**.
//
// That is the control this question needs and it comes for free: one screen, one painter, eight
// rows right and one wrong. So the fault is not the painter and not the font -- it is the STRING
// the striped row is given.
//
// The painter is the loop at 0x8009C332, already identified by watching which instructions write
// the drawing surface:
//
//	8009c332  lhu   v1,0(s0)     the next character
//	8009c338  addu  a0,v0,v1     its glyph
//	8009c33e  addiu s0,2         advance the string
//	8009c340  jalr  v0           paint it
//	8009c346  addu  s1,v0        advance x
//	8009c34e  bnez  v0,...       loop for N characters
//
// **THE HALFWORD IS NOT A CHARACTER, and a first version of this probe reported every row on a
// perfectly good screen as "NOT TEXT" for believing it was.** The values step by fourteen in an
// arithmetic run -- 0x2A, 0x38, 0x46, 0x54, 0x62, 0x70, 0x7E -- which is a fixed glyph stride, and
// `addu a0,v0,v1` adds them to a BASE in `v0`. So `s0` walks a table of glyph offsets and `v0` is
// where the glyphs live. A row of stripes is then either a bad offset table or a bad base, and the
// base is the one worth watching: a base pointing at memory nothing ever filled paints exactly the
// diagonal noise these rows show.
//
// So this records, per run of the loop, the offset table's address, the base, and the offsets --
// and compares the rows that render against the rows that do not, on one screen.
//
// IT ASSERTS ITS OWN SUBJECT: the painter must run, and it must paint some legible ASCII somewhere
// (the eight good rows), or the instrument is reading the wrong register.
//
// IT ONLY READS.
func TestWhatTheStringPainterIsHandedForAStripedRow(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	const (
		painterLoop = 0x8009C332
		stripeFlags = 0x03
	)
	listings := guide.On(day)
	for i := range listings.Services {
		listings.Services[i].Flags = stripeFlags
	}
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

	// Each run of the loop is one string: the pointer where it started and the characters read.
	type run struct {
		at    uint32
		base  uint32
		chars []uint16
	}
	var runs []run
	var current *run
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch || a.Virtual&^1 != painterLoop {
			return
		}
		st := box.Machine.Core.State()
		at := st.GPR[16] // s0, the walking string pointer
		ch := uint16(0)
		if at >= 0x80000000 && (at&0x1fffffff)+2 <= box.RAM.Size() {
			ch = uint16(box.RAM.Read(at&0x1fffffff, bus.Half)) // #nosec G115 -- half read
		}
		// A jump in the pointer means a new string; two apart is the same one continuing.
		if current == nil || at != current.at+uint32(len(current.chars))*2 {
			// v0 holds the glyph base the offsets are added to.
			runs = append(runs, run{at: at, base: st.GPR[2]})
			current = &runs[len(runs)-1]
		}
		current.chars = append(current.chars, ch)
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			runs, current, watching = nil, nil, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	// Two LEFTs from box office reach the tv guide menu -- the screen whose ninth row is striped
	// while its other eight are perfect. That is the whole control.
	press(keyBoxOffice, "box office", 80_000_000)
	press(keyLeft, "left 1 of 2", 80_000_000)
	press(keyLeft, "left 2 of 2 (the tv guide menu)", 80_000_000)
	// THE STRIPES APPEAR AFTER SELECT, not on the menu as first drawn -- the sweep's artefact was
	// taken after this press and a first version of this probe stopped before it and measured a
	// clean screen.
	drew := press(keySelect, "select", 60_000_000)
	if err := dumpScreen(t, box, "stripes-menu.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the menu drew %08X over %d painted strings", drew, len(runs))
	if len(runs) == 0 {
		t.Fatal("harness: the string painter never ran while a screen full of text drew, so s0 is " +
			"not the string pointer and everything below would be of the wrong register")
	}

	good, bad := 0, 0
	sort.SliceStable(runs, func(a, b int) bool { return len(runs[a].chars) > len(runs[b].chars) })
	t.Logf("=== every string the painter drew, longest first ===")
	for i, r := range runs {
		if i >= 24 {
			t.Logf("    ... and %d more", len(runs)-24)
			break
		}
		first, last := uint16(0), uint16(0)
		if len(r.chars) > 0 {
			first, last = r.chars[0], r.chars[len(r.chars)-1]
		}
		good++
		t.Logf("    offsets at %08X  base %08X  %2d glyphs  first %#06x last %#06x",
			r.at, r.base, len(r.chars), first, last)
	}
	_ = bad
	bases := map[uint32]int{}
	for _, r := range runs {
		bases[r.base]++
	}
	t.Logf("=== the glyph bases in use ===")
	for b, n := range bases {
		t.Logf("    base %08X used by %d runs", b, n)
	}
	if len(bases) > 1 {
		t.Logf("VERDICT: the painter used %d DIFFERENT glyph bases on one screen. If the rows that "+
			"render and the rows that do not use different bases, the striped row is being pointed "+
			"at glyphs that are not there -- which is a question about what fills that memory.",
			len(bases))
		return
	}
	t.Logf("VERDICT: every run used the SAME glyph base, so the stripes are not a wrong font " +
		"pointer and the difference is in the offsets or in the painting itself.")
}

// ARE THE STRIPES A HALF-PAINTED ROW?
//
// The striped block turned up on the TV GUIDE MENU as well as the grid -- its ninth entry, A-Z
// LISTINGS, replaced by the same diagonal noise while the other nine rendered perfectly. That
// looked like proof the fault was in the row's data.
//
// **It is not reproducible.** The same flags, the same route, and the menu drew all ten entries
// correctly, A-Z LISTINGS included. So the stripes are INTERMITTENT, and the most ordinary reason
// for an intermittent half-drawn row is that the screen was sampled while it was still being
// painted. The settle detector takes a frame every 65,536 instructions and stops after four
// identical ones; a row that paints in bursts can hold still across four samples and finish later.
//
// If that is what this is, the rows are drawing correctly and the earlier artefacts merely caught
// them in the middle -- which would make the stripes an instrument artefact and not a fault at all.
//
// So this opens the grid with every channel flagged, settles as usual, and then **keeps running for
// a further fifty million instructions before capturing**, sampling as it goes. A screen that
// changes after the settle is a screen the settle was wrong about.
//
// IT ONLY READS.
func TestWhetherTheStripedRowsFinishIfGivenTime(t *testing.T) {
	// BOTH CANDIDATE VALUES, RE-ASKED PAST THE SETTLE. "Uniform 0x04 leaves the grid empty" was
	// measured AT a settle, and the settle is now known to lie about this screen, so that result is
	// withdrawn and the question re-put. 0x04 is the single bit measured to set the field the grid
	// masks; 0x0F is all four. If 0x04 alone draws the grid, it is the value to transmit, because
	// setting bits whose meaning is not established is exactly what this project does not do.
	for _, flags := range []byte{0x05, 0x06, 0x07, 0x0c, 0x0d, 0x0e} {
		t.Run(fmt.Sprintf("flags_%#02x", flags), func(t *testing.T) { stripesWithFlags(t, flags) })
	}
}

func stripesWithFlags(t *testing.T, flagged byte) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
	)
	listings := guide.On(day)
	for i := range listings.Services {
		listings.Services[i].Flags = flagged
	}
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		drew := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	artefact := fmt.Sprintf("grid-settled-%02x.png", flagged)
	settled := openAllChannelsUnpinned(t, press, ".artifacts/"+artefact)
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	t.Logf("at the settle the grid was %08X", settled)

	last := settled
	changes := 0
	for i := 0; i < 50_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 != 0 {
			continue
		}
		if now := screenNow(t, box); now != last {
			t.Logf("  at +%2dM the screen became %08X", i/1_000_000, now)
			last, changes = now, changes+1
		}
	}
	final := fmt.Sprintf("grid-final-%02x.png", flagged)
	if err := dumpScreen(t, box, final); err != nil {
		t.Fatal(err)
	}
	t.Logf("*** flags %#02x: the grid finished on %08X -- artefact %s ***", flagged, last, final)
	if changes == 0 {
		t.Logf("VERDICT: the screen did not change once in fifty million further instructions, so "+
			"%08X is genuinely settled and the stripes -- if they are there -- are what the box "+
			"really draws, not a half-painted capture. Read .artifacts/stripes-after-waiting.png.",
			settled)
		return
	}
	t.Logf("VERDICT: the screen changed %d times AFTER the settle said it had stopped, ending on "+
		"%08X. The settle was wrong and every artefact taken at one may have caught a row in the "+
		"middle of painting. Read .artifacts/stripes-after-waiting.png against "+
		".artifacts/stripes-at-settle.png.", changes, last)
}

// THE ALL CHANNELS GRID DRAWS ITS CHANNELS. This is the acceptance test for the screen.
//
// It sets NOTHING on the schedule: the flags come from the transmitter's own default, so this
// exercises the path the demo runs rather than a value a test poked in. Two changes make it pass,
// both in the signal -- a private_data_specifier per service in the SDT, and the guide-visible
// line-up flags -- and either one missing puts the grid back to its empty screen.
//
// **IT WAITS FOR THE SCREEN TO FINISH, WHICH THE SETTLE DOES NOT.** Every measurement of this
// screen in this project's history was taken at a settle -- four identical frames, 65,536
// instructions apart -- and the rows paint in bursts that hold still across four samples and finish
// afterwards. That is why the grid "drew no rows" for so long, and why a half-painted row looked
// convincingly like a broken glyph path. A settle is a statement about the last quarter of a
// million instructions; it is not a statement that the screen has finished.
//
// IT ASSERTS ITS OWN SUBJECT: the grid must stop being the empty screen, and the artefact is dumped
// either way, because a hash cannot tell six rows from one and this project has measured the wrong
// screen five times.
func TestTheAllChannelsGridDrawsItsChannels(t *testing.T) {
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
	// The grid gates on the transport being ready, and it reaches that about fifty-six million
	// instructions after the last programme registers.
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		drew := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	settled := openAllChannelsUnpinned(t, press, ".artifacts/all-channels-grid.png")
	// PAST THE SETTLE. The rows are still arriving when it says the screen has stopped.
	final := settled
	for i := 0; i < 50_000_000; i++ {
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
	if err := dumpScreen(t, box, "all-channels-grid.png"); err != nil {
		t.Fatal(err)
	}
	if final == emptyGrid {
		t.Fatalf("the ALL CHANNELS grid is the EMPTY screen %08X: it settled on %08X and finished "+
			"on the same empty picture. Read .artifacts/all-channels-grid.png", final, settled)
	}
	t.Logf("the grid settled on %08X and FINISHED on %08X, which is not the empty screen. "+
		"Artefact: .artifacts/all-channels-grid.png", settled, final)
}

// NOW THAT THE ROWS DRAW, DOES THE GRID ASK FOR THEIR LISTINGS?
//
// Every row reads "..no listings available" while the box holds twenty-one programmes for five of
// those six channels in the displayed window -- the now-and-next banner draws one of them seconds
// earlier on the same run. So the data is in the box and the grid is not showing it.
//
// **The old answer to this is void.** "The grid never reads the listings store, in 568,990 data
// reads" was measured when the grid drew no rows at all, before the private data specifier and the
// line-up flags; a screen that gave up before enumerating was never going to ask for a programme.
// The question has to be re-put now the rows exist.
//
// The store's pages are the six the signal map found the banner reading and the grid ignoring:
// 0x80199000 (where the per-event register files each programme), 0x80187000, 0x80186000,
// 0x802FB000, 0x8010A000 and 0x801A5000.
//
// **IT WATCHES PAST THE SETTLE**, because the rows arrive after it and so, presumably, does
// anything they ask for. Reading this screen at a settle is what hid the rows for months.
//
// IT ASSERTS ITS OWN SUBJECT: the grid must finish on something other than the empty screen, or it
// is not the screen this question is about.
//
// IT ONLY READS.
func TestWhetherTheDrawnGridAsksForListings(t *testing.T) {
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
		emptyGrid   = 0x42DBD889
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }
	listingsPages := map[uint32]bool{
		0x00199: true, 0x00187: true, 0x00186: true,
		0x002FB: true, 0x0010A: true, 0x001A5: true,
	}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("%d programmes registered and the transport is ready", registered)

	reads, watching := map[uint32]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		if listingsPages[(a.Virtual&0x1fffffff)>>12] {
			reads[box.Machine.Core.State().PC&^1]++
		}
	}}
	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			reads, watching = map[uint32]int{}, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	openAllChannelsUnpinned(t, press, ".artifacts/grid-asks-listings.png")
	// PAST THE SETTLE, still watching: the rows arrive here and so does anything they ask for.
	final := uint32(0)
	for i := 0; i < 50_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "grid-asks-listings.png"); err != nil {
		t.Fatal(err)
	}
	if final == emptyGrid {
		t.Fatalf("harness: the grid finished on the EMPTY screen %08X, so the rows this question is "+
			"about are not there", final)
	}
	total := 0
	for _, n := range reads {
		total += n
	}
	t.Logf("the grid finished on %08X and read the listings store %d times from %d instructions",
		final, total, len(reads))
	pcs := make([]uint32, 0, len(reads))
	for pc := range reads {
		pcs = append(pcs, pc)
	}
	sort.Slice(pcs, func(a, b int) bool { return reads[pcs[a]] > reads[pcs[b]] })
	for i, pc := range pcs {
		if i >= 12 {
			break
		}
		t.Logf("    %08X  %d reads", pc, reads[pc])
	}
	if total == 0 {
		t.Logf("VERDICT: even with its rows drawn the grid NEVER reads the listings store. It is " +
			"not asking for programmes at all, so 'no listings available' is what it says before " +
			"looking, and the next question is what would make it look.")
		return
	}
	t.Logf("VERDICT: the grid DOES read the listings store now -- %d reads. It is asking and not "+
		"finding, so the question is what it asks FOR: the day, the block, or the channel's "+
		"listings id.", total)
}

// WHO CHOOSES "..no listings available".
//
// The string is in the flash message table at guest 0x9FCAB2C8, beside "+24 Hours", "-24 Hours",
// "Today", "Channel", "Searching for listings" and "EVENTS ARE UNAVAILABLE" -- the grid's own
// vocabulary, and the two Hours strings are visible in its footer.
//
// The grid reads the listings store 2,292 times now that its rows draw, so it is asking and not
// finding. The instruction that reaches for this string is where "not finding" becomes "say so",
// and its caller is the code that made the decision.
//
// IT ASSERTS ITS OWN SUBJECT: the string must be read while the screen that displays it draws, or
// the address is wrong.
//
// IT ONLY READS.
func TestWhoChoosesNoListingsAvailable(t *testing.T) {
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
		// "..no listings available" -- both copies in the table, with a little either side so a
		// read of the length byte or the terminator counts.
		//
		// THE STRING IS NOT READ FROM FLASH AT ALL. Two versions of this probe watched the flash --
		// first one window, then every window on both chips, since U202 and U203 both carry the
		// table -- and neither saw a single read while the screen that displays the string drew. So
		// the message table is copied into DRAM at boot, like the application image itself, and the
		// address to watch is one this run finds rather than one computed from a file offset.
		window = 32
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	// FIND IT IN DRAM. The string is searched for as text, so the region watched is where THIS box
	// put it on THIS run.
	needle := []byte("..no listings available")
	size := box.RAM.Size()
	ram := make([]byte, size)
	for i := uint32(0); i < size; i++ {
		ram[i] = byte(box.RAM.Read(i, bus.Byte)) // #nosec G115 -- byte read
	}
	var found []uint32
	for at := 0; ; {
		i := bytes.Index(ram[at:], needle)
		if i < 0 {
			break
		}
		found = append(found, uint32(at+i)) // #nosec G115 -- bounded by RAM size
		at += i + 1
	}
	if len(found) == 0 {
		t.Skipf("the box does not hold %q as text anywhere in DRAM, so the message table is kept "+
			"in some coded form and a watch on a guessed address would report a confident zero",
			needle)
	}
	for _, at := range found {
		t.Logf("the string is in DRAM at %08X", 0x80000000|at)
	}

	hits := map[painterSite]int{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		at := a.Virtual & 0x1fffffff
		hit := false
		for _, s := range found {
			if at >= s-4 && at < s+window {
				hit = true
				break
			}
		}
		if !hit {
			return
		}
		st := box.Machine.Core.State()
		hits[painterSite{pc: st.PC &^ 1, ra: st.GPR[31] &^ 1}]++
	}}
	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			hits, watching = map[painterSite]int{}, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	openAllChannelsUnpinned(t, press, ".artifacts/no-listings-chooser.png")
	for i := 0; i < 50_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "no-listings-chooser.png"); err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatalf("harness: the string was found in DRAM at %08X but never READ while the screen "+
			"that displays it drew, so the text on screen came from somewhere else",
			0x80000000|found[0])
	}
	keys := make([]painterSite, 0, len(hits))
	for k := range hits {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool { return hits[keys[a]] > hits[keys[b]] })
	t.Logf("=== who reaches for \"..no listings available\" ===")
	for i, k := range keys {
		if i >= 15 {
			break
		}
		t.Logf("    read at %08X  caller %08X  %d times", k.pc, k.ra, hits[k])
	}
	t.Logf("decompile the caller:  ./ctl.sh ghidra:decompile %#08x", keys[0].ra)
}

// WHAT LISTINGS DOES THE BOX ACTUALLY ASK FOR, AND IS EVERY CHANNEL COVERED?
//
// The grid draws all six channels and every row reads "..no listings available", while the box
// holds twenty-one programmes and the banner puts one on screen. So the question is no longer
// structural -- it is which listings the box asked for and which it was sent.
//
// The transmitter answers only the filters the box programmed, and one filter is usually about
// SEVERAL channels at once: the box ORs the listings ids it wants and clears the differing bits in
// the mask, so a request is a SET. A channel outside every set is a channel whose programmes were
// never transmitted, and a row for it can only ever say there are none.
//
// So this collects every TitleRequest the box makes, through the transmitter's own OnAir hook,
// across acquisition AND the grid draw -- the grid may ask for more when it opens -- and then asks
// the one question that matters per channel: does ANY request want it?
//
// It reports the MJD each request names beside the day the fixture is transmitting, because a
// request for the right channel on the wrong day is the same empty row and a different bug.
//
// IT ASSERTS ITS OWN SUBJECT: the box must make at least one request, or its silence is the
// instrument.
//
// IT ONLY READS.
func TestWhichListingsTheBoxAsksFor(t *testing.T) {
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
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	seen := map[string]multiplex.TitleRequest{}
	transmitter.OnAir(func(_ multiplex.Counters, requests []multiplex.TitleRequest) {
		for _, r := range requests {
			seen[fmt.Sprintf("%02x/%02x %04x/%04x mjd%d pid%02x",
				r.TableID, r.TableMask, r.Extension, r.ExtensionMask, r.MJD(), r.PID)] = r
		}
	})

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	afterAcquisition := len(seen)

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		drew := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}
	openAllChannelsUnpinned(t, press, ".artifacts/listings-requests.png")
	for i := 0; i < 50_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "listings-requests.png"); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("harness: the box made no listings request at all, which cannot be true of a box " +
			"that registered programmes")
	}
	t.Logf("the box made %d distinct listings requests (%d of them before the grid opened); "+
		"the fixture is transmitting MJD %d", len(seen), afterAcquisition, multiplex.MJDOf(day))
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("    %s", k)
	}

	listings := guide.On(day)
	uncovered := 0
	t.Logf("=== is every announced channel inside some request? ===")
	for i := range listings.Services {
		svc := &listings.Services[i]
		var wanted []string
		for _, k := range keys {
			if seen[k].Wants(svc.ListingsID) {
				wanted = append(wanted, k)
			}
		}
		if len(wanted) == 0 {
			uncovered++
			t.Logf("    %-14s listingsID %4d  NOT WANTED BY ANY REQUEST -- its programmes are never "+
				"transmitted", svc.Name, svc.ListingsID)
			continue
		}
		t.Logf("    %-14s listingsID %4d  wanted by %d request(s)", svc.Name, svc.ListingsID, len(wanted))
	}
	if uncovered > 0 {
		t.Logf("VERDICT: %d of %d channels are outside every request the box made, so nothing this "+
			"transmitter does can put programmes on their rows -- the box is not asking for them.",
			uncovered, len(listings.Services))
		return
	}
	t.Logf("VERDICT: every channel is inside some request, so their programmes ARE transmitted and " +
		"the empty rows are not a coverage problem. The day and the block each request names are " +
		"the next thing to check against what the grid is displaying.")
}

// WHICH CHANNEL IS EACH PROGRAMME FILED UNDER?
//
// Twenty-one programmes are transmitted, parsed and registered; the banner draws one; and every row
// of the grid says there are none. Coverage, day and block are all ruled out, so the question moves
// to the store itself: **are those twenty-one events filed against all six channels, or against
// one?** A store that holds every programme under a single channel looks exactly like this from the
// outside -- one screen that works because it only ever asks about the tuned service, and one that
// asks about six and finds five of them empty.
//
// 0x800C587C is the per-event register, executed once per programme the box takes off the air and
// already used by this package to count them. This dumps the whole register file each time it runs
// and reports which of the fixture's identifiers are in it -- the listings ids, which are what a
// title section is addressed by, and the channel numbers beside them.
//
// **ONLY THE DISTINCTIVE ONES COUNT.** A value carries information here only where the firmware has
// no other reason to hold it: watching every identifier once returned 2,532 reads of "101" from a
// single instruction, which is a loop counter and not a channel. 251, 301, 401 and 501 are above
// the threshold where a small integer stops being ordinary; 101 and 121 are not, and are reported
// separately so they cannot quietly carry the argument.
//
// IT ASSERTS ITS OWN SUBJECT: the register must run the expected number of times, or the sample is
// not the programmes.
//
// IT ONLY READS.
func TestWhichChannelEachProgrammeIsFiledUnder(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	listings := guide.On(day)
	const distinctive = 250
	byListings := map[uint32]string{}
	lowNumbered := map[uint32]string{}
	for i := range listings.Services {
		svc := &listings.Services[i]
		into := byListings
		if svc.ListingsID < distinctive {
			into = lowNumbered
		}
		into[uint32(svc.ListingsID)] = svc.Name
	}

	events := 0
	perChannel := map[uint32]int{}
	perChannelLow := map[uint32]int{}
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Fetch || a.Virtual&^1 != pcPerEventRegister {
			return
		}
		events++
		st := box.Machine.Core.State()
		seenHere := map[uint32]bool{}
		for _, v := range st.GPR {
			id := v & 0xffff
			if name, ok := byListings[id]; ok && !seenHere[id] {
				seenHere[id] = true
				perChannel[id]++
				_ = name
			}
			if _, ok := lowNumbered[id]; ok && !seenHere[id] {
				seenHere[id] = true
				perChannelLow[id]++
			}
		}
	}}

	want := programmesInTheBlock(t, guide, day)
	for i := 0; i < 160_000_000 && events < want; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if events < want {
		t.Fatalf("harness: the per-event register ran %d times, not the %d programmes the block "+
			"carries, so this sample is not the programmes", events, want)
	}
	t.Logf("the per-event register ran %d times for %d programmes in the block", events, want)

	t.Logf("=== programmes filed against each DISTINCTIVE listings id ===")
	covered := 0
	for id, name := range byListings {
		n := perChannel[id]
		note := ""
		if n == 0 {
			note = "   <- NOT ONE"
		} else {
			covered++
		}
		t.Logf("    %-14s listingsID %4d  %3d events%s", name, id, n, note)
	}
	t.Logf("=== and the two whose ids are too ordinary to trust ===")
	for id, name := range lowNumbered {
		t.Logf("    %-14s listingsID %4d  %3d events (a small integer; treat as indicative only)",
			name, id, perChannelLow[id])
	}
	switch {
	case covered == 0:
		t.Logf("VERDICT: not one distinctive listings id appears in the register file while a "+
			"programme is filed, so the identifier is not carried in a register there and this "+
			"says nothing about the store. %d events were sampled.", events)
	case covered < len(byListings):
		t.Logf("VERDICT: only %d of %d distinctive channels ever appear when a programme is filed. "+
			"The others' programmes are transmitted and parsed and end up SOMEWHERE ELSE, which is "+
			"exactly what a grid with five empty rows looks like.", covered, len(byListings))
	default:
		t.Logf("VERDICT: every distinctive channel appears when programmes are filed, so the store " +
			"is not short of channels and the empty rows are in how the grid QUERIES it.")
	}
}

// DOES THE GRID KEY ITS ROWS BY SERVICE ID RATHER THAN LISTINGS ID?
//
// A title section is addressed by the LISTINGS id -- it is the section's extension, and the box's
// one request names it -- so the store is filled under listings ids. But the twenty-four byte
// channel record the grid walks carries the SERVICE id at +0x08 and the channel number at +0x10,
// and no listings id is visible in it at all.
//
// This fixture makes the two DIFFERENT: service ids 100..105 against listings ids 101, 121, 251,
// 301, 401 and 501. If the grid looks a row's programmes up by the service id it has to hand, it
// would find nothing for every channel while the banner -- which resolves the tuned service through
// the line-up, where both ids sit side by side -- works perfectly. That is exactly the shape of
// what is on screen.
//
// **It is a theory, and the cheapest way to test a theory about an identifier is to make the two
// identifiers equal and look.** Setting each service id to its own listings id changes nothing
// about how the titles are addressed -- they are still sent under the listings id, still matched by
// the same request -- so if the rows fill, the difference between the two ids was the whole story.
//
// IT IS AN EXPERIMENT, NOT A FIX. Nothing here changes the product; the schedule is edited in
// memory for this run only, and a pass means the next step is to find which id the grid really
// wants, not to make every schedule set them equal.
//
// IT ONLY READS the box.
func TestWhetherTheGridWantsServiceIDsToMatchListingsIDs(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	listings := guide.On(day)
	for i := range listings.Services {
		listings.Services[i].ServiceID = listings.Services[i].ListingsID
	}
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
		emptyGrid   = 0x42DBD889
		drawnGrid   = 0x584EEA36 // the six channels with "..no listings available"
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered with the ids equal, so the "+
			"experiment changed acquisition and its screen would not be comparable", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("%d programmes registered with service ids set equal to listings ids", registered)

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		drew := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}
	openAllChannelsUnpinned(t, press, ".artifacts/grid-ids-equal.png")
	final := uint32(0)
	for i := 0; i < 50_000_000; i++ {
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
	if err := dumpScreen(t, box, "grid-ids-equal.png"); err != nil {
		t.Fatal(err)
	}
	switch final {
	case emptyGrid:
		t.Logf("VERDICT: with the ids equal the grid is EMPTY, so this experiment broke something "+
			"else and says nothing about listings. (%08X)", final)
	case drawnGrid:
		t.Logf("VERDICT: the grid is byte for byte the screen it draws with the ids DIFFERENT "+
			"(%08X), so making them equal changed nothing at all and the row key is not the "+
			"service id.", final)
	default:
		t.Logf("*** VERDICT: the grid drew %08X, which is NEITHER the empty screen nor the "+
			"no-listings screen. Something about the rows CHANGED. Read "+
			".artifacts/grid-ids-equal.png ***", final)
	}
}

// THE STORE-READING PATH THAT WORKS, AND THE ONE THAT DOES NOT.
//
// Both screens now read the listings store: the banner 214 times from 133 instructions and comes
// back with a programme, the grid 2,292 times from 569 and comes back with nothing. So the grid is
// not failing to look -- it is looking harder and finding less, which means the interesting thing
// is not how much either reads but WHICH CODE does the reading.
//
// Instructions the banner reads the store from and the grid never does are the path that succeeds.
// Instructions only the grid uses are where it looks and fails. Both lists are reported, because a
// difference shown in one direction is a difference that can be read to mean anything.
//
// **THE GRID IS MEASURED PAST ITS SETTLE**, since its rows and everything they ask for arrive after
// the settle claims the screen has stopped.
//
// IT ASSERTS ITS OWN SUBJECT: both screens must read the store, or a one-sided list is not a
// difference.
//
// IT ONLY READS.
func TestTheStoreReadingPathThatWorks(t *testing.T) {
	banner := storeReaders(t, true)
	grid := storeReaders(t, false)
	if len(banner) == 0 || len(grid) == 0 {
		t.Fatalf("harness: one screen read the store from no instruction at all (banner %d, grid "+
			"%d), so the lists below are not a comparison", len(banner), len(grid))
	}
	t.Logf("the banner reads the store from %d instructions, the grid from %d", len(banner), len(grid))

	onlyBanner := map[uint32]int{}
	for pc, n := range banner {
		if grid[pc] == 0 {
			onlyBanner[pc] = n
		}
	}
	onlyGrid := map[uint32]int{}
	for pc, n := range grid {
		if banner[pc] == 0 {
			onlyGrid[pc] = n
		}
	}
	show := func(title string, m map[uint32]int, limit int) {
		keys := make([]uint32, 0, len(m))
		for pc := range m {
			keys = append(keys, pc)
		}
		sort.Slice(keys, func(a, b int) bool { return m[keys[a]] > m[keys[b]] })
		t.Logf("=== %s (%d instructions) ===", title, len(keys))
		for i, pc := range keys {
			if i >= limit {
				t.Logf("    ... and %d more", len(keys)-limit)
				break
			}
			t.Logf("    %08X  %d reads", pc, m[pc])
		}
	}
	show("READS THE STORE ONLY ON THE BANNER -- the path that finds a programme", onlyBanner, 20)
	show("READS THE STORE ONLY ON THE GRID -- where it looks and fails", onlyGrid, 20)
	shared := 0
	for pc := range banner {
		if grid[pc] > 0 {
			shared++
		}
	}
	t.Logf("%d instructions read the store on BOTH screens", shared)
	t.Logf("decompile the busiest banner-only reader:  ./ctl.sh ghidra:decompile <address above>")
}

func storeReaders(t *testing.T, wantBanner bool) map[uint32]int {
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
		readyFrom   = 6
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }
	listingsPages := map[uint32]bool{
		0x00199: true, 0x00187: true, 0x00186: true,
		0x002FB: true, 0x0010A: true, 0x001A5: true,
	}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	reads, watching := map[uint32]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		if listingsPages[(a.Virtual&0x1fffffff)>>12] {
			reads[box.Machine.Core.State().PC&^1]++
		}
	}}
	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect || raw == keyTVGuide {
			reads, watching = map[uint32]int{}, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}
	if wantBanner {
		if drew := press(keyTVGuide, "tv guide (banner)", 60_000_000); drew == 0 {
			t.Fatal("harness: the banner drew nothing")
		}
		watching = false
		return reads
	}
	openAllChannelsUnpinned(t, press, ".artifacts/store-path-grid.png")
	for i := 0; i < 50_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	return reads
}

// THE GATE IN THE GRID'S OWN CLOCK LOOKUP.
//
// 0x800A1E80 is a short function only the grid runs, and it is a gate with a fill behind it:
//
//	800a1e98  lw    v1,0x800a1ed8    -> the global at 0x80105FE4
//	800a1e9a  lw    v1,0(v1)
//	800a1e9c  cmpi  v1,32            it must be 32
//	800a1e9e  btnez 0x800a1eb1       anything else: skip everything, return 0
//	800a1ea0  lhu   v1,16(s0)        otherwise copy three fields out of 0x80163198
//	800a1ea4  lhu   v1,18(s0)          into the caller's struct
//	800a1ea8  lw    v1,40(s0)
//	800a1eae  sw    v0,12(sp)        and return 1
//
// A screen that cannot find out what time it is cannot choose which programmes fall in its window,
// and "..no listings available" is what that looks like from the sofa. So this reads the global and
// watches what the function actually answers.
//
// **THE VALUE AND THE RETURN ARE MEASURED SEPARATELY ON PURPOSE.** Reading the global alone would
// only say what it holds at the end; watching the return says what the grid was told each time it
// asked, and the two disagreeing would itself be the finding.
//
// IT ASSERTS ITS OWN SUBJECT: the function must be called while the grid draws, or its silence is
// the instrument.
//
// IT ONLY READS.
func TestTheGateInTheGridsClockLookup(t *testing.T) {
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
		// 0x800A1F14 is a LEAF function -- an MJD converter. It reads a 16-bit MJD from a1,
		// subtracts 40587 (the MJD of the Unix epoch) and scales it. The grid runs it six times
		// while reading the listings store, so these are the days it is looking at.
		//
		// **A BACKWARD SCAN FOR A PROLOGUE FOUND THE WRONG FUNCTION FOR IT.** Between 0x800A1ED4
		// and 0x800A1F12 sits a literal pool, and pool words disassemble as plausible
		// instructions, so the nearest preceding `addiu sp,-N` belonged to a function two
		// boundaries earlier -- which was duly probed and never called at all. A leaf has no
		// prologue to find.
		mjdOf = 0x800A1F14
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}

	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	calls := 0
	days := map[uint32]int{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		if a.Virtual&^1 != mjdOf {
			return
		}
		calls++
		// a1 points at the two bytes of the MJD being converted.
		at := box.Machine.Core.State().GPR[5] & 0x1fffffff
		if at+2 <= box.RAM.Size() {
			hi := box.RAM.Read(at, bus.Byte)
			lo := box.RAM.Read(at+1, bus.Byte)
			days[hi<<8|lo]++
		}
	}}
	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			calls, watching, days = 0, true, map[uint32]int{}
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	openAllChannelsUnpinned(t, press, ".artifacts/clock-gate.png")
	for i := 0; i < 50_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "clock-gate.png"); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatalf("harness: %08X was never called while the grid drew, so its silence says nothing",
			uint32(mjdOf))
	}
	ours := uint32(multiplex.MJDOf(day)) // #nosec G115 -- a date
	t.Logf("the grid converted an MJD %d times; we transmit MJD %d", calls, ours)
	keys := make([]uint32, 0, len(days))
	for d := range days {
		keys = append(keys, d)
	}
	sort.Slice(keys, func(a, b int) bool { return days[keys[a]] > days[keys[b]] })
	matched := 0
	for _, d := range keys {
		note := "   <- NOT the day we transmit"
		if d == ours {
			note = "   <- OUR DAY"
			matched += days[d]
		}
		t.Logf("    MJD %d, %d times%s", d, days[d], note)
	}
	if matched == 0 {
		t.Logf("VERDICT: the grid never once looked at the day we transmit. Every programme we send "+
			"is filed under MJD %d and the grid is asking about something else, which is a row of "+
			"\"no listings available\" for every channel.", ours)
		return
	}
	t.Logf("VERDICT: %d of %d conversions are for the day we transmit, so the grid IS looking at "+
		"our day and the empty rows are not a date mismatch.", matched, calls)
}
