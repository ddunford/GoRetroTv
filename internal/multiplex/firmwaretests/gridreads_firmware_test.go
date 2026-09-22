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

// DOES THE ALL CHANNELS GRID EVER LOOK AT OUR LINE-UP?
//
// The grid draws a header, a date, a clock and correctly spaced half-hour columns, and no rows.
// Two things are now established on THIS port rather than inherited from the browser oracle: the
// box holds exactly six service records, one per announced service, at an 18-byte stride
// (lineupcount_firmware_test.go) -- so the "twelve channels" in the record is the oracle's number
// in the oracle's address space and does not describe us; and the grid draw is millions of
// instructions of MIPS, every one in RAM, because the screen is interpreted OpenTV o-code and the
// flash addresses the task was written around never execute.
//
// That leaves the question this answers: **while the grid is drawing, does anything read the line-up
// the box built from our broadcast?** The two answers want opposite work. If it reads the array and
// still draws nothing, the fault is in what the records SAY -- a field we are transmitting wrongly,
// which is ours to fix in the signal. If it never reads them, the grid is asking something else
// entirely and the line-up is a red herring, as it was on the oracle, where a read watch over the
// same records during the grid open logged zero.
//
// IT WATCHES THE VALUES AS WELL AS THE ADDRESSES, because the array is only one place our channel
// numbers could live. A read anywhere in DRAM that RETURNS 501 or 401 -- numbers the fixture chose
// and the firmware has no other reason to hold -- is the grid handling one of our channels, wherever
// it got it from.
//
// IT ASSERTS ITS OWN SUBJECT twice: the route must prove it reached the grid rather than the menu,
// and the observer must see data reads at all, or its zero is the instrument and not the screen.
//
// IT ONLY READS. Nothing is written into the guest.
func TestWhetherTheAllChannelsGridReadsTheLineUp(t *testing.T) {
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

	listings := guide.On(day)
	records := serviceRecords(t, box, listings.Services)
	lo, hi := records[0].base, records[len(records)-1].base+18
	t.Logf("the line-up is %d records at guest %08X..%08X", len(records), 0x80000000|lo, 0x80000000|hi)

	// The numbers the fixture chose -- BUT ONLY THE DISTINCTIVE ONES, and that restriction is a
	// correction rather than a precaution. Watching every identifier put service ids 100 to 105 on
	// the list, which are ordinary small integers a firmware holds for a hundred unrelated reasons:
	// the probe came back with 195 "hits", the top four of them 2,532 reads of the number 101 from
	// one instruction. That is not our channel being handled, it is a loop counter. A value only
	// carries information here if the firmware has no other reason to hold it, so the threshold is
	// set where the fixture's own numbers stop being ordinary.
	const distinctive = 250
	ours := map[uint32]string{}
	for i := range listings.Services {
		s := &listings.Services[i]
		if uint32(s.Channel) >= distinctive {
			ours[uint32(s.Channel)] = s.Name + " channel number"
		}
		if uint32(s.ListingsID) >= distinctive {
			ours[uint32(s.ListingsID)] = s.Name + " listings id"
		}
	}
	if len(ours) == 0 {
		t.Fatalf("harness: no announced identifier is above %d, so there is no value distinctive "+
			"enough to watch for and a zero below would mean nothing", distinctive)
	}
	t.Logf("watching for reads returning any of %d distinctive identifiers", len(ours))

	var anyRead int
	inArray := map[uint32]int{}   // PC -> reads inside the line-up records
	valueHits := map[string]int{} // "PC value" -> times a read returned one of our numbers
	// WHERE THE GRID'S DATA ACTUALLY LIVES. Knowing it does not read the line-up says where the
	// answer is NOT; a histogram of every read by 4 KB page says where to look next, and it is
	// unbiased in a way that watching for values we already suspect is not.
	pages := map[uint32]int{}
	// Distinct addresses per page as well as reads: a table being walked touches MANY addresses
	// once or twice each, while a hot loop variable touches one address thousands of times. The two
	// look identical in a read count and want completely different follow-ups.
	spread := map[uint32]map[uint32]bool{}
	// THE RECONCILIATION THIS TASK NEEDS. TASK-7.1 was written around four flash addresses the
	// record says are "executed" -- and a PC census proved that not one of them EXECUTES here, in
	// 6.5 million instructions all of which are in RAM. That looked like the record being wrong
	// about the address space. There is another reading: the grid is interpreted OpenTV o-code, and
	// an interpreter FETCHES ITS BYTECODE AS DATA. What the oracle calls an executed o-code address
	// would appear here as a flash READ, not a MIPS execution. So the same addresses are watched
	// again, on the other side of the bus.
	named := map[uint32]string{
		0x9FC75F2E: "channel count write", 0x9FC75F7A: "channel count read A",
		0x9FC762FC: "channel count read B", 0x9FC77D40: "layout selector",
	}
	flashReads := map[uint32]int{} // 4 KB page in the o-code region -> reads
	namedReads := map[uint32]int{} // one of the four, read as data
	// WHAT IS IN THE TABLES IT DOES WALK. Knowing a page is table-shaped says where to look;
	// the address it read and the value it got back is the looking. Recorded per address so the
	// structure shows itself -- a stride, a run of pointers, a block of text -- rather than being
	// guessed at from a count.
	walked := map[uint32]uint32{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Fetch || a.Write {
			return
		}
		anyRead++
		pc := box.Machine.Core.State().PC &^ 1
		if p := a.Virtual & 0x1fffffff; p >= lo && p < hi {
			inArray[pc]++
		}
		if _, mine := ours[a.Value]; mine {
			valueHits[hexPC(pc)+" "+ours[a.Value]]++
		}
		page := (a.Virtual & 0x1fffffff) >> 12
		pages[page]++
		// The page number is the PHYSICAL address shifted, so the KSEG0 nibble is already gone --
		// 0x8045D000 is page 0x0045D and not 0x8045D. Writing it the other way recorded nothing at
		// all, and the check below is what said so rather than a silent empty table.
		if page == 0x0045D || page == 0x00494 {
			walked[a.Virtual&0x1fffffff] = a.Value
		}
		if spread[page] == nil {
			spread[page] = map[uint32]bool{}
		}
		spread[page][a.Virtual&0xfff] = true
		// Flash is quoted at 0xBFC00000 and mirrored at 0x9FC00000; compare on the offset so both
		// windows onto the same chip count once.
		if off := a.Virtual & 0x00ffffff; a.Virtual >= 0x9FC00000 {
			flashReads[off>>12]++
			for at := range named {
				if off >= (at&0x00ffffff)-3 && off <= (at&0x00ffffff)+3 {
					namedReads[at]++
				}
			}
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, settled := 0, before, uint32(0)
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
				settled = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		t.Logf("%-28s drew %08X", name, settled)
		return settled
	}

	// LET IT GO QUIET FIRST. The record is explicit that a key sent the instant the box finishes
	// something does nothing at all. This instrument pressed straight after acquisition and got
	// away with it only while the box happened to be idle by then; moving the NIT onto its own PID
	// shifted the timing by a hair and the first press stopped landing, which read as a regression
	// in the transmitter and was a fragility in the harness.
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	menu := press(keyBoxOffice, "box office", 80_000_000)
	// PROVE WHICH TAB, DO NOT ASSUME IT MOVED. one LEFT from box office draws 0xFE8D1CCC, which is STILL
	// THE BOX OFFICE MENU -- the wedge guard names it as a constant for this reason -- and a route that only checks "the screen changed"
	// accepts it as the tv guide tab and then measures box office for the rest of the run. That is
	// the FOURTH time this shape has cost this project a measurement, and the first three are in
	// the record. The screenshot is what caught it: the artefact said MOVIES BY START TIME.
	// THE TWO SCREENS THIS ROUTE MUST LAND ON, PINNED, AND BOTH VERIFIED BY EYE against the
	// artefacts rather than inferred from "the hash changed". A hash cannot tell ALL CHANNELS from
	// the same menu with its highlight moved, and that is not a hypothetical: two instruments, one
	// of them committed, reported "ALL CHANNELS" for a screen whose own screenshot says MOVIES BY
	// START TIME -- the box office menu's first entry, reached because the route accepted the box
	// office menu as the tv guide tab.
	const tvGuideMenu = 0xDDBC18E9 // the ten-entry TV GUIDE menu, ALL CHANNELS highlighted
	const allChannels = 0x42DBD889 // "7.00pm Thu 24 / ALL CHANNELS / Today 7.00pm 7.30pm 8.00pm"
	tab := menu
	for attempt := 1; attempt <= 6 && tab != tvGuideMenu; attempt++ {
		tab = press(keyLeft, "left to the tv guide tab", 80_000_000)
	}
	if tab != tvGuideMenu {
		t.Fatalf("harness: never left the box office menu (%08X), so the route did not reach the "+
			"tv guide tab", menu)
	}

	// SELECT IS LOST IF IT ARRIVES WHILE THE MENU IS PAINTING, and a hash alone cannot tell the
	// grid from the same menu with its highlight moved -- both differ from the screen before. So
	// every attempt is DUMPED and the picture is the proof, which is the only thing that has
	// caught this: two instruments, one of them committed, reported "ALL CHANNELS" for a screen
	// whose artefact says MOVIES BY START TIME.
	grid := uint32(0)
	for attempt := 1; attempt <= 6; attempt++ {
		runUntil(t, box, transmitter, 8_000_000, func(int) bool { return false })
		anyRead, watching = 0, true
		for k := range inArray {
			delete(inArray, k)
		}
		for k := range valueHits {
			delete(valueHits, k)
		}
		for k := range pages {
			delete(pages, k)
		}
		for k := range spread {
			delete(spread, k)
		}
		for k := range walked {
			delete(walked, k)
		}
		for k := range flashReads {
			delete(flashReads, k)
		}
		for k := range namedReads {
			delete(namedReads, k)
		}
		grid = press(keySelect, fmt.Sprintf("select ALL CHANNELS (try %d)", attempt), 60_000_000)
		watching = false
		if grid != allChannels {
			continue
		}
		break
	}
	if grid != allChannels {
		t.Fatalf("harness: select never reached ALL CHANNELS (%08X); the screen settled on %08X, "+
			"and a screen that is merely DIFFERENT from the menu is exactly what has been measured "+
			"by mistake before", uint32(allChannels), grid)
	}

	if err := dumpScreen(t, box, "grid-reads-all-channels.png"); err != nil {
		t.Fatal(err)
	}
	if anyRead == 0 {
		t.Fatal("harness: the observer saw no data read at all during the draw, so its zeros below " +
			"are the instrument rather than the screen")
	}
	t.Logf("the grid draw performed %d data reads", anyRead)

	inArrayTotal := 0
	for _, n := range inArray {
		inArrayTotal += n
	}
	if inArrayTotal == 0 {
		t.Logf("VERDICT: the grid NEVER READS the line-up records, in %d data reads. It is not "+
			"drawing nothing because our records say something wrong -- it is not asking them. The "+
			"next question is what it asks INSTEAD, and the value hits below are the lead.", anyRead)
	} else {
		pcs := make([]uint32, 0, len(inArray))
		for pc := range inArray {
			pcs = append(pcs, pc)
		}
		sort.Slice(pcs, func(a, b int) bool { return inArray[pcs[a]] > inArray[pcs[b]] })
		t.Logf("VERDICT: the grid READS the line-up -- %d reads from %d PCs. What the records SAY is "+
			"then the fault, and that is ours to fix in the broadcast.", inArrayTotal, len(pcs))
		for i, pc := range pcs {
			if i >= 12 {
				break
			}
			t.Logf("    %08X  %d reads", pc, inArray[pc])
		}
	}

	reportGridPages(t, pages, spread, lo, hi)
	reportWalkedTables(t, walked)

	total := 0
	for _, n := range flashReads {
		total += n
	}
	t.Logf("the draw read flash %d times across %d pages", total, len(flashReads))
	if len(namedReads) == 0 {
		t.Logf("and NONE of the four addresses TASK-7.1 names was read as data either. They are " +
			"neither executed nor fetched here, so they belong to the oracle's own instrumentation " +
			"rather than to anything this port does.")
	} else {
		for at, n := range namedReads {
			t.Logf("    %08X %-22s read as DATA %d times -- the task's address is real here, just "+
				"fetched rather than executed", at, named[at], n)
		}
	}

	if len(valueHits) == 0 {
		t.Logf("and NOT ONE READ anywhere in DRAM returned a distinctive channel number or listings " +
			"id of ours. While the grid draws, our channels are not being handled at all -- so it " +
			"is not filtering them out, it never has them.")
		return
	}
	keys := make([]string, 0, len(valueHits))
	for k := range valueHits {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool { return valueHits[keys[a]] > valueHits[keys[b]] })
	t.Logf("%d distinct (PC, identifier) pairs read one of our numbers:", len(keys))
	for i, k := range keys {
		if i >= 20 {
			t.Logf("    ... and %d more", len(keys)-i)
			break
		}
		t.Logf("    %-52s %d", k, valueHits[k])
	}
}

// hexPC formats a program counter the one way this package formats them.
func hexPC(pc uint32) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = digits[pc&0xf]
		pc >>= 4
	}
	return string(out)
}

