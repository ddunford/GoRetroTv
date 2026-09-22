package firmwaretests_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// TASK-6.13, the read path, read off the box rather than reasoned about.
//
// The guide does not poll a store: it registers a notification slot and waits,
// and 0x800C579C wakes it only when the arriving section's day and its
// tableID & 3 match what the slot asked for. Those two bits are the six-hour
// block of the day the guide is showing, so a transmitter that stamps one
// table id serves one quarter of the day and leaves the other three looking
// exactly like a box with no listings at all.
//
// Two times of day, one date, everything else identical. Both must find a
// slot: if the evening one does not, the instrument is broken rather than the
// box, because the evening is the case the whole demo has been running on.
func TestTheGuideSubscribesForTheBlockOfTheDayItIsIn(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	for _, hour := range []int{12, 19} {
		day := time.Date(1998, 12, 24, hour, 0, 0, 0, time.UTC)
		t.Run(day.Format("15:04"), func(t *testing.T) {
			box := restoredBox(t)
			transmitter, err := multiplex.New(box, guide, dict,
				multiplex.FixedClock{At: day}, demoSchedule())
			if err != nil {
				t.Fatal(err)
			}
			// WAITING FOR THE LINE-UP, NOT FOR THE LISTINGS. What the guide
			// subscribes for is its day and its block, and it knows both from
			// the clock alone -- so this run stops as soon as the box has
			// acquired, which is about a seventh of the instructions a run
			// that waited for programmes would cost. The package is already
			// the slowest in this repo under the race detector and every
			// firmware box in it is paid for twice there.
			if at := runUntil(t, box, transmitter, 30_000_000, func(i int) bool {
				if i%4096 != 0 {
					return false
				}
				read, err := multiplex.Read(box.Demux)
				return err == nil && len(read.ListingsPIDs) > 0
			}); at < 0 {
				t.Fatal("the box never armed a listings PID, so it has not acquired and the guide " +
					"has no day to subscribe for")
			}
			// THE SETTLE BEFORE THE PRESS IS LOAD-BEARING. A key sent the
			// instant the box finishes something does nothing at all -- no
			// slot, and the screen never changes -- and this measurement
			// first reported a guide that had subscribed to nothing because
			// of it.
			runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })
			if err := box.CSI.Key(tvGuideKey, 0); err != nil {
				t.Fatal(err)
			}
			slot, ok := guideSubscription(t, box, transmitter, 60_000_000)
			if !ok {
				t.Fatal("the guide was opened and subscribed to nothing, so either the tables have " +
					"moved or the press did not land")
			}
			t.Logf("%s", slot)
			if want := multiplex.MJDOf(day); int(slot.DayKey) != want {
				t.Errorf("the guide is listening for day %d, want the %d the broadcast claims",
					slot.DayKey, want)
			}
			if want := multiplex.QuarterOf(hour * 3600); int(slot.TableIDLow) != want {
				t.Errorf("at %02d:00 the guide is listening for block %d, want %d -- the block it is "+
					"in. If this is off by one, look at the box's own clock before changing the table",
					hour, slot.TableIDLow, want)
			}
		})
	}
}

// The regression that the whole task is: a programme on air OUTSIDE the
// 18:00-24:00 block reaching the screen.
//
// It is the encoder screen test's instrument turned on a different question --
// broadcast a title, press tv guide, hash the frame, and require a different
// title to draw a different frame. No character recognition, and no way for
// the check to pass on a box that drew FURTHER SCHEDULE INFORMATION IS NOT
// AVAILABLE, which is what this exact run did before the day was cut into
// blocks: 67 of 67 programmes in the store and an empty banner.
func TestTheOnAirTitleReachesTheScreenOutsideTheEveningBlock(t *testing.T) {
	dict := demoDictionary(t)
	// Midday: block 2, and the block the demo's pinned 19:00 never exercised.
	day := time.Date(1998, 12, 24, 12, 0, 0, 0, time.UTC)
	draw := func(t *testing.T, title string) uint32 {
		t.Helper()
		dir := t.TempDir()
		body := `{"bouquet":"Sky Digital","services":[{"name":"Sky One","channel":101,` +
			`"serviceId":100,"listingsId":101,"programmes":[` +
			`{"start":"12:00","minutes":60,"title":"` + title + `"},` +
			`{"start":"19:00","minutes":60,"title":"Walker Texas Ranger"}]}]}`
		if err := os.WriteFile(filepath.Join(dir, "default.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		guide, err := multiplex.LoadGuide(dir)
		if err != nil {
			t.Fatal(err)
		}
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
		if err != nil {
			t.Fatal(err)
		}
		registered := 0
		if at := runUntil(t, box, transmitter, 60_000_000,
			registeringProgrammes(box, 2, &registered)); at < 0 {
			t.Fatalf("%q was never taken by the box, so nothing was drawn from it", title)
		}
		// A settle before the press, without which the key does nothing at
		// all. Twelve million rather than the thirty the encoder screen test
		// uses: this package is the slowest in the repo under the race
		// detector, where every one of these instructions is paid for about
		// twelve times over, and the press is proven to land by the screen
		// this function goes on to hash.
		runUntil(t, box, transmitter, 12_000_000, func(int) bool { return false })
		before := screenNow(t, box)
		if err := box.CSI.Key(tvGuideKey, 0); err != nil {
			t.Fatal(err)
		}
		hash := drawnScreen(t, box, transmitter, 60_000_000, before)
		t.Logf("%-14q at midday drew %08X", title, hash)
		return hash
	}
	if draw(t, "Dream Team") == draw(t, "Blue Peter") {
		t.Error("two different midday titles drew the same screen, so nothing of the schedule is " +
			"on it -- the box has the programmes and is listening to a block they were not sent in")
	}
}

// guideSubscription presses on until the guide has registered a subscription
// and returns it.
//
// It stops at the event rather than running a fixed budget, and it tolerates
// the instrument's harness failure WHILE POLLING only: before the guide is
// opened there may be no table to read, which is the state being waited out.
// The read that is reported is taken afterwards and is not tolerated.
func guideSubscription(t *testing.T, box *board.Runtime, transmitter *multiplex.Multiplex,
	budget int) (multiplex.GuideSlot, bool) {
	t.Helper()
	const sample = 100_000
	runUntil(t, box, transmitter, budget, func(i int) bool {
		if i%sample != 0 {
			return false
		}
		slot, ok, err := multiplex.GuideSubscription(box.RAM)
		// A SLOT THAT EXISTS IS NOT A SLOT THAT HAS BEEN FILLED IN. This used to stop at
		// `slot.At != 0` -- the structure being somewhere rather than nowhere -- and would
		// therefore hand back a slot the guide had allocated and not yet programmed, day zero and
		// block zero, as though that were the subscription. It only ever showed up when something
		// shifted the timing: widening the card's acknowledgement policy moved the press a little
		// later relative to the guide's own work and the 12:00 case started reporting "listening
		// for day 0". The day is what the caller asserts on, so waiting for the day is the
		// condition that matches the subject.
		return err == nil && ok && slot.At != 0 && slot.DayKey != 0
	})
	slot, ok, err := multiplex.GuideSubscription(box.RAM)
	if err != nil {
		t.Fatalf("the guide's notification tables could not be read: %v", err)
	}
	return slot, ok
}
