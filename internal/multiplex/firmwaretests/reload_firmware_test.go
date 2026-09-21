package firmwaretests_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
	"github.com/ddunford/goretrotv/internal/multiplex/multiplextest"
)

// TC-6.8 against the real firmware: an edit made while the box is running
// reaches the box, and a malformed one does not.
//
// The loader tests prove the file handling. This proves the only thing that
// matters to somebody editing a schedule -- that the box takes the new
// programmes -- and it is a different claim, because a line-up rebroadcast
// under a version number the box has already parsed is DROPPED. Without the
// version bump every test above would still pass and nothing would ever
// change on screen.
func TestAnEditReachesARunningBox(t *testing.T) {
	dir := t.TempDir()
	multiplextest.Schedule(t, dir, "default.json", 3)
	guide, err := multiplex.LoadGuide(dir)
	if err != nil {
		t.Fatal(err)
	}
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	// A faster carousel than the demo's. An edit is only picked up on a
	// line-up wave, so the demo's sixty-million-instruction period would make
	// this test spend most of its life waiting for a timer it is not testing.
	// The two SETTLES are left alone, because those are what the box needs.
	air := demoSchedule()
	air.LineupPeriod = 16_000_000
	air.TitlePeriod = 16_000_000
	transmitter, err := multiplex.New(box, guide, demoDictionary(t),
		multiplex.FixedClock{At: day}, air)
	if err != nil {
		t.Fatal(err)
	}

	// The first schedule reaches the box.
	registered := 0
	if at := runUntil(t, box, transmitter, 40_000_000,
		registeringProgrammes(box, 3, &registered)); at < 0 {
		t.Fatalf("the box took %d of the first schedule's 3 programmes", registered)
	}

	// Now edit it, the way somebody would with the demo running.
	multiplextest.Schedule(t, dir, "default.json", 6)
	edited := 0
	at := runUntil(t, box, transmitter, 60_000_000, registeringProgrammes(box, 6, &edited))
	t.Logf("after the edit the box took %d programmes by instruction %d", edited, at)
	if at < 0 {
		t.Fatalf("the box took %d of the edited schedule's 6 programmes in 60M instructions; "+
			"an edit that never reaches the air is the whole point of this case", edited)
	}

	// A malformed edit must not take the line-up off the air. The box has
	// already been told the good schedule, so what is checked is that the
	// transmitter keeps broadcasting rather than erroring out of the loop --
	// a Pump that returned an error here would halt the machine.
	if err := os.WriteFile(filepath.Join(dir, "default.json"), []byte(`{"bouquet":`), 0o600); err != nil {
		t.Fatal(err)
	}
	var refused error
	transmitter.OnReload(func(changed bool, problem error) {
		if problem != nil {
			refused = problem
		}
	})
	if at := runUntil(t, box, transmitter, 40_000_000, func(int) bool { return refused != nil }); at < 0 {
		t.Fatal("the transmitter never noticed the broken schedule, so it cannot have reported it")
	}
	t.Logf("the broken edit was refused and named: %v", refused)
	if counts := transmitter.Counts(); counts.Lineup == 0 || counts.Titles == 0 {
		t.Errorf("the broadcast stopped after the broken edit: %+v", counts)
	}
}
