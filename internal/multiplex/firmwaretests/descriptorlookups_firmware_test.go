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

// EVERY DESCRIPTOR THE GRID LOOKS UP, BY TAG, WITH WHAT IT GOT BACK.
//
// The row-creating native 0x800CB06C asks its parser for descriptor 0x5F, checks the value it gets
// back is 1, and only then asks for 0xB2 -- reading it INTO the row record. So the row's content is
// a 0xB2 descriptor, and this port transmits none.
//
// Before building one, two things have to be known and neither can be reasoned out:
//
//   - DOES THE 0x5F GATE EVEN PASS? This port sends private_data_specifier 2, which is what DVB
//     registers to BSkyB, and the firmware tests the parsed value against 1. Those are only in
//     conflict if the parser stores the raw specifier; if it maps known specifiers to an index,
//     2 arriving as 1 is exactly right. Changing the value on that guess would be a change made
//     on a reading of a listing, which this project has been wrong about twice in one day.
//   - IS 0xB2 ASKED FOR, AND WHAT DOES IT ANSWER? The record already shows the ROW CALLBACK at
//     0x800CB7B8 reaching its own 0xB2 query once the specifier existed. Whether the row CREATOR
//     reaches its one is a different question about a different function.
//
// SO HOOK THE LOOKUP ITSELF. Both call sites go through a pool word -- 0x800CB338 in the creator,
// 0x800CBA38 in the row callback -- and those hold the address of the descriptor-get. Hooking that
// one function catches every lookup either makes, and its second argument is the TAG.
//
// THE RESULT IS TAKEN AT THE RETURN, with the frame popped. Reading a register when control merely
// leaves a range catches calls on the way out and reports a live register as a result; that mistake
// is filed and withdrawn in this project's own record.
//
// IT ASSERTS ITS OWN SUBJECT: the pool words must hold a plausible address and the function must be
// entered, or a tally of zero lookups is the instrument and not the box.
//
// IT ONLY READS.
func TestEveryDescriptorTagTheGridLooksUp(t *testing.T) {
	const (
		creatorPool  = 0x800CB338 // the descriptor-get used by the row CREATOR 0x800CB06C
		callbackPool = 0x800CBA38 // ...and by the row CALLBACK 0x800CB7B8
		a1, v0, sp   = 5, 2, 29
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
	targets := map[uint32]string{}
	for pool, who := range map[uint32]string{
		creatorPool: "row creator 0x800CB06C", callbackPool: "row callback 0x800CB7B8",
	} {
		at := word(pool) &^ 1
		if at < 0x80000000 || at >= 0x80800000 {
			t.Fatalf("harness: the pool word at %08X holds %08X, which is not a guest function, "+
				"so the lookup used by the %s was not found", pool, word(pool), who)
		}
		t.Logf("%s looks descriptors up through %08X (pool %08X)", who, at, pool)
		targets[at] = who
	}

	// THE PARSER CALLBACKS ARE THE ONLY PROOF A DESCRIPTOR WAS FOUND. A lookup returns 4 whether or
	// not it found anything -- the record already shows a completed 0x5F lookup writing zero -- so
	// the tally below says what was ASKED FOR and nothing about what was there. 0x800CB000 is the
	// callback the row creator hands over for tag 0xB2 and it runs only when the tag matches; if it
	// never executes, the descriptor is not in the stream, whatever the return codes say.
	const (
		parserB2 = 0x800CB000
		parser5F = 0x800CAE88
	)
	parserRuns := map[uint32]int{}

	type tally struct {
		asked   int
		nonZero int
		results map[uint32]int
	}
	byTag := map[uint32]*tally{}
	entered := 0
	inside := false
	var curTag, entrySP, curAt uint32
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Fetch {
			return
		}
		st := box.Machine.Core.State()
		pc := a.Virtual &^ 1
		if pc == parserB2 || pc == parser5F {
			parserRuns[pc]++
		}
		if who, isTarget := targets[pc]; isTarget && !inside {
			_ = who
			inside, entrySP, curAt = true, st.GPR[sp], pc
			curTag = st.GPR[a1]
			entered++
			if byTag[curTag] == nil {
				byTag[curTag] = &tally{results: map[uint32]int{}}
			}
			byTag[curTag].asked++
			return
		}
		if inside && (pc < curAt || pc > curAt+0x600) && st.GPR[sp] >= entrySP {
			inside = false
			byTag[curTag].results[st.GPR[v0]]++
			if st.GPR[v0] != 0 {
				byTag[curTag].nonZero++
			}
		}
	}}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
	settled := openAllChannels(t, press, ".artifacts/descriptors-grid.png", true)
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "descriptors-grid.png"); err != nil {
		t.Fatal(err)
	}
	if entered == 0 {
		t.Fatalf("harness: the descriptor lookup was never entered while the grid drew, so this " +
			"tally of zero is the instrument. Read .artifacts/descriptors-grid.png")
	}
	t.Logf("the grid settled on %08X; %d descriptor lookups", settled, entered)

	tags := make([]uint32, 0, len(byTag))
	for tag := range byTag {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(a, b int) bool { return byTag[tags[a]].asked > byTag[tags[b]].asked })
	t.Logf("=== every descriptor tag looked up while the grid drew ===")
	for _, tag := range tags {
		v := byTag[tag]
		got := make([]string, 0, len(v.results))
		for r, n := range v.results {
			got = append(got, fmt.Sprintf("%d x%d", int32(r), n)) // #nosec G115 -- a status code
		}
		sort.Strings(got)
		name := ""
		switch tag {
		case 0x5f:
			name = " (private_data_specifier -- the gate)"
		case 0xb2:
			name = " (THE ROW'S CONTENT)"
		case 0x48:
			name = " (service)"
		case 0x4a:
			name = " (linkage)"
		case 0x4d:
			name = " (short event)"
		}
		t.Logf("    tag %#04x%-38s asked %2d, %d non-zero, returned %v",
			tag, name, v.asked, v.nonZero, got)
	}
	t.Logf("=== the parser callbacks, which run ONLY when the tag is actually present ===")
	t.Logf("    0x800CAE88  the 0x5F parser   ran %d times", parserRuns[parser5F])
	t.Logf("    0x800CB000  the 0xB2 parser   ran %d times", parserRuns[parserB2])
	if parserRuns[parserB2] == 0 {
		t.Logf("    THE 0xB2 PARSER NEVER RAN, so no 0xB2 descriptor reached the box -- the " +
			"lookups above asked for one and found none. That is a fact about the BROADCAST, " +
			"not about the screen.")
	}
	if byTag[0xb2] == nil {
		t.Logf("TAG 0xB2 WAS NEVER ASKED FOR. The 0x5F gate above it did not pass, so the row's " +
			"content was never even looked for -- and the specifier, not the missing descriptor, " +
			"is the thing to fix first.")
	}
}
