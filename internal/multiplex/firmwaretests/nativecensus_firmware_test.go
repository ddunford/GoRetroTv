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

// WHAT THE TWO SCREENS ASK THE FIRMWARE FOR.
//
// The ALL CHANNELS grid enters the MIPS listings module ZERO times while the banner enters it
// through eleven doors and reads the listings 1,436 times. So the grid's o-code simply never asks —
// and o-code cannot be decompiled, because it is interpreted bytecode with no processor module in
// any disassembler. **The one place an o-code screen becomes legible is where it calls out.**
//
// Every native call the o-code makes is `scall (module, function)` and each enters through a fixed
// shim, 236 of them, resolved from the flash by tools/opentv-natives.py. Watching those addresses
// execute turns "the screen ran some bytecode" into "the screen asked the firmware for THESE
// THINGS". Set the two screens side by side and the difference is not a guess about what the grid
// might want — it is the list of calls the working screen makes and the broken one does not.
//
// **THE BANNER IS THE CONTROL AND IT IS THE RIGHT ONE**: it is the only other screen that presents
// a programme off this broadcast, so the natives it calls and the grid does not are the listings
// API. A native the grid calls and the banner does not is the grid's own furniture, and that
// direction is reported too, because a comparison with only one side shown is how a difference gets
// mistaken for a cause.
//
// IT ASSERTS ITS OWN SUBJECT: both screens must call natives at all (an o-code screen that called
// none would mean the shim table is wrong, not that the screen is idle), and the grid is measured
// with the transport gate OPEN so its row loop runs all six channels.
//
// IT ONLY READS.
func TestWhichNativesEachScreenCalls(t *testing.T) {
	banner := nativeCensusFor(t, true)
	grid := nativeCensusFor(t, false)

	if len(banner) == 0 || len(grid) == 0 {
		t.Fatalf("harness: one of the screens called no native at all (banner %d, grid %d), which "+
			"cannot be true of interpreted o-code -- the shim table is wrong and every difference "+
			"below is an artefact", len(banner), len(grid))
	}
	name := func(shim uint32) string {
		id := nativeShims[shim]
		return sprintNative(id.module, id.fn, id.impl)
	}

	type diff struct {
		shim           uint32
		bannerN, gridN int
	}
	all := map[uint32]bool{}
	for s := range banner {
		all[s] = true
	}
	for s := range grid {
		all[s] = true
	}
	var onlyBanner, onlyGrid, both []diff
	for s := range all {
		d := diff{shim: s, bannerN: banner[s], gridN: grid[s]}
		switch {
		case d.gridN == 0:
			onlyBanner = append(onlyBanner, d)
		case d.bannerN == 0:
			onlyGrid = append(onlyGrid, d)
		default:
			both = append(both, d)
		}
	}
	sort.Slice(onlyBanner, func(a, b int) bool { return onlyBanner[a].bannerN > onlyBanner[b].bannerN })
	sort.Slice(onlyGrid, func(a, b int) bool { return onlyGrid[a].gridN > onlyGrid[b].gridN })

	t.Logf("the banner called %d distinct natives, the grid %d, %d in common",
		len(banner), len(grid), len(both))

	t.Logf("=== NATIVES THE BANNER CALLS AND THE GRID NEVER DOES ===")
	t.Logf("    (the working listings screen's API; the grid asks for none of this)")
	for i, d := range onlyBanner {
		if i >= 40 {
			t.Logf("    ... and %d more", len(onlyBanner)-40)
			break
		}
		t.Logf("    %-34s  banner %5d", name(d.shim), d.bannerN)
	}

	t.Logf("=== NATIVES THE GRID CALLS AND THE BANNER NEVER DOES ===")
	t.Logf("    (the grid's own furniture -- shown so a difference is not mistaken for a cause)")
	for i, d := range onlyGrid {
		if i >= 25 {
			t.Logf("    ... and %d more", len(onlyGrid)-25)
			break
		}
		t.Logf("    %-34s  grid %5d", name(d.shim), d.gridN)
	}
}

func sprintNative(module, fn int, impl uint32) string {
	return fmt.Sprintf("(%d,0x%02X) impl %08X", module, fn, impl)
}

// nativeCensusFor draws one screen and counts every native shim it enters.
func nativeCensusFor(t *testing.T, wantBanner bool) map[uint32]int {
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

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	// Both screens get the transport gate open, so the grid's row loop runs all six channels and a
	// missing native cannot be blamed on the screen giving up early.
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	calls, watching := map[uint32]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		at := a.Virtual &^ 1
		if _, isNative := nativeShims[at]; isNative {
			calls[at]++
		}
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect || raw == keyTVGuide {
			calls, watching = map[uint32]int{}, true
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

	if wantBanner {
		if drew := press(keyTVGuide, "tv guide (banner)", 60_000_000); drew == 0 {
			t.Fatal("harness: the banner drew nothing, so its native census is of nothing")
		}
	} else {
		grid := openAllChannels(t, press, ".artifacts/native-census-grid.png", false)
		if err := dumpScreen(t, box, "native-census-grid.png"); err != nil {
			t.Fatal(err)
		}
		t.Logf("the grid drew %08X with the transport at state %d", grid, state())
	}
	total := 0
	for _, n := range calls {
		total += n
	}
	t.Logf("%d native calls across %d distinct natives", total, len(calls))
	return calls
}
