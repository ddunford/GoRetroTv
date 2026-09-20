package broadcast_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
)

// A schedule with three distinguishable periods and two distinguishable
// settles, so any wave seen out of place names the rung it came from.
func testSchedule() broadcast.Schedule {
	return broadcast.Schedule{
		ClockPeriod:  100,
		LineupPeriod: 300,
		TitlePeriod:  700,
		ClockSettle:  1_000,
		LineupSettle: 5_000,
	}
}

// Each rung emits one section carrying its own name, so a transcript reads as
// the order things went on air.
func namedSource(t *testing.T) (broadcast.Source, *[]string) {
	t.Helper()
	var log []string
	rung := func(name string, pid uint16) func(uint64) ([]broadcast.Emission, error) {
		return func(now uint64) ([]broadcast.Emission, error) {
			log = append(log, fmt.Sprintf("%s@%d", name, now))
			return []broadcast.Emission{{PID: pid, Section: []byte(name)}}, nil
		}
	}
	return broadcast.Source{
		Clock:  rung("clock", 0x14),
		Lineup: rung("lineup", 0x11),
		Titles: rung("titles", 0x37),
	}, &log
}

// TC-6.5, the scheduling half. The box programs its day-addressed listings
// request ONCE, so a line-up that arrives before the clock has taken effect
// leaves it asking for the epoch for ever. Both halves of the hold are
// checked: that the line-up waits for a clock wave at all, and that it waits
// out the settle afterwards. Dropping either produces a box that acquires
// against the wrong day and reports no error at all.
func TestTheLineupIsHeldUntilTheClockHasGoneOutAndSettled(t *testing.T) {
	source, log := namedSource(t)
	carousel, err := broadcast.NewCarousel(testSchedule(), source)
	if err != nil {
		t.Fatal(err)
	}

	// Up to the settle boundary, exclusive: clock waves only.
	for now := uint64(0); now < 1_000; now++ {
		wave, err := carousel.Wave(now)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range wave {
			if string(e.Section) != "clock" {
				t.Fatalf("%q went out at instruction %d, before the clock had settled at 1000", e.Section, now)
			}
		}
	}
	if len(*log) != 10 {
		t.Errorf("clock waves in the first 1000 instructions = %d, want 10 at a period of 100", len(*log))
	}

	// And at the boundary itself it is released.
	wave, err := carousel.Wave(1_000)
	if err != nil {
		t.Fatal(err)
	}
	sections := make([]string, 0, len(wave))
	for _, e := range wave {
		sections = append(sections, string(e.Section))
	}
	if len(sections) != 2 || sections[0] != "clock" || sections[1] != "lineup" {
		t.Fatalf("the wave at the settle boundary was %v, want the clock then the line-up", sections)
	}
}

// The second rung, same rule: until acquisition has run the box has armed no
// title PID, and the hardware collects a section addressed to a filter that
// does not exist and drops it silently.
func TestTitlesAreHeldUntilTheLineupHasGoneOutAndSettled(t *testing.T) {
	source, log := namedSource(t)
	carousel, err := broadcast.NewCarousel(testSchedule(), source)
	if err != nil {
		t.Fatal(err)
	}
	// The first line-up goes out at 1000, so titles are due from 6000.
	for now := uint64(0); now < 6_000; now++ {
		wave, err := carousel.Wave(now)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range wave {
			if string(e.Section) == "titles" {
				t.Fatalf("titles went out at instruction %d, before the line-up had settled at 6000", now)
			}
		}
	}
	if got := countPrefix(*log, "lineup@"); got == 0 {
		t.Fatal("no line-up wave went out at all, so this proves nothing about what the titles waited for")
	}

	wave, err := carousel.Wave(6_000)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(wave, "titles") {
		t.Error("the titles were still held at instruction 6000, one settle after the first line-up")
	}
}

// The hold starts when a rung actually transmits, not when it is first asked.
// A title source with no armed PID yet legitimately has nothing to say, and if
// an empty answer started the clock on the rung above it, the whole ladder
// would release against a box that had received nothing.
func TestAnEmptyWaveDoesNotStartTheRungAboveIt(t *testing.T) {
	var clockCalls int
	source := broadcast.Source{
		Clock: func(now uint64) ([]broadcast.Emission, error) {
			clockCalls++
			if clockCalls <= 5 {
				return nil, nil // nothing to say yet
			}
			return []broadcast.Emission{{PID: 0x14, Section: []byte("clock")}}, nil
		},
		Lineup: func(now uint64) ([]broadcast.Emission, error) {
			return []broadcast.Emission{{PID: 0x11, Section: []byte("lineup")}}, nil
		},
		Titles: func(now uint64) ([]broadcast.Emission, error) { return nil, nil },
	}
	carousel, err := broadcast.NewCarousel(testSchedule(), source)
	if err != nil {
		t.Fatal(err)
	}
	// The fifth clock wave is at instruction 400 and is still empty; the
	// sixth, at 500, is the first that transmits. So the line-up may not
	// appear before 500+1000.
	for now := uint64(0); now < 1_500; now++ {
		wave, err := carousel.Wave(now)
		if err != nil {
			t.Fatal(err)
		}
		if contains(wave, "lineup") {
			t.Fatalf("the line-up went out at %d; the first clock section only went out at 500, so the hold expires at 1500", now)
		}
	}
	wave, err := carousel.Wave(1_500)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(wave, "lineup") {
		t.Error("the line-up was still held at 1500, one settle after the first clock section actually went out")
	}
}

