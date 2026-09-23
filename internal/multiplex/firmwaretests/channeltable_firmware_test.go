package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE GRID'S CHANNEL TABLE, AND THE FIELD THAT IS -1 ON EVERY CHANNEL.
//
// Following the row type back through two copies lands on a six-byte-per-channel table, and its
// contents are unmistakable -- those are OUR listings ids:
//
//	802AD9BC  0065 FFFF 0001     Sky One      (101)
//	802AD9C2  0079 FFFF 0001     Sky Soap     (121)
//	802AD9C8  00FB FFFF 0001     Sky Travel   (251)
//	802AD9CE  012D FFFF 0001     Sky Movies   (301)
//	802AD9D4  0191 FFFF 0001     Sky Sports 1 (401)
//	802AD9DA  01F5 FFFF 0001     Sky News     (501)
//
// So an entry is [listings id][something][type], the grid copies the type into each row, and the
// middle halfword is 0xFFFF -- minus one, nobody's id -- on every channel. A row is type 1 and
// draws "..no listings available"; the programme draw wants type 2.
//
// WHO WRITES THIS TABLE IS THE QUESTION. Whatever fills the middle field is the thing that decides
// a channel has something to show, and this port has never made it happen. Watching the table from
// BOOT names the code that builds it and every value it puts there.
//
// THE ADDRESS IS A HYPOTHESIS AND THE RUN CHECKS IT, because a watch can only start before the
// grid opens if the address is known in advance. The table is verified by its CONTENT -- the
// listings ids the fixture announces have to be in it -- rather than by trusting the number.
//
// IT ONLY READS.
func TestWhoFillsTheGridsChannelTable(t *testing.T) {
	const (
		tableAt   = 0x802AD9BC // measured; checked against the fixture's own ids below
		entrySize = 6
		entries   = 8
	)
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	// THE STAGING BUFFER. Every entry in the table is copied from the SAME six bytes, so that is
	// where the real producer writes -- the table is just where the copies land. Watching it names
	// the code that decides a channel's middle field is -1 and its type is 1.
	const stagingAt = 0x80175F0C

	type write struct {
		at    uint32
		value uint32
		width uint32
		pc    uint32
		from  uint32
		setBy uint32
		setAt uint32
	}
	var writes, staging []write
	var lastSource uint32
	latest := map[uint32]write{}
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			return
		}
		// THE COPY'S SOURCE. The table is written by the same byte-copy loop that fills the row
		// objects -- 800f99fe lb a2,0(v0) / 800f9a02 sb a2,0(v1) -- so the table is a COPY too,
		// and the byte read immediately before each store says what it is a copy OF.
		if !a.Write {
			if box.Machine.Core.State().PC&^1 == 0x800F99FE {
				lastSource = a.Virtual | 0x80000000
			}
			return
		}
		at := a.Virtual | 0x80000000
		inTable := at >= tableAt && at < tableAt+entries*entrySize
		inStaging := at >= stagingAt && at < stagingAt+entrySize
		if !inTable && !inStaging {
			return
		}
		width := uint32(1)
		switch a.Size {
		case bus.Half:
			width = 2
		case bus.Word:
			width = 4
		}
		w := write{at: at, value: a.Value, width: width,
			pc: box.Machine.Core.State().PC &^ 1, from: lastSource}
		if inStaging {
			// KEEP ONLY THE LATEST PER OFFSET. 0x80175F0C is a reused stack local, not a
			// dedicated buffer -- five hundred writes from thirty instructions land in it over a
			// run. The only ones that matter are whatever was there when the copy took it, so
			// each offset remembers its most recent writer and the table write below claims it.
			latest[at-stagingAt] = w
			staging = append(staging, w)
			return
		}
		if src, ok := latest[at-tableAt-((at-tableAt)/entrySize)*entrySize]; ok {
			w.setBy, w.setAt = src.pc, src.value
		}
		writes = append(writes, w)
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntilHooked(t, box, transmitter, hooks, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntilHooked(t, box, transmitter, hooks, 20_000_000, func(int) bool { return false })

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
	settled := openAllChannelsFinished(t, press, ".artifacts/channeltable-grid.png")
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "channeltable-grid.png"); err != nil {
		t.Fatal(err)
	}

	// THE SUBJECT: this must BE the channel table, and the fixture's own ids are the check.
	half := func(at uint32) uint32 { return box.RAM.Read(at&0x1fffffff, bus.Half) }
	listings := guide.On(day)
	ours := map[uint32]string{}
	for i := range listings.Services {
		ours[uint32(listings.Services[i].ListingsID)] = listings.Services[i].Name
	}
	found := 0
	t.Logf("the grid settled on %08X", settled)
	t.Logf("=== the channel table at %08X ===", uint32(tableAt))
	for n := uint32(0); n < entries; n++ {
		at := tableAt + n*entrySize
		id, mid, kind := half(at), half(at+2), half(at+4)
		name := ours[id]
		if name != "" {
			found++
		}
		t.Logf("    %08X  id %04X %-14s middle %04X  type %d", at, id, name, mid, kind)
	}
	if found < len(ours) {
		t.Fatalf("harness: only %d of the fixture's %d listings ids are in the table at %08X, so "+
			"that is not the grid's channel list and every write below is of something else",
			found, len(ours), uint32(tableAt))
	}

	if len(writes) == 0 {
		t.Logf("=== NOTHING WROTE THE TABLE IN THIS ENTIRE RUN ===")
		t.Logf("    So it is built before anything this probe can see -- it is in the restored " +
			"snapshot, and the middle field has been -1 since boot.")
		return
	}
	byPC := map[uint32]int{}
	for _, w := range writes {
		byPC[w.pc]++
	}
	pcs := make([]uint32, 0, len(byPC))
	for pc := range byPC {
		pcs = append(pcs, pc)
	}
	sort.Slice(pcs, func(a, b int) bool { return byPC[pcs[a]] > byPC[pcs[b]] })
	t.Logf("=== %d writes to the table, from %d instructions ===", len(writes), len(pcs))
	for _, pc := range pcs {
		t.Logf("    MIPS %08X  %d writes", pc, byPC[pc])
	}
	t.Logf("=== %d writes to the STAGING BUFFER at %08X -- the real producer ===",
		len(staging), uint32(stagingAt))
	stagingPCs := map[uint32]int{}
	for _, w := range staging {
		stagingPCs[w.pc]++
	}
	spcs := make([]uint32, 0, len(stagingPCs))
	for pc := range stagingPCs {
		spcs = append(spcs, pc)
	}
	sort.Slice(spcs, func(a, b int) bool { return stagingPCs[spcs[a]] > stagingPCs[spcs[b]] })
	for _, pc := range spcs {
		t.Logf("    MIPS %08X  %d writes", pc, stagingPCs[pc])
	}
	for i, w := range staging {
		if i >= 24 {
			t.Logf("    ... and %d more", len(staging)-24)
			break
		}
		off := w.at - stagingAt
		field := []string{"id", "id", "MIDDLE", "MIDDLE", "type", "type"}[off]
		t.Logf("    +%d %-6s <- %02X   MIPS %08X", off, field, w.value&0xff, w.pc)
	}

	t.Logf("=== the first 30 in order ===")
	for i, w := range writes {
		if i >= 30 {
			break
		}
		off := (w.at - tableAt) % entrySize
		field := []string{"id", "id", "MIDDLE", "MIDDLE", "type", "type"}[off]
		t.Logf("    %08X <- %02X  (entry %d %-6s)  put there by MIPS %08X",
			w.at, w.value&0xff, (w.at-tableAt)/entrySize, field, w.setBy)
	}
}
