package firmwaretests_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT DECIDES "No satellite signal is being received".
//
// It sits over the picture whenever the demo tunes to a channel. The record establishes what the
// message IS -- a channel-level error, one entry in a table beside "There is a technical fault with
// this channel" -- and that it is drawn with ZERO demodulator traffic, so it is not read off the
// front end. What it has never established is what SETS it.
//
// The obvious next move was to send a transport stream, and measuring first killed it: the packet
// path is gated on the demux register at 0x140, the box never writes that register ONCE across 673
// other demux accesses, and the only code in the image that touches it manipulates bits in the
// upper half rather than the enable. So the stream would have gone nowhere and the code that would
// ask for one never runs.
//
// So ask the box instead, the way the grid's "..no listings available" was cracked: find the string
// where THIS box put it in DRAM, watch for the moment it is first reached for, and keep a ring of
// the o-code addresses leading up to it. The interpreter fetches bytecode from flash as data, so a
// flash read is an o-code program counter, and the tail of that ring is the decision.
//
// THE ROUTE IS THE WORKING GRID AND A SELECT, because that is where the message appears: SELECT on
// a drawn programme tunes to the channel and the banner comes up with the message over it.
//
// IT ASSERTS ITS OWN SUBJECT TWICE: the string must be found in DRAM, and the box must actually
// reach for it. A run where it never does has measured something other than the failure.
//
// IT ONLY READS.
func TestWhatDecidesNoSatelliteSignal(t *testing.T) {
	const (
		ringSize      = 3000
		mainFetchSite = 0x80069298
	)
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
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	needle := []byte("No satellite signal is being received")
	size := box.RAM.Size()
	ram := make([]byte, size)
	for i := uint32(0); i < size; i++ {
		ram[i] = byte(box.RAM.Read(i, bus.Byte)) // #nosec G115 -- byte read
	}
	var found []uint32
	for at := 0; ; {
		i := bytes.Index(ram[at:], needle)
		if i < 0 {
			break
		}
		found = append(found, uint32(at+i)) // #nosec G115 -- bounded by RAM size
		at += i + 1
	}
	if len(found) == 0 {
		t.Skipf("the box does not hold %q as text in DRAM, so the message table is kept coded and "+
			"a watch on a guessed address would report a confident zero", needle)
	}
	for _, at := range found {
		t.Logf("the string is in DRAM at %08X", 0x80000000|at)
	}
	inString := func(off uint32) bool {
		for _, at := range found {
			if off >= at && off < at+uint32(len(needle)) { // #nosec G115 -- small constant
				return true
			}
		}
		return false
	}

	var (
		watching   bool
		ring       [ringSize]uint32
		ringAt     int
		ringFilled bool
		trail      []uint32
		caught     bool
		flashReads int
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		if a.Virtual >= 0x9FC00000 {
			flashReads++
			if box.Machine.Core.State().PC&^1 != mainFetchSite {
				return
			}
			ring[ringAt] = 0x9FC00000 | (a.Virtual & 0x00ffffff)
			if ringAt++; ringAt == ringSize {
				ringAt, ringFilled = 0, true
			}
			return
		}
		if caught || !inString(a.Virtual&0x1fffffff) {
			return
		}
		caught = true
		if ringFilled {
			trail = append(trail, ring[ringAt:]...)
		}
		trail = append(trail, ring[:ringAt]...)
	}}

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
	watching = true
	openAllChannels(t, press, ".artifacts/nosignal-grid.png", true)
	for i := 0; i < 40_000_000 && !caught; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	before := screenNow(t, box)
	viewing := uint32(0)
	for attempt := 1; attempt <= 6 && (viewing == 0 || viewing == before); attempt++ {
		viewing = press(keySelect, "select to view the channel", 80_000_000)
	}
	for i := 0; i < 40_000_000 && !caught; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "nosignal-viewing.png"); err != nil {
		t.Fatal(err)
	}
	if flashReads == 0 {
		t.Fatal("harness: no flash was read at all while this ran, so the o-code trail is empty " +
			"for reasons that have nothing to do with the message")
	}
	if !caught {
		t.Fatalf("harness: the box never reached for %q in this run (%d flash reads, viewing "+
			"%08X), so whatever it drew, it is not the message. Read "+
			".artifacts/nosignal-viewing.png", needle, flashReads, viewing)
	}
	t.Logf("the box is viewing %08X; %d o-code instructions were captured before it reached for "+
		"the string", viewing, len(trail))

	type edge struct{ from, to uint32 }
	var edges []edge
	for i := 1; i < len(trail); i++ {
		from, to := trail[i-1], trail[i]
		if to > from && to-from <= 16 {
			continue
		}
		edges = append(edges, edge{from, to})
	}
	const tail = 40
	first := 0
	if len(edges) > tail {
		first = len(edges) - tail
		t.Logf("    ... %d earlier transitions elided", first)
	}
	t.Logf("=== the control flow immediately before the message was chosen ===")
	for _, e := range edges[first:] {
		t.Logf("    %08X -> %08X", e.from, e.to)
	}
	if len(trail) > 0 {
		t.Logf("the last o-code instruction before the string was touched is %08X -- disassemble "+
			"around it with tools/ocode-disasm.py", trail[len(trail)-1])
	}
}

