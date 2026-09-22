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

// EVERY SECTION SUBSCRIPTION THE BOX HOLDS, WALKED OFF THE RUNNING MACHINE.
//
// `0x800A852C` is the section dispatcher, and decompiling it with its real entry seeded shows how a
// section reaches a consumer:
//
//	bucket = *TABLE + (*MASK & pid) * 4          a hash of the PID
//	node   = bucket; while (node[2] != pid) node = node[1]
//	for (sub = node[0]; sub; sub = sub->next)
//	    if ((*sub >> 8 & (*sub & 0xff ^ tableID)) == 0)      the table id, under a per-node mask
//	        if (sub->flags & 4)  deliver(sub, section, pid)
//	        else descend on section[3], then section[8], then section[10]
//
// So subscriptions are a TREE: table id at the top, then three halfwords of the section. For a Sky
// title section those are the listings id at `[3..4]` and the MJD filter at `[8..9]`. **A section
// reaches nobody unless a matching subscription exists**, and that makes the tree the box's own
// statement of what it is prepared to receive — far better evidence than the demux match units,
// which only say what the hardware will admit.
//
// The literal pool gives the two globals: the mask lives at `0x80107028` and the table base at
// `0x8010702C`, and the delivery callback is `0x800A8430` -- which is independently the entry of
// one of the functions the store-reading differential turned up, so the two readings agree.
//
// This walks the whole structure and prints it. Every node is bounds-checked and every list is
// length-capped, because a walk of guest pointers that goes wrong does not crash -- it wanders, and
// a wandering walk prints plausible nonsense.
//
// IT ASSERTS ITS OWN SUBJECT: the table must be populated and at least one subscription must exist
// on a PID the box has armed, or the structure is not where the pool says and the listing below
// would be of arbitrary memory.
//
// IT ONLY READS.
func TestEverySectionSubscriptionTheBoxHolds(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		maskAt      = 0x80107028
		tableAt     = 0x8010702C
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
		maxList     = 256
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

	size := box.RAM.Size()
	ok := func(at uint32) bool {
		return at >= 0x80000000 && at < 0xA0000000 && (at&0x1fffffff)+16 <= size
	}
	word := func(at uint32) uint32 { return box.RAM.Read(at&0x1fffffff, bus.Word) }
	half := func(at uint32) uint16 {
		return uint16(box.RAM.Read(at&0x1fffffff, bus.Half)) // #nosec G115 -- half read
	}

	mask, base := word(maskAt), word(tableAt)
	t.Logf("the dispatcher's hash mask is %08X and its table is at %08X", mask, base)
	if !ok(base) || mask == 0 || mask > 0xffff {
		t.Fatalf("harness: mask %08X and table %08X are not a plausible hash of a PID, so the "+
			"globals are not where the literal pool says and nothing below would be the tree",
			mask, base)
	}

	// EVERY BUCKET, AND EACH NODE'S OWN PID -- NOT A LOOKUP BY THE PID WE EXPECT.
	//
	// A first version hashed each armed PID into a bucket and searched the chain for a node whose
	// pid field matched. It reported ninety-three subscriptions on PID 0x10 and **zero on 0x33**,
	// which is the PID twenty-one programmes demonstrably arrive on -- so the traversal, not the
	// box, was wrong. The mask is 0x1F, so 0x10, 0x30, 0x50 and 0x70 all land in bucket 16, and a
	// chain search that stops at the first plausible node attributes one PID's subscriptions to
	// another.
	//
	// Enumerating every bucket and taking each node's pid from the NODE removes the guess.
	type sub struct {
		pid          uint16
		depth        int
		tag, tagMask uint16
		flags        uint16
		leaf         bool
	}
	var found []sub
	var walk func(pid uint16, node uint32, depth int)
	walk = func(pid uint16, node uint32, depth int) {
		for n := 0; node != 0 && n < maxList; n++ {
			if !ok(node) {
				return
			}
			k := half(node)
			flags := half(node + 2)
			// Only the TOP level is a masked table-id test; below it the whole halfword is
			// compared against a field of the section, so splitting it would invent a mask.
			s := sub{pid: pid, depth: depth, tag: k, flags: flags, leaf: flags&4 != 0}
			if depth == 0 {
				s.tag, s.tagMask = k&0xff, k>>8
			}
			found = append(found, s)
			if !s.leaf && depth < 4 {
				walk(pid, word(node+12), depth+1)
			}
			node = word(node + 8)
		}
	}
	buckets := 0
	for b := uint32(0); b <= mask; b++ {
		node := word(base + b*4)
		for n := 0; node != 0 && n < maxList; n++ {
			if !ok(node) {
				break
			}
			pid := uint16(word(node+8) & 0x1fff) // #nosec G115 -- thirteen bits
			buckets++
			walk(pid, word(node), 0)
			node = word(node + 4)
		}
	}
	if len(found) == 0 {
		t.Fatalf("harness: %d PID nodes were found across %d buckets but not one subscription "+
			"under any of them, so the walk did not land on the tree", buckets, mask+1)
	}
	t.Logf("%d PID nodes across %d buckets", buckets, mask+1)

	sort.Slice(found, func(a, b int) bool {
		if found[a].pid != found[b].pid {
			return found[a].pid < found[b].pid
		}
		return found[a].depth < found[b].depth
	})
	t.Logf("=== %d subscription nodes ===", len(found))
	byPID := map[uint16]int{}
	for _, s := range found {
		byPID[s.pid]++
	}
	pids := make([]uint16, 0, len(byPID))
	for p := range byPID {
		pids = append(pids, p)
	}
	sort.Slice(pids, func(a, b int) bool { return byPID[pids[a]] > byPID[pids[b]] })
	armedPIDs := map[uint16]bool{}
	for _, f := range box.Demux.ArmedFilters() {
		armedPIDs[f.PID] = true
	}
	for _, p := range pids {
		note := ""
		if armedPIDs[p] {
			note = "  (armed in the demux)"
		}
		t.Logf("  PID %#04x: %d nodes%s", p, byPID[p], note)
	}
	// GROUPED BY THE TOP-LEVEL TABLE ID, because that is the question. A title section's extension
	// is the LISTINGS ID, so a branch under table 0xA0..0xA4 lists the channels the box will accept
	// programmes for -- and a channel missing from it is a channel whose programmes are delivered
	// to nobody, however correctly they are transmitted.
	type branch struct {
		table, tableMask uint16
		children         []uint16
	}
	var branches []*branch
	var current *branch
	for _, s := range found {
		if s.depth == 0 {
			current = &branch{table: s.tag, tableMask: s.tagMask}
			branches = append(branches, current)
			continue
		}
		if current != nil && s.depth == 1 {
			current.children = append(current.children, s.tag)
		}
	}
	listings := guide.On(day)
	ours := map[uint16]string{}
	for i := range listings.Services {
		ours[listings.Services[i].ListingsID] = listings.Services[i].Name
	}
	t.Logf("=== the tree, by top-level table id ===")
	for _, b := range branches {
		kind := ""
		switch {
		case b.table >= 0xa0 && b.table <= 0xa4:
			kind = "  <- A SKY TITLE TABLE: its children are listings ids"
		case b.table == 0xc1:
			kind = "  <- the A-Z index: its children are letters"
		}
		t.Logf("  table %02X under mask %02X, %d children%s",
			b.table, b.tableMask, len(b.children), kind)
		line, shown := "", 0
		for _, c := range b.children {
			if name, mine := ours[c]; mine {
				t.Logf("        child %04X = %s  <- ONE OF OURS", c, name)
				continue
			}
			line += fmt.Sprintf(" %04X", c)
			if shown++; shown%12 == 0 {
				t.Logf("       %s", line)
				line = ""
			}
		}
		if line != "" {
			t.Logf("       %s", line)
		}
		if b.table >= 0xa0 && b.table <= 0xa4 {
			covered := 0
			for _, c := range b.children {
				if _, mine := ours[c]; mine {
					covered++
				}
			}
			t.Logf("    -> %d of our %d channels are subscribed under this table",
				covered, len(ours))
		}
	}

	// The question this was built for: is there a subscription whose top-level key admits a Sky
	// title section on the listings PID?
	titlePIDs := map[uint16]bool{}
	for p := range byPID {
		if p >= 0x30 && p <= 0x37 {
			titlePIDs[p] = true
		}
	}
	admits := 0
	for _, s := range found {
		if !titlePIDs[s.pid] || s.depth != 0 {
			continue
		}
		for table := byte(0xa0); table <= 0xa4; table++ {
			if s.tagMask&(s.tag^uint16(table)) == 0 { // #nosec G115 -- bytes
				admits++
				t.Logf("    a title table %#02x is admitted by PID %#04x tag %02X/%02X",
					table, s.pid, s.tag, s.tagMask)
				break
			}
		}
	}
	t.Logf("%d top-level subscriptions on a title PID admit a 0xA0..0xA4 section; the fixture "+
		"announces %d channels", admits, len(listings.Services))
}

