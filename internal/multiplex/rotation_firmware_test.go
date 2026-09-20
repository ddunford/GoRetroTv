package multiplex_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// subscribingSlots are the day-of-eight slots on which this box programs a
// listings filter. The other five arm the right PID and program nothing, so
// nothing can be delivered to them.
//
// This test PINS A DEFECT rather than describing intended behaviour, and it is
// written to fail the moment the defect is fixed -- which is the only way a
// characterisation test earns its place. Two things make it worth having:
//
//   - the demo has to choose an in-world day, and choosing one outside this set
//     produces an empty guide with no error anywhere;
//   - the finding was first measured through a census that assumed a 0xFE table
//     mask and no extension mask, both of which turned out to be wrong, so it
//     had to be re-taken through a census that assumes neither. It survived.
//     Without this test that re-measurement is a paragraph nobody re-runs.
var subscribingSlots = map[int]bool{1: true, 3: true, 6: true}

// TASK-6.13. Eight consecutive days, one box each.
func TestOnlyThreeDaysInEightProgramAListingsFilter(t *testing.T) {
	listings := demoListings(t)
	for d := 0; d < 8; d++ {
		day := time.Date(1998, 3, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, d)
		t.Run(day.Format("2006-01-02"), func(t *testing.T) {
			box := restoredBox(t)
			transmitter, err := multiplex.New(box, listings, demoDictionary(t),
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
			const budget = 20_000_000
			var after multiplex.Subscription
			subscribedAt := -1
			for i := 0; i < budget; i++ {
				if err := transmitter.Pump(box.Machine.Retired); err != nil {
					t.Fatal(err)
				}
				if i%4096 == 0 {
					read, err := multiplex.Read(box.Demux)
					if err != nil {
						t.Fatal(err)
					}
					after = read
					if len(read.Titles) > 0 {
						subscribedAt = i
						break
					}
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
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
			if subscribedAt > budget/2 {
				t.Errorf("subscribing took %d of a %d-instruction budget; the five negative slots are no "+
					"longer demonstrably long enough to have subscribed", subscribedAt, budget)
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
			for _, service := range listings.Services {
				if request.Wants(service.ListingsID) {
					covered++
				}
			}
			if covered != len(listings.Services) {
				t.Errorf("the box's filter %#04x/%#04x covers %d of %d channels; the rest would never be broadcast",
					request.Extension, request.ExtensionMask, covered, len(listings.Services))
			}
		})
	}
}
