package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT WRITES THE GRID'S ROW STATE, AND WHETHER ANYTHING EVER WRITES 2.
//
// The grid keeps a 336-byte record per row in the o-code data segment, and the halfword at its
// start is tested against 2 before a programme is drawn:
//
//	9fc73914  gets ; 9fc73917 push_2 ; 9fc73919 jne_nn 0x9fc7393d
//
// All six rows hold 1. The records are at 0x8048ADB4 and every 336 bytes after it, which was
// measured by catching the load in the act rather than computed, because nothing in this port knew
// where `ds` was.
//
// KNOWING THE FIELD IS 1 DOES NOT SAY WHY. Three shapes of answer are possible and they want
// different work next, so this separates them rather than assuming one:
//
//   - the field is written 1 and never anything else, in which case the writer's o-code is where
//     the real condition lives and this names it;
//   - the field is written 2 for a moment and then written back to 1, in which case something
//     later REJECTS the row and the second write is the interesting one;
//   - the field is never written at all while the grid draws, in which case it is initialised
//     elsewhere -- at boot, or by the screen that owns it -- and the grid is reading a leftover.
//
// EVERY WRITE IS ATTRIBUTED TO THE O-CODE THAT MADE IT, not just to a MIPS PC. The interpreter is
// one loop, so its own PC says nothing; the o-code address is the last opcode fetched from flash at
// 0x80069298, which is what makes a write legible at all.
//
// IT ASSERTS ITS OWN SUBJECT: the row records must be located first, by the same catch-it-in-the-act
// that measured them, and a run that cannot find them reports that rather than watching nothing.
//
// IT ONLY READS.
func TestWhatWritesTheGridsRowState(t *testing.T) {
	const (
		mainFetchSite = 0x80069298
		getsAt        = 0x9FC73914
		rowStride     = 336
		rows          = 6
		// WHERE THE RECORDS HAVE BEEN EVERY TIME. Watching can only start before the grid opens
		// if the address is known in advance -- and the whole point of this run is that NOTHING
		// writes the field while the grid draws, so the write happens earlier. This is the
		// address measured on three separate runs; it is a HYPOTHESIS and the run checks it,
		// because a watch on a guessed address reports a confident zero.
		expectedBase = 0x8048ADB4
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

	type write struct {
		at    uint32
		value uint32
		ocode uint32
	}
	var (
		watching = true // from the very start: the write happens before the grid opens
		armed    int
		lastOp   uint32
		base     uint32
		writes   []write
		values   = map[uint32]int{}
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching {
			return
		}
		if a.Virtual >= 0x9FC00000 && !a.Fetch && !a.Write {
			if box.Machine.Core.State().PC&^1 != mainFetchSite {
				return
			}
			lastOp = 0x9FC00000 | (a.Virtual & 0x00ffffff)
			if lastOp == getsAt {
				armed = 12
			}
			return
		}
		if a.Fetch {
			return
		}
		at := a.Virtual | 0x80000000
		// Locate the row records exactly as they were measured: the one halfword read in the
		// window after the gets opcode.
		if !a.Write && armed > 0 {
			armed--
			if a.Size == bus.Half && (at < 0x80060000 || at >= 0x80070000) {
				if base == 0 || at < base {
					base = at
				}
			}
			return
		}
		if !a.Write || a.Size != bus.Half {
			return
		}
		for row := uint32(0); row < rows; row++ {
			if at == expectedBase+row*rowStride {
				writes = append(writes, write{at: at, value: a.Value, ocode: lastOp})
				values[a.Value]++
				return
			}
		}
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
	settled := openAllChannelsFinished(t, press, ".artifacts/rowstatewrite-grid.png")
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "rowstatewrite-grid.png"); err != nil {
		t.Fatal(err)
	}
	if base == 0 {
		t.Fatalf("harness: the row records were never located -- the gets at %#08X was not seen "+
			"followed by a halfword read outside the interpreter, so nothing below was watched. "+
			"Read .artifacts/rowstatewrite-grid.png", uint32(getsAt))
	}
	if base != expectedBase {
		t.Fatalf("harness: the row records are at %08X on this run, not the %08X this probe "+
			"watched from the start. Every write reported below is of the wrong address; update "+
			"expectedBase and run it again", base, uint32(expectedBase))
	}
	t.Logf("the grid settled on %08X; the row records start at %08X, as expected", settled, base)
	if len(writes) == 0 {
		t.Logf("=== NOTHING WROTE ANY ROW'S STATE WHILE THE GRID DREW ===")
		t.Logf("    So the field is not set by the drawing pass at all: it is initialised " +
			"elsewhere -- at boot, or by whatever owns the screen -- and the grid reads what it " +
			"finds. The next question is who owns it, not what the row loop decided.")
		return
	}
	t.Logf("=== every write to a row's state field, from boot onwards (%d) ===", len(writes))
	for i, w := range writes {
		if i >= 40 {
			t.Logf("    ... and %d more", len(writes)-40)
			break
		}
		t.Logf("    row %d at %08X <- %d   from o-code %08X",
			(w.at-base)/rowStride, w.at, w.value, w.ocode)
	}
	vals := make([]uint32, 0, len(values))
	for v := range values {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(a, b int) bool { return values[vals[a]] > values[vals[b]] })
	t.Logf("=== the values written ===")
	for _, v := range vals {
		note := ""
		if v == 2 {
			note = "  <- 2: a row that WOULD have drawn a programme"
		}
		t.Logf("    %d written %d times%s", v, values[v], note)
	}
	if values[2] == 0 {
		t.Logf("NOTHING EVER WROTE 2. The row state is never set to the value the programme draw " +
			"requires, so the condition is upstream of this field entirely -- disassemble the " +
			"o-code at the addresses above to find what decides the value it does write.")
	}
}
