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

// WHICH INDEX THE SEARCHING SCREEN IS WAITING FOR, ASKED BY WATCHING IT LOOK.
//
// "ALL PROGRAMMES A-Z" draws FOR YOUR INFORMATION / Searching for listings / Please wait, and it
// keeps drawing it. Twelve cycles of table 0xC1 under letter 'A' did not move it -- with a control
// proving the screen is identical when nothing is delivered at all.
//
// The obvious next move is to guess a different extension, and there are ninety-two to choose from.
// The parser hands a decoded array to whatever sits in a list-head slot, and THE SLOT INDEX IS THE
// EXTENSION, so a screen waiting for an index is a screen reading the slot it wants. Watching the
// three tables says which, without sending anything at all:
//
//	0x800C51A0 -> base of twenty-seven heads: slot 0 is extension 0x0000, slots 1..26 are 'A'..'Z'
//	0x800C51A4 -> the single head for extension 0x00FF
//	0x800C51A8 -> sixteen categories by four blocks, slot (ext & 0x0F) * 4 + ((ext & 0xC0) >> 6)
//
// IT ASSERTS ITS OWN SUBJECT. A screen that reads none of the three is a screen this reading does
// not explain, and that is reported as a harness result rather than as "the box wants nothing".
func TestWhichIndexSlotTheSearchingScreenReads(t *testing.T) {
	const (
		headsPool      = 0x800C51A0
		ffPool         = 0x800C51A4
		categoriesPool = 0x800C51A8
		letterSlots    = 27 // 0x0000 plus 'A'..'Z'
		categorySlots  = 64 // sixteen categories by four blocks
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
	// NO TRANSPORT-STATE WAIT. Running on to state 6 leaves the box not taking input at all: every
	// press returns the screen it started on, which reads exactly like a broken route and is not
	// one. Registration plus a quiet period is what the probes that navigate successfully do.
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	word := func(virtual uint32) uint32 { return box.RAM.Read(virtual&0x1fffffff, bus.Word) }
	heads, ff, categories := word(headsPool), word(ffPool), word(categoriesPool)
	t.Logf("letter heads at %08X, the 0x00FF head at %08X, the category table at %08X",
		heads, ff, categories)
	for name, base := range map[string]uint32{"letter heads": heads, "category table": categories} {
		if base&0xf0000000 == 0 || base&3 != 0 {
			t.Fatalf("harness: the %s pointer is %08X, which is not a word-aligned guest address, "+
				"so the slot reads below would be noise", name, base)
		}
	}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		settled := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-32s %08X -> %08X", name, before, settled)
		return settled
	}

	menu := press(0x7D, "box office", 80_000_000)
	tab := menu
	for attempt := 1; attempt <= 4 && (tab == menu || tab == boxOfficeMenu || tab == 0); attempt++ {
		tab = press(0x5A, "left to tv guide", 80_000_000)
	}
	if tab == menu || tab == boxOfficeMenu || tab == 0 {
		t.Fatalf("harness: the tv guide tab was never reached (%08X)", tab)
	}
	moves := 0
	for attempt := 0; attempt < 40 && moves < 9; attempt++ {
		if press(0x59, fmt.Sprintf("down (move %d/9)", moves+1), 4_000_000) != 0 {
			moves++
		}
	}
	if moves < 9 {
		t.Fatalf("harness: the highlight moved only %d of 9 entries", moves)
	}
	az := uint32(0)
	for attempt := 1; attempt <= 6 && (az == 0 || az == tab); attempt++ {
		az = press(0x5C, fmt.Sprintf("select A-Z (try %d)", attempt), 80_000_000)
	}
	if az == 0 || az == tab {
		t.Fatalf("harness: A-Z LISTINGS was never opened")
	}
	// Let the category menu paint before selecting through it; it draws itself about four million
	// instructions after it opens and a SELECT sent earlier is swallowed.
	for i := 0; i < 8_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	menuAZ := screenNow(t, box)
	inner := uint32(0)
	for attempt := 1; attempt <= 6 && (inner == 0 || inner == menuAZ); attempt++ {
		inner = press(0x5C, fmt.Sprintf("select ALL PROGRAMMES (try %d)", attempt), 80_000_000)
	}
	if inner == 0 || inner == menuAZ {
		t.Fatalf("harness: SELECT never left the A-Z category menu (%08X)", menuAZ)
	}
	if err := dumpScreen(t, box, "whichslot-searching.png"); err != nil {
		t.Fatal(err)
	}

	// NOW WATCH IT LOOK.
	slotReads := map[string]int{}
	anyRead := 0
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Write || a.Fetch {
			return
		}
		anyRead++
		at := a.Virtual | 0x80000000
		switch {
		case at >= heads && at < heads+letterSlots*4:
			slot := (at - heads) / 4
			name := "extension 0x0000"
			if slot > 0 {
				name = fmt.Sprintf("letter %q (extension %#04x)", rune('A'+slot-1), 0x40+slot)
			}
			slotReads[name]++
		case at >= ff && at < ff+4:
			slotReads["extension 0x00FF"]++
		case at >= categories && at < categories+categorySlots*4:
			slot := (at - categories) / 4
			slotReads[fmt.Sprintf("category %d block %d (extension %#04x)",
				slot/4, slot%4, 0x0100|(slot%4)<<6|(slot/4))]++
		}
	}}
	for i := 0; i < 30_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if anyRead == 0 {
		t.Fatal("harness: the observer saw no data read at all, so its silence is the instrument")
	}
	if len(slotReads) == 0 {
		t.Fatalf("the searching screen read NONE of the three index tables in 30,000,000 "+
			"instructions (%d data reads seen overall). Either it is not waiting on table 0xC1 at "+
			"all, or the three pool words are not the tables -- do not read this as 'the box wants "+
			"nothing'", anyRead)
	}
	names := make([]string, 0, len(slotReads))
	for name := range slotReads {
		names = append(names, name)
	}
	sort.Slice(names, func(a, b int) bool { return slotReads[names[a]] > slotReads[names[b]] })
	t.Logf("=== the slots ALL PROGRAMMES A-Z reads while it says 'Searching for listings' ===")
	for _, name := range names {
		t.Logf("    %-44s %d reads", name, slotReads[name])
	}
	t.Logf("each slot named above is an extension to transmit table 0xC1 under")
}
