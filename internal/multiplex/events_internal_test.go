package multiplex

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/broadcast"
)

// WHICH PROGRAMME IS "PRESENT" AND WHICH IS "FOLLOWING" IS THE ONLY JUDGEMENT IN THE EIT.
//
// Everything else the transmitter does with an event is transcription -- an MJD, a BCD duration, a
// descriptor -- and broadcast's own tests hold the wire format. This is the part that decides what
// the box is TOLD is on, and it is in the package because the choice is unexported and testing it
// through a running box would take four minutes to answer a question about two integers.
//
// The trap it exists to hold is a gap in the schedule. Tying "following" to having found a
// "present" is the obvious implementation and it is wrong: a channel that is off air between
// programmes then reports nothing coming next, which is the same thing it reports at the end of
// the day, and the two are different.
func TestWhichProgrammeIsOnAndWhichIsNext(t *testing.T) {
	t.Parallel()
	service := &ListedService{
		Name:      "Sky One",
		ServiceID: 100,
		Programmes: []ListedProgramme{
			{Start: "18:00", Minutes: 60, Title: "The Simpsons"},
			{Start: "19:00", Minutes: 60, Title: "Dream Team"},
			// A GAP: nothing runs between 20:00 and 21:00.
			{Start: "21:00", Minutes: 60, Title: "Walker Texas Ranger"},
		},
	}
	at := func(clock string) time.Time {
		t.Helper()
		when, err := time.Parse("2006-01-02 15:04", "1998-12-24 "+clock)
		if err != nil {
			t.Fatal(err)
		}
		return when
	}
	name := func(event *broadcast.Event) string {
		if event == nil {
			return ""
		}
		return event.Name
	}

	m := &Multiplex{}
	for _, c := range []struct {
		clock, present, following string
	}{
		{"19:30", "Dream Team", "Walker Texas Ranger"},
		{"19:00", "Dream Team", "Walker Texas Ranger"}, // the instant it starts
		{"20:00", "", "Walker Texas Ranger"},           // the gap: nothing on, something next
		{"17:00", "", "The Simpsons"},                  // before the day's first programme
		{"22:30", "", ""},                              // after the last one ends
		{"18:59", "The Simpsons", "Dream Team"},        // the instant before a handover
		{"21:59", "Walker Texas Ranger", ""},           // the last programme has nothing after it
	} {
		present, following, err := m.onAirAndNext(service, at(c.clock))
		if err != nil {
			t.Fatal(err)
		}
		if got := name(present); got != c.present {
			t.Errorf("at %s the present event is %q, want %q", c.clock, got, c.present)
		}
		if got := name(following); got != c.following {
			t.Errorf("at %s the following event is %q, want %q", c.clock, got, c.following)
		}
		if present != nil && present.Running != broadcast.RunningRunning {
			t.Errorf("at %s the present event's running status is %d, want %d -- a box told the "+
				"programme on air is not running has been told two different things",
				c.clock, present.Running, broadcast.RunningRunning)
		}
		if following != nil && following.Running == broadcast.RunningRunning {
			t.Errorf("at %s the following event claims to be running, which puts two programmes "+
				"on air at once", c.clock)
		}
	}
}

// THE START TIME IS THE BOX'S OWN DAY, and the day is taken from the clock rather than from the
// zero time. Truncating to 24 hours is midnight only while the in-world clock happens to be UTC,
// which is a coincidence this project should not build a schedule on.
func TestAnEventStartsOnTheDayTheClockIsIn(t *testing.T) {
	t.Parallel()
	service := &ListedService{
		ServiceID:  100,
		Programmes: []ListedProgramme{{Start: "19:00", Minutes: 90, Title: "Dream Team"}},
	}
	london := time.FixedZone("BST", 3600)
	when := time.Date(1999, 7, 4, 19, 30, 0, 0, london)
	m := &Multiplex{}
	present, _, err := m.onAirAndNext(service, when)
	if err != nil {
		t.Fatal(err)
	}
	if present == nil {
		t.Fatal("nothing was on air at 19:30 during a programme that runs 19:00 to 20:30")
	}
	want := time.Date(1999, 7, 4, 19, 0, 0, 0, london)
	if !present.Start.Equal(want) {
		t.Errorf("the event starts at %s, want %s", present.Start, want)
	}
	if present.Duration != 90*time.Minute {
		t.Errorf("the event runs %s, want 1h30m", present.Duration)
	}
}

func TestProgrammeMediaIsSelectedByServiceAndInWorldTime(t *testing.T) {
	t.Parallel()
	listings := &Listings{Services: []ListedService{
		{Name: "Sky One", ServiceID: 100, Programmes: []ListedProgramme{
			{Start: "18:00", Minutes: 60, Title: "Friends"},
			{Start: "19:00", Minutes: 60, Title: "Dream Team",
				Media: &ProgrammeMedia{Kind: MediaKindTestPattern}},
		}},
		{Name: "Sky News", ServiceID: 101, Programmes: []ListedProgramme{
			{Start: "19:00", Minutes: 60, Title: "Sky News Tonight"},
		}},
	}}
	at := func(clock string) time.Time {
		when, err := time.Parse("2006-01-02 15:04", "1998-12-24 "+clock)
		if err != nil {
			t.Fatal(err)
		}
		return when
	}

	service, programme, kind, ok := listings.MediaFor(100, at("19:30"))
	if !ok || service != "Sky One" || programme != "Dream Team" || kind != MediaKindTestPattern {
		t.Fatalf("configured selection = %q %q %q %t", service, programme, kind, ok)
	}
	for _, c := range []struct {
		service uint16
		at      string
	}{
		{100, "18:30"}, // a real programme with no configured source
		{101, "19:30"}, // another selected service with no configured source
		{999, "19:30"}, // no such service
		{100, "20:00"}, // the configured event has ended
	} {
		if service, programme, kind, ok := listings.MediaFor(c.service, at(c.at)); ok {
			t.Errorf("service %d at %s unexpectedly selected %q %q %q", c.service, c.at,
				service, programme, kind)
		}
	}
}
