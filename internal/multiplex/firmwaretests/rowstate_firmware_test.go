package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE FIELD THE GRID TESTS BEFORE IT DRAWS A PROGRAMME, AND WHAT IT ACTUALLY HOLDS.
//
// The grid's decision is in o-code, it is taken before the listings module is asked anything (it is
// never asked anything at all), and tracing the interpreter's flash reads finds it exactly:
//
//	9fc7390c  push_fp_minus16
//	9fc7390d  push_ds
//	9fc7390e  add_nnnnnnnn 0x0002e250    ; ds + 0x2E250 + 336 * row
//	9fc73913  add
//	9fc73914  gets                       ; load a SIXTEEN-BIT field from it
//	9fc73915  stoiu
//	9fc73917  push_2
//	9fc73919  jne_nn 0x9fc7393d          ; field != 2 -> the "..no listings available" path
//	    .     9fc73926  callr 0x9fc744cd ; field == 2 -> THE PROGRAMME DRAW, never executed
//
// The stride is 336 bytes -- ((x<<2 + x)<<2 + x)<<4, which is 21x<<4 -- so the grid keeps a record
// per row in the o-code data segment, and the halfword at its start is a state the row must be in
// before a programme is drawn into it. All six rows hold something that is not 2.
//
// KNOWING THE CONDITION IS NOT KNOWING THE VALUE, so this reads it rather than reasoning about it.
// The address cannot be computed from here -- `ds` is the interpreter's data segment base and
// nothing in this port knows it -- so the read is caught in the act: when the interpreter's opcode
// fetch lands on 0x9FC73914, the very next DRAM read is that `gets`, and its address and value are
// the answer. That also yields `ds` itself, by subtraction.
//
// IT ASSERTS ITS OWN SUBJECT: the fetch must be seen, and a run that catches none of the six rows
// has measured something other than the grid.
//
// IT ONLY READS.
func TestWhatTheGridsRowStateFieldHolds(t *testing.T) {
	const (
		mainFetchSite = 0x80069298
		getsAt        = 0x9FC73914 // the `gets` that loads the field
		fieldBase     = 0x0002E250 // the offset from `ds` the o-code adds
		rowStride     = 336
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

	type read struct {
		at    uint32
		value uint32
		size  int
	}
	var reads []read
	var window [][]read
	var current []read
	fetches := 0
	armed := 0
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write {
			return
		}
		if a.Virtual >= 0x9FC00000 && !a.Fetch {
			if box.Machine.Core.State().PC&^1 != mainFetchSite {
				return
			}
			if 0x9FC00000|(a.Virtual&0x00ffffff) == getsAt {
				fetches++
				if len(current) > 0 {
					window = append(window, current)
					current = nil
				}
				armed = 12
			}
			return
		}
		if a.Fetch || armed == 0 {
			return
		}
		// THE FIRST READ AFTER THE FETCH IS NOT THE OPERAND. The interpreter reads its own
		// dispatch table -- 209 signed halfword offsets at 0x800692E0 -- before it runs the
		// handler, so a naive pairing reports an address inside the interpreter and a value that
		// is instruction bytes. The next dozen reads are collected instead and the one outside the
		// interpreter is the load this is about.
		armed--
		r := read{at: a.Virtual | 0x80000000, value: a.Value, size: sizeBytes(a.Size)}
		current = append(current, r)
		const interpFrom, interpTo = 0x80060000, 0x80070000
		if r.at >= interpFrom && r.at < interpTo {
			return
		}
		// THE GETS IS THE SIXTEEN-BIT ONE. Everything else in the window is the interpreter
		// walking its own frame and operand stack in words; `gets` is the only halfword load, and
		// the o-code asked for a halfword -- push_2 / jne against a stoiu'd short.
		if r.size != 2 {
			return
		}
		reads = append(reads, r)
	}}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching = true
		}
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
	settled := openAllChannelsFinished(t, press, ".artifacts/rowstate-grid.png")
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "rowstate-grid.png"); err != nil {
		t.Fatal(err)
	}
	if fetches == 0 {
		t.Fatalf("harness: the opcode at %#08X was never fetched while the grid drew, so either "+
			"the o-code moved or this is not the grid. Read .artifacts/rowstate-grid.png",
			uint32(getsAt))
	}
	if len(reads) == 0 {
		t.Fatalf("harness: the gets opcode was fetched %d times and no read outside the "+
			"interpreter followed any of them, so the pairing is wrong and no value below would "+
			"mean anything", fetches)
	}
	if len(current) > 0 {
		window = append(window, current)
	}
	t.Logf("the grid settled on %08X; the opcode was fetched %d times and %d reads outside the "+
		"interpreter followed", settled, fetches, len(reads))
	for i, w := range window {
		if i >= 2 {
			break
		}
		t.Logf("=== every read after fetch %d, in order ===", i+1)
		for _, r := range w {
			t.Logf("    %08X  size %d  value %d (%#x)", r.at, r.size, r.value, r.value)
		}
	}

	// The lowest address is row 0, so `ds` falls out by subtraction -- worth recording because
	// nothing else in this port knows where the o-code's data segment is.
	lowest := reads[0].at
	for _, r := range reads {
		if r.at < lowest {
			lowest = r.at
		}
	}
	t.Logf("row 0's record is at %08X, so ds = %08X and the rows are %d bytes apart",
		lowest, lowest-fieldBase, rowStride)

	seen := map[uint32]uint32{}
	for _, r := range reads {
		seen[r.at] = r.value
	}
	addrs := make([]uint32, 0, len(seen))
	for at := range seen {
		addrs = append(addrs, at)
	}
	sort.Slice(addrs, func(a, b int) bool { return addrs[a] < addrs[b] })
	t.Logf("=== the state field of every row the grid tested ===")
	twos := 0
	for _, at := range addrs {
		row := (at - lowest) / rowStride
		note := ""
		if seen[at] == 2 {
			note = "  <- 2: this row WOULD have drawn a programme"
			twos++
		}
		t.Logf("    row %2d at %08X  =  %d%s", row, at, seen[at], note)
	}
	t.Logf("%d of %d rows hold 2. The grid draws a programme only for those, and "+
		"\"..no listings available\" for the rest.", twos, len(addrs))
}