// DOES EVERY SECTION GO THROUGH THAT DISPATCHER?
//
// The subscription walk came back self-contradictory: ninety-three nodes on PID 0x10 and **zero on
// PID 0x33**, which is the PID the box's own request names and on which twenty-one programmes
// demonstrably arrive. A structure that says nothing is subscribed to the PID that is receiving is
// either the wrong structure or the wrong traversal, and either way nothing in it can be believed
// yet.
//
// One measurement decides which. `0x800A852C` takes the section pointer in a0 and a PID context in
// a1, so counting its executions BY TABLE ID says whether title sections go through it at all:
//
//   - if 0xA3 sections reach it, the dispatcher is the universal path and the walk is simply wrong,
//     most likely in how it finds the per-PID node inside a hash bucket -- note 0x10 and 0x30 both
//     hash to bucket 16 under the mask 0x1F, so a chain search that stops early lands on a
//     neighbour;
//   - if they never reach it, titles are delivered by some other route entirely and the tree is not
//     the listings path, however promising its contents look.
//
// **The contents DO look promising, which is exactly why this check comes first.** The depth-0 node
// on that PID is table 0xC1 under mask 0xFF with children 0x41, 0x42 ... 0x57 -- the letters 'A'
// onwards, matching the A-Z index consumer this project has already decoded, which dispatches on a
// letter and frees the list for anything outside 'A'..'Z'. A subscription nobody has ever fed is a
// good story, and a good story on an untrusted walk is how a wrong finding gets filed.
//
// IT ONLY READS.
func TestWhetherTitleSectionsReachTheDispatcher(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const dispatcher = 0x800A852C
	byTable := map[uint32]int{}
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Fetch || a.Virtual&^1 != dispatcher {
			return
		}
		// a0 is the section pointer; its first byte is the table id.
		at := box.Machine.Core.State().GPR[4] & 0x1fffffff
		if at+1 <= box.RAM.Size() {
			byTable[box.RAM.Read(at, bus.Byte)]++
		}
	}}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	for i := 0; i < 160_000_000 && registered < want; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			registered++
		}
	}
	if len(byTable) == 0 {
		t.Fatalf("harness: %08X never ran while %d programmes were taken off the air, so it is not "+
			"on the delivery path at all and the subscription tree is not the listings route",
			uint32(dispatcher), registered)
	}
	keys := make([]uint32, 0, len(byTable))
	for k := range byTable {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool { return byTable[keys[a]] > byTable[keys[b]] })
	t.Logf("=== sections that reached the dispatcher while %d programmes registered ===", registered)
	titles := 0
	for _, k := range keys {
		note := ""
		if k >= 0xa0 && k <= 0xa4 {
			note = "   <- A SKY TITLE SECTION"
			titles += byTable[k]
		}
		t.Logf("    table %#02x  %d sections%s", k, byTable[k], note)
	}
	if titles == 0 {
		t.Logf("VERDICT: NO title section reaches the dispatcher, so titles are delivered by some " +
			"other route and the subscription tree -- whatever it holds -- is not the path that " +
			"carries programmes. The A-Z looking subscription in it is not evidence about listings.")
		return
	}
	t.Logf("VERDICT: %d title sections DO reach the dispatcher, so it is the universal delivery "+
		"path and the subscription walk is simply traversing it wrongly -- most likely the per-PID "+
		"node search, since 0x10 and 0x30 share a bucket under the mask 0x1F.", titles)
}
