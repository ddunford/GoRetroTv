package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHETHER THE LINE-UP'S KIND BYTE DECIDES THE GRID'S ROW TYPE.
//
// The row type is not copied from nowhere. Following it back: the row record takes it from an
// object, the object from a six-byte-per-channel table, and that table is built at 0x800BF96C by
//
//	jalr a1 ; lbu a0,12(s0)   ; call map(byte at s0+12)
//	sh   v0,0(a1)             ; the RESULT is the type
//
// and the map is a plain lookup, disassembled at 0x800BFA44:
//
//	1 -> 1     2 -> 2     3 -> 4     4 -> 8     5 -> 16     6 -> 512
//
// So the type the programme draw wants -- 2 -- comes from an input of 2, and the 0x10 branch the
// o-code also tests comes from 5. The byte is currently 1 and the row is type 1.
//
// THE LINE-UP'S KIND BYTE IS THE ONLY THING THIS PORT SENDS THAT COULD BE IT. It is transmitted at
// +2 of the 0xB1 entry, it has been 1 in every feed ever sent, and 1 is what the map is being
// handed. That is a correlation and this run turns it into either a finding or a dead end.
//
// IT IS NOT THE SAME QUESTION AS THE EARLIER KIND SWEEP, which measured whether PROGRAMMES
// REGISTER and found that only kind 1 stores any. This measures the ROW TYPE, which is set whether
// or not anything was stored -- so a value can move one and not the other, and if kind 2 gives a
// type-2 row while storing nothing, that is a real tension worth knowing about rather than a
// contradiction.
//
// IT IS ENTIRELY IN THE SIGNAL.
func TestWhetherTheLineUpKindDecidesTheRowType(t *testing.T) {
	const stateWriteAt = 0x800CB138
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	assign := []byte{1, 2, 3, 4, 5, 6}
	if len(listings.Services) != len(assign) {
		t.Fatalf("harness: this probe assigns %d distinct service types and the schedule lists %d "+
			"channels, so a row could not be traced back to a value", len(assign), len(listings.Services))
	}
	for i := range listings.Services {
		listings.Services[i].Kind = assign[i]
		t.Logf("%-14s channel %4d  line-up kind %d", listings.Services[i].Name,
			listings.Services[i].Channel, assign[i])
	}

	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	// THE REGISTRATION COUNT IS NOT A PRECONDITION HERE. Only the channel left at kind 1 stores
	// its programmes -- that is a measured finding of its own -- so the wait is expected to time
	// out. What this probe is about is the ROW TYPE, which is set whether or not a programme was
	// ever stored.
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	runUntil(t, box, transmitter, 120_000_000, registeringProgrammes(box, want, &registered))
	t.Logf("%d of %d programmes registered (only the kind-1 channel stores; that is expected)",
		registered, want)
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	states := map[uint32]uint32{}
	var order []uint32
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write || a.Fetch || a.Size != bus.Half {
			return
		}
		if box.Machine.Core.State().PC&^1 != stateWriteAt {
			return
		}
		at := a.Virtual | 0x80000000
		if _, seen := states[at]; !seen {
			order = append(order, at)
		}
		states[at] = a.Value
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
	settled := openAllChannels(t, press, ".artifacts/kindtype-grid.png", true)
	for i := 0; i < 50_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "kindtype-grid.png"); err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatalf("harness: no row type was stored at %#08X while the grid drew, so this says "+
			"nothing about service_type. Read .artifacts/kindtype-grid.png", uint32(stateWriteAt))
	}
	t.Logf("the grid settled on %08X; %d row records were typed", settled, len(states))
	sort.Slice(order, func(a, b int) bool { return order[a] < order[b] })
	moved := 0
	t.Logf("=== the type each row record was given ===")
	for i, at := range order {
		note := ""
		if states[at] != 1 {
			note = "   <- NOT 1: the service_type moved it"
			moved++
		}
		t.Logf("    record %08X  type %d%s", at, states[at], note)
		_ = i
	}
	if moved == 0 {
		t.Logf("EVERY ROW IS STILL TYPE 1 with six different service_types on the wire, so the " +
			"row's type is NOT the SDT's service_type. The correlation was a coincidence, and " +
			"the object the type is copied from is something else.")
		return
	}
	t.Logf("%d of %d rows took a type other than 1, so the row type DOES follow the "+
		"service_type -- read the artefact and map value to row", moved, len(order))
}
