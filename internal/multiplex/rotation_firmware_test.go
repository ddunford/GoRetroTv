package multiplex_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// subscribingSlots are the day-of-eight slots on which this box programs a
// listings match unit FOR THE DAY IT IS IN. The other five arm the right PID
// and program nothing.
//
// THE TIME OF DAY IS PART OF THIS CLAIM AND THE SWEEP BELOW RUNS AT NOON. An
// evening box also programmes a unit for TOMORROW when tomorrow's slot is one
// of these three, so at 19:00 the same box subscribes on six days in eight.
// That is not a contradiction and it is not this test's subject -- but a
// reading of "three days in eight" taken as a fact about the calendar is what
// sent TASK-6.13 after a rotation for a week, when the real variable was the
// hour on the clock (docs/reference/digibox-emulation.md -> 'A day of listings
// is FOUR SIX-HOUR BLOCKS').
//
// This test PINS A FIRMWARE BEHAVIOUR the product routes around, and it is
// written to fail the moment that behaviour changes. It is worth having for
// two reasons:
//
//   - the transmitter DERIVES its addressing on the other five days, and that
//     is a host intervention justified entirely by this measurement. If the box
//     started programming a unit on all eight, the derivation would be dead
//     code pretending to be a workaround;
//   - the finding was first taken through a census that assumed a 0xFE table
//     mask and no extension mask, both of which turned out to be wrong, so it
//     had to be re-taken through a census that assumes neither. It survived.
//     Without this test that re-measurement is a paragraph nobody re-runs.
var subscribingSlots = map[int]bool{1: true, 3: true, 6: true}

// TASK-6.13. Eight consecutive days, one box each, all of them at noon.
func TestOnlyThreeDaysInEightProgramAListingsFilter(t *testing.T) {
	guide := demoGuide(t)
	for d := 0; d < 8; d++ {
		day := time.Date(1998, 3, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, d)
		t.Run(day.Format("2006-01-02"), func(t *testing.T) {
			box := restoredBox(t)
			transmitter, err := multiplex.New(box, guide, demoDictionary(t),
				multiplex.FixedClock{At: day}, demoSchedule())
			if err != nil {
				t.Fatal(err)
			}
			// A budget rather than a duration: a slot that is going to
			// subscribe stops here as soon as it has, and only the five that
			// never do pay the whole window. The line-up goes out at the
			// carousel's 8,000,000-instruction clock settle and the three
			// subscribing slots act on it within about 725,000 more, so this
			// is a margin of more than ten -- and the positive cases assert
			// that margin below rather than leaving it to be believed.
			const budget = 14_000_000
			var after multiplex.Subscription
			subscribedAt := runUntil(t, box, transmitter, budget, func(i int) bool {
				if i%4096 != 0 {
					return false
				}
				read, err := multiplex.Read(box.Demux)
				if err != nil {
					t.Fatal(err)
				}
				after = read
				return len(read.Titles) > 0
			})
			mjd := multiplex.MJDOf(day)
			slot := mjd % 8

			if transmitter.Counts().Lineup == 0 {
				t.Fatalf("no line-up was transmitted at all, so this says nothing about day %d", mjd)
			}
			if !subscribingSlots[slot] {
				if len(after.Titles) > 0 {
					t.Errorf("slot %d (MJD %d) now programs a listings filter. If TASK-6.13 has been "+
						"fixed, add %d to subscribingSlots and let the in-world clock use every day",
						slot, mjd, slot)
				}
				return
			}
			if len(after.Titles) == 0 {
				t.Fatalf("slot %d (MJD %d) programmed no listings filter, and it is one of the three that does", slot, mjd)
			}
			t.Logf("slot %d (MJD %d) subscribed after %d instructions", slot, mjd, subscribedAt)
			// The three that subscribe do so at about 8.6 million, so the
			// five that do not are given several million more than that
			// before they are believed. If subscribing ever creeps close to
			// the budget, the negative cases stop meaning anything and this
			// says so rather than going quietly green.
			if margin := budget - subscribedAt; margin < 4_000_000 {
				t.Errorf("subscribing took %d of a %d-instruction budget, leaving %d; the five negative "+
					"slots are no longer demonstrably long enough to have subscribed", subscribedAt, budget, margin)
			}
			request := after.Titles[0]
			if request.MJD() != mjd {
				t.Errorf("the box asks for MJD %d, want the %d the broadcast claimed", request.MJD(), mjd)
			}
			if want := uint16(0x30 | slot); request.PID != want {
				t.Errorf("listings PID %#02x, want %#02x = 0x30 | (MJD mod 8)", request.PID, want)
			}
			// One filter covers the whole line-up: the value is the OR of the
			// listings ids and the mask clears the bits that differ.
			covered := 0
			for _, service := range guide.On(day).Services {
				if request.Wants(service.ListingsID) {
					covered++
				}
			}
			if covered != len(guide.On(day).Services) {
				t.Errorf("the box's filter %#04x/%#04x covers %d of %d channels; the rest would never be broadcast",
					request.Extension, request.ExtensionMask, covered, len(guide.On(day).Services))
			}
		})
	}
}
