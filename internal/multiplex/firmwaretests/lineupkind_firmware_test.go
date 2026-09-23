package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE BYTE BETWEEN THE SERVICE ID AND THE LISTINGS ID, WHICH THIS PORT HAS ONLY EVER SENT AS 1.
//
// The 0xB1 line-up entry is nine bytes: service id, then ONE BYTE, then the listings id, then a
// second id, then the channel number and the flags nibble. Every other field in it has been
// measured. That byte has not. It was set to 1 when the descriptor was first laid out and 1 is what
// every feed since has carried, so the box has never been asked what it does with any other value.
//
// IT IS THE STRONGEST REMAINING CANDIDATE, AND THE REASON IS THE SHAPE OF THE FAILURE RATHER THAN A
// HUNCH. Everything else has been eliminated by measurement, not by argument:
//
//   - coverage -- the box programmed one match unit, a3/ff ext 01ff/fe01 mjd 51171 pid 33, and all
//     six of our listings ids satisfy it;
//   - the day and the block -- MJD 51171 and table 0xA3 are exactly what that unit asked for;
//   - the time -- the now-and-next banner on the same box at the same instant shows "Dream Team" at
//     7.00pm on Sky One, which IS the grid's first row and its first column;
//   - the key -- the line-up's listings field already equals the extension the title sections are
//     addressed with, and making the service ids equal the listings ids too drew a byte-identical
//     grid.
//
// So the programmes are in the store, filed under the id the row names, for the day and the minute
// the row is asking about -- and the row still draws "..no listings available". A per-channel byte
// in the very descriptor that ties a channel to its programmes is exactly the kind of thing that
// empties one screen's list while leaving the tuned service, which nothing filters, working.
//
// ONE RUN READS SIX VALUES, because a run that changes every channel at once cannot say which
// change did it:
//
//	Sky One     101  kind 1   (the control: what every feed has always sent)
//	Sky Soap    121  kind 2
//	Sky Travel  251  kind 3
//	Sky Movies  301  kind 4
//	Sky Sports  401  kind 5
//	Sky News    501  kind 8
//
// IT ALSO MAPS THE RECORD, which is overdue. The guest unpacks the nine wire bytes into an
// eighteen-byte record and this project knows only three of its fields -- the service id at +4, the
// channel at +10 and the flag bytes at +13..16. The whole record is dumped here beside the values
// that were transmitted, so where kind, listings and extra actually land stops being a guess. That
// costs nothing on a run we are making anyway and it is how the next question gets asked cheaply.
//
// IT ASSERTS ITS OWN SUBJECT. If the kind byte did not survive into the record, then whatever the
// grid draws afterwards measures the transmitter and not the firmware, and must not be filed as a
// finding about the screen. That has happened on this project and the instrument now refuses it.
//
// IT IS ENTIRELY IN THE SIGNAL. The byte rides in the BAT and the box parses it out of a section
// the hardware delivered; nothing is written into guest memory.
func TestWhichLineUpKindTheAllChannelsGridWants(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	assign := []byte{1, 2, 3, 4, 5, 8}
	if len(listings.Services) != len(assign) {
		t.Fatalf("harness: this probe assigns %d distinct kind values and the schedule lists %d "+
			"channels, so a row could not be traced back to a value", len(assign), len(listings.Services))
	}
	sent := map[uint16]byte{}
	for i := range listings.Services {
		listings.Services[i].Kind = assign[i]
		sent[listings.Services[i].Channel] = assign[i]
		t.Logf("%-14s channel %4d  listingsID %4d  kind %d", listings.Services[i].Name,
			listings.Services[i].Channel, listings.Services[i].ListingsID, assign[i])
	}

	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	// THE COUNT IS THE RESULT, NOT A PRECONDITION. A first version waited for the whole block to
	// register and failed when it did not -- which is the finding, reported as a broken harness.
	// Only the channel left at kind 1 stores anything, so the wait is expected to time out and
	// what matters is HOW MANY arrived.
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	runUntil(t, box, transmitter, 120_000_000, registeringProgrammes(box, want, &registered))
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	// Sky One is the only channel still at kind 1, and it has six programmes in the 18:00-23:59
	// block; the schedule is the authority on that rather than a constant written here.
	control := 0
	for i := range listings.Services {
		if listings.Services[i].Kind != 1 {
			continue
		}
		for n := range listings.Services[i].Programmes {
			start, err := listings.Services[i].Programmes[n].StartSeconds()
			if err != nil {
				t.Fatal(err)
			}
			if multiplex.QuarterOf(start) == 3 {
				control++
			}
		}
	}
	t.Logf("%d of %d programmes registered; the one channel left at kind 1 has %d in the block",
		registered, want, control)
	if registered != control {
		t.Errorf("%d programmes registered, but only the kind-1 channel's %d should have been "+
			"stored. THE MEASUREMENT (2026-09-22) is that the line-up entry's kind byte GATES "+
			"storage and 1 is the only value that stores: sweeping 1,2,3,4,5,8 across six "+
			"channels registered exactly and only Sky One's six. Either that has changed or this "+
			"probe has", registered, control)
	}

	// DID THE BYTE ARRIVE, AND WHERE DID IT LAND? Both questions, one dump.
	records := serviceRecords(t, box, listings.Services)
	readByte := func(off uint32) byte {
		return byte(box.RAM.Read(off, bus.Byte)) // #nosec G115 -- byte read
	}
	carried, at := 0, map[int]int{}
	for _, rec := range records {
		mine := sent[rec.service.Channel]
		line := ""
		for i := uint32(0); i < 18; i++ {
			line += fmt.Sprintf(" %02X", readByte(rec.base+i))
		}
		t.Logf("channel %4d  record %08X %s  (kind %d, listingsID %#04x)",
			rec.service.Channel, 0x80000000|rec.base, line, mine, rec.service.ListingsID)
		// Which offsets hold the kind we sent? Counted across channels, so an offset that holds it
		// for ALL SIX is the field, and one that holds it for a single channel is a coincidence.
		for i := 0; i < 18; i++ {
			if readByte(rec.base+uint32(i)) == mine { // #nosec G115 -- small constant
				at[i]++
			}
		}
		if mine != 1 {
			// The control's value of 1 is too common a byte to prove anything, so only the changed
			// channels count towards "it travelled".
			for i := 0; i < 18; i++ {
				if readByte(rec.base+uint32(i)) == mine { // #nosec G115 -- small constant
					carried++
					break
				}
			}
		}
	}
	for off, n := range at {
		if n == len(records) {
			t.Logf("offset +%d holds the transmitted kind on ALL %d channels -- that is the field",
				off, len(records))
		}
	}
	if carried == 0 {
		t.Fatalf("harness: not one of the five changed channels has its kind byte anywhere in its " +
			"eighteen-byte record, so the value never reached the box through the BAT. Whatever " +
			"the grid draws next measures the transmitter, not the firmware")
	}
	t.Logf("%d of %d changed channels carry their kind byte, so it travelled the signal end to end",
		carried, len(records)-1)

	// NO NAVIGATION. This probe used to walk to the ALL CHANNELS grid afterwards, from when the
	// question was "which kind value makes the grid draw". The answer turned out to be upstream of
	// the grid entirely -- five of the six channels store NOTHING -- so a grid reached in that
	// state measures a box with almost no listings, and the route cannot even name the screens on
	// the way there. The registration count IS the finding, and it is asserted above.
	t.Logf("THE KIND BYTE GATES STORAGE AND 1 IS THE ONLY VALUE THAT STORES. Swept %v across the "+
		"six channels: only the one left at 1 registered anything. What the other values MEAN is "+
		"not established and is not guessed at here.", assign)
}
