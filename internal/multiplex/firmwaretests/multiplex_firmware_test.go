package firmwaretests_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/firmware"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// The per-event register, once per record. Counting it answers "how many
// programmes did the box take off the air" without reading our own bytes back
// at ourselves.
const pcPerEventRegister = 0x800c587c

// A demo-shaped schedule: the clock repeats from the start, the line-up waits
// for it to land, and the titles wait for the line-up. The periods are chosen
// for how quickly someone watching sees the guide fill and nothing downstream
// depends on them; the two SETTLES are the part that matters.
func demoSchedule() broadcast.Schedule {
	return broadcast.Schedule{
		ClockPeriod:  20_000_000,
		LineupPeriod: 60_000_000,
		TitlePeriod:  60_000_000,
		ClockSettle:  8_000_000,
		LineupSettle: 4_000_000,
	}
}

// demoScheduleWithIndex is demoSchedule plus the A-Z index rung, for the probes that exercise it.
//
// IT IS SEPARATE ON PURPOSE. A section every two million instructions is real extra work for the
// box, and this package's probes are pinned to screen hashes taken under a particular load: adding
// the index to the shared fixture moved the TV GUIDE menu from the frame those pins name to the
// NEXT one, and a dozen routes stopped finding a screen they had always found. The shipped
// carousel (cmd/goretrotv) carries the index; the probes that do not test it keep the broadcast
// they were calibrated against, and the ones that do say so here.
func demoScheduleWithIndex() broadcast.Schedule {
	schedule := demoSchedule()
	schedule.IndexPeriod = 2_000_000
	schedule.TitleSettle = 4_000_000
	return schedule
}

// demoScheduleWithEvents is demoSchedule plus the present/following EIT rung, for the probes that
// exercise it.
//
// IT IS SEPARATE FOR THE SAME REASON THE INDEX IS. The rung is inert until the box tunes -- it
// transmits nothing while PID 0x0012 is unarmed -- but once it does, twelve sections a wave is
// real extra work, and this package's probes are pinned to screen hashes taken under a particular
// load. The shipped carousel carries what it is proved to need; the probes that do not test the
// EIT keep the broadcast they were calibrated against.
func demoScheduleWithEvents() broadcast.Schedule {
	schedule := demoSchedule()
	schedule.EventPeriod = 4_000_000
	return schedule
}

// The flash images and the snapshot, read once for the whole package.
//
// This file builds a couple of dozen boxes, and re-reading five megabytes of
// flash and verifying it against its manifest for each one is pure overhead:
// the images are immutable and board.New copies what it needs. Caching them is
// what keeps this package inside Go's per-package timeout under the race
// detector.
var (
	cachedImages   *firmware.Set
	cachedSnapshot []byte
)

