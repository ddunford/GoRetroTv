package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH INDEX THE ALL CHANNELS GRID READS, IF IT READS ONE AT ALL.
//
// This is the experiment that cracked the A-Z screen, pointed at the grid. ALL PROGRAMMES A-Z drew
// "Searching for listings" for ever, and guessing which of the ninety-two table 0xC1 extensions it
// wanted would have taken ninety-two runs. It did not need guessing: the parser hands a decoded
// array to whatever sits in a list-head slot, AND THE SLOT INDEX IS THE EXTENSION, so a screen
// waiting for an index is a screen READING THE SLOT IT WANTS. Watching the three tables named it in
// one run -- letter 'A' -- without transmitting anything.
//
// The grid is the same shape of problem and deserves the same question rather than another sweep.
// It draws six correct rows off the line-up and "..no listings available" in every cell, while the
// A-Z screen next door resolves programmes from the same store and the banner shows one for the
// same channel at the same minute. Delivering all sixty-four genre extensions before it opened
// changed nothing, with 64 of 64 category slots filled and a control.
//
//	0x800C51A0 -> base of twenty-seven heads: slot 0 is extension 0x0000, slots 1..26 are 'A'..'Z'
//	0x800C51A4 -> the single head for extension 0x00FF
//	0x800C51A8 -> sixteen categories by four blocks, slot (ext & 0x0F) * 4 + ((ext & 0xC0) >> 6)
//
// EITHER ANSWER IS WORTH THE RUN, which is what makes it worth making. A slot the grid reads names
// the extension to transmit under. NO slot read at all eliminates table 0xC1 for this screen
// outright -- and that matters, because the grid and the A-Z screen failed in ways that look
// identical from outside, and the temptation to assume the same cause is exactly what a control is
// for.
//
// IT ASSERTS ITS OWN SUBJECT: the observer must see data reads at all, and the grid must be the
// screen actually on display -- this project has measured the wrong screen five times.
//
// IT ONLY READS, and it transmits nothing beyond the ordinary broadcast.
func TestWhichIndexSlotTheAllChannelsGridReads(t *testing.T) {
	const (
		headsPool      = 0x800C51A0
		ffPool         = 0x800C51A4
		categoriesPool = 0x800C51A8
		letterSlots    = 27
		categorySlots  = 64
	)
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

	word := func(virtual uint32) uint32 { return box.RAM.Read(virtual&0x1fffffff, bus.Word) }
	heads, ff, categories := word(headsPool), word(ffPool), word(categoriesPool)
	t.Logf("letter heads at %08X, the 0x00FF head at %08X, the category table at %08X",
		heads, ff, categories)
	for name, base := range map[string]uint32{"letter heads": heads, "category table": categories} {
		if base&0xf0000000 == 0 || base&3 != 0 {
			t.Fatalf("harness: the %s pointer is %08X, which is not a word-aligned guest address, "+
				"so every slot read below would be noise", name, base)
		}
	}

	// BY THE INSTRUCTION THAT READ IT, NOT JUST BY THE SLOT. The carousel keeps transmitting while
	// the grid draws, and the 0xC1 parser reads heads[ext - 0x40] on EVERY section it decodes -- so
	// a naive count reports the grid reading all twenty-six letters when what it is really seeing
	// is the transmitter. The parser lives at 0x800C4C34..0x800C4F00; anything reading these tables
	// from outside that range is a genuine consumer, and that is the only interesting kind.
	// THE WHOLE 0xC1 MODULE, not just the parser. 0x800C4C34 decodes a section and reads the list
	// head it is about to hand its array to; 0x800C4F94 FREES a list and reads the same head to do
	// it. Both are the module talking to itself, and both fire constantly because the carousel
	// keeps delivering -- a first version of this excluded only the parser and reported the grid
	// reading twenty-one letters, every one of them from 0x800C4FAC inside the free routine.
	const (
		moduleFrom = 0x800C4C34
		moduleTo   = 0x800C5010
	)
	type slotRead struct {
		slot string
		pc   uint32
	}
	slotReads := map[slotRead]int{}
	parserReads := 0
	anyRead := 0
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		anyRead++
		at := a.Virtual | 0x80000000
		name := ""
		switch {
		case at >= heads && at < heads+letterSlots*4:
			slot := (at - heads) / 4
			name = "extension 0x0000 (slot 0)"
			if slot > 0 {
				name = fmt.Sprintf("letter %q (extension %#04x)", rune('A'+slot-1), 0x40+slot)
			}
		case at >= ff && at < ff+4:
			name = "extension 0x00FF"
		case at >= categories && at < categories+categorySlots*4:
			slot := (at - categories) / 4
			name = fmt.Sprintf("category %d block %d (extension %#04x)",
				slot/4, slot%4, 0x0100|(slot%4)<<6|(slot/4))
		default:
			return
		}
		pc := box.Machine.Core.State().PC &^ 1
		if pc >= moduleFrom && pc < moduleTo {
			parserReads++
			return
		}
		slotReads[slotRead{slot: name, pc: pc}]++
	}}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			slotReads, parserReads, anyRead, watching = map[slotRead]int{}, 0, 0, true
		}
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}

	settled := openAllChannelsFinished(t, press, ".artifacts/gridslot-grid.png")
	// PAST THE SETTLE: the rows paint in bursts that hold still across four samples and carry on
	// afterwards, and anything they ask for arrives here too.
	final := settled
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
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
	if err := dumpScreen(t, box, "gridslot-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid settled on %08X and finished on %08X", settled, final)
	if anyRead == 0 {
		t.Fatal("harness: the observer saw no data read at all while the grid drew, so its " +
			"silence is the instrument and not the screen")
	}
	t.Logf("the 0xC1 module itself read a list head %d times while this ran -- the parser handing "+
		"arrays in and the free routine taking them out again -- and that is excluded below",
		parserReads)
	if len(slotReads) == 0 {
		t.Logf("=== THE GRID READ NONE OF THE THREE INDEX TABLES ===")
		t.Logf("    %d data reads were seen while it drew, and not one of them touched a table "+
			"0xC1 list head from outside the parser. So the grid is NOT waiting on an index under "+
			"any extension, and its resemblance to the A-Z screen's failure is a coincidence of "+
			"appearance. That eliminates 0xC1 for this screen by measurement rather than by "+
			"ninety-two sweeps, and it is the useful half of this run.", anyRead)
		t.Logf("    READ .artifacts/gridslot-grid.png to confirm the grid is what was on screen")
		return
	}
	keys := make([]slotRead, 0, len(slotReads))
	for k := range slotReads {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool { return slotReads[keys[a]] > slotReads[keys[b]] })
	t.Logf("=== the index slots ALL CHANNELS reads while it draws, EXCLUDING the parser ===")
	for i, k := range keys {
		if i >= 25 {
			t.Logf("    ... and %d more", len(keys)-25)
			break
		}
		t.Logf("    %-46s read from %08X   %d times", k.slot, k.pc, slotReads[k])
	}
	t.Logf("each slot named above is an extension to transmit table 0xC1 under -- that is how the " +
		"A-Z screen was solved, and it took one run rather than ninety-two")
}
