package firmwaretests_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DOES THE BOX TAKE A LINE-UP THAT DOES NOT FIT IN ONE SECTION?
//
// A DVB section stops at 1021 bytes, which is twenty-five services once each carries a name and a
// guide row, so a real Sky line-up of around a hundred and forty channels has always had to travel
// as a multi-section TABLE -- section_number 0 through last_section_number, reassembled by the
// receiver. This port built one section per table until now, and the question that decides whether
// the split was worth building is not whether the bytes are well formed. It is whether the GUEST
// reassembles them.
//
// IT IS ASKED OF THE BOX'S OWN COUNTER, not of the screen. 0x800A4B60 is the bounds-checked loop
// head over the channel database -- `slt s1,v1`, index against limit -- and v1 there is how many
// channels the box believes it has. A grid can be short for a dozen reasons; the limit is the box
// stating the number.
//
// THE CONTROL IS THE SAME CODE WITH A LINE-UP THAT FITS ONE SECTION. If a six-channel run and a
// forty-channel run report the same limit, the extra sections reached nothing and the split is
// bytes nobody reads. If the limit tracks the line-up, the box is reassembling the table.
//
// IT ASSERTS ITS OWN SUBJECT: the small run must produce a non-zero limit. A box that counts no
// channels at all has not acquired a line-up, and the large run would then be measuring the same
// silence twice.
//
// IT ONLY READS. The schedules are written to the test's own temporary directory; nothing in
// listings/ is touched and nothing is poked into the guest.
func TestABigLineupReachesTheBoxAcrossSeveralSections(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	run := func(t *testing.T, channels int) int32 {
		t.Helper()
		guide := generatedGuide(t, channels)
		dict := demoDictionary(t)
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			demoSchedule())
		if err != nil {
			t.Fatal(err)
		}
		// THE PROGRAMMES ARE NOT WHAT THIS MEASURES, so a short registration is logged rather
		// than fatal. The channel count comes from the line-up tables -- the SDT and the BAT --
		// and the titles are a different wave on a different PID; a forty-channel schedule simply
		// takes far longer to deliver every programme than a six-channel one, and waiting for all
		// of them would make this a test of the title carousel's cadence.
		want := programmesInTheBlock(t, guide, day)
		registered := 0
		// Capped, because delivering every programme of a big line-up is the title carousel's
		// cadence rather than this probe's subject, and the channel count does not wait on it.
		budget := min(200_000_000+channels*10_000_000, 500_000_000)
		runUntil(t, box, transmitter, budget, registeringProgrammes(box, want, &registered))
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })
		t.Logf("%d channels in the schedule, %d of %d programmes registered in %dM instructions",
			channels, registered, want, budget/1_000_000)

		// THE COUNTER ONLY RUNS WHEN A SCREEN ASKS FOR CHANNELS, which is why this drives the
		// route rather than watching an idle box: a first pass that simply ran for two hundred
		// million instructions saw the loop head zero times and reported a limit of nought.
		var limit int32
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if !a.Fetch || a.Virtual&^1 != channelLoopHead {
				return
			}
			// v1 is the limit the running index is tested against: the box's own count.
			if got := int32(box.Machine.Core.State().GPR[3]); got > limit { // #nosec G115 -- register width
				limit = got
			}
		}}
		pump := func() error { return transmitter.Pump(box.Machine.Retired) }
		menu := uint32(0)
		for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
			menu = pressAndLetItFinishHooked(t, box, pump, hooks, keyBoxOffice, pressBudget)
		}
		if menu != boxOfficeMenu {
			t.Fatalf("harness: box office drew %08X with a %d-channel line-up", menu, channels)
		}
		tab := uint32(0)
		for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
			tab = pressAndLetItFinishHooked(t, box, pump, hooks, keyLeft, pressBudget)
		}
		if tab != tvGuideMenuScreen {
			t.Fatalf("harness: never reached the TV GUIDE menu with a %d-channel line-up; drew %08X",
				channels, tab)
		}
		grid := uint32(0)
		for attempt := 1; attempt <= attempts && (grid == 0 || grid == tab); attempt++ {
			grid = pressAndLetItFinishHooked(t, box, pump, hooks, keySelect, pressBudget)
		}
		for i := 0; i < 60_000_000; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		if err := dumpScreen(t, box, fmt.Sprintf("biglineup-%d.png", channels)); err != nil {
			t.Fatal(err)
		}
		t.Logf("   grid %08X, the box's channel loop ran against a limit of %d   .artifacts/biglineup-%d.png",
			screenNow(t, box), limit, channels)
		return limit
	}

	// SIX IS THE CONTROL AND ONE HUNDRED AND FORTY IS THE QUESTION. Forty was the first number
	// tried and the box counted forty, which says nothing about a ceiling -- it was the number put
	// in. A hundred and forty is what Sky claimed at the 1998 digital launch, so it is the size
	// this has to carry, and it takes six SDT sections and two BAT sections to state.
	counted := map[int]int32{}
	for _, channels := range []int{6, 40, 140} {
		counted[channels] = run(t, channels)
		if channels == 6 && counted[6] <= 0 {
			t.Fatalf("harness: with a six-channel line-up the box counted %d channels, so it has "+
				"acquired nothing and a bigger line-up would measure the same silence", counted[6])
		}
	}

	t.Logf("=== WHAT THE BOX COUNTED ===")
	for _, channels := range []int{6, 40, 140} {
		verdict := "took them all"
		if got := counted[channels]; int(got) != channels {
			verdict = fmt.Sprintf("SHORT BY %d", channels-int(got))
		}
		t.Logf("    %3d channels -> limit %3d   %s", channels, counted[channels], verdict)
	}
	for _, channels := range []int{40, 140} {
		if int(counted[channels]) != channels {
			t.Errorf("a %d-channel line-up left the box counting %d. The bytes are well formed -- "+
				"TestALargeLineupSpansSeveralCorrectlyNumberedSections proves every service appears "+
				"exactly once across the sections -- so a short count is the guest's own ceiling or "+
				"a section it never saw, and either is a finding worth having before a real line-up "+
				"is written", channels, counted[channels])
		}
	}
}