// WHAT VALUE SENDS THE CHANNEL VIEW DOWN THE NO-SIGNAL PATH.
//
// The bytecode trail above identifies the decision precisely:
//
//	9fc6acc9  pushs_mm_ind_fp_nn 26 fc  ; signed halfword from object+0x26
//	9fc6accd  push_3
//	9fc6acd0  and
//	9fc6acd6  jz 0x9fc6ad1a             ; zero skips the channel-error message
//
// This probe catches the data read made by that load, rather than assigning a meaning to the
// field from its position. It also follows the executed writer back to the event-object member
// whose value selects bit 1 versus bit 0x40, and attributes every observed write to its MIPS and
// o-code sites. It changes no guest state.
func TestWhatValueChoosesNoSatelliteSignal(t *testing.T) {
	const (
		mainFetchSite = 0x80069298
		comparedAt    = 0x80494DD0
	)
	targets := map[uint32]struct {
		label string
		size  int
	}{
		0x9FC6ACB5: {"channel object field before message", 2},
		0x9FC6ACC9: {"channel status bits", 2},
		0x9FC6BAE1: {"value compared with 0x0107", 4},
	}
	type read struct {
		opcode uint32
		label  string
		at     uint32
		value  uint32
		pc     uint32
		size   int
	}
	type write struct {
		at, value, opcode, pc, ra uint32
		size                      int
	}
	type enqueue struct {
		at, ra uint32
		words  [2]uint32
	}

	guide := demoGuide(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, demoDictionary(t), multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	moduleRecords := box.RAM.Read(0x00105E9C, bus.Word)
	if moduleRecords < 0x80000000 {
		t.Fatalf("harness: VM module-record pointer is %#08x", moduleRecords)
	}
	moduleOne := moduleRecords + 100
	var targetEnqueue enqueue
	var contrastEnqueue enqueue
	var contrastRecord [3]uint32
	var enqueueInputs []enqueue
	var pendingEnqueue enqueue
	enqueueByIndex := make(map[uint32]enqueue)
	observeQueue := func(a bus.ObservedAccess) {
		st := box.Machine.Core.State()
		if st.PC&^1 == 0x80083832 && st.GPR[5] == 1 {
			base := st.GPR[4] & 0x1fffffff
			pendingEnqueue = enqueue{at: st.GPR[4], ra: st.GPR[31] &^ 1, words: [2]uint32{
				box.RAM.Read(base, bus.Word),
				box.RAM.Read(base+4, bus.Word),
			}}
			enqueueInputs = append(enqueueInputs, pendingEnqueue)
		}
		if a.Write && a.Virtual|0x80000000 == moduleOne+80 && pendingEnqueue.at != 0 {
			enqueueByIndex[a.Value] = pendingEnqueue
		}
	}
	var earlyWrites []write
	var earlyLastOp uint32
	acquireHooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		observeQueue(a)
		if !a.Fetch && !a.Write && a.Virtual >= 0x9FC00000 &&
			box.Machine.Core.State().PC&^1 == mainFetchSite {
			earlyLastOp = 0x9FC00000 | (a.Virtual & 0x00ffffff)
			return
		}
		at := a.Virtual | 0x80000000
		if a.Write && at >= comparedAt && at < comparedAt+4 {
			st := box.Machine.Core.State()
			earlyWrites = append(earlyWrites, write{at: at, value: a.Value,
				opcode: earlyLastOp, pc: st.PC &^ 1, ra: st.GPR[31] &^ 1, size: sizeBytes(a.Size)})
		}
	}}
	want, registered := programmesInTheBlock(t, guide, day), 0
	if at := runUntilHooked(t, box, transmitter, acquireHooks, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	var (
		active        = true // the compared event object may be constructed before the guide opens
		armed         int
		opcode        uint32
		reads         []read
		writes        = earlyWrites
		seen          = map[uint32]int{}
		field         uint32
		subject       uint32
		eventWords    [3]uint32
		eventModule   uint32
		dequeueModule uint32
		lastOp        uint32
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !active {
			return
		}
		st := box.Machine.Core.State()
		pc := st.PC &^ 1
		// The VM event dequeuer at 0x80083E20 calls 0x80083644 with the selected
		// module number in a0 and its output buffer in a1. The native (1, 0x22)
		// wrapper later copies that buffer into the o-code event object. Correlate
		// those two executed calls; addresses and queue layout alone do not prove
		// which module supplied this particular record.
		if pc == 0x8008364C {
			dequeueModule = st.GPR[4]
		}
		observeQueue(a)
		if !a.Fetch && a.Virtual >= 0x9FC00000 && box.Machine.Core.State().PC&^1 == mainFetchSite {
			op := 0x9FC00000 | (a.Virtual & 0x00ffffff)
			lastOp = op
			if _, ok := targets[op]; ok {
				opcode, armed = op, 60
			}
			return
		}
		at := a.Virtual | 0x80000000
		if a.Write {
			if at >= comparedAt-8 && at < comparedAt+4 &&
				pc >= 0x800FBDFA && pc < 0x800FBE40 && st.GPR[31]&^1 == 0x80084020 {
				eventModule = dequeueModule
				queued := enqueueByIndex[box.RAM.Read((moduleOne+84)&0x1fffffff, bus.Word)]
				if at == comparedAt+3 && a.Value == 0x85 {
					targetEnqueue = queued
				}
				if at == comparedAt+3 && a.Value == 0x07 {
					base := uint32((comparedAt - 8) & 0x1fffffff)
					contrastRecord = [3]uint32{
						box.RAM.Read(base, bus.Word), box.RAM.Read(base+4, bus.Word),
						box.RAM.Read(base+8, bus.Word),
					}
					if contrastRecord[2] == 0x0107 {
						contrastEnqueue = queued
					}
				}
			}
			if (field != 0 && at == field) || (at >= comparedAt && at < comparedAt+4) {
				writes = append(writes, write{at: at, value: a.Value, opcode: lastOp,
					pc: st.PC &^ 1, ra: st.GPR[31] &^ 1, size: sizeBytes(a.Size)})
			}
			return
		}
		if a.Fetch || armed == 0 {
			return
		}
		armed--
		if (at >= 0x80060000 && at < 0x80070000) || at >= 0x9FC00000 {
			return
		}
		// The VM stack and frame are word accesses. The opcode's subject is the only byte or
		// halfword data read outside the interpreter in this short handler window.
		wantSize := targets[opcode].size
		if sizeBytes(a.Size) != wantSize {
			return
		}
		if opcode == 0x9FC6BAE1 && box.Machine.Core.State().PC&^1 != 0x8006C0EE {
			return
		}
		reads = append(reads, read{opcode: opcode, label: targets[opcode].label, at: at,
			value: a.Value, pc: box.Machine.Core.State().PC &^ 1, size: wantSize})
		if opcode == 0x9FC6ACC9 {
			field = at
		}
		if opcode == 0x9FC6BAE1 {
			subject = at
			base := (at - 8) & 0x1fffffff
			for i := range eventWords {
				eventWords[i] = box.RAM.Read(base+uint32(i*4), bus.Word)
			}
		}
		seen[opcode]++
		if opcode != 0x9FC6BAE1 {
			armed = 0
		}
	}}
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, _ string, budget int) uint32 {
		if raw == keySelect {
			active = true
		}
		return pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
	}
	openAllChannels(t, press, ".artifacts/nosignal-field-grid.png", true)
	before := screenNow(t, box)
	for attempt := 0; attempt < 6 && screenNow(t, box) == before; attempt++ {
		press(keySelect, "select to view the channel", 80_000_000)
	}
	for i := 0; i < 40_000_000 && seen[0x9FC6ACC9] == 0; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	active = false
	for op, target := range targets {
		if seen[op] == 0 {
			t.Errorf("harness: bytecode %#08x (%s) ran no observable subject read", op, target.label)
		}
	}
	for _, r := range reads {
		t.Logf("%08X %-22s read %08X size %d = %d (%#x), MIPS %08X",
			r.opcode, r.label, r.at, r.size, r.value, r.value, r.pc)
	}
	if field == 0 {
		t.Fatal("harness: the channel-status field was not located")
	}
	if subject != comparedAt {
		t.Fatalf("harness: the value compared with 0x0107 came from %08X, not measured address %08X",
			subject, uint32(comparedAt))
	}
	t.Logf("the 12-byte event record at %08X is %08X %08X %08X",
		subject-8, eventWords[0], eventWords[1], eventWords[2])
	if eventModule == 0 {
		t.Fatal("harness: the measured event was not correlated with a VM module dequeue")
	}
	t.Logf("the event was dequeued from VM module %d", eventModule)
	if targetEnqueue.at == 0 {
		first := 0
		if len(enqueueInputs) > 20 {
			first = len(enqueueInputs) - 20
		}
		for _, e := range enqueueInputs[first:] {
			t.Logf("candidate module 1 enqueue %08X %08X at %08X; caller %08X",
				e.words[0], e.words[1], e.at, e.ra)
		}
		t.Fatal("harness: module 1 supplied the event but its matching enqueue was not observed")
	}
	t.Logf("the matching module 1 enqueue read %08X %08X at %08X; caller %08X",
		targetEnqueue.words[0], targetEnqueue.words[1],
		targetEnqueue.at, targetEnqueue.ra)
	if contrastRecord[2] == 0x0107 {
		t.Logf("contrast event %08X %08X %08X came from %08X %08X at %08X; caller %08X",
			contrastRecord[0], contrastRecord[1], contrastRecord[2],
			contrastEnqueue.words[0], contrastEnqueue.words[1], contrastEnqueue.at,
			contrastEnqueue.ra)
	} else {
		t.Log("no 0x0107 contrast event reached this object during the measured route")
	}
	if len(writes) == 0 {
		t.Fatalf("harness: %08X changed across reads but no write to it was observed after its first read", field)
	}
	firstWrite := 0
	if len(writes) > 24 {
		firstWrite = len(writes) - 24
		t.Logf("... %d earlier writes to the two measured fields elided", firstWrite)
	}
	for _, w := range writes[firstWrite:] {
		t.Logf("WRITE %08X size %d <- %d (%#x), MIPS %08X RA %08X after o-code %08X",
			w.at, w.size, w.value, w.value, w.pc, w.ra, w.opcode)
	}
}
