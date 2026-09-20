package broadcast_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
)

// mjdEpoch is day zero of the Modified Julian Date, which is the fixed point
// every date in this file is anchored on rather than on a number someone
// produced. MJD 50000 is 10 October 1995; check any other value against that.
var mjdEpoch = time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC)

func mjdOf(day time.Time) int {
	return int(day.UTC().Truncate(24*time.Hour).Sub(mjdEpoch) / (24 * time.Hour))
}

// TC-6.5. The listings request is day-addressed, and this is what actually
// addresses it.
//
// Three things were measured here on 2026-09-20 and two of them correct the
// record, so they are written out with the evidence rather than asserted:
//
//  1. **The TOT is the clock table; the TDT is not.** The fixture's match
//     units carry 0x73 (TOT) and nothing matches 0x70 (TDT) at all. Feeding a
//     TDT alone moves nothing -- not the day, not the PID -- and feeding a TOT
//     moves the whole request together. Every earlier run in this package fed
//     a TDT and got a day anyway, because the box already had one.
//
//  2. **The box asks for its clock's own day, not the day after.** The record
//     reads "the box asks for the day after its clock" from a pair of
//     observations; swept across five dates here the requested MJD equals the
//     TOT's MJD exactly, every time.
//
//  3. **The listings PID is 0x30 | (MJD mod 8).** Sky's eight title PIDs are
//     a day-of-eight rotation, confirmed on eight consecutive days: MJD 50873
//     -> 0x31, 50874 -> 0x32, 50875 -> 0x33 ... 50880 -> 0x30. The record had
//     only "the box moved within 0x30-0x37".
//
// And one thing that is NOT yet understood and must not be smoothed over: on
// five of those eight days the box arms the right PID and then programs no
// title match unit at all, so there is nothing to deliver to. It is tracked
// rather than guessed at, and the days used below are ones that do subscribe.
func TestTheClockTableChoosesTheDayAndThePIDTheBoxAsksFor(t *testing.T) {
	// The fixture is a warm box: it boots with a day already in hand, so
	// "no clock table" does not mean "no clock". That resting day is the
	// control every other case is measured against.
	const restingMJD = 50814 // 1 January 1998

	for _, tc := range []struct {
		name    string
		feedTOT bool
		feedTDT bool
		when    time.Time
		wantMJD int
	}{
		{"no clock table leaves the box on the day it woke with", false, false, time.Time{}, restingMJD},
		{"a TDT alone is not a clock table and moves nothing", false, true,
			time.Date(1998, 6, 15, 12, 0, 0, 0, time.UTC), restingMJD},
		{"a TOT moves the whole request to its own day", true, false,
			time.Date(1998, 6, 15, 12, 0, 0, 0, time.UTC), 50979},
		{"and to a different day in a different month", true, false,
			time.Date(1998, 3, 6, 12, 0, 0, 0, time.UTC), 50878},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := restoredBox(t)
			bouquetID, networkID := guestSubscription(t, box)
			if _, asking := titleSubscription(box); asking {
				t.Fatal("the fixture already carries a listings filter, so nothing below measures what programmed it")
			}

			if tc.feedTOT {
				section, err := broadcast.TOT(tc.when, broadcast.TimeOffset{
					Country: "GBR", Region: 0, OffsetMinutes: 0,
					ChangeUTC: tc.when.AddDate(0, 3, 0), NextOffsetMinutes: 60,
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := box.Demux.Push(0x14, section); err != nil {
					t.Fatal(err)
				}
			}
			if tc.feedTDT {
				section, err := broadcast.TDT(tc.when)
				if err != nil {
					t.Fatal(err)
				}
				if err := box.Demux.Push(0x14, section); err != nil {
					t.Fatal(err)
				}
			}
			if tc.feedTOT || tc.feedTDT {
				for i := 0; i < clockSettle; i++ {
					if err := box.Step(); err != nil {
						t.Fatal(err)
					}
				}
			}

			pushAcquisitionBAT(t, box, bouquetID, networkID)
			req := runUntilTheBoxAsksForTitles(t, box, acquisitionBudget)

			gotMJD := int(req.Filter[0])<<8 | int(req.Filter[1])
			if gotMJD != tc.wantMJD {
				t.Errorf("the box asked for MJD %d (%s), want %d (%s)",
					gotMJD, mjdEpoch.AddDate(0, 0, gotMJD).Format("2006-01-02"),
					tc.wantMJD, mjdEpoch.AddDate(0, 0, tc.wantMJD).Format("2006-01-02"))
			}
			if tc.feedTOT && gotMJD != mjdOf(tc.when) {
				t.Errorf("the box asked for MJD %d but its clock table said %d; the request does not "+
					"follow the clock and every section addressed from it would be mis-filed",
					gotMJD, mjdOf(tc.when))
			}
			// The PID is not independent of the day, and checking it is what
			// distinguishes "the day moved" from "the whole request moved".
			if wantPID := uint16(0x30 | (gotMJD % 8)); req.PID != wantPID {
				t.Errorf("the box armed PID %#02x for MJD %d, want %#02x = 0x30 | (%d mod 8)",
					req.PID, gotMJD, wantPID, gotMJD)
			}
		})
	}
}

// pushAcquisitionBAT gives the box the channel list that makes it acquire. The
// same BAT every time, so the only thing varying between the cases above is
// the clock.
func pushAcquisitionBAT(t *testing.T, box *board.Runtime, bouquetID, networkID uint16) {
	t.Helper()
	section, err := broadcast.BAT(bouquetID, 1, "Sky", []broadcast.Transport{{
		ID: networkID, NetworkID: networkID, FrequencyMHz: 11778, OrbitTenths: 282,
		SymbolRate: 27500, FEC: 2,
		Services: []broadcast.Service{{ID: 0x0064, Name: "Sky One"}},
		Lineup: []broadcast.LineupEntry{
			{ServiceID: 0x0064, Kind: 1, Listings: 0x0bb8, Extra: 0x1770, Channel: 101},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Demux.Push(0x11, section); err != nil {
		t.Fatal(err)
	}
}
