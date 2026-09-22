package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE MAP FROM THE SIGNAL TO THE SCREEN.
//
// Everything so far has been a single question at a time, and the answers have been negatives: the
// grid does not read the line-up records, does not read the sorted channel arrays, does not call
// the channel-detail function, does not read the listings store, and no missing table explains any
// of it because the box never asks for one. What has been missing is the whole picture — WHERE OUR
// BROADCAST ENDS UP, and WHAT THE SCREEN ACTUALLY LOOKS AT.
//
// So this builds both halves and puts them side by side.
//
// **The broadcast's landing sites are measured, not assumed, and that needs a control.** Acquiring
// with the transmitter running writes a great deal of memory, and most of it is the box being a
// box: task switches, timers, heap churn, the display. Attributing all of that to the signal would
// name fifty pages and mean nothing. So a SECOND box runs exactly the same number of instructions
// with the transmitter silent — no sections at all, ever — and the pages it writes are subtracted.
// What is left is memory that exists BECAUSE OF THE BROADCAST:
//
//	landing sites = pages written while acquiring - pages written while silent
//
// Then the first box walks to ALL CHANNELS and every DRAM read is histogrammed by page. Each landing
// site is reported with how much the screen reads it, and the screen's own busiest reads are
// reported with whether the broadcast ever touched them. Three shapes are possible and they want
// completely different work:
//
//	the grid reads landing sites            -> it has our data and rejects it; the fault is the DATA
//	the grid reads pages we never wrote     -> it is looking somewhere the signal does not reach
//	the grid reads almost nothing           -> it gave up before looking, which is what the store
//	                                           measurement already suggests
//
// **THE BANNER IS THE CONTROL FOR THE READ HALF**, because it demonstrably draws a real programme
// off this broadcast. Whatever landing sites IT reads are the ones a working listings screen uses,
// and the grid's silence on those same pages is then a measurement rather than an absence.
//
// IT ONLY READS. Nothing is written into either guest.
func TestTheMapFromTheSignalToTheGrid(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	const acquireFor = 140_000_000

	// --- the silent control: the same box, the same instructions, no sections ever ---
	silent := map[uint32]int{}
	{
		box := restoredBox(t)
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if a.Write && !a.Fetch {
				silent[(a.Virtual&0x1fffffff)>>12]++
			}
		}}
		for i := 0; i < acquireFor; i++ {
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("the SILENT box wrote %d pages in %d instructions", len(silent), acquireFor)
	}

	// --- the real box, acquiring our broadcast ---
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	broadcast := map[uint32]int{}
	acquiring := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Write && !a.Fetch {
			broadcast[(a.Virtual&0x1fffffff)>>12]++
		}
	}}
	for i := 0; i < acquireFor; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(acquiring); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("the ACQUIRING box wrote %d pages in the same %d instructions", len(broadcast), acquireFor)

	// A landing site is a page the broadcast run writes and the silent run does not -- or writes so
	// much more that the difference cannot be ordinary churn. The threshold is deliberately blunt:
	// a page written at all by the silent box is shared machinery whatever the ratio.
	landing := map[uint32]int{}
	for page, n := range broadcast {
		if silent[page] == 0 {
			landing[page] = n
		}
	}
	sites := make([]uint32, 0, len(landing))
	for page := range landing {
		sites = append(sites, page)
	}
	sort.Slice(sites, func(a, b int) bool { return landing[sites[a]] > landing[sites[b]] })
	if len(sites) == 0 {
		t.Fatal("harness: every page the acquiring box wrote was also written by the silent box, " +
			"so this found no memory attributable to the broadcast at all -- which cannot be true " +
			"of a box that has just taken twenty-one programmes off the air")
	}
	t.Logf("=== %d LANDING SITES: pages our broadcast writes and a silent box never does ===", len(sites))
	for i, page := range sites {
		if i >= 25 {
			t.Logf("    ... and %d more", len(sites)-25)
			break
		}
		t.Logf("    %08X  %d writes", 0x80000000|(page<<12), landing[page])
	}

	// --- what each screen reads ---
	readsBy := func(name string, open func(pressFunc) uint32) map[uint32]int {
		t.Helper()
		reads, watching := map[uint32]int{}, false
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if !watching || a.Write || a.Fetch {
				return
			}
			reads[(a.Virtual&0x1fffffff)>>12]++
		}}
		press := func(raw uint8, label string, budget int) uint32 {
			t.Helper()
			if raw == keySelect || raw == keyTVGuide {
				reads, watching = map[uint32]int{}, true
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
		drew := open(press)
		total := 0
		for _, n := range reads {
			total += n
		}
		t.Logf("%s settled on %08X over %d reads across %d pages", name, drew, total, len(reads))
		return reads
	}

	// The banner first: it works, and it is reached from a clean screen.
	banner := readsBy("the now-and-next banner", func(press pressFunc) uint32 {
		return press(keyTVGuide, "tv guide (banner)", 60_000_000)
	})
	// Clear it before walking to the grid, or every screen on the way has a different hash.
	for attempt := 1; attempt <= 4 && screenNow(t, box) != 0; attempt++ {
		if err := box.CSI.Key(keyBackUp, 0); err != nil {
			t.Fatal(err)
		}
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })
		if screenNow(t, box) == 0xA6A21DC5 {
			break
		}
	}
	grid := readsBy("the ALL CHANNELS grid", func(press pressFunc) uint32 {
		return openAllChannels(t, press, ".artifacts/signal-map-grid.png", true)
	})
	if err := dumpScreen(t, box, "signal-map-grid.png"); err != nil {
		t.Fatal(err)
	}

	// --- the map ---
	t.Logf("=== DO THE SCREENS READ WHAT THE BROADCAST WROTE? ===")
	t.Logf("    %-12s %10s %10s %10s", "landing site", "writes", "banner", "grid")
	bannerTotal, gridTotal := 0, 0
	for i, page := range sites {
		if i >= 25 {
			break
		}
		t.Logf("    %08X %10d %10d %10d",
			0x80000000|(page<<12), landing[page], banner[page], grid[page])
		bannerTotal += banner[page]
		gridTotal += grid[page]
	}
	t.Logf("across ALL %d landing sites: the banner reads them %d times, the grid %d times",
		len(sites), sumOver(banner, sites), sumOver(grid, sites))

	t.Logf("=== AND WHAT THE GRID READS INSTEAD, busiest first ===")
	busiest := make([]uint32, 0, len(grid))
	for page := range grid {
		busiest = append(busiest, page)
	}
	sort.Slice(busiest, func(a, b int) bool { return grid[busiest[a]] > grid[busiest[b]] })
	for i, page := range busiest {
		if i >= 20 {
			break
		}
		origin := "the box's own memory -- the broadcast never writes it"
		if landing[page] > 0 {
			origin = "A LANDING SITE of our broadcast"
		} else if silent[page] > 0 && broadcast[page] > 0 {
			origin = "written by both a silent and an acquiring box (shared machinery)"
		}
		t.Logf("    %08X  %8d reads   %s", 0x80000000|(page<<12), grid[page], origin)
	}
}

func sumOver(counts map[uint32]int, pages []uint32) int {
	n := 0
	for _, p := range pages {
		n += counts[p]
	}
	return n
}

// keyBackUp is the handset's "back up" key, used to clear the banner so a route to another screen
// starts from the picture rather than from an overlay.
const keyBackUp = 0x79