// NextDue exists so a driver can schedule one clock event instead of asking
// every instruction. It is only worth having if it never sleeps past a wave,
// so it is checked against the ground truth of asking every instruction --
// the same source, the same schedule, the same transcript.
//
// TWO schedules, because one of them proves nothing about most of the
// function. Under a schedule whose clock period is the shortest, the clock is
// always the soonest thing due and the line-up and title arms can never lower
// the answer: deleting them outright leaves this test green. The second
// schedule inverts that -- the slowest clock, the fastest titles, and settles
// that deliberately do not land on a wave boundary -- so every arm decides the
// result somewhere in the run.
func TestDrivingFromNextDueEmitsExactlyWhatPollingEveryInstructionDoes(t *testing.T) {
	const horizon = 50_000

	for _, tc := range []struct {
		name     string
		schedule broadcast.Schedule
	}{
		{"a fast clock, where the clock is always the soonest thing due", testSchedule()},
		{"a slow clock, where the line-up and the titles decide when to wake", broadcast.Schedule{
			ClockPeriod:  4_000,
			LineupPeriod: 900,
			TitlePeriod:  130,
			ClockSettle:  1_111, // deliberately not a multiple of any period
			LineupSettle: 2_345,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dense, denseLog := namedSource(t)
			denseCarousel, err := broadcast.NewCarousel(tc.schedule, dense)
			if err != nil {
				t.Fatal(err)
			}
			for now := uint64(0); now <= horizon; now++ {
				if _, err := denseCarousel.Wave(now); err != nil {
					t.Fatal(err)
				}
			}

			sparse, sparseLog := namedSource(t)
			sparseCarousel, err := broadcast.NewCarousel(tc.schedule, sparse)
			if err != nil {
				t.Fatal(err)
			}
			for now := uint64(0); now <= horizon; {
				if _, err := sparseCarousel.Wave(now); err != nil {
					t.Fatal(err)
				}
				due := sparseCarousel.NextDue()
				if due <= now {
					t.Fatalf("NextDue returned %d at instruction %d, which would spin", due, now)
				}
				now = due
			}

			if len(*denseLog) == 0 {
				t.Fatal("the dense run transmitted nothing, so the comparison is vacuous")
			}
			if len(*sparseLog) != len(*denseLog) {
				t.Fatalf("driving from NextDue produced %d waves, polling produced %d", len(*sparseLog), len(*denseLog))
			}
			for i := range *denseLog {
				if (*sparseLog)[i] != (*denseLog)[i] {
					t.Fatalf("wave %d differs: NextDue gave %q, polling gave %q", i, (*sparseLog)[i], (*denseLog)[i])
				}
			}
		})
	}
}

// A period of zero is due again the instant it fires, which presents as a hang
// inside one Advance rather than as the mistake it is. platform/clock rejects
// it for the same reason and so does this.
func TestTheCarouselRefusesAScheduleOrSourceItCannotHonour(t *testing.T) {
	good := testSchedule()
	source, _ := namedSource(t)

	for _, tc := range []struct {
		name     string
		schedule broadcast.Schedule
		source   broadcast.Source
	}{
		{"a zero clock period", broadcast.Schedule{LineupPeriod: 1, TitlePeriod: 1}, source},
		{"a zero line-up period", broadcast.Schedule{ClockPeriod: 1, TitlePeriod: 1}, source},
		{"a zero title period", broadcast.Schedule{ClockPeriod: 1, LineupPeriod: 1}, source},
		{"no clock source", good, broadcast.Source{Lineup: source.Lineup, Titles: source.Titles}},
		{"no line-up source", good, broadcast.Source{Clock: source.Clock, Titles: source.Titles}},
		{"no title source", good, broadcast.Source{Clock: source.Clock, Lineup: source.Lineup}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := broadcast.NewCarousel(tc.schedule, tc.source); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// A source that fails must stop the wave rather than transmit a partial one:
// half a carousel is a box that acquires from tables that do not agree.
func TestASourceErrorStopsTheWave(t *testing.T) {
	boom := errors.New("no schedule loaded")
	source, _ := namedSource(t)
	source.Lineup = func(now uint64) ([]broadcast.Emission, error) { return nil, boom }
	carousel, err := broadcast.NewCarousel(testSchedule(), source)
	if err != nil {
		t.Fatal(err)
	}
	for now := uint64(0); now < 1_000; now++ {
		if _, err := carousel.Wave(now); err != nil {
			t.Fatalf("the clock rung failed at %d, before the line-up was ever asked: %v", now, err)
		}
	}
	if _, err := carousel.Wave(1_000); !errors.Is(err, boom) {
		t.Errorf("Wave returned %v, want the source's own error", err)
	}
}

func contains(wave []broadcast.Emission, section string) bool {
	for _, e := range wave {
		if string(e.Section) == section {
			return true
		}
	}
	return false
}

func countPrefix(log []string, prefix string) int {
	n := 0
	for _, line := range log {
		if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}
