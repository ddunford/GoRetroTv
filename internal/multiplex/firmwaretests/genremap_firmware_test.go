package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE WHOLE GENRE MAP, READ OFF THE SCREENS THEMSELVES.
//
// Each genre screen carries a four-byte filter at 0x80494904 -- a service-type mask at +0..1, the
// GENRE IT WANTS at +2, a zero at +3 -- and a channel is accepted when At9 of its 0xB2 guide-row
// descriptor equals that byte. Both halves are measured: the filter live at the instruction that
// loads it, and At9 by putting 3 and 6 in it and watching ENTERTAINMENT and MOVIES draw the right
// channels.
//
// So the remaining five genres need no experiment at all. The filter sits at a FIXED address, so
// opening a screen and reading one byte is the whole measurement -- no hooks, no differential, no
// second run. That is worth saying out loud: the expensive instruments earlier in this file were
// needed to find WHERE to look, and once that is known the reading is a memory load.
//
// At9 IS THREE BITS, so the genres are 0..7 -- eight values for the menu's eight grid entries,
// with ALL CHANNELS presumably the one that filters nothing. Whether 0 is ALL CHANNELS is NOT
// asserted here: this probe reports what each screen asks for and lets the numbers speak.
//
// IT ASSERTS ITS OWN SUBJECT: the filter's mask half must read 0x0013 on every screen, because a
// screen whose filter is not where this expects it would report a genre of whatever else lives at
// that address.
//
// IT ONLY READS.
func TestWhatGenreEachScreenAsksFor(t *testing.T) {
	const (
		filterAt     = 0x80494904
		expectedMask = 0x0013
	)
	entries := []struct {
		key  uint8
		name string
	}{
		{0x01, "ALL CHANNELS"},
		{0x02, "ENTERTAINMENT"},
		{0x03, "MOVIES"},
		{0x04, "SPORTS"},
		{0x05, "NEWS & DOCUMENTARIES"},
		{0x06, "KIDS"},
		{0x07, "MUSIC & RADIO"},
		{0x08, "SPECIALIST"},
	}
	guide := demoGuide(t)
	dict := demoDictionary(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	t.Logf("=== WHAT EACH TV GUIDE SCREEN ASKS FOR ===")
	seen := map[byte]string{}
	for _, entry := range entries {
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
			screen = press(entry.key, fmt.Sprintf("%s (%d)", entry.name, attempt), pressBudget)
		}
		if screen == 0 || screen == tab {
			t.Fatalf("harness: key %d (%s) never left the TV GUIDE menu (%08X)",
				entry.key, entry.name, tab)
		}

		base := uint32(filterAt) & 0x1fffffff
		mask := box.RAM.Read(base, bus.Half)
		genre := byte(box.RAM.Read(base+2, bus.Byte)) // #nosec G115 -- one byte
		spare := byte(box.RAM.Read(base+3, bus.Byte)) // #nosec G115 -- one byte
		// ALL CHANNELS HAS NO FILTER AT ALL, and that is the finding rather than a fault. Its
		// mask reads 0000, so the AND can never pass and the genre byte beside it is not a genre
		// -- which is how a screen that shows EVERY channel is built: it does not run this test.
		// A first version of this probe called that an error; it is the control.
		if mask == 0 {
			t.Logf("    %-22s NO FILTER (mask 0000) -- it does not run the per-channel test at "+
				"all, which is what showing every channel means", entry.name)
			continue
		}
		if mask != expectedMask {
			t.Errorf("%s: the filter's mask half reads %04X, not %04X or 0000, so %#02x at +2 is "+
				"not this screen's genre but whatever else lives there", entry.name, mask,
				uint32(expectedMask), genre)
			continue
		}
		if other, dup := seen[genre]; dup {
			t.Errorf("%s asks for genre %d and so does %s -- two screens cannot share a genre, "+
				"so one of these readings is of a filter that had not been rewritten yet",
				entry.name, genre, other)
		}
		seen[genre] = entry.name
		t.Logf("    %-22s genre %d   (mask %04X, +3 %#02x)   screen %08X",
			entry.name, genre, mask, spare, screen)
	}
	t.Log("these are the values a channel's 0xB2 At9 must carry to appear on each screen; At9 is " +
		"three bits, so 0..7 is the whole space and the eight entries above fill it")
}
