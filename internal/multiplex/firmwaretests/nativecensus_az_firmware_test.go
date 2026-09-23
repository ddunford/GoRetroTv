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

// THE WORKING LISTINGS SCREEN AGAINST THE BROKEN ONE.
//
// The grid enters the MIPS listings module ZERO times while it draws, so its decision is in o-code,
// and o-code cannot be decompiled. The one place it becomes legible is where it CALLS OUT: every
// native call is `scall (module, function)` through one of 236 fixed shims.
//
// That comparison has been made once before, against the now-and-next banner, and it was the wrong
// control for this question even though it was the only one available. The banner presents ONE
// programme for the CURRENTLY TUNED service; the grid presents many, for channels it enumerates
// itself. Half the difference between them is that.
//
// **ALL PROGRAMMES A-Z is the right control, and it exists now.** It is a LIST screen, it enumerates
// programmes it was handed rather than the one on screen, and it resolves each to a title, a start
// time and a channel number off exactly the store the grid refuses to ask. Two list screens, one
// drawing programmes and one drawing "..no listings available", with the same broadcast underneath:
// the natives the working one calls and the broken one does not are the shortest description
// anyone has of what the grid is missing.
//
// BOTH DIRECTIONS ARE REPORTED, because a comparison with one side shown is how a difference gets
// mistaken for a cause.
//
// IT ASSERTS ITS OWN SUBJECT: the A-Z screen must have actually DRAWN programmes, not merely been
// reached -- a census of a screen still saying "Searching for listings" would be a census of the
// searching state and would read exactly like a finding about the grid.
func TestWhatTheWorkingListScreenAsksForAndTheGridDoesNot(t *testing.T) {
	atoz := nativeCensusAtoZ(t)
	grid := nativeCensusFor(t, false)
	if len(atoz) == 0 || len(grid) == 0 {
		t.Fatalf("harness: a screen called no natives at all (A-Z %d, grid %d), so the shim table "+
			"is wrong and neither list below means anything", len(atoz), len(grid))
	}
	t.Logf("ALL PROGRAMMES A-Z calls %d distinct natives, the grid %d", len(atoz), len(grid))

	show := func(title, note string, only map[uint32]int) {
		keys := make([]uint32, 0, len(only))
		for at := range only {
			keys = append(keys, at)
		}
		sort.Slice(keys, func(a, b int) bool { return only[keys[a]] > only[keys[b]] })
		t.Logf("=== %s ===", title)
		t.Logf("    (%s)", note)
		for i, at := range keys {
			if i >= 20 {
				t.Logf("    ... and %d more", len(keys)-20)
				break
			}
			id := nativeShims[at]
			t.Logf("    (%d,%#02x) shim %08X impl %08X   %5d calls",
				id.module, id.fn, at, id.impl, only[at])
		}
	}
	onlyAtoZ, onlyGrid := map[uint32]int{}, map[uint32]int{}
	for at, n := range atoz {
		if grid[at] == 0 {
			onlyAtoZ[at] = n
		}
	}
	for at, n := range grid {
		if atoz[at] == 0 {
			onlyGrid[at] = n
		}
	}
	show("NATIVES THE WORKING A-Z LIST CALLS AND THE GRID NEVER DOES",
		"the listings API of a screen that resolves programmes; the grid asks for none of it",
		onlyAtoZ)
	show("NATIVES THE GRID CALLS AND THE A-Z LIST NEVER DOES",
		"the grid's own furniture, shown so a difference is not mistaken for a cause",
		onlyGrid)
	shared := 0
	for at := range atoz {
		if grid[at] > 0 {
			shared++
		}
	}
	t.Logf("%d natives are called by both", shared)
	t.Logf("decompile a promising impl:  ./ctl.sh ghidra:decompile 0x<impl>")
}

// nativeCensusAtoZ counts the natives ALL PROGRAMMES A-Z calls once it is open, with the carousel
// feeding the index so the screen has real programmes to resolve.
func nativeCensusAtoZ(t *testing.T) map[uint32]int {
	t.Helper()
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
	// A whole alphabet must go round before the screen has anything: one index wave carries one
	// letter, and the screen waits until every one of the twenty-six heads exists.
	runUntil(t, box, transmitter, 80_000_000, func(int) bool { return false })

	calls, watching := map[uint32]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		if at := a.Virtual &^ 1; func() bool { _, ok := nativeShims[at]; return ok }() {
			calls[at]++
		}
	}}
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	selects := 0
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			selects++
			if selects == 2 { // the one that opens ALL PROGRAMMES A-Z
				calls, watching = map[uint32]int{}, true
			}
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
	opened := openAllProgrammesAtoZ(t, press, ".artifacts/nativecensus-az.png")
	final := opened
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
	if err := dumpScreen(t, box, "nativecensus-az.png"); err != nil {
		t.Fatal(err)
	}
	// THE SUBJECT. A screen still searching has not resolved anything, and its native list would be
	// the searching state's -- which would read exactly like a finding about the grid.
	if final == opened {
		t.Fatalf("ALL PROGRAMMES A-Z never left the screen it opened on (%08X), so it drew no "+
			"programmes and this census is of the searching state. Read "+
			".artifacts/nativecensus-az.png", opened)
	}
	t.Logf("A-Z opened on %08X and drew %08X -- READ .artifacts/nativecensus-az.png before "+
		"believing the lists below", opened, final)
	return calls
}

// pressAndLetItFinishHooked is pressAndLetItFinish with an observer attached, so a census can watch
// what a press causes without reimplementing the press.
func pressAndLetItFinishHooked(t *testing.T, box *board.Runtime, pump func() error,
	hooks board.StepHooks, raw uint8, budget int) uint32 {
	t.Helper()
	before := screenNow(t, box)
	if err := box.CSI.Key(raw, 0); err != nil {
		t.Fatal(err)
	}
	step := func() {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	stable, last, settled := 0, before, uint32(0)
	for i := 0; i < budget; i++ {
		step()
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
	if settled == 0 {
		return 0
	}
	for i := 0; i < paintTail; i++ {
		step()
	}
	return screenNow(t, box)
}

var _ = fmt.Sprint
