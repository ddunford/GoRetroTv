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

// WHAT THE GENRE GRID READS TO DECIDE A CHANNEL DOES NOT BELONG.
//
// ALL CHANNELS and ENTERTAINMENT are the SAME grid widget on the same broadcast -- same chrome,
// same column headers -- and one draws six channels while the other draws none
// (TestWhichCategorySlotEachGenreScreenReads, and the pictures). So there is a per-channel test,
// it runs six times, and it says no six times.
//
// THE FIELD IT TESTS IS READ IN BOTH RUNS, so a set difference of ADDRESSES cannot find it: ALL
// CHANNELS reads the same field and accepts. What differs is the BRANCH -- the reject path runs
// only in the genre screen, and the accept path only in ALL CHANNELS. That is exactly how the
// 0xB2's first scalar was found: 0x9FC73961 was named as the branch, and what it had just read
// was the answer.
//
// So this counts guest PCs, and the one thing it must not do is compare a delivered screen against
// silence -- the mistake the 0xC1 sweeps made, where the cost of merely arriving swamped the
// signal. Both sides here open a grid from the same menu with the same broadcast, which is
// like-for-like by construction, and A SECOND ALL CHANNELS RUN IS THE NOISE FLOOR. This emulator
// is deterministic, so that floor should be exactly zero; if it is not, nothing below is a finding.
//
// IT ASSERTS ITS OWN SUBJECT TWICE: the two screens must actually differ (or there is no reject to
// find), and the noise floor must be zero (or the differential is measuring divergence).
//
// IT ONLY READS.
func TestWhatTheGenreGridRejects(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	// The grid is interpreted OpenTV o-code held in FLASH and fetched as DATA, so a flash READ is
	// the o-code program counter. Both are collected: the native PCs name the RTOS side, and the
	// flash reads name the o-code, which is where the 0xB2's branch turned out to live.
	type draw struct {
		screen uint32
		pcs    map[uint32]bool
		ocode  map[uint32]bool
	}

	open := func(t *testing.T, key uint8, name string) draw {
		t.Helper()
		guide := demoGuide(t)
		dict := demoDictionary(t)
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			demoSchedule())
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

		pump := func() error { return transmitter.Pump(box.Machine.Retired) }
		press := azPressFunc(t, box, pump)
		menu := uint32(0)
		for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
			menu = press(keyBoxOffice, "box office", pressBudget)
		}
		if menu != boxOfficeMenu {
			t.Fatalf("harness: box office drew %08X, not %08X", menu, uint32(boxOfficeMenu))
		}
		tab := uint32(0)
		for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
			tab = press(keyLeft, fmt.Sprintf("left to the tv guide menu (%d)", attempt), pressBudget)
		}
		if tab != tvGuideMenuScreen {
			t.Fatalf("harness: never reached the TV GUIDE menu; drew %08X", tab)
		}

		// THE WATCH GOES ON BEFORE THE PRESS, because the decision is taken WHILE the grid draws.
		// Watching after it settles is how the category sweep nearly missed a screen that looks
		// once and stops.
		out := draw{pcs: map[uint32]bool{}, ocode: map[uint32]bool{}}
		// THE O-CODE IS A FLASH READ, not a fetch. The grid is interpreted bytecode held in FLASH
		// and fetched as DATA, so the address a flash read names IS the o-code program counter --
		// the reconciliation that found the 0xB2's branch.
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if a.Fetch || a.Write {
				return
			}
			if at := a.Virtual &^ 0x20000000; at >= 0x9fc00000 && at < 0xa0000000 {
				out.ocode[at] = true
			}
		}}
		for attempt := 1; attempt <= attempts && (out.screen == 0 || out.screen == tab); attempt++ {
			out.screen = pressAndLetItFinishWatching(t, box, pump, hooks, key, pressBudget,
				func(int) { out.pcs[box.Machine.Core.State().PC&^1] = true })
		}
		if out.screen == 0 || out.screen == tab {
			t.Fatalf("harness: key %d (%s) never left the TV GUIDE menu (%08X)", key, name, tab)
		}
		t.Logf("%-14s %08X   %d native PCs, %d o-code addresses",
			name, out.screen, len(out.pcs), len(out.ocode))
		return out
	}

	all := open(t, 0x01, "ALL CHANNELS")
	control := open(t, 0x01, "ALL CHANNELS again")
	genre := open(t, 0x02, "ENTERTAINMENT")

	if all.screen == genre.screen {
		t.Fatalf("harness: ALL CHANNELS and ENTERTAINMENT both drew %08X, so there is no reject "+
			"to find and this differential has nothing to measure", all.screen)
	}
	only := func(a, b map[uint32]bool, also ...map[uint32]bool) []uint32 {
		var out []uint32
	next:
		for pc := range a {
			if b[pc] {
				continue
			}
			for _, m := range also {
				if m[pc] {
					continue next
				}
			}
			out = append(out, pc)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}
	floorPCs := only(control.pcs, all.pcs)
	floorOCode := only(control.ocode, all.ocode)
	t.Logf("NOISE FLOOR from two runs of the SAME screen: %d native PCs, %d o-code addresses",
		len(floorPCs), len(floorOCode))
	if len(floorPCs) != 0 || len(floorOCode) != 0 {
		t.Fatalf("harness: two runs of ALL CHANNELS diverged by %d native PCs and %d o-code "+
			"addresses. This emulator is deterministic, so a non-zero floor means the two runs are "+
			"not the same experiment -- everything below would be that divergence wearing a "+
			"finding's clothes", len(floorPCs), len(floorOCode))
	}

	rejectPCs := only(genre.pcs, all.pcs, control.pcs)
	rejectOCode := only(genre.ocode, all.ocode, control.ocode)
	acceptPCs := only(all.pcs, genre.pcs)
	acceptOCode := only(all.ocode, genre.ocode)

	show := func(label string, addrs []uint32) {
		t.Logf("%s: %d", label, len(addrs))
		var spans [][2]uint32
		for _, a := range addrs {
			if n := len(spans); n > 0 && a-spans[n-1][1] <= 8 {
				spans[n-1][1] = a
				continue
			}
			spans = append(spans, [2]uint32{a, a})
		}
		sort.SliceStable(spans, func(i, j int) bool {
			return spans[i][1]-spans[i][0] > spans[j][1]-spans[j][0]
		})
		for i, sp := range spans {
			if i >= 12 {
				t.Logf("    ... %d narrower runs", len(spans)-i)
				break
			}
			t.Logf("    %08X..%08X  %d bytes", sp[0], sp[1], sp[1]-sp[0]+2)
		}
	}
	t.Logf("=== ONLY WHEN THE GENRE SCREEN REJECTS ALL SIX CHANNELS ===")
	show("native PCs", rejectPCs)
	show("o-code addresses", rejectOCode)
	t.Logf("=== ONLY WHEN ALL CHANNELS ACCEPTS THEM ===")
	show("native PCs", acceptPCs)
	show("o-code addresses", acceptOCode)

	if len(rejectPCs) == 0 && len(rejectOCode) == 0 {
		t.Fatalf("the genre screen ran NOTHING the ALL CHANNELS screen did not, against a floor "+
			"of zero, yet it drew a different picture (%08X against %08X). That is not possible "+
			"if both were watched properly, so read the watch before believing either",
			genre.screen, all.screen)
	}
	t.Logf("the o-code runs above are the ones to disassemble with tools/ocode-disasm.py: the " +
		"0xB2's first scalar was found exactly this way, by naming the branch and reading what it " +
		"had just loaded")
}
