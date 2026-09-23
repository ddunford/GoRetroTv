package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHETHER THE GRID'S ROW TYPE IS THE SDT'S SERVICE_TYPE.
//
// The row record's first halfword decides whether the grid draws a programme or
// "..no listings available", and it is not computed -- it is COPIED, at MIPS 0x800CB138, out of an
// object the row is built from:
//
//	800cb134  lw   v1,128(sp)   ; the object
//	800cb136  lhu  v1,4(v1)     ; its type
//	800cb138  sh   v1,0(s0)     ; into the row record
//
// Every row reads 1. AND THIS PORT TRANSMITS SERVICE_TYPE 1 FOR EVERY SERVICE -- broadcast's
// serviceType() defaults to 1 and nothing has ever set anything else. That is a correlation, not a
// finding, and it costs one run to turn into either.
//
// It is worth the run because the o-code branches on THREE values -- 1, 2 and 0x10 -- and a
// service_type is exactly the kind of thing a guide would switch a row's presentation on.
//
// ONE RUN READS SIX VALUES, each channel a different one, because a run that changes every channel
// at once cannot say which change did it:
//
//	Sky One   1 (the control: what every feed has always sent)   Sky Movies  3
//	Sky News  2 (the value the programme draw wants)             Sky Travel  4
//	Sky Sports 0x10 (the third branch in the o-code)             Sky Soap    0x19
//
// THE ROW TYPE IS READ AT THE STORE, not from the records afterwards: the record base moves when
// the broadcast changes, and a hardcoded address would report whatever is at the old one.
//
// IT IS ENTIRELY IN THE SIGNAL -- the service_type rides in the SDT and nothing is written into
// guest memory.
func TestWhetherTheGridsRowTypeIsTheServiceType(t *testing.T) {
	const stateWriteAt = 0x800CB138
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	assign := []byte{1, 2, 0x10, 3, 4, 0x19}
	if len(listings.Services) != len(assign) {
		t.Fatalf("harness: this probe assigns %d distinct service types and the schedule lists %d "+
			"channels, so a row could not be traced back to a value", len(assign), len(listings.Services))
	}
	for i := range listings.Services {
		listings.Services[i].Type = assign[i]
		t.Logf("%-14s channel %4d  service_type %#02x",
			listings.Services[i].Name, listings.Services[i].Channel, assign[i])
	}

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
	settled := openAllChannelsFinished(t, press, ".artifacts/servicetype-grid.png")
	for i := 0; i < 50_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "servicetype-grid.png"); err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatalf("harness: no row type was stored at %#08X while the grid drew, so this says "+
			"nothing about service_type. Read .artifacts/servicetype-grid.png", uint32(stateWriteAt))
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