// generatedGuide writes an n-channel schedule to the test's own directory and loads it.
//
// THE NUMBERS FOLLOW THE REAL EPG'S SECTIONS, because the grid separates genre groups and a line-up
// scattered across genres would tell a different story from a real one. Sky's 1998 EPG put
// entertainment in the 100s, lifestyle in the 200s, movies at 300-339, music at 340-399, sport in
// the 400s, news and documentaries at 500-599 and kids in the 600s -- which is the same seven
// groups this project measured the guide filtering on.
func generatedGuide(t *testing.T, n int) *multiplex.Guide {
	t.Helper()
	type programme struct {
		Start   string `json:"start"`
		Minutes int    `json:"minutes"`
		Title   string `json:"title"`
	}
	type service struct {
		Name       string      `json:"name"`
		Channel    int         `json:"channel"`
		ServiceID  int         `json:"serviceId"`
		ListingsID int         `json:"listingsId"`
		Genre      int         `json:"genre"`
		Programmes []programme `json:"programmes"`
	}
	// base is the first channel number of each genre's range, in the genre order this project
	// measured: 1 specialist, 2 kids, 3 entertainment, 4 music, 5 news, 6 movies, 7 sports.
	base := map[int]int{1: 201, 2: 601, 3: 101, 4: 341, 5: 501, 6: 301, 7: 401}
	used := map[int]int{}
	var services []service
	for i := 0; i < n; i++ {
		genre := i%7 + 1
		channel := base[genre] + used[genre]
		used[genre]++
		var day []programme
		for hour := 0; hour < 24; hour += 2 {
			day = append(day, programme{
				Start:   fmt.Sprintf("%02d:00", hour),
				Minutes: 120,
				Title:   fmt.Sprintf("Programme %02d on %d", hour, channel),
			})
		}
		services = append(services, service{
			Name: fmt.Sprintf("Channel %03d", channel), Channel: channel,
			ServiceID: 2000 + i, ListingsID: 4000 + i, Genre: genre, Programmes: day,
		})
	}
	blob, err := json.Marshal(map[string]any{"bouquet": "Sky", "services": services})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "default.json"), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	guide, err := multiplex.LoadGuide(dir)
	if err != nil {
		t.Fatalf("the generated %d-channel schedule did not load: %v", n, err)
	}
	return guide
}
