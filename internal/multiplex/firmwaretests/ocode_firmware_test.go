package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE GRID'S DECISION IS IN O-CODE, SO TRACE THE O-CODE.
//
// The divergence hunt ended somewhere useful: **the grid enters the listings module at
// 0x800A8000..0x800AB000 zero times.** Not once. The banner enters it through eleven different
// doors and reads the listings pages 1,436 times; the grid never opens any of them. The MIPS
// listings code is not refusing the grid — the grid never calls it.
//
// That is why every MIPS-level probe has come back with a negative. **The decision is not in MIPS.**
// The ALL CHANNELS screen is interpreted OpenTV o-code, its bytecode lives in flash, and the
// interpreter FETCHES IT AS DATA — which is exactly what makes it traceable from here. A read of
// flash by the interpreter is an o-code program counter, and the grid reads flash pages
// 0x9FC4D000 and 0x9FC8C000 thousands of times while it draws.
//
// **And there is an old claim to settle with the same run.** TASK-7.1 was written around four flash
// addresses taken from the browser oracle — 0x9FC75F2F, 0x9FC75F7B, 0x9FC762FD and 0x9FC77D41 —
// and this port measured them as neither executed nor read, which is what retired the task's
// premise. But that measurement was taken with the transport at state 4, when the grid's own row
// loop ran once and gave up. **The grid does much more now.** Whether those addresses are read with
// the first gate open is a different question from the one that was answered, and it is answered
// here.
//
// IT ASSERTS ITS OWN SUBJECT: the grid must read flash at all while it draws, or the o-code trace
// is empty for reasons that have nothing to do with the screen.
//
// IT ONLY READS.
func TestWhatOCodeTheGridRunsAndWhereItStops(t *testing.T) {
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
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d, so the grid's first gate is open", state())

	// The four addresses TASK-7.1 was written around, masked of the MIPS16 bit so a read of the
	// halfword containing them counts.
	named := map[uint32]string{
		0x9FC75F2E: "oracle: channel count write",
		0x9FC75F7A: "oracle: channel count read A",
		0x9FC762FC: "oracle: channel count read B",
		0x9FC77D40: "oracle: layout selector",
	}

	var (
		watching  bool
		flashRead int
		pages     = map[uint32]int{}
		words     = map[uint32]int{}
		hits      = map[uint32]int{}
		order     []uint32
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		// Flash is quoted at 0xBFC00000 and mirrored at 0x9FC00000; compare on the offset so both
		// windows onto the same chip count once.
		if a.Virtual < 0x9FC00000 {
			return
		}
		at := 0x9FC00000 | (a.Virtual & 0x00ffffff)
		flashRead++
		pages[at>>12]++
		words[at&^3]++
		for want := range named {
			if at >= want-3 && at <= want+3 {
				hits[want]++
			}
		}
		if len(order) < 4000 {
			order = append(order, at)
		}
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching, flashRead = true, 0
			pages, words, hits, order = map[uint32]int{}, map[uint32]int{}, map[uint32]int{}, order[:0]
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

	grid := openAllChannels(t, press, ".artifacts/ocode-grid.png", false)
	if err := dumpScreen(t, box, "ocode-grid.png"); err != nil {
		t.Fatal(err)
	}
	if flashRead == 0 {
		t.Fatal("harness: the grid read flash not once while drawing, so there is no o-code trace " +
			"here and the zeros below describe the instrument")
	}
	t.Logf("the grid drew %08X and read flash %d times across %d pages", grid, flashRead, len(pages))

	busy := make([]uint32, 0, len(pages))
	for p := range pages {
		busy = append(busy, p)
	}
	sort.Slice(busy, func(a, b int) bool { return pages[busy[a]] > pages[busy[b]] })
	t.Logf("=== the flash pages the grid's interpreter walks ===")
	for i, p := range busy {
		if i >= 15 {
			t.Logf("    ... and %d more", len(busy)-15)
			break
		}
		t.Logf("    %08X  %d reads", p<<12, pages[p])
	}

	t.Logf("=== TASK-7.1's four oracle addresses, re-asked with the first gate OPEN ===")
	total := 0
	for at, what := range named {
		t.Logf("    %08X  %-32s read %d times", at, what, hits[at])
		total += hits[at]
	}
	if total == 0 {
		t.Logf("VERDICT: still none of them, so they remain the oracle's own addresses and the " +
			"retirement of TASK-7.1's premise stands even with the grid doing six times the work.")
	} else {
		t.Logf("VERDICT: %d reads across the four. They ARE this port's addresses after all, and "+
			"they were invisible only because the grid gave up before reaching them.", total)
	}

	// The busiest single words in the o-code region are the interpreter's hot bytecode: the
	// dispatch it runs most. Reporting them gives the next probe somewhere to stand.
	hot := make([]uint32, 0, len(words))
	for w := range words {
		hot = append(hot, w)
	}
	sort.Slice(hot, func(a, b int) bool { return words[hot[a]] > words[hot[b]] })
	t.Logf("=== the o-code words the grid reads most ===")
	for i, w := range hot {
		if i >= 20 {
			break
		}
		t.Logf("    %08X  %d reads", w, words[w])
	}
}
