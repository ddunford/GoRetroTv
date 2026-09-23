package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE TV GUIDE'S GENRE SCREENS, DRAWN FROM THE BROADCAST.
//
// This is the acceptance for the genre work and it is deliberately an ORDINARY run: the demo
// schedule exactly as it ships, no sweep, no field set from the test. Every channel's genre rides
// in the 0xB2 guide-row descriptor's three-bit scalar, which the guide's screens filter on:
//
//	1 SPECIALIST   2 KIDS   3 ENTERTAINMENT   4 MUSIC & RADIO
//	5 NEWS & DOCUMENTARIES   6 MOVIES   7 SPORTS
//
// and the demo carries Sky One and Sky Soap at 3, Sky Travel at 1, Sky Movies at 6, Sky Sports 1
// at 7 and Sky News at 5. Nothing carries 2 or 4, so KIDS and MUSIC & RADIO must stay EMPTY --
// which is as much a check as a full screen is, because a filter that matched everything would
// fill them too.
//
// EVERY SCREEN IS PHOTOGRAPHED, because a hash says eight screens differ and only the picture says
// which channels are on them. The genre map is exactly the kind of thing that would look right
// with two names swapped.
//
// IT ASSERTS ITS OWN SUBJECT: ALL CHANNELS must draw, or this is a box with no guide at all and
// every empty genre screen below would be empty for a reason that has nothing to do with genres.
func TestTheGenreScreensDrawTheirChannels(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	for _, s := range guide.On(day).Services {
		t.Logf("%-14s genre %d", s.Name, s.Genre)
	}

	open := func(t *testing.T, raw uint8, name, artefact string) uint32 {
		t.Helper()
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			demoSchedule())
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
		screen := uint32(0)
		for attempt := 1; attempt <= attempts && (screen == 0 || screen == tab); attempt++ {
			screen = press(raw, fmt.Sprintf("%s (%d)", name, attempt), pressBudget)
		}
		if screen == 0 || screen == tab {
			t.Fatalf("harness: %#02x (%s) never left the TV GUIDE menu (%08X)", raw, name, tab)
		}
		// The grid opens on "Searching for listings" and fills tens of millions of instructions
		// later, so a picture taken at the press is of a screen that has not decided yet.
		for i := 0; i < 60_000_000; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		if err := dumpScreen(t, box, artefact); err != nil {
			t.Fatal(err)
		}
		filled := screenNow(t, box)
		t.Logf("%-22s %08X   .artifacts/%s", name, filled, artefact)
		return filled
	}

	all := open(t, 0x01, "ALL CHANNELS", "genre-all.png")
	if all == 0 {
		t.Fatalf("harness: ALL CHANNELS drew nothing, so this box has no guide and the screens " +
			"below would be empty for a reason that has nothing to do with genres. Read " +
			".artifacts/genre-all.png")
	}

	// The two empty screens are named so a reader meets them as expectations rather than as a
	// disappointment, and the hashes of the empty genre screens are already measured.
	const emptyKids, emptyMusic = 0xF5A782AA, 0x774741E3
	for _, entry := range []struct {
		raw    uint8
		name   string
		expect string
	}{
		{0x02, "ENTERTAINMENT", "Sky One and Sky Soap, genre 3"},
		{0x03, "MOVIES", "Sky Movies, genre 6"},
		{0x04, "SPORTS", "Sky Sports 1, genre 7"},
		{0x05, "NEWS & DOCUMENTARIES", "Sky News, genre 5"},
		{0x06, "KIDS", "NOTHING -- no demo channel carries genre 2"},
		{0x07, "MUSIC & RADIO", "NOTHING -- no demo channel carries genre 4"},
		{0x08, "SPECIALIST", "Sky Travel, genre 1"},
	} {
		drew := open(t, entry.raw, entry.name, fmt.Sprintf("genre-%s.png",
			map[uint8]string{0x02: "entertainment", 0x03: "movies", 0x04: "sports",
				0x05: "news", 0x06: "kids", 0x07: "music", 0x08: "specialist"}[entry.raw]))
		t.Logf("    expect %s -- READ the picture, a hash cannot tell one channel from another",
			entry.expect)
		switch entry.raw {
		case 0x06:
			if drew != emptyKids {
				t.Errorf("KIDS drew %08X where the empty screen is %08X. Nothing carries genre 2, "+
					"so a KIDS screen with a channel on it means the filter is matching something "+
					"other than the genre", drew, uint32(emptyKids))
			}
		case 0x07:
			if drew != emptyMusic {
				t.Errorf("MUSIC & RADIO drew %08X where the empty screen is %08X. Nothing carries "+
					"genre 4, so a filled screen means the filter is matching something other "+
					"than the genre", drew, uint32(emptyMusic))
			}
		default:
			if drew == emptyKids || drew == emptyMusic {
				t.Errorf("%s drew %08X, which is one of the EMPTY genre screens -- so the channel "+
					"that carries its genre did not reach it", entry.name, drew)
			}
		}
	}
}
