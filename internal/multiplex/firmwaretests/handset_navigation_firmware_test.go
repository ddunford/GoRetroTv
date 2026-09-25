package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// These are the two distinct buttons on the period Sky Digital handset. The browser used to label
// 0x80 as TV Guide and 0xCC as Standby; from a tuned channel that made its "TV Guide" button perform
// Sky's return-to-viewing action and show the search-and-scan banner. Exercise the real guest from
// that state so this mapping cannot drift back to labels inferred from an idle-screen hash.
func TestSkyAndTVGuideAreDistinctFromAViewingChannel(t *testing.T) {
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

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := azPressFunc(t, box, pump)
	openAllChannels(t, press, ".artifacts/handset-navigation-grid.png", true)
	viewing := press(keySelect, "select to view", pressBudget)
	if viewing == 0 || viewing == tvGuideMenuScreen {
		t.Fatalf("harness: select did not leave the guide for viewing; drew %08X", viewing)
	}

	sky := press(0x80, "Sky", pressBudget)
	if sky == 0 || sky == tvGuideMenuScreen {
		t.Fatalf("Sky 0x80 drew %08X; it must return to viewing, not open TV Guide", sky)
	}
	if err := dumpScreen(t, box, "handset-navigation-sky.png"); err != nil {
		t.Fatal(err)
	}

	tvGuide := press(0xCC, "tv guide", pressBudget)
	if tvGuide != tvGuideMenuScreen {
		t.Fatalf("tv guide 0xCC drew %08X, want the full TV GUIDE menu %08X",
			tvGuide, uint32(tvGuideMenuScreen))
	}
	if err := dumpScreen(t, box, "handset-navigation-tv-guide.png"); err != nil {
		t.Fatal(err)
	}
}