func restoredBox(t *testing.T) *board.Runtime {
	t.Helper()
	// -short SKIPS EVERY BOX IN THIS PACKAGE, which is what makes a per-turn gate
	// possible. Restoring a box and running millions of guest instructions costs
	// tens of seconds under -race; the full suite is a pre-push and CI concern
	// (./ctl.sh test), and ./ctl.sh test:fast is the same suite with these
	// skipped. The gate is HERE, at the one function every firmware test in this
	// package goes through, so a test added later gets it without anyone
	// remembering to.
	if testing.Short() {
		t.Skip("skipping a real-firmware box under -short; run ./ctl.sh test for these")
	}
	// AND -race SKIPS THEM TOO, which is a decision rather than a convenience. These tests drive a
	// deliberately single-threaded emulator -- "no goroutine in the instruction loop" is one of
	// this project's architecture decisions -- so the detector is hunting data races in a loop
	// that structurally cannot have one. What it costs is not marginal: measured 2026-09-23 on one
	// real-firmware test, 1.88s plain against 15.06s under -race, a multiplier of EIGHT. At that
	// rate the firmware package alone runs for hours and `go test -race` cannot finish inside any
	// timeout worth setting, so the race pass was not slow, it was UNRUNNABLE -- and a check
	// nobody can run gates nothing.
	//
	// The races worth finding are in internal/web and the transport, which stay under the detector
	// and cost seconds. The boxes are covered by the plain pass instead; ./ctl.sh test runs both.
	if underRaceDetector {
		t.Skip("skipping a real-firmware box under -race: the emulator is single-threaded by " +
			"design, the detector costs 8x, and the plain pass covers these")
	}
	dir := filepath.Join("..", "..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		t.Skip("private firmware is not installed")
	}
	snapshot := filepath.Join("..", "..", "..", "snapshots", "post-acquisition.snapshot")
	if _, err := os.Stat(snapshot); os.IsNotExist(err) {
		t.Skip("private post-acquisition snapshot is not installed")
	}
	if cachedImages == nil {
		images, err := firmware.Load(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		blob, err := os.ReadFile(snapshot) // #nosec G304 -- fixed local private test fixture
		if err != nil {
			t.Fatal(err)
		}
		cachedImages, cachedSnapshot = images, blob
	}
	box, err := board.New(cachedImages, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Restore(bytes.NewReader(cachedSnapshot)); err != nil {
		t.Fatal(err)
	}
	return box
}

// programmesInTheBlock is how many of a day's programmes the box will take.
//
// A DAY IS BROADCAST IN FOUR SIX-HOUR BLOCKS AND THE BOX REGISTERS THE ONE IT
// IS LISTENING TO (TASK-6.13). Counting a whole day and waiting for it is how
// every firmware test in this package started failing the moment the blocks
// were introduced: the box took its block, discarded the rest, and the loop
// ran its whole budget waiting for programmes that were never going to be
// stored.
//
// It is a LOWER BOUND, not the total. The box also takes whichever block its
// own match unit named -- 0xA3 by day, so a midday box takes the afternoon
// block and the evening one -- and a test that asserted equality would be
// pinning a second, unrelated firmware behaviour by accident.
func programmesInTheBlock(t *testing.T, guide *multiplex.Guide, day time.Time) int {
	t.Helper()
	block := multiplex.QuarterOf(day.Hour()*3600 + day.Minute()*60)
	count := 0
	for _, service := range guide.On(day).Services {
		for _, programme := range service.Programmes {
			start, err := programme.StartSeconds()
			if err != nil {
				t.Fatal(err)
			}
			if multiplex.QuarterOf(start) == block {
				count++
			}
		}
	}
	if count == 0 {
		t.Fatalf("no programme in the schedule falls in the %02d:00 block, so a box that took "+
			"everything correctly would still register nothing", day.Hour())
	}
	return count
}

func demoGuide(t *testing.T) *multiplex.Guide {
	t.Helper()
	guide, err := multiplex.LoadGuide(filepath.Join("..", "..", "..", "listings"))
	if err != nil {
		t.Fatal(err)
	}
	return guide
}

func demoDictionary(t *testing.T) *broadcast.HuffmanDictionary {
	t.Helper()
	path := filepath.Join("..", "..", "..", "dictionaries", "skyuk.dict")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("the Sky EPG huffman dictionary is not installed; see dictionaries/MANIFEST.md")
	}
	dict, err := broadcast.LoadHuffmanDictionary(path)
	if err != nil {
		t.Fatal(err)
	}
	return dict
}

// The whole point of the package, end to end and against the real firmware: a
// box restored from a snapshot, a transmitter driven off the instruction
// counter, and programmes registered at the end of it.
//
// It counts the per-event register rather than inspecting our own sections,
// because everything up to that point can be right while the box takes none of
// it -- which is the failure this project has met at every rung.
func TestTheBoxTakesProgrammesOffTheModelledMultiplex(t *testing.T) {
	box := restoredBox(t)
	guide := demoGuide(t)
	dict := demoDictionary(t)

	// A day the box subscribes on. The eight-day PID rotation currently only
	// programmes a title filter on MJD mod 8 in {1,3,6}; 15 June 1998 is MJD
	// 50979, which is 3.
	day := time.Date(1998, 6, 15, 19, 30, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	programmes := programmesInTheBlock(t, guide, day)
	registered := 0
	const budget = 40_000_000
	doneAt := runUntil(t, box, transmitter, budget, registeringProgrammes(box, programmes, &registered))

	counts := transmitter.Counts()
	t.Logf("on air: %d clock waves, %d line-up waves, %d title waves, %d title waves with nowhere to go",
		counts.Clock, counts.Lineup, counts.Titles, counts.TitlesUnaddressed)
	if problem := transmitter.LastProblem(); problem != "" {
		t.Logf("last subscription problem: %s", problem)
	}

	sub, err := multiplex.Read(box.Demux)
	if err != nil {
		t.Fatalf("the box is no longer asking for anything readable: %v", err)
	}
	for _, request := range sub.Titles {
		t.Logf("the box asks for table %#02x extension %#04x MJD %d on PID %#02x",
			request.TableID, request.Extension, request.MJD(), request.PID)
	}

	if counts.Clock == 0 {
		t.Fatal("no clock wave was transmitted, so nothing downstream could have been released")
	}
	if counts.Lineup == 0 {
		t.Fatal("no line-up wave was transmitted, so the box was never offered a channel list")
	}
	if len(sub.Titles) == 0 {
		t.Fatal("the box programmed no listings filter, so no programme could have reached it")
	}
	// The day must be the one the broadcast claimed. If it is not, the line-up
	// beat the clock and the box is addressing a day nothing will be sent for
	// -- the exact failure the carousel's settle exists to prevent, and one
	// that otherwise presents as an empty guide with no error anywhere.
	wantMJD := int(day.UTC().Truncate(24*time.Hour).Sub(
		time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
	for _, request := range sub.Titles {
		if request.MJD() != wantMJD {
			t.Errorf("the box is asking for MJD %d but the broadcast claims %d", request.MJD(), wantMJD)
		}
		if want := uint16(0x30 | (request.MJD() % 8)); request.PID != want {
			t.Errorf("listings PID %#02x for MJD %d, want %#02x", request.PID, request.MJD(), want)
		}
	}
	if doneAt < 0 {
		t.Fatalf("the box registered %d of its block's %d programmes in %d instructions",
			registered, programmes, budget)
	}
	t.Logf("the box took the %d programmes of its block by instruction %d of a %d budget",
		programmes, doneAt, budget)
}
