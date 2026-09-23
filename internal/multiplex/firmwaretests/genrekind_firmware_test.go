package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// CAN A CHANNEL CARRY ITS GENRE IN THE LINE-UP'S KIND BYTE WITHOUT LOSING ITS ROW?
//
// Two things are measured and neither is in doubt. A genre screen compares against a number it
// loads from 0x80494906 -- 3 for ENTERTAINMENT, 6 for MOVIES -- and what it compares that number
// AGAINST is +4 of a six-byte per-channel record whose +0 is the channel's listings id:
//
//	802AD9BC  00 65 FF FF 00 01      Sky One, listings id 0065, and the 0x0001 is the Kind byte
//
// lineupDescriptors defaults Kind to 1 when a caller leaves it zero, so all six channels answer 1,
// none answers 3, and the search keeps the 0xFFFFFFFF it preset. That is the empty grid.
//
// THE OBVIOUS FIX IS NOT OBVIOUSLY SAFE, which is the whole reason for this probe. The record has
// kind 2 making the ALL CHANNELS grid draw with 0 of 21 programmes registered and the banner
// reporting no satellite signal -- so Kind is load-bearing somewhere else too, and "set Kind to the
// genre" could buy a genre screen at the cost of the one screen that already works.
//
// SIX KINDS ACROSS SIX CHANNELS IN ONE RUN, because a sweep that changes every channel to the same
// value cannot say which change did what, and one that changes them one at a time costs six boots.
// Each channel gets a different Kind and then three screens are opened and photographed:
//
//	ALL CHANNELS   does it still draw six rows, or has Kind cost them their rows?
//	ENTERTAINMENT  does the channel with Kind 3 -- and only it -- appear?
//	MOVIES         does the channel with Kind 6 -- and only it -- appear?
//
// THE VERDICT IS THE PICTURE. A hash says three screens differ; only the artefact says which
// channels are on them, and this project has measured the wrong screen six times.
//
// IT IS ENTIRELY IN THE SIGNAL: the only thing that changes is what the 0xB1 line-up entry says.
func TestWhetherAChannelCanCarryItsGenreInTheLineUpKind(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	if len(listings.Services) != 6 {
		t.Fatalf("harness: this sweep gives each of six channels its own Kind and the schedule "+
			"lists %d, so a row could not be traced back to a value", len(listings.Services))
	}
	for i := range listings.Services {
		listings.Services[i].Kind = byte(i + 1) // #nosec G115 -- six services
		t.Logf("%-14s channel %4d  listings id %04X  line-up Kind = %d",
			listings.Services[i].Name, listings.Services[i].Channel,
			listings.Services[i].ListingsID, i+1)
	}
	t.Log("ENTERTAINMENT asks for 3 (Sky Travel here) and MOVIES asks for 6 (Sky News here) -- " +
		"deliberately NOT the channels those names suggest, so a screen that draws the right row " +
		"is answering the Kind and not the channel's name")

	// The six-byte per-channel records the genre search walks, captured from the box that produced
	// the pointer. Whether +4 follows Kind is the one thing that decides whether Kind is the field
	// the genre screen compares against or merely the field that decides a channel exists at all.
	const recordAt = 0x800CBA16
	shapes := map[uint32][]byte{}

	open := func(t *testing.T, key uint8, name, artefact string) uint32 {
		t.Helper()
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			demoSchedule())
		if err != nil {
			t.Fatal(err)
		}
		// THE REGISTRATION COUNT IS NOT A PRECONDITION and must not be one. Kind 2 is measured to
		// stop programmes registering at all, so a sweep that waits for all of them would fail in
		// the one case it exists to characterise. It waits, reports what it got, and carries on.
		want := programmesInTheBlock(t, guide, day)
		registered := 0
		runUntil(t, box, transmitter, 120_000_000, registeringProgrammes(box, want, &registered))
		t.Logf("%-14s %d of %d programmes registered", name, registered, want)
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

		pump := func() error { return transmitter.Pump(box.Machine.Retired) }
		press := azPressFunc(t, box, pump)
		menu := uint32(0)
		for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
			menu = press(keyBoxOffice, "box office", pressBudget)
		}
		if menu != boxOfficeMenu {
			t.Fatalf("harness: box office drew %08X, not %08X", menu, uint32(boxOfficeMenu))
		}
		tab := uint32(0)
		for attempt := 1; attempt <= attempts && tab != tvGuideMenuScreen; attempt++ {
			tab = press(keyLeft, fmt.Sprintf("left to the tv guide menu (%d)", attempt), pressBudget)
		}
		if tab != tvGuideMenuScreen {
			t.Fatalf("harness: never reached the TV GUIDE menu; drew %08X", tab)
		}
		// THE CAPTURE RIDES ON THE PRESS, not on the wait after it. The search runs WHILE the grid
		// draws, so a watch that starts once the screen has settled sees nothing -- measured: a
		// first version hooked the sixty-million-instruction fill and reported that no per-channel
		// record was walked at all, which is a fact about where the hook was.
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if a.Fetch || a.Write || box.Machine.Core.State().PC&^1 != recordAt {
				return
			}
			if at := a.Value; at >= 0x80000000 && at < 0x80800000 {
				row := make([]byte, 6)
				for i := range row {
					row[i] = byte(box.RAM.Read((at&0x1fffffff)+uint32(i), bus.Byte)) // #nosec G115
				}
				shapes[at] = row
			}
		}}
		screen := uint32(0)
		for attempt := 1; attempt <= attempts && (screen == 0 || screen == tab); attempt++ {
			screen = pressAndLetItFinishWatching(t, box, pump, hooks, key, pressBudget, nil)
			t.Logf("key %d -> %s (%d) drew %08X", key, name, attempt, screen)
		}
		if screen == 0 || screen == tab {
			t.Fatalf("harness: key %d (%s) never left the TV GUIDE menu (%08X)", key, name, tab)
		}
		// The grid opens on "Searching for listings" and fills tens of millions of instructions
		// later, so a picture taken at the press is of a screen that has not decided yet.
		for i := 0; i < 60_000_000; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		if err := dumpScreen(t, box, artefact); err != nil {
			t.Fatal(err)
		}
		filled := screenNow(t, box)
		t.Logf("%-14s opened %08X and settled on %08X -- READ .artifacts/%s",
			name, screen, filled, artefact)
		return filled
	}

	all := open(t, 0x01, "ALL CHANNELS", "genrekind-all.png")
	ent := open(t, 0x02, "ENTERTAINMENT", "genrekind-entertainment.png")
	mov := open(t, 0x03, "MOVIES", "genrekind-movies.png")

	t.Logf("=== READ THE THREE PICTURES, THEY ARE THE VERDICT ===")
	t.Logf("    ALL CHANNELS   %08X   six rows means Kind did not cost them their rows", all)
	t.Logf("    ENTERTAINMENT  %08X   Sky Travel alone means Kind 3 is the genre", ent)
	t.Logf("    MOVIES         %08X   Sky News alone means Kind 6 is the genre", mov)
	if ent == mov {
		t.Errorf("ENTERTAINMENT and MOVIES both drew %08X, so the Kind sweep did not separate "+
			"them and the screens are still showing whatever they showed before", ent)
	}
	if all == ent || all == mov {
		t.Errorf("a genre screen drew the same picture as ALL CHANNELS (%08X), which would mean "+
			"the filter stopped filtering rather than that it started matching", all)
	}

	// AND WHAT THE RECORDS SAY NOW. With Kind 1..6 across the six channels, +4 either follows Kind
	// -- in which case Kind IS the halfword the genre screen compares, and a genre can never be
	// carried there because Kind also decides whether the channel exists at all -- or it stays
	// 0x0001, in which case +4 is a field this transmitter does not send and the genre lives
	// somewhere nobody has fed yet.
	if len(shapes) == 0 {
		t.Log("no per-channel records were walked, so the Kind question below is unanswered here")
		return
	}
	addrs := make([]uint32, 0, len(shapes))
	for at := range shapes {
		addrs = append(addrs, at)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })
	t.Logf("=== THE %d PER-CHANNEL RECORDS, WITH KIND 1..6 ON AIR ===", len(addrs))
	for _, at := range addrs {
		t.Logf("    %08X  % 02X", at, shapes[at])
	}
	t.Log("compare +0 against the listings ids above: if +4 moved off 0001 it follows Kind, and " +
		"if it did not, the genre is a field this broadcast has never carried")
}
