package multiplex_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// The block boundaries, which are the whole of TASK-6.13.
//
// They are not derived from anything: they were read off the box's own
// notification slot at eleven times of one day, and the three edges below are
// the three the readings changed at. A change to this table is a claim about
// the firmware, and the firmware test beside it is how such a claim is checked.
func TestADaysListingsAreCutIntoFourSixHourBlocks(t *testing.T) {
	for _, c := range []struct {
		at      string
		quarter int
	}{
		{"00:00", 0}, {"05:59", 0},
		{"06:00", 1}, {"11:59", 1},
		{"12:00", 2}, {"17:59", 2},
		{"18:00", 3}, {"23:59", 3},
	} {
		when, err := time.Parse("15:04", c.at)
		if err != nil {
			t.Fatal(err)
		}
		seconds := when.Hour()*3600 + when.Minute()*60
		if got := multiplex.QuarterOf(seconds); got != c.quarter {
			t.Errorf("%s is in block %d, want %d", c.at, got, c.quarter)
		}
		if got, want := multiplex.TitleTableID(c.quarter), byte(0xa0|c.quarter); got != want {
			t.Errorf("block %d is carried under table %#02x, want %#02x", c.quarter, got, want)
		}
	}
	// A schedule converted round a day boundary must still land in a real
	// block rather than indexing off the end of the array.
	if got := multiplex.QuarterOf(-3600); got != 3 {
		t.Errorf("an hour before midnight is in block %d, want 3", got)
	}
	if got := multiplex.QuarterOf(25 * 3600); got != 0 {
		t.Errorf("an hour after the day ends is in block %d, want 0", got)
	}
}

// The eight title PIDs are a day-of-eight rotation, and the wrap is the case
// that was wrong: on MJD mod 8 == 7 an evening box has 0x37 and 0x30 armed at
// once, and the transmitter used to pick whichever the demux listed last.
//
// Anchored on MJD 50000 = 10 October 1995 rather than on a number this project
// produced (the measurement discipline's rule 6).
func TestTitlePIDIsTheDaysOwnPlaceInTheRotation(t *testing.T) {
	anchor := time.Date(1995, 10, 10, 0, 0, 0, 0, time.UTC)
	if mjd := multiplex.MJDOf(anchor); mjd != 50000 {
		t.Fatalf("10 October 1995 is MJD %d, want 50000; every case below is measured from it", mjd)
	}
	for day := 0; day < 8; day++ {
		mjd := 50000 + day
		want := uint16(0x30 | (mjd % 8)) // #nosec G115 -- three bits
		if got := multiplex.TitlePID(mjd); got != want {
			t.Errorf("MJD %d is carried on PID %#02x, want %#02x", mjd, got, want)
		}
	}
	if got := multiplex.TitlePID(51175); got != 0x37 {
		t.Errorf("MJD 51175 is slot 7 and carried on PID %#02x, want 0x37 -- the day the box also "+
			"has 0x30 armed for tomorrow", got)
	}
}

// A subscription reports what it was told and nothing else. Arms is what the
// transmitter asks before it pushes, because a PID the box has not armed
// swallows a section and reports nothing.
func TestASubscriptionAnswersOnlyForThePIDsItHolds(t *testing.T) {
	sub := multiplex.Subscription{ListingsPIDs: []uint16{0x30, 0x37}}
	for _, pid := range []uint16{0x30, 0x37} {
		if !sub.Arms(pid) {
			t.Errorf("PID %#02x is armed and Arms says it is not", pid)
		}
	}
	for _, pid := range []uint16{0x31, 0x36, 0x00} {
		if sub.Arms(pid) {
			t.Errorf("PID %#02x is not armed and Arms says it is", pid)
		}
	}
	if (multiplex.Subscription{}).Arms(0x33) {
		t.Error("a box that has armed nothing answers for a PID")
	}
}
