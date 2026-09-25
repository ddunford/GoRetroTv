package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DO THE TWO BROKEN SCREENS READ THE BOX'S STORED PROGRAMMES AT ALL?
//
// **The ALL CHANNELS grid is not a special case, and that changes what to look for.** Opened under
// its own letter with a 0xC1 array assembled in its heap, A-Z LISTINGS draws its header, the
// in-world date, the clock — and an empty body. That is the same shape as the grid exactly:
// furniture, no content. And it does not read the array we fed it, in 5.4 million data reads.
//
// Set against the screens that work, a pattern falls out that no amount of following the grid alone
// would have shown:
//
//	BOX OFFICE menu     six rows        works     a static menu
//	TV GUIDE menu       ten rows        works     a static menu
//	now-and-next banner a real programme works    ONE channel's listing
//	ALL CHANNELS grid   nothing                   listings, many channels
//	A-Z LISTINGS        nothing                   listings, many programmes
//
// The two that fail are the two that present the LISTINGS DATABASE across many entries. So the
// question stops being "what is wrong with the grid" and becomes the one that covers both:
//
//	does either screen consult the box's stored programmes, or do they refuse before they look?
//
// **THE BANNER IS THE CONTROL AND IT IS A STRONG ONE**, because it demonstrably puts a real
// programme title on screen off the same store, on the same box, seconds apart. If it reads the
// store and the other two do not, the gate is common to both and sits BEFORE any query — which is a
// single thing to find rather than two screens to debug.
//
// IT FINDS THE STORE RATHER THAN ASSUMING IT: the programme titles are swept for in DRAM as text,
// so the region watched is where this box actually put them on this run. If they are not found as
// text the probe says so and stops, because a watch on a region chosen by guesswork reports a
// confident zero.
//
// IT ONLY READS.
func TestWhetherTheBrokenScreensReadTheListingsStore(t *testing.T) {
	for _, screen := range []struct {
		name  string
		open  func(*testing.T, pressFunc) uint32
		works bool
	}{
		{name: "the now-and-next banner (control: it works)", works: true,
			open: func(t *testing.T, press pressFunc) uint32 {
				return press(keySky, "sky (banner)", 60_000_000)
			}},
		{name: "the ALL CHANNELS grid",
			open: func(t *testing.T, press pressFunc) uint32 {
				return openAllChannels(t, press, ".artifacts/store-grid.png", false)
			}},
	} {
		t.Run(screen.name, func(t *testing.T) {
			readListingsStore(t, screen.open, screen.works)
		})
	}
}

func readListingsStore(t *testing.T, open func(*testing.T, pressFunc) uint32, expectReads bool) {
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

	// FIND THE STORE BY WATCHING THE BOX FILL IT, DURING ACQUISITION.
	//
	// **The titles are not held as text.** A sweep of all of DRAM for the programme titles the
	// broadcast carried finds none of them, so the box keeps its listings in some coded form and a
	// text search cannot locate the store. That closed the obvious handle.
	//
	// The handle that works is the one this package already relies on: 0x800C587C is the per-event
	// register, executed once per programme the box takes off the air. **It has to be watched WHILE
	// the box is acquiring, not afterwards** -- a first version watched for it after acquisition
	// finished and captured nothing in forty million instructions, for the good reason that the box
	// had already taken everything it wanted. The addresses that instruction touches are the
	// store's own, measured on this run rather than chosen.
	want := programmesInTheBlock(t, guide, day)
	store := map[uint32]bool{}
	registered := 0
	// The register instruction itself performs NO load or store -- a first attempt watched only its
	// own accesses and captured nothing, which is what "an instruction that files a programme
	// touches no memory" was telling us. It is a marker, not the write. So the window after it is
	// what gets recorded, and WRITES only: filing a programme writes it somewhere.
	window := 0
	acquire := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			if a.Virtual&^1 == pcPerEventRegister {
				registered++
				window = 300
			}
			return
		}
		if window > 0 {
			window--
			if a.Write {
				store[a.Virtual&0x1fffffff] = true
			}
		}
	}}
	for i := 0; i < 120_000_000 && registered < want; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(acquire); err != nil {
			t.Fatal(err)
		}
	}
	if registered < want {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	// The transport is made ready for every screen, so the grid's first gate is open and cannot be
	// the reason for a null result here.
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("%d programmes registered, transport state %d", registered, state())

	if len(store) == 0 {
		t.Fatal("harness: no write followed the per-event register in a three-hundred-access " +
			"window, so no store address was captured and a zero below would be the instrument")
	}
	offs := make([]uint32, 0, len(store))
	for at := range store {
		offs = append(offs, at)
	}
	sort.Slice(offs, func(a, b int) bool { return offs[a] < offs[b] })
	lo, hi := offs[0]&^0xfff, (offs[len(offs)-1]|0xfff)+1
	t.Logf("the per-event register touched %d distinct words, %08X..%08X; watching %08X..%08X",
		len(offs), 0x80000000|offs[0], 0x80000000|offs[len(offs)-1], 0x80000000|lo, 0x80000000|hi)

	reads, watching := map[uint32]int{}, false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		if at := a.Virtual & 0x1fffffff; at >= lo && at < hi {
			reads[box.Machine.Core.State().PC&^1]++
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect || raw == keySky {
			reads, watching = map[uint32]int{}, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", name, drew)
		return drew
	}

	drew := open(t, press)
	if drew == 0 {
		t.Fatal("harness: the screen drew nothing new, so nothing was measured")
	}
	total := 0
	for _, n := range reads {
		total += n
	}
	t.Logf("the screen drew %08X and read the listings store %d times from %d instructions",
		drew, total, len(reads))
	if total > 0 {
		pcs := make([]uint32, 0, len(reads))
		for pc := range reads {
			pcs = append(pcs, pc)
		}
		sort.Slice(pcs, func(a, b int) bool { return reads[pcs[a]] > reads[pcs[b]] })
		for i, pc := range pcs {
			if i >= 10 {
				break
			}
			t.Logf("    %08X  %d reads", pc, reads[pc])
		}
	}
	switch {
	case expectReads && total == 0:
		t.Fatalf("harness: the CONTROL screen -- the one that demonstrably puts a real programme "+
			"title on screen -- read the store zero times. The region %08X..%08X is not where this "+
			"box reads its listings from, so a zero for the other screens would prove nothing",
			0x80000000|lo, 0x80000000|hi)
	case expectReads:
		t.Logf("VERDICT: the control reads the store, so the watch is on the right memory and a " +
			"zero from another screen is a real negative.")
	case total == 0:
		t.Logf("VERDICT: this screen NEVER READS THE LISTINGS STORE. It refuses before it looks, " +
			"so nothing about what we broadcast can be the cause -- the gate is upstream of any " +
			"query, and it is the same shape of failure A-Z LISTINGS shows.")
	default:
		t.Logf("VERDICT: this screen DOES read the listings store, %d times. It is asking and "+
			"getting an answer it will not draw, so the fault is in what the records SAY.", total)
	}
}
