package multiplex_test

import (
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

func restoredBox(t *testing.T) *board.Runtime {
	t.Helper()
	dir := filepath.Join("..", "..", "firmware")
	if _, err := os.Stat(filepath.Join(dir, firmware.FileU202)); os.IsNotExist(err) {
		t.Skip("private firmware is not installed")
	}
	snapshot := filepath.Join("..", "..", "snapshots", "post-acquisition.snapshot")
	if _, err := os.Stat(snapshot); os.IsNotExist(err) {
		t.Skip("private post-acquisition snapshot is not installed")
	}
	images, err := firmware.Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	box, err := board.New(images, true)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(snapshot) // #nosec G304 -- fixed local private test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err := box.Restore(file); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return box
}

func demoGuide(t *testing.T) *multiplex.Guide {
	t.Helper()
	guide, err := multiplex.LoadGuide(filepath.Join("..", "..", "listings"))
	if err != nil {
		t.Fatal(err)
	}
	return guide
}

func demoDictionary(t *testing.T) *broadcast.HuffmanDictionary {
	t.Helper()
	path := filepath.Join("..", "..", "dictionaries", "skyuk.dict")
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

	registered, firstAt := 0, 0
	const budget = 120_000_000
	for i := 0; i < budget; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			if registered == 0 {
				firstAt = i
			}
			registered++
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}

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
	if registered == 0 {
		t.Fatal("the box registered no programmes at all")
	}
	t.Logf("the box registered %d programmes, the first %d instructions in", registered, firstAt)
}
