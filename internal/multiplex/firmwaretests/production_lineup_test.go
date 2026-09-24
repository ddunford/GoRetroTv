package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

func TestProductionLineupCarriesTheNamedSkyDigitalLaunchChannels(t *testing.T) {
	listings := productionGuide(t).On(time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC))
	if got, want := len(listings.Services), 68; got != want {
		t.Fatalf("production lineup has %d named TV services, want %d from the launch table", got, want)
	}

	want := map[uint16]string{
		101: "BBC One", 106: "Sky One", 145: "Sky Travel", 301: "Sky Premier",
		401: "Sky Sports 1", 501: "Sky News", 604: "Nickelodeon", 688: "TV Travel Shop",
	}
	for _, service := range listings.Services {
		if name, ok := want[service.Channel]; ok {
			if service.Name != name {
				t.Errorf("channel %d is %q, want %q", service.Channel, service.Name, name)
			}
			delete(want, service.Channel)
		}
	}
	for channel, name := range want {
		t.Errorf("production lineup is missing channel %d %s", channel, name)
	}
}

func TestProductionLineupReachesTheFirmwareAndDrawsTheGrid(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	guide := productionGuide(t)
	box := restoredBox(t)
	schedule := demoSchedule()
	schedule.IndexPeriod = 2_000_000
	schedule.TitleSettle = 4_000_000
	transmitter, err := multiplex.New(box, guide, demoDictionary(t), multiplex.FixedClock{At: day},
		schedule)
	if err != nil {
		t.Fatal(err)
	}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	bestCoverage := 0
	if at := runUntil(t, box, transmitter, 120_000_000, func(i int) bool {
		if i%1_000 != 0 {
			return false
		}
		sub, readErr := multiplex.Read(box.Demux)
		if readErr != nil {
			return false
		}
		for _, request := range sub.Titles {
			if request.MJD() != multiplex.MJDOf(day) {
				continue
			}
			covered := 0
			for _, service := range guide.On(day).Services {
				if request.Wants(service.ListingsID) {
					covered++
				}
			}
			bestCoverage = max(bestCoverage, covered)
			if covered == len(guide.On(day).Services) {
				t.Logf("title request extension %04X/%04X covers all %d production channels",
					request.Extension, request.ExtensionMask, covered)
				return true
			}
		}
		return false
	}); at < 0 {
		t.Fatalf("the box's title filter covered at most %d of %d production channels",
			bestCoverage, len(guide.On(day).Services))
	}
	// One complete serial pass includes all four six-hour tables; the guest's hardware rejects the
	// blocks it did not request, just as it would on the broadcast multiplex.
	if at := runUntil(t, box, transmitter, 700_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("the production broadcast registered %d of the %d programmes in the active block",
			registered, want)
	}

	var limit int32
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch && a.Virtual&^1 == channelLoopHead {
			if got := int32(box.Machine.Core.State().GPR[3]); got > limit { // #nosec G115 -- register width
				limit = got
			}
		}
	}}
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		return pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
	}
	// The production lineup changes the grid by definition, so its first unknown hash is the
	// candidate to photograph and inspect. The shared six-channel fixture keeps the old strict pins.
	grid := openAllChannels(t, press, ".artifacts/production-lineup-grid.png", true)
	if err := runUntilHooked(t, box, transmitter, hooks, 60_000_000, func(int) bool {
		return limit == int32(len(guide.On(day).Services))
	}); err < 0 {
		t.Fatalf("the guest's channel loop reached %d services, want %d", limit,
			len(guide.On(day).Services))
	}
	if err := dumpScreen(t, box, "production-lineup-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("production lineup: %d channels, grid %08X, screenshot .artifacts/%s",
		limit, grid, "production-lineup-grid.png")
}
