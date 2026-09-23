package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHO SETS THE TYPE THE GRID'S ROWS INHERIT.
//
// The row record's first halfword decides whether a programme is drawn, and it is copied straight
// out of an object -- 800cb134 lw v1,128(sp) / 800cb136 lhu v1,4(v1) / 800cb138 sh v1,0(s0). All
// six rows resolve to ONE object and its type field reads 1.
//
// Following the resolver is a chain of lock/unlock wrappers around inner functions, and each hop
// is another decompile. Asking the machine is one run: watch the object's type field for writes
// from boot, and record the instruction that makes each one.
//
// THE ADDRESS IS A HYPOTHESIS AND THE RUN CHECKS IT. Watching can only start before the grid opens
// if the address is known in advance, and it is only known from previous runs. So the watch uses
// the measured address and then VERIFIES it against the type load at 0x800CB136 -- if the object
// has moved, every write reported is of the wrong address and the run says so instead of reporting
// a confident nothing.
//
// IT ONLY READS.
func TestWhoSetsTheTypeTheGridsRowsInherit(t *testing.T) {
	const (
		typeLoadAt   = 0x800CB136 // lhu v1,4(v1) -- reads the object's type
		copyLoadAt   = 0x800F99FE // lb a2,0(v0) -- the load half of the byte-copy loop
		expectedType = 0x802AF140 // ...at this address, measured on three runs
	)
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	type write struct {
		value uint32
		pc    uint32
		when  uint64
		at    uint32
		width uint32
		from  uint32
	}
	var writes []write
	var loadedFrom, lastSource uint32
	loads := 0
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			return
		}
		at := a.Virtual | 0x80000000
		pc := box.Machine.Core.State().PC &^ 1
		if !a.Write {
			if pc == typeLoadAt {
				loads++
				loadedFrom = at
			}
			// THE COPY'S SOURCE. 0x800F9A02 is the store half of a byte-copy loop --
			//     800f99fe  lb   a2,0(v0)    ; source
			//     800f9a02  sb   a2,0(v1)    ; destination
			// -- so the object is a COPY, and the byte read immediately before each store is where
			// its type came from. That address is the template this whole chain leads back to.
			if pc == copyLoadAt {
				lastSource = at
			}
			return
		}
		// ANY WRITE THAT COVERS THE FIELD, not just one starting exactly on it. The type is a
		// HALFWORD, so the value 1 is the bytes 00 01 and a byte store to the odd address sets it
		// without ever touching the even one -- a first version matched the even address alone and
		// reported thirteen writes of ZERO into a field that demonstrably reads 1.
		width := uint32(1)
		switch a.Size {
		case bus.Half:
			width = 2
		case bus.Word:
			width = 4
		}
		if at < expectedType+2 && at+width > expectedType {
			writes = append(writes, write{value: a.Value, pc: pc, when: box.Machine.Retired,
				at: at, width: width, from: lastSource})
		}
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
	settled := openAllChannels(t, press, ".artifacts/rowobjecttype-grid.png", true)
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "rowobjecttype-grid.png"); err != nil {
		t.Fatal(err)
	}
	if loads == 0 {
		t.Fatalf("harness: the type load at %#08X never ran, so the grid did not build its rows "+
			"and nothing below describes them", uint32(typeLoadAt))
	}
	if loadedFrom != expectedType {
		t.Fatalf("harness: the row type was loaded from %08X on this run, not the %08X this probe "+
			"watched from boot. The object has moved; update expectedType and run it again -- "+
			"every write reported would otherwise be of the wrong address",
			loadedFrom, uint32(expectedType))
	}
	t.Logf("the grid settled on %08X; the type was read from %08X %d times",
		settled, loadedFrom, loads)
	if len(writes) == 0 {
		t.Logf("=== NOTHING WROTE THE TYPE FIELD IN THIS ENTIRE RUN ===")
		t.Logf("    So it is not set while the box is running at all: the value is in the " +
			"restored snapshot, and the object predates everything this probe can see.")
		return
	}
	// THE TABLE THE COPIES COME FROM. The sources are six bytes apart, so the grid is copying
	// entries out of an array -- and every entry's type reads 1 where a programme row wants
	// another value. Dumping it is how the next question gets asked: what would have to be in here.
	lowest := writes[0].from
	for _, w := range writes {
		if w.from != 0 && w.from < lowest {
			lowest = w.from
		}
	}
	if lowest > 0x40 {
		start := (lowest - 0x30) &^ 1
		t.Logf("=== the table the row objects are copied out of, from %08X ===", start)
		for off := uint32(0); off < 0x90; off += 6 {
			at := start + off
			line := ""
			for b := uint32(0); b < 6; b++ {
				line += fmt.Sprintf("%02X", byte(box.RAM.Read((at+b)&0x1fffffff, bus.Byte))) // #nosec G115 -- byte read
				if b%2 == 1 {
					line += " "
				}
			}
			mark := ""
			for _, w := range writes {
				if w.from >= at && w.from < at+6 {
					mark = "   <- a row was copied from here"
					break
				}
			}
			t.Logf("    %08X  %s%s", at, line, mark)
		}
	}

	t.Logf("=== every write to the object's type field (%d) ===", len(writes))
	for i, w := range writes {
		if i >= 30 {
			t.Logf("    ... and %d more", len(writes)-30)
			break
		}
		t.Logf("    %08X(%d) <- %d   copied from %08X   MIPS %08X",
			w.at, w.width, w.value, w.from, w.pc)
	}
}