// reportGridPages says which 4 KB pages the grid read from, busiest first.
//
// It names the line-up's own page explicitly whether or not it appears, because "the page holding
// the records is absent from a list of the pages that were read" is the same finding as "zero reads
// inside the records" arrived at independently, and two routes to it is what makes it a reading.
func reportGridPages(t *testing.T, pages map[uint32]int, spread map[uint32]map[uint32]bool, lo, hi uint32) {
	t.Helper()
	if len(pages) == 0 {
		t.Fatal("harness: no page was read at all, so the histogram is the instrument failing")
	}
	keys := make([]uint32, 0, len(pages))
	for p := range pages {
		keys = append(keys, p)
	}
	sort.Slice(keys, func(a, b int) bool { return pages[keys[a]] > pages[keys[b]] })
	t.Logf("the draw touched %d distinct 4 KB pages; the busiest:", len(keys))
	for i, p := range keys {
		if i >= 14 {
			break
		}
		mark := ""
		if p >= lo>>12 && p <= hi>>12 {
			mark = "  <- the line-up's own page"
		}
		t.Logf("    guest %08X..%08X  %7d reads over %4d distinct addresses%s",
			0x80000000|(p<<12), 0x80000000|(p<<12)+0xfff, pages[p], len(spread[p]), mark)
	}
	if n := pages[lo>>12]; n == 0 {
		t.Logf("    the line-up's page (%08X) is NOT among them at all", 0x80000000|((lo>>12)<<12))
	}
}

