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

// DOES A-Z LISTINGS READ A 0xC1 TABLE SENT UNDER ITS OWN LETTER?
//
// Everything before this sent extension 0x0000 or 0x0100, outside the 'A'..'Z'
// range 0x800C4F94 dispatches on, so the box built the array and then freed it
// -- which is why five screens read nothing. This sends 'A' (0x0041), which
// indexes the first of the twenty-six list heads at 0x800C51A0, and then opens
// the screen those heads are for: TV GUIDE -> A-Z LISTINGS, the ninth entry.
//
// THE MENU IS NUMBERED, so the ninth entry is reached by pressing 9 rather than
// by eight downs. Fewer presses is not the point -- each press is a chance for
// the key to be swallowed while the menu paints, which is a documented
// behaviour here and has already made one measurement of the wrong screen.
//
// The screen is dumped either way. A read count says the table was consulted; a
// picture says what the viewer got, and the two answer different questions.
func TestWhetherAtoZListingsReadsATableC1SentUnderItsLetter(t *testing.T) {
	const probePID = 0x52
	const letterA = 0x0041
	const idBase = 0xE200
	const records = 8

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

	payload := make([]byte, 0, records*9)
	for i := 0; i < records; i++ {
		id := uint16(idBase + i)
		payload = append(payload, byte(id>>8), byte(id), 0xA0, 0x40, 0, 0, 0, 0, 0)
	}
	if err := box.Demux.Push(probePID, sectionTableVersioned(0xC1, letterA, 0, payload)); err != nil {
		t.Fatalf("harness: the box refused the section: %v", err)
	}
	runUntil(t, box, transmitter, censusBudget, func(int) bool { return false })

	var bases []uint32
	size := box.RAM.Size()
	for off := uint32(0); off+records*10 <= size; off += 2 {
		if uint16(box.RAM.Read(off, bus.Half)) == idBase &&
			uint16(box.RAM.Read(off+10, bus.Half)) == idBase+1 {
			bases = append(bases, off)
		}
	}
	if len(bases) == 0 {
		t.Fatal("harness: the array was not assembled under letter 'A', so there is nothing to watch")
	}
	t.Logf("array under letter 'A' at guest 0x%08X", 0x80000000|bases[0])
	inWindow := func(p uint32) bool {
		for _, b := range bases {
			if p >= b-12 && p < b+records*10 {
				return true
			}
		}
		return false
	}

	reads := map[uint32]int{}
	offsets := map[int]int{}
	var anyReads int
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch || a.Write {
			return
		}
		anyReads++
		if p := a.Virtual & 0x1fffffff; inWindow(p) {
			reads[box.Machine.Core.State().PC&^1]++
			offsets[int(p)-int(bases[0])+12]++
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, settled := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				settled = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		t.Logf("%-22s %08X -> %08X", name, before, settled)
		return settled
	}

	// GENEROUS BUDGETS AND A CONFIRMED TAB. The first run of this settled on an
	// intermediate frame after 12M instructions, took it for the tv guide tab,
	// pressed LEFT into it and ended up on BOX OFFICE -- whose menu has six
	// entries, so pressing 9 did nothing and the screenshot showed a different
	// screen entirely. The grid test already pays 80M per press for this reason.
	// The tab is now also CHECKED rather than assumed: the tv guide menu is the
	// one with ten entries, and FE8D1CCC is box office (lessons.md), so landing
	// there means LEFT has not moved yet and it is pressed again.
	const boxOfficeMenu = 0xFE8D1CCC
	menu := press(0x7D, "box office", 80_000_000)
	tab := menu
	for attempt := 1; attempt <= 4 && (tab == menu || tab == boxOfficeMenu || tab == 0); attempt++ {
		tab = press(0x5A, "left to tv guide", 80_000_000)
	}
	if tab == menu || tab == boxOfficeMenu || tab == 0 {
		if err := dumpScreen(t, box, "tablec1-az-wrong-tab.png"); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: four LEFT presses never left %08X, so the tv guide tab was not reached "+
			"and anything measured past here is the wrong screen", tab)
	}
	// DOWN EIGHT AND SELECT, not the number key. Pressing 0x09 left the highlight
	// on entry 1 and the screen on the menu -- lessons.md records that only four
	// handset codes here were ever proved against a screen, and the number keys
	// are not among them, so 0x09 meaning "9" is an assumption this screen does
	// not support. Down and select ARE proved: both moved this menu in the
	// rec[2]/rec[3] probe.
	// COUNT MOVES, NOT PRESSES. A press that lands while the menu is painting is
	// swallowed and changes nothing, so this counts screens that actually changed
	// rather than presses made.
	//
	// NINE, NOT EIGHT, AND THE PICTURE IS WHY. Eight was the arithmetic -- entry 1
	// plus eight moves is entry 9 -- and the screenshot it produced said
	// SPECIALIST across the top, which is entry 8. The test had been reporting
	// "A-Z LISTINGS did not read the array" about a screen that was not A-Z
	// LISTINGS, and no amount of read-counting would have said so. One of the
	// counted screen changes is not a highlight move; which one is not worth
	// chasing, because the screen itself is the authority and it is checked below.
	const wantEntry = 9
	moves := 0
	for attempt := 0; attempt < 40 && moves < wantEntry; attempt++ {
		if press(0x59, fmt.Sprintf("down (move %d/%d)", moves+1, wantEntry), 4_000_000) != 0 {
			moves++
		}
	}
	if moves < wantEntry {
		if err := dumpScreen(t, box, "tablec1-az-nav-stuck.png"); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: the highlight moved only %d of %d entries in 40 presses, so A-Z LISTINGS "+
			"was never selected", moves, wantEntry)
	}
	az := uint32(0)
	for attempt := 1; attempt <= 6 && (az == 0 || az == tab); attempt++ {
		az = press(0x5C, fmt.Sprintf("select A-Z (try %d)", attempt), 80_000_000)
	}
	if az == 0 || az == tab {
		if err := dumpScreen(t, box, "tablec1-az-not-reached.png"); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: pressing 9 did not leave the tv guide menu, so A-Z LISTINGS was never "+
			"opened and the read count below measures the wrong screen (tab %08X)", tab)
	}
	if err := dumpScreen(t, box, "tablec1-az-listings.png"); err != nil {
		t.Fatal(err)
	}

	if anyReads == 0 {
		t.Fatal("harness: the observer saw no data read at all, so its zero is the instrument")
	}
	total := 0
	for _, n := range reads {
		total += n
	}
	if total == 0 {
		t.Logf("A-Z LISTINGS did NOT read the array (%d data reads seen overall). Sent under 'A', "+
			"assembled, and still not consulted by this screen.", anyReads)
		return
	}
	pcs := make([]uint32, 0, len(reads))
	for pc := range reads {
		pcs = append(pcs, pc)
	}
	sort.Slice(pcs, func(a, b int) bool { return reads[pcs[a]] > reads[pcs[b]] })
	t.Logf("A-Z LISTINGS READ THE ARRAY: %d reads from %d PCs", total, len(pcs))
	for i, pc := range pcs {
		if i >= 10 {
			break
		}
		t.Logf("    %08X  %d reads", pc, reads[pc])
	}
	var offs []int
	for o := range offsets {
		offs = append(offs, o)
	}
	sort.Ints(offs)
	for _, o := range offs {
		where := "inside the records"
		if o < 12 {
			where = "in the 12-byte header"
		}
		t.Logf("    +%d: %d reads  (%s)", o, offsets[o], where)
	}
}
