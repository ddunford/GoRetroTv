package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE GRID DRAWS IF ITS ROW STATE SAYS 2 -- A BOUNDING EXPERIMENT, NOT A FIX.
//
// **THIS POKE MUST NEVER SHIP AND IS NOT PROPOSED AS A FIX.** Writing guest memory to make a screen
// appear is exactly what this project's binding constraint forbids, for the reason it gives: a
// screen produced that way proves nothing about a Digibox and has to be unpicked later. It is
// allowed HERE because an instrument may read and write memory freely, and because the question it
// answers cannot be answered any other way.
//
// THE QUESTION IS "HOW MUCH IS LEFT". The grid tests a halfword per row -- ds + 0x2E250 + 336*row,
// read by the `gets` at 0x9FC73914 -- against 2, and draws "..no listings available" when it is
// anything else. All six rows hold 1, nothing writes 2 anywhere in a run, and the records are
// created with 1 by a native called from o-code 0x9FC77F23. So the condition is upstream of the
// field, and the field is the last gate before the programme draw at 0x9FC744CD.
//
// Forcing the field says which of two situations this is, and they want completely different work:
//
//   - THE ROWS FILL. Then everything downstream -- the query, the store lookup, the cell painter --
//     works, and the whole remaining problem is what legitimately sets this one halfword. That is a
//     bounded question about one field.
//   - THE ROWS STAY EMPTY. Then the state is necessary and not sufficient, the programme draw
//     fails for its own reasons too, and chasing the field would have been chasing a symptom.
//
// THE POKE IS SURGICAL rather than sprayed: it watches for the native's own write of 1 to a row's
// state and writes 2 to that exact address immediately afterwards. So it changes one halfword per
// row, at the moment the row is created, and touches nothing else -- a blanket poke on a timer
// would be writing over whatever else the screen was doing.
//
// THE CONTROL IS THE SAME RUN WITHOUT THE POKE, because "..no listings available" is what this
// screen draws unaided.
func TestWhatTheGridDrawsIfItsRowStateSaysTwo(t *testing.T) {
	control := gridWithRowState(t, false, "rowstate-poke-control.png")
	poked := gridWithRowState(t, true, "rowstate-poke-forced.png")
	t.Logf("=== control (rows left at 1):   %08X", control)
	t.Logf("=== poked   (rows forced to 2): %08X", poked)
	if control == poked {
		t.Logf("FORCING THE STATE CHANGED NOTHING. The halfword is necessary and NOT sufficient: " +
			"the programme draw at 0x9FC744CD either never runs or fails for its own reasons, so " +
			"the field is a symptom and not the cause. Read both artefacts.")
		return
	}
	t.Logf("THE GRID MOVED. Read .artifacts/rowstate-poke-control.png and -forced.png -- if the " +
		"cells filled, everything downstream of this one halfword works and the whole remaining " +
		"question is what legitimately sets it")
}

func gridWithRowState(t *testing.T, poke bool, artefact string) uint32 {
	t.Helper()
	const (
		rowStride    = 336
		rows         = 6
		expectedBase = 0x8048ADB4 // measured on four runs; checked below
		wantState    = 2
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

	var pending []uint32
	poked := 0
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write || a.Fetch || a.Size != bus.Half || a.Value != 1 {
			return
		}
		at := a.Virtual | 0x80000000
		for row := uint32(0); row < rows; row++ {
			if at == expectedBase+row*rowStride {
				pending = append(pending, at)
				return
			}
		}
	}}
	// The write is applied OUTSIDE the observer: writing to the bus from inside its own callback
	// is re-entrant, and a hook that mutates what it is observing is a hook that cannot be trusted.
	drain := func() {
		if !poke || len(pending) == 0 {
			pending = pending[:0]
			return
		}
		for _, at := range pending {
			box.RAM.Write(at&0x1fffffff, bus.Half, wantState)
			poked++
		}
		pending = pending[:0]
	}
	pump := func() error {
		drain()
		return transmitter.Pump(box.Machine.Retired)
	}
	press := azPressFunc(t, box, func() error { return pump() })
	_ = press

	pressHooked := func(raw uint8, name string, budget int) uint32 {
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
	settled := openAllChannelsFinished(t, pressHooked, ".artifacts/"+artefact)
	final := settled
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	drain()
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	// THE SUBJECT. A poke that never landed is a control with extra steps.
	state := func(row uint32) uint32 {
		return box.RAM.Read((expectedBase+row*rowStride)&0x1fffffff, bus.Half)
	}
	t.Logf("the grid settled on %08X and finished on %08X; row states now %d %d %d %d %d %d",
		settled, final, state(0), state(1), state(2), state(3), state(4), state(5))
	if poke && poked == 0 {
		t.Fatalf("harness: not one row state was poked, so this run is the control with extra "+
			"steps. The records were expected at %08X -- if they have moved, every address here "+
			"is wrong", uint32(expectedBase))
	}
	if poke {
		t.Logf("%d row states were forced from 1 to %d at the moment the native created them",
			poked, wantState)
	}

	// THE REST OF THE RECORD, because the state is only its first halfword and the cells drew
	// EMPTY. Whatever a populated row looks like, this is what an unpopulated one looks like, and
	// the next question is which of these 336 bytes a programme would occupy. Printed for row 0
	// only: six identical dumps would say nothing the first does not.
	var line string
	for off := uint32(0); off < rowStride; off++ {
		if off%32 == 0 {
			if line != "" {
				t.Logf("    %s", line)
			}
			line = fmt.Sprintf("+%03d ", off)
		}
		line += fmt.Sprintf("%02X", box.RAM.Read((expectedBase+off)&0x1fffffff, bus.Byte))
	}
	if line != "" {
		t.Logf("    %s", line)
	}
	nonZero := 0
	for off := uint32(0); off < rowStride; off++ {
		if box.RAM.Read((expectedBase+off)&0x1fffffff, bus.Byte) != 0 {
			nonZero++
		}
	}
	t.Logf("row 0's record: %d of %d bytes are non-zero", nonZero, rowStride)
	return final
}
