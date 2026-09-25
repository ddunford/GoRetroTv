package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHERE THE GRID TURNS AWAY FROM THE LISTINGS, AND THE CONDITION THAT SENDS IT.
//
// The map says the two screens do not differ by how much of our broadcast they consume — the grid
// reads the landing sites 22,066 times against the banner's 5,216. They differ by WHICH part. Six
// landing sites the working banner reads and the grid touches exactly zero times:
//
//	80199000  80187000  80186000  802FB000  8010A000  801A5000
//
// That is the listings data path. The banner enters it; the grid never does. So there is a branch
// somewhere that both screens reach and only one takes, and whatever it tests is the condition this
// whole phase has been looking for. It could be a signal lock, an acquisition flag, a "guide ready"
// bit, a subscription, a day or a time window — naming it in advance would be a guess, and this
// project has paid for those. **Finding the branch is what identifies it.**
//
// The method is a divergence hunt rather than a search:
//
//  1. Draw the BANNER and record every instruction that reads those six pages, WITH ITS CALLER.
//     Those callers are the functions that enter the listings path.
//  2. Draw the GRID and record every unique instruction it executes.
//  3. Intersect. A caller the grid NEVER EXECUTES is below the divergence — it was never reached.
//     A caller the grid DOES EXECUTE is the divergence itself: both screens got there, and only one
//     went on to read. **The branch inside that function is the condition.**
//
// If several callers are executed by both, the useful one is the shallowest, which is why they are
// reported with their read counts and sorted: the function doing the most reading in the banner and
// still running in the grid is where to look first.
//
// EACH SCREEN GETS ITS OWN BOX, because measuring both on one leaves the banner over the picture and
// every screen on the route to the grid then has a different hash.
//
// IT ASSERTS ITS OWN SUBJECT: the banner must read the listings pages (or there is no path to find)
// and the grid must execute a healthy number of instructions (or its empty set is the instrument).
//
// IT ONLY READS.
func TestWhereTheGridTurnsAwayFromTheListings(t *testing.T) {
	readers, bannerPCs, entries := listingsPathOf(t, true)
	_, gridPCs, gridEntries := listingsPathOf(t, false)

	if len(readers) == 0 {
		t.Fatal("harness: the banner read the listings pages from no instruction at all, so there " +
			"is no path to diverge from and the intersection below would be empty for the wrong " +
			"reason")
	}
	if len(gridPCs) < 1000 {
		t.Fatalf("harness: the grid executed only %d distinct instructions, which is too few for a "+
			"screen that draws a header, a date, a clock and a time axis -- its empty set would be "+
			"the instrument", len(gridPCs))
	}
	t.Logf("the banner executed %d distinct instructions, the grid %d", len(bannerPCs), len(gridPCs))

	type entry struct {
		caller   uint32
		reads    int
		sites    int
		inGrid   bool
		inBanner bool
	}
	byCaller := map[uint32]*entry{}
	for site, n := range readers {
		e := byCaller[site.ra]
		if e == nil {
			e = &entry{caller: site.ra}
			byCaller[site.ra] = e
		}
		e.reads += n
		e.sites++
	}
	list := make([]*entry, 0, len(byCaller))
	for _, e := range byCaller {
		e.inGrid, e.inBanner = gridPCs[e.caller], bannerPCs[e.caller]
		list = append(list, e)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].reads > list[b].reads })

	shared, only := 0, 0
	for _, e := range list {
		if e.inGrid {
			shared++
		} else {
			only++
		}
	}
	t.Logf("=== %d functions enter the listings path while the banner draws ===", len(list))
	t.Logf("    %d of them the GRID ALSO EXECUTES -- those are the divergence points", shared)
	t.Logf("    %d it never reaches at all", only)

	t.Logf("=== THE DIVERGENCE POINTS: both screens run these, only the banner goes on to read ===")
	shownAny := false
	for _, e := range list {
		if !e.inGrid {
			continue
		}
		shownAny = true
		t.Logf("    caller %08X  %d reads across %d instructions, IN BOTH SCREENS", e.caller, e.reads, e.sites)
	}
	if !shownAny {
		t.Logf("    NONE. The grid executes not one of the functions that enter the listings path, " +
			"so the turn happens further up still: it is not refusing at the door, it never walks " +
			"down the corridor. The callers below are the path, and the next step is to find what " +
			"calls the shallowest of them.")
	}

	// THE MODULE'S DOORS. Every place outside 0x800A8000..0x800AB000 that calls into it while the
	// banner draws, and whether the grid ever executes that instruction. A door the grid REACHES is
	// the divergence: both screens stand in front of it and only one goes through.
	doors := make([]uint32, 0, len(entries))
	for at := range entries {
		doors = append(doors, at)
	}
	sort.Slice(doors, func(a, b int) bool { return entries[doors[a]] > entries[doors[b]] })
	t.Logf("=== %d DOORS into the listings module while the banner draws ===", len(doors))
	reached := 0
	for i, at := range doors {
		if i >= 25 {
			t.Logf("    ... and %d more", len(doors)-25)
			break
		}
		mark := "the grid never executes it"
		if gridPCs[at] {
			mark = "*** THE GRID EXECUTES THIS ONE ***"
			reached++
		}
		t.Logf("    door %08X  entered %d times  %s", at, entries[at], mark)
	}
	t.Logf("the grid entered the listings module %d times itself", len(gridEntries))
	if reached == 0 {
		t.Logf("NONE of the shown doors is executed by the grid: the turn is further out still.")
	}

	t.Logf("=== THE LISTINGS PATH ITSELF, busiest first (banner) ===")
	for i, e := range list {
		if i >= 20 {
			t.Logf("    ... and %d more", len(list)-20)
			break
		}
		where := "the grid NEVER runs it"
		if e.inGrid {
			where = "the grid RUNS IT TOO"
		}
		t.Logf("    caller %08X  %6d reads  %s", e.caller, e.reads, where)
	}
}

