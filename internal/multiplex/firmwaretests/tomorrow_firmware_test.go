package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE SECOND TITLE SUBSCRIPTION, WHICH THIS PORT HAS NEVER ANSWERED.
//
// The box holds TWO title subscriptions after acquisition, not one. The subscription tree shows
// table 0xA3 -- today's 18:00-23:59 block, which the broadcast answers on PID 0x33 -- and table
// 0xA0, the 00:00-05:59 block, under a different handle. The day rotation puts that block's PID at
// 0x30 | (MJD mod 8), and for the day after the fixture's that is 0x34, which the demux is armed
// for and which this port has never transmitted a single byte on.
//
// So at 19:00 the box is asking for tonight AND for the small hours that follow it, and gets half
// an answer. That is already recorded as a gap; what has not been asked is whether the ALL CHANNELS
// grid is the thing that cares.
//
// IT IS WORTH ASKING BECAUSE OF WHAT THE GRID IS. It draws "Today" across the top and offers
// "+24 Hours" and "-24 Hours" at the bottom, so it is a screen with a notion of a WHOLE DAY and of
// the days either side of it -- where the now-and-next banner needs one programme and ALL
// PROGRAMMES A-Z needs a list with no notion of a day at all. Both of those work. A screen that
// refuses to draw until it holds a day it can page through would look exactly like this one does,
// and it would explain why the grid asks the listings module NOTHING: a check that fails before any
// query is made.
//
// IT IS ENTIRELY IN THE SIGNAL. The sections are ordinary title sections for the following day,
// built by the shipping builder and pushed at the PID the box armed for them.
//
// THE CONTROL IS THE SAME RUN WITHOUT THEM, because "..no listings available" is what this screen
// draws unaided and a run without a control cannot tell a fix from the status quo.
func TestWhetherTheGridWantsTomorrowsBlockToo(t *testing.T) {
	control := gridWithTomorrow(t, false, "tomorrow-control.png")
	delivered := gridWithTomorrow(t, true, "tomorrow-delivered.png")
	t.Logf("=== control   (today only):        %08X", control)
	t.Logf("=== delivered (today + tomorrow):  %08X", delivered)
	if control == delivered {
		t.Logf("the grid finished on the same screen either way, so answering the box's SECOND " +
			"title subscription changes nothing it draws. The gap is real and still worth " +
			"closing, but it is not what the grid is waiting for")
		return
	}
	t.Logf("THE GRID MOVED. Read .artifacts/tomorrow-control.png and -delivered.png -- only the " +
		"picture says whether the cells filled")
}

func gridWithTomorrow(t *testing.T, deliver bool, artefact string) uint32 {
	t.Helper()
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

	if deliver {
		// TOMORROW'S FIRST BLOCK, ON TOMORROW'S PID. The MJD in the filter bytes and the PID are
		// both functions of the day, so neither is chosen: multiplex.TitlePID is the eight-PID
		// rotation the box was measured arming, and the table id's low two bits are the block.
		tomorrow := day.AddDate(0, 0, 1)
		mjd := multiplex.MJDOf(tomorrow)
		pid := multiplex.TitlePID(mjd)
		listings := guide.On(tomorrow)
		if listings == nil || len(listings.Services) == 0 {
			t.Skip("the guide has no schedule for the following day, so there is nothing to send")
		}
		sent := 0
		for i := range listings.Services {
			service := &listings.Services[i]
			var records []broadcast.TitleRecord
			for n := range service.Programmes {
				start, err := service.Programmes[n].StartSeconds()
				if err != nil {
					t.Fatal(err)
				}
				if multiplex.QuarterOf(start) != 0 {
					continue // the box's second subscription is table 0xA0: the 00:00-05:59 block
				}
				records = append(records, broadcast.TitleRecord{
					EventID:  uint16(n + 1), // #nosec G115 -- a day's programmes
					Start:    start,
					Duration: service.Programmes[n].Minutes * 60,
					Title:    service.Programmes[n].Title,
					Genre:    service.Programmes[n].Genre,
					Rating:   service.Programmes[n].Rating,
				})
			}
			if len(records) == 0 {
				continue
			}
			filter := [2]byte{byte(mjd >> 8), byte(mjd)} // #nosec G115 -- an MJD is sixteen bits
			section, err := broadcast.TitleSection(multiplex.TitleTableID(0), service.ListingsID,
				filter, 1, 0, 0, dict, records)
			if err != nil {
				t.Fatal(err)
			}
			if err := box.Demux.Push(pid, section); err != nil {
				t.Fatalf("harness: tomorrow's block for %s was refused on PID %#04x (%v), so the "+
					"box is not armed for it and the grid below measures the push",
					service.Name, pid, err)
			}
			sent++
			// One at a time with guest instructions between: a burst on one PID is handed to a box
			// that never runs the task that drains it.
			for j := 0; j < 400_000; j++ {
				if err := transmitter.Pump(box.Machine.Retired); err != nil {
					t.Fatal(err)
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
		}
		if sent == 0 {
			t.Skip("no channel has a 00:00-05:59 programme tomorrow, so the second subscription " +
				"cannot be answered from this schedule")
		}
		t.Logf("delivered %d of tomorrow's block-0 title sections on PID %#04x (MJD %d)",
			sent, pid, mjd)
	}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := azPressFunc(t, box, pump)
	settled := openAllChannelsFinished(t, press, ".artifacts/"+artefact)
	final := settled
	for i := 0; i < 60_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid settled on %08X and finished on %08X", settled, final)
	return final
}
