package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DOES THE BOX ASK FOR TOMORROW, AND AT WHAT HOUR?
//
// gort-k1z built the next-day broadcast and proved it at 23:15, where the grid's midnight column
// fills. On the live demo at 21:21 it is empty on every channel, including the two that
// demonstrably have programmes there -- Sky News from 00:00 and Sky Movies from 00:00. Same code,
// same schedule file. The only thing that differs is the hour.
//
// THE GATE THAT COULD EXPLAIN IT IS NAMED IN THE CODE. titlesFor refuses a day whose listings PID
// the box has not armed, counts it as TitlesUnaddressed and returns nothing -- deliberately, because
// this carousel answers subscriptions rather than broadcasting at them. If the box does not arm
// tomorrow's PID until late in the evening, then at 21:21 there is nothing wrong with the
// transmitter and everything missing from the screen.
//
// AND THE COUNTER THAT WOULD HAVE SAID SO IS NOT LOGGED. Counters.TitlesUnaddressed calls itself
// "the single most useful number here" and cmd/goretrotv prints Clock, Lineup, Titles, Derived,
// Index and Events -- not it. So the live box reported title_waves=2 while tomorrow was being
// refused, and 2 is the number of WAVES, not of days: titleWave increments it once per pass, after
// concatenating today and tomorrow. A reading of "2" was taken to mean both days went out. It does
// not mean that, and nothing in the log distinguished the two.
//
// SO THIS MEASURES THE ARMING ITSELF, at three hours of the same evening block: one the project has
// always tested, one the live box fails at, and the one gort-k1z proved. It samples the demux
// rather than the transmitter, because ArmedPIDs is the box's own statement of what it wants and
// cannot be confused with what we chose to send.
//
// IT ASSERTS ITS OWN SUBJECT: today's PID must be armed in every run. A box that has armed nothing
// is mid-boot, and "tomorrow is not armed either" would then be a fact about the boot rather than
// about the hour.
//
// IT ONLY READS.
func TestWhenTheBoxArmsTomorrowsListingsPID(t *testing.T) {
	for _, at := range []time.Time{
		time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC),  // the hour every other probe pins
		time.Date(1998, 12, 24, 21, 21, 0, 0, time.UTC), // the hour the live box failed at
		time.Date(1998, 12, 24, 23, 15, 0, 0, time.UTC), // the hour gort-k1z proved
	} {
		t.Run(at.Format("15h04"), func(t *testing.T) {
			guide := demoGuide(t)
			dict := demoDictionary(t)
			box := restoredBox(t)
			transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: at},
				demoSchedule())
			if err != nil {
				t.Fatal(err)
			}
			today := multiplex.TitlePID(multiplex.MJDOf(at))
			tomorrow := multiplex.TitlePID(multiplex.MJDOf(at.AddDate(0, 0, 1)))

			want := programmesInTheBlock(t, guide, at)
			registered := 0
			if got := runUntil(t, box, transmitter, 120_000_000,
				registeringProgrammes(box, want, &registered)); got < 0 {
				t.Fatalf("harness: only %d of %d programmes registered at %s, so this box has not "+
					"acquired the day it is in and says nothing about the day after",
					registered, want, at.Format("15:04"))
			}
			// AND THEN KEEP WATCHING. A PID the box arms later is still a PID it asks for, and a
			// single sample after registration would report "never" for an arming that had simply
			// not happened yet.
			firstArmed := -1
			runUntil(t, box, transmitter, 120_000_000, func(i int) bool {
				if firstArmed < 0 && armed(box.Demux.ArmedPIDs(), tomorrow) {
					firstArmed = i
				}
				return false
			})

			pids := box.Demux.ArmedPIDs()
			counts := transmitter.Counts()
			when := "NEVER, in 240 million instructions"
			if firstArmed >= 0 {
				when = fmt.Sprintf("after %d instructions of the second pass", firstArmed)
			}
			t.Logf("%s  today PID %#02x armed=%v | tomorrow PID %#02x armed=%v (%s)",
				at.Format("15:04"), today, armed(pids, today), tomorrow, armed(pids, tomorrow), when)
			t.Logf("    armed PIDs %#02x", pids)
			t.Logf("    titles=%d unaddressed=%d derived=%d index=%d",
				counts.Titles, counts.TitlesUnaddressed, counts.TitlesDerived, counts.Index)

			if !armed(pids, today) {
				t.Fatalf("harness: the box has NOT armed today's listings PID %#02x at %s, so it "+
					"is not asking for the day it is in and nothing here is about tomorrow",
					today, at.Format("15:04"))
			}
		})
	}
}

// armed reports whether the box has this PID open.
func armed(pids []uint16, pid uint16) bool {
	for _, p := range pids {
		if p == pid {
			return true
		}
	}
	return false
}
