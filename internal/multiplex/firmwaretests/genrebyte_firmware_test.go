package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH BYTE OF THE 0xB2 DESCRIPTOR IS THE CHANNEL'S GENRE.
//
// The native code settles where the genre comes from, and it is a descriptor this port already
// transmits. FUN_800cb7b8 is the per-channel test the genre grid runs, and the branch a genre
// screen takes is:
//
//	iVar1 = (*DAT_800cba38)(local_8c, 0xb2, &local_10, ...);      // search the SI for 0xB2
//	...
//	if (((*param_4 & *(ushort *)(*(int *)(param_2 + 8) + 4)) != 0) &&
//	    (*(char *)(param_4 + 1) == local_10)) { ... accept }
//
// param_4 is the screen's four-byte filter, measured live at 0x80494904: a service-type MASK at
// +0..1 (0x0013, which is what lets kinds 1 and 2 through), the GENRE at +2 (3 for ENTERTAINMENT,
// 6 for MOVIES) and a zero at +3. param_2+8 is the per-channel record whose +4 is the mask derived
// from Kind. So the test is two halves -- the type mask ANDs, and the genre byte must EQUAL
// local_10 -- and local_10 is filled by searching the SI for descriptor 0xB2.
//
// THE 0xB2 IS THE GUIDE ROW THIS PORT ALREADY SENDS. Its first scalar is measured (record+8, the
// flag that turns "..no listings available" into a drawn row); At9, At10 and At11 are transmitted
// and have never been read back. One of them is the genre.
//
// AT11 IS OUT BY CONSTRUCTION AND THE BUILDER SAID SO. A first version of this run put 3 in At11
// and guideRowDescriptor refused it: "0xB2 field At11 is 3 and the parser takes one bit". At9 is
// three bits, so it holds 0..7 and can carry both 3 and 6; At10 is a whole byte. At11 can be 0 or
// 1 and therefore cannot be a genre at all, whatever else it is for. That is the encoding rejecting
// a test design, which is the cheapest kind of correction there is.
//
// ONE RUN NAMES IT, WITH THE ANSWER REPLICATED. Two channels carry 3 -- one in At9, one in At10 --
// and two carry 6 the same way. ENTERTAINMENT asks for 3 and MOVIES asks for 6, so each screen
// names the byte independently, and the two must agree or neither reading stands.
//
// SKY SOAP IS DELIBERATELY NOT A TEST SLOT. It has nothing on air at 19:00, so guideRow builds no
// 0xB2 for it at all and it could never match whatever the answer is. Giving it a candidate byte
// would manufacture a false negative for that byte.
//
// IT IS ENTIRELY IN THE SIGNAL: the only thing that changes is what the 0xB2 descriptor carries.
func TestWhichByteOfTheGuideRowIsTheGenre(t *testing.T) {
	const entertainment, movies = 3, 6
	guide := demoGuide(t)
	dict := demoDictionary(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	carriers := map[string]string{}
	for i := range listings.Services {
		s := &listings.Services[i]
		s.RowAt8 = 1 // the flag that makes a row draw at all; measured, and not under test here
		switch s.Name {
		case "Sky One":
			s.Genre, carriers[s.Name] = entertainment, "At9"
		case "Sky Travel":
			s.RowAt10, carriers[s.Name] = entertainment, "At10"
		case "Sky Movies":
			s.Genre, carriers[s.Name] = movies, "At9"
		case "Sky Sports 1":
			s.RowAt10, carriers[s.Name] = movies, "At10"
		}
	}
	if len(carriers) != 4 {
		t.Fatalf("harness: this run needs four named channels to carry the candidate bytes and "+
			"found %d, so a screen that filled could not be traced back to a byte", len(carriers))
	}
	for name, byteName := range carriers {
		want := entertainment
		if byteName == "At9" && name != "Sky One" || byteName == "At10" && name != "Sky Travel" {
			want = movies
		}
		t.Logf("%-14s carries %d in %s", name, want, byteName)
	}
	t.Log("ENTERTAINMENT asks for 3 and MOVIES asks for 6, and each value is carried twice -- " +
		"once in At9 and once in At10 -- so each screen names the byte independently")

	open := func(t *testing.T, key uint8, name, artefact string) uint32 {
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
			t.Fatalf("harness: only %d of %d programmes registered, so the guide has less than "+
				"the schedule and an empty screen would not be about the genre", registered, want)
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
			screen = press(key, fmt.Sprintf("key %d -> %s (%d)", key, name, attempt), pressBudget)
		}
		if screen == 0 || screen == tab {
			t.Fatalf("harness: key %d (%s) never left the TV GUIDE menu (%08X)", key, name, tab)
		}
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
		t.Logf("%-14s settled on %08X -- READ .artifacts/%s", name, filled, artefact)
		return filled
	}

	all := open(t, 0x01, "ALL CHANNELS", "genrebyte-all.png")
	ent := open(t, 0x02, "ENTERTAINMENT", "genrebyte-entertainment.png")
	mov := open(t, 0x03, "MOVIES", "genrebyte-movies.png")

	// THE EMPTY GENRE SCREENS ARE KNOWN HASHES, which makes "did anything change" answerable
	// without opening a picture -- and the picture is still what says WHICH channel, because a
	// hash cannot tell Sky One from Sky Travel.
	const emptyEntertainment, emptyMovies = 0x8F911D41, 0x70C50E7D
	t.Logf("=== READ THE PICTURES, THEY NAME THE CHANNEL AND THEREFORE THE BYTE ===")
	t.Logf("    ALL CHANNELS   %08X", all)
	t.Logf("    ENTERTAINMENT  %08X  (empty %08X)  Sky One means At9, Sky Travel means At10",
		ent, uint32(emptyEntertainment))
	t.Logf("    MOVIES         %08X  (empty %08X)  Sky Movies means At9, Sky Sports means At10",
		mov, uint32(emptyMovies))
	switch {
	case ent == emptyEntertainment && mov == emptyMovies:
		t.Logf("BOTH GENRE SCREENS ARE STILL EMPTY, so neither At9 nor At10 is the genre. The " +
			"0xB2 is still where it comes from -- the native code searches for that descriptor " +
			"and compares its result against the screen's number -- so what this builder puts in " +
			"those two fields is not what the parser reads as the genre. The candidates left are " +
			"inside the descriptor and not among the scalars: the length-prefixed Prefix that " +
			"guideRowDescriptor emits and the parser steps over, or a field past the text. Read " +
			"guideRowDescriptor against the parser at 0x800CB008.")
	case ent != emptyEntertainment && mov != emptyMovies:
		t.Logf("BOTH SCREENS MOVED, which is the replicate agreeing -- open both pictures and " +
			"check they name the SAME byte. ENTERTAINMENT showing Sky One with MOVIES showing " +
			"Sky Movies is At9; Sky Travel with Sky Sports is At10. If they name different " +
			"bytes, neither is established and something other than the genre moved them.")
	default:
		t.Errorf("one genre screen moved and the other did not (ENTERTAINMENT %08X, MOVIES %08X). "+
			"Each value is carried twice, once per candidate byte, so one screen moving alone "+
			"means something other than the genre changed and neither picture is safe to read",
			ent, mov)
	}
}