// listingsPathOf draws one screen and reports which instructions read the listings landing sites
// (with their callers) and every instruction the screen executed.
// listingsModuleLo/Hi bound the functions that actually read the listings. Every caller the
// divergence hunt found reading those pages and NOT executed by the grid falls in here --
// 0x800A8520, 0x800A86DC, 0x800A9046, 0x800AA968 -- and 0x800AA8F8, the instruction that writes the
// transport state, is in the same range. The grid runs the channel enumeration at 0x800A4xxx and
// nothing in this module at all.
const (
	listingsModuleLo = 0x800A8000
	listingsModuleHi = 0x800AB000
)

func listingsPathOf(t *testing.T, wantBanner bool) (map[painterSite]int, map[uint32]bool, map[uint32]int) {
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
	// Both screens are measured with the transport ready, so the grid's first gate is open and
	// cannot be why it fails to enter the path.
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	entries := map[uint32]int{}
	// The six landing sites the banner reads and the grid does not, from the signal map.
	listingsPages := map[uint32]bool{
		0x00199: true, 0x00187: true, 0x00186: true,
		0x002FB: true, 0x0010A: true, 0x001A5: true,
	}
	readers, executed := map[painterSite]int{}, map[uint32]bool{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching {
			return
		}
		if a.Fetch {
			at := a.Virtual &^ 1
			executed[at] = true
			// THE MODULE'S ENTRY POINTS. The listings functions all live in one range, and the
			// grid runs none of them; what matters is who calls into that range and whether the
			// grid reaches THAT. At a fetch inside the module with ra pointing OUTSIDE it, ra is a
			// cross-module return address -- somebody outside called in.
			if at >= listingsModuleLo && at < listingsModuleHi {
				if ra := box.Machine.Core.State().GPR[31] &^ 1; ra < listingsModuleLo || ra >= listingsModuleHi {
					entries[ra]++
				}
			}
			return
		}
		if a.Write {
			return
		}
		if listingsPages[(a.Virtual&0x1fffffff)>>12] {
			st := box.Machine.Core.State()
			readers[painterSite{pc: st.PC &^ 1, ra: st.GPR[31] &^ 1}]++
		}
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect || raw == keySky {
			readers, executed, watching = map[painterSite]int{}, map[uint32]bool{}, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	if wantBanner {
		if drew := press(keySky, "sky (banner)", 60_000_000); drew == 0 {
			t.Fatal("harness: the banner drew nothing, so the listings path was not walked")
		}
		total := 0
		for _, n := range readers {
			total += n
		}
		if total == 0 {
			t.Fatal("harness: the banner read the listings pages zero times, contradicting the " +
				"signal map it is built on -- the pages or the route are wrong")
		}
		t.Logf("the banner read the listings pages %d times from %d instructions", total, len(readers))
		return readers, executed, entries
	}
	grid := openAllChannels(t, press, ".artifacts/divergence-grid.png", false)
	if err := dumpScreen(t, box, "divergence-grid.png"); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, n := range readers {
		total += n
	}
	t.Logf("the grid drew %08X and read the listings pages %d times", grid, total)
	return readers, executed, entries
}