// reportWalkedTables prints what the grid actually found in the two table-shaped pages it walks.
//
// Addresses in order with the value read at each, because a structure announces itself that way and
// not in a total: a fixed stride between non-zero entries is an array, a run of 0x8______ words is a
// pointer table, printable bytes are text, and a page of zeros is a list that was never filled --
// which is the single most likely shape for a screen that draws no rows.
func reportWalkedTables(t *testing.T, walked map[uint32]uint32) {
	t.Helper()
	if len(walked) == 0 {
		t.Fatal("harness: neither table-shaped page was recorded, so the filter above no longer " +
			"matches the pages the histogram named and this says nothing")
	}
	addrs := make([]uint32, 0, len(walked))
	for a := range walked {
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })
	var zero, pointers, printable int
	for _, a := range addrs {
		v := walked[a]
		switch {
		case v == 0:
			zero++
		case v>>28 == 8:
			pointers++
		}
		if b := v >> 24; b >= 0x20 && b < 0x7f {
			printable++
		}
	}
	t.Logf("the two walked tables: %d distinct addresses read -- %d returned zero, %d returned a "+
		"guest pointer, %d had a printable top byte", len(addrs), zero, pointers, printable)
	// THE 0x8045D TABLE IS AN ARRAY AND ITS RUNS ARE THE FINDING. Ascending 16-bit values at a
	// fixed stride is an index; where the values step by one they are a contiguous block, and where
	// they jump the block ended. The LENGTH of each block is what can be compared against something
	// we know -- six services, sixty-seven programmes -- and a length is a reading in a way that
	// "the table looks like an index" is not.
	var idx []uint32
	for _, a := range addrs {
		if a>>12 == 0x0045D && (a&7) == 2 {
			idx = append(idx, a)
		}
	}
	if len(idx) > 1 {
		type run struct{ first, last, n uint32 }
		var runs []run
		cur := run{first: walked[idx[0]], last: walked[idx[0]], n: 1}
		for _, a := range idx[1:] {
			v := walked[a]
			if v == cur.last+1 {
				cur.last, cur.n = v, cur.n+1
				continue
			}
			runs = append(runs, cur)
			cur = run{first: v, last: v, n: 1}
		}
		runs = append(runs, cur)
		t.Logf("the 8045D table: %d entries at an 8-byte stride, in %d ascending runs", len(idx), len(runs))
		for i, r := range runs {
			if i >= 10 {
				t.Logf("        ... and %d more runs", len(runs)-i)
				break
			}
			t.Logf("        %5d entries, ids %04X..%04X", r.n, r.first, r.last)
		}
	}

	// A sample from EACH page rather than the first forty-eight of the pair, because the two are
	// different structures and the first one alone would hide the second entirely.
	for _, page := range []uint32{0x0045D, 0x00494} {
		var in []uint32
		for _, a := range addrs {
			if a>>12 == page {
				in = append(in, a)
			}
		}
		if len(in) == 0 {
			t.Logf("    page %08X: not read at all", 0x80000000|(page<<12))
			continue
		}
		t.Logf("    page %08X: %d distinct addresses, first and last twelve:",
			0x80000000|(page<<12), len(in))
		for i, a := range in {
			if i >= 12 && i < len(in)-12 {
				continue
			}
			if i == len(in)-12 && len(in) > 24 {
				t.Logf("        ...")
			}
			t.Logf("        %08X = %08X", 0x80000000|a, walked[a])
		}
	}
}
