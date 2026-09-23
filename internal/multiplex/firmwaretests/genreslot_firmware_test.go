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

// WHICH CATEGORY NUMBER IS WHICH GENRE, ASKED BY WATCHING EACH SCREEN LOOK.
//
// The `0xC1` dispatcher's fourth arm is extensions 0x0100..0x01CF, and it indexes a table of
// sixteen categories by four six-hour blocks at 0x800C51A8, slot (ext & 0x0F) * 4 + ((ext & 0xC0)
// >> 6). Boundaries measured: 0x01CF accepted, 0x01D0 refused. What is NOT established is which
// number is ENTERTAINMENT and which is SPORTS, and the menu's order is not evidence -- it is the
// guide's presentation order, chosen by whoever wrote the resource file.
//
// THE SLOT INDEX IS THE EXTENSION, so a screen waiting for an index is a screen reading the slot
// it wants. That is how A-Z LISTINGS was solved in one run rather than ninety-two sweeps, and it
// works here unchanged.
//
// IT VALIDATES ITS OWN NAVIGATION BEFORE IT TRUSTS IT. The TV GUIDE menu numbers its ten entries,
// so a number key should open one directly -- three presses instead of counting DOWNs, and this
// project has twice measured the wrong screen by counting presses. But the exhaustive handset
// sweep found the number keys do NOTHING from the idle picture, so whether they select inside a
// menu is an assumption. Entry 1 is ALL CHANNELS, whose two settled frames this package already
// names, so pressing "1" is a navigation test with a known answer: if it opens the grid the method
// works and the other seven entries can be trusted to it, and if it does not the probe says so and
// stops rather than reporting the slots of screens it cannot name.
//
// ALL CHANNELS IS ALSO THE NEGATIVE CONTROL. It reads no 0xC1 slot at all -- measured, 17.8M data
// reads and none touching a head -- so an instrument that reports it reading a category is an
// instrument counting the transmitter.
//
// IT ONLY READS.
//
// ANSWERED 2026-09-23, AND THE QUESTION WAS THE WRONG ONE. Every screen was measured on its own
// acquired box and NONE of them reads a category slot:
//
//	ALL CHANNELS     71A6DFE8   reads NO category slot (7,392,674 data reads)
//	ENTERTAINMENT    8F911D41   MOVIES        70C50E7D   SPORTS       586CEF46
//	NEWS & DOCS      D0C7E187   KIDS          F5A782AA   MUSIC/RADIO  774741E3
//	SPECIALIST       9B5B156F
//
// The picture says why, and it is the picture rather than the count that settles it: a genre
// screen opens as THE ALL CHANNELS GRID WITH NO ROWS IN IT -- same chrome, same "Today 7.00pm
// 7.30pm 8.00pm" headers, zero channels. It is the same grid widget filtered to a genre, and
// nothing in the line-up passes the filter. Nobody is waiting for an index, so broadcasting a
// category index would have fed a table nobody reads.
//
// The question therefore becomes what the grid reads to decide a channel's GENRE, and that is
// TestWhatTheGenreGridRejects, which diffs a screen that accepts six channels against one that
// rejects the same six. This probe stays because the negative control it carries is the thing
// that makes that differential believable, and because it is the only place the number keys are
// proved to select.
func TestWhichCategorySlotEachGenreScreenReads(t *testing.T) {
	const (
		categoriesPool = 0x800C51A8
		categorySlots  = 64 // sixteen categories by four blocks
		// THE WHOLE 0xC1 MODULE IS EXCLUDED, not just the parser. 0x800C4C34 decodes a section and
		// reads the head it is about to hand its array to; 0x800C4F94 frees a list and reads the
		// same head to do it. Both fire constantly because the carousel keeps delivering, and a
		// version of gridslot that excluded only the parser reported the grid reading twenty-one
		// letters -- every one from inside the free routine.
		moduleFrom = 0x800C4C34
		moduleTo   = 0x800C5010
	)
	// The ten entries the guide draws, in the order it draws them. The numbers are the menu's own.
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
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	type looked struct {
		screen uint32
		slots  map[string]int
		reads  int
	}

	open := func(t *testing.T, key uint8, name string) looked {
		t.Helper()
		guide := demoGuide(t)
		dict := demoDictionary(t)
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			demoScheduleWithIndex())
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
			t.Fatalf("harness: never reached the TV GUIDE menu (%08X); drew %08X",
				uint32(tvGuideMenuScreen), tab)
		}

		out := looked{slots: map[string]int{}}
		for attempt := 1; attempt <= attempts && (out.screen == 0 || out.screen == tab); attempt++ {
			out.screen = press(key, fmt.Sprintf("key %d -> %s (%d)", key, name, attempt), pressBudget)
		}
		if out.screen == 0 || out.screen == tab {
			t.Fatalf("harness: the number key %#02x never left the TV GUIDE menu (%08X), so this "+
				"run measured the menu", key, tab)
		}
		if err := dumpScreen(t, box, fmt.Sprintf("genreslot-%d.png", key)); err != nil {
			t.Fatal(err)
		}

		// NOW WATCH IT LOOK. The table pointer is read AFTER the screen is open, because the
		// module allocates its pools during acquisition and a pointer read earlier is a guess.
		base := box.RAM.Read(categoriesPool&0x1fffffff, bus.Word)
		if base&0xf0000000 == 0 || base&3 != 0 {
			t.Fatalf("harness: the category table pointer is %08X, which is not a word-aligned "+
				"guest address, so every slot read below would be noise", base)
		}
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if a.Write || a.Fetch {
				return
			}
			out.reads++
			at := a.Virtual | 0x80000000
			if at < base || at >= base+categorySlots*4 {
				return
			}
			if pc := box.Machine.Core.State().PC &^ 1; pc >= moduleFrom && pc < moduleTo {
				return // the 0xC1 module talking to itself while the carousel delivers
			}
			slot := (at - base) / 4
			out.slots[fmt.Sprintf("category %2d block %d (extension %#04x)",
				slot/4, slot%4, 0x0100|(slot%4)<<6|(slot/4))]++
		}}
		for i := 0; i < 30_000_000; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		if out.reads == 0 {
			t.Fatal("harness: the observer saw no data read at all, so its silence is the instrument")
		}
		return out
	}

	report := func(name string, w looked) {
		names := make([]string, 0, len(w.slots))
		for slot := range w.slots {
			names = append(names, slot)
		}
		sort.Slice(names, func(a, b int) bool { return w.slots[names[a]] > w.slots[names[b]] })
		if len(names) == 0 {
			t.Logf("%-22s %08X   reads NO category slot (%d data reads seen)",
				name, w.screen, w.reads)
			return
		}
		for i, slot := range names {
			prefix := "                      "
			if i == 0 {
				prefix = fmt.Sprintf("%-22s", name)
			}
			t.Logf("%s %s  %d reads", prefix, slot, w.slots[slot])
		}
	}

	// THE NAVIGATION TEST FIRST, with a known answer.
	all := open(t, entries[0].key, entries[0].name)
	report(entries[0].name, all)
	if !atAllChannels(all.screen) {
		t.Fatalf("pressing \"1\" from the TV GUIDE menu drew %08X, which is neither frame ALL "+
			"CHANNELS settles on (%08X searching, %08X filled) -- so the number keys do NOT select "+
			"a menu entry and every reading below would be of a screen this probe cannot name. "+
			"Read .artifacts/genreslot-1.png and walk the menu with DOWN instead",
			all.screen, uint32(allChannelsOpening), uint32(allChannelsFilled))
	}
	if len(all.slots) != 0 {
		t.Errorf("ALL CHANNELS read %d category slots, and it is measured to read none -- so this "+
			"instrument is counting the transmitter rather than the screen, and the genre "+
			"readings below are not to be believed", len(all.slots))
	}
	t.Logf("the number keys select: \"1\" opened ALL CHANNELS (%08X) and it read no category slot, "+
		"which is both the navigation test and the negative control", all.screen)

	t.Logf("=== the category slot each genre screen reads while it waits for an index ===")
	for _, entry := range entries[1:] {
		report(entry.name, open(t, entry.key, entry.name))
	}
	t.Logf("each slot named above is the extension to transmit table 0xC1 under for that genre; " +
		"the pictures are .artifacts/genreslot-*.png")
}
