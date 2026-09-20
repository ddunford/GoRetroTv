package multiplex_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// The claim the derived addressing rests on: on a day the box programs no
// match unit, sections addressed from the in-world clock still reach the guest
// and are registered exactly as a filtered day's are.
//
// It is tested on a day in each of the non-subscribing slots rather than one,
// because "it worked on the day I tried" is how a five-in-eight rule gets
// mistaken for a universal one -- which is the mistake this whole area has
// already produced twice.
//
// THIS EXERCISES A HOST INTERVENTION. Push routes by PID, and on real hardware
// the match unit is what admits a section, so a Digibox might drop what this
// delivers. That is stated in DerivedTitleRequest and counted in
// Counters.TitlesDerived rather than left to be discovered.
func TestTheBoxTakesProgrammesAddressedFromTheClockAlone(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)

	for _, offset := range []int{1, 3, 4, 6, 7} { // MJD 50874, 50876, 50877, 50879, 50880
		day := time.Date(1998, 3, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, offset)
		programmes := 0
		for _, service := range guide.On(day).Services {
			programmes += len(service.Programmes)
		}
		mjd := multiplex.MJDOf(day)
		if subscribingSlots[mjd%8] {
			t.Fatalf("%s is in slot %d, which programs a filter, so it proves nothing about derivation",
				day.Format("2006-01-02"), mjd%8)
		}
		t.Run(day.Format("2006-01-02"), func(t *testing.T) {
			box := restoredBox(t)
			transmitter, err := multiplex.New(box, guide, dict,
				multiplex.FixedClock{At: day}, demoSchedule())
			if err != nil {
				t.Fatal(err)
			}
			registered := 0
			const budget = 30_000_000
			doneAt := runUntil(t, box, transmitter, budget, registeringProgrammes(box, programmes, &registered))
			counts := transmitter.Counts()
			t.Logf("slot %d (MJD %d): %d derived waves, %d registered of %d by instruction %d",
				mjd%8, mjd, counts.TitlesDerived, registered, programmes, doneAt)

			if counts.TitlesDerived == 0 {
				t.Fatal("nothing was derived, so this says nothing about the derived path")
			}
			sub, err := multiplex.Read(box.Demux)
			if err != nil {
				t.Fatal(err)
			}
			if len(sub.Titles) > 0 {
				t.Fatalf("the box programmed a match unit after all, so the derived path was not what was tested")
			}
			if registered != programmes {
				t.Errorf("the box registered %d of %d programmes from clock-addressed sections", registered, programmes)
			}
		})
	}
}
