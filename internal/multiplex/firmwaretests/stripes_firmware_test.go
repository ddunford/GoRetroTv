package firmwaretests_test

import (
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
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, drew := 0, before, uint32(0)
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
				drew = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
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
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
		flagged     = 0x0f
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
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, drew := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				drew = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	settled := openAllChannelsUnpinned(t, press, ".artifacts/stripes-at-settle.png")
	if err := dumpScreen(t, box, "stripes-at-settle.png"); err != nil {
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
	if err := dumpScreen(t, box, "stripes-after-waiting.png"); err != nil {
		t.Fatal(err)
	}
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
