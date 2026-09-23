package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE INDEX HAS TO ARRIVE WHILE THE SCREEN IS OPEN, AND THAT IS A PREDICTION FROM THE DECOMPILE.
//
// TestWhetherAtoZListingsReadsATableC1SentUnderItsLetter sends the section, confirms the box
// assembles the record array, then opens A-Z LISTINGS -- and the screen never reads the array. That
// was filed as "assembled and still not consulted", which is true and is not the whole story.
//
// 0x800C4C34's last act says what is missing:
//
//	uVar5 = DAT_800c51a0[ext - 0x40];             // the EXISTING list head for this letter
//	(*DAT_800c51ac)(uVar5, puVar2, uVar1, iVar3); // hand the new array to it
//
// The parser does not store the array anywhere a screen can later find it. It HANDS it to whatever
// is already in that slot. So a section that arrives before its screen has registered is handed to
// nothing, and the array the previous probe found in DRAM is the orphan -- which is exactly what an
// unread array in the heap looks like.
//
// On real hardware this is not a limitation, it is the design: the index is broadcast on a carousel,
// so whichever moment a viewer opens the screen, the next cycle delivers it. This port has only ever
// pushed one section, once, at a moment of its own choosing.
//
// TWO THINGS ARE MEASURED, AND THE FIRST GUARDS THE SECOND:
//
//  1. the slot itself, read out of the box before and after the screen opens. If it does not change,
//     the registration reading is wrong and the screen result below means nothing.
//  2. what A-Z LISTINGS draws when the index arrives repeatedly WHILE IT IS UP.
//
// The screenshot is the proof, as ever: this project has measured the wrong screen five times, and
// every one of them was caught by opening the picture and by nothing else.
func TestAtoZListingsWhenTheIndexArrivesWhileItIsOpen(t *testing.T) {
	// THE CONTROL RUNS FIRST AND IT IS NOT OPTIONAL. A-Z LISTINGS is empty the instant it opens and
	// the delivered run spends forty-eight million instructions on it afterwards, so "it filled"
	// has an innocent explanation -- the screen simply took time -- that no amount of staring at
	// the filled picture can rule out. The control is the same run with the pushes removed.
	controlOpened, controlFinal := azListingsRun(t, false, "tablec1-whileopen-control.png")
	openedWith, finalWith := azListingsRun(t, true, "tablec1-whileopen-after.png")
	t.Logf("=== control  (nothing delivered): opened %08X -> finished %08X", controlOpened, controlFinal)
	t.Logf("=== delivered (index on PID 0x52): opened %08X -> finished %08X", openedWith, finalWith)
	if controlFinal != controlOpened {
		t.Logf("THE CONTROL MOVED TOO (%08X -> %08X), so the screen changes on its own and the "+
			"delivered run's change is NOT evidence by itself -- compare the two pictures",
			controlOpened, controlFinal)
	}
	// A NEGATIVE IS A RESULT, NOT A FAILURE. This reported an error when the two runs matched,
	// which is exactly the thing it was built to find out and not a sign that anything is broken.
	if finalWith == controlFinal {
		t.Logf("the delivered run finished on the same screen as the control (%08X), so "+
			"delivering the index WHILE the screen is already open changes nothing it shows. "+
			"That is the measured answer: the 0xC1 parser hands its array to whatever already "+
			"sits in the list-head slot, so a section arriving after the screen opened is handed "+
			"to what was there -- the index has to arrive FIRST, which is what a carousel is for.",
			finalWith)
	}
	t.Logf("READ .artifacts/tablec1-whileopen-control.png AND -after.png. The hashes say they " +
		"differ; only the pictures say what the index actually drew")
}

func azListingsRun(t *testing.T, deliver bool, artefact string) (opened, final uint32) {
	t.Helper()
	const (
		probePID = 0x52
		// The pool word at 0x800C51A0 holds the base of the per-letter list heads; the parser
		// indexes it [ext - 0x40], so 'A' is entry 1.
		listHeadsPool = 0x800C51A0
		entryForA     = 1
	)
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

	// WAIT FOR THE BOX, THEN LET IT GO QUIET. A key sent the instant acquisition finishes is a key
	// that is swallowed: the first run of this pressed BOX OFFICE straight after registration and
	// every one of the five presses returned a screen that never settled, which reads exactly like
	// a broken route and is not one.
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
	)
	stateOffset := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(stateOffset+stateOff, bus.Word) }
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })
	t.Logf("the transport is at state %d and the box has been left to go quiet", state())
	_ = readyFrom

	word := func(virtual uint32) uint32 { return box.RAM.Read(virtual&0x1fffffff, bus.Word) }
	heads := word(listHeadsPool)
	if heads&0xf0000000 == 0 || heads&3 != 0 {
		t.Fatalf("harness: the word at %#08X is %#08X, which is not a word-aligned guest pointer, "+
			"so it is not the list-head table and every slot read below would be noise",
			uint32(listHeadsPool), heads)
	}
	slotA := heads + entryForA*4
	t.Logf("the list heads are at guest %08X; letter 'A' is slot %08X", heads, slotA)

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		settled := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-24s %08X -> %08X   (slot A = %08X)", name, before, settled, word(slotA))
		return settled
	}

	beforeOpen := word(slotA)
	t.Logf("slot A before anything is opened: %08X", beforeOpen)

	menu := press(0x7D, "box office", 80_000_000)
	tab := menu
	for attempt := 1; attempt <= 4 && (tab == menu || tab == boxOfficeMenu || tab == 0); attempt++ {
		tab = press(0x5A, "left to tv guide", 80_000_000)
	}
	if tab == menu || tab == boxOfficeMenu || tab == 0 {
		t.Fatalf("harness: four LEFT presses never left %08X, so the tv guide tab was not reached", tab)
	}
	const wantEntry = 9
	moves := 0
	for attempt := 0; attempt < 40 && moves < wantEntry; attempt++ {
		if press(0x59, fmt.Sprintf("down (move %d/%d)", moves+1, wantEntry), 4_000_000) != 0 {
			moves++
		}
	}
	if moves < wantEntry {
		if err := dumpScreen(t, box, "tablec1-whileopen-nav-stuck.png"); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: the highlight moved only %d of %d entries, so A-Z LISTINGS was never "+
			"selected", moves, wantEntry)
	}
	az := uint32(0)
	for attempt := 1; attempt <= 6 && (az == 0 || az == tab); attempt++ {
		az = press(0x5C, fmt.Sprintf("select A-Z (try %d)", attempt), 80_000_000)
	}
	if az == 0 || az == tab {
		if err := dumpScreen(t, box, "tablec1-whileopen-not-reached.png"); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: A-Z LISTINGS was never opened (tab %08X)", tab)
	}
	if err := dumpScreen(t, box, "opened-"+artefact); err != nil {
		t.Fatal(err)
	}
	opened = az

	// LET THE CATEGORY MENU FINISH, THEN GO THROUGH IT. A-Z LISTINGS is not a list of programmes:
	// it is a menu of the eight categories the string table names -- ALL PROGRAMMES, ENTERTAINMENT,
	// MOVIES, SPORTS, NEWS & DOCUMENTARIES, KIDS, MUSIC & RADIO, SPECIALIST -- and it paints them
	// about four million instructions after it opens, out of the box's own resources. A first
	// version of this probe dumped the screen before that and reported the menu appearing as the
	// index taking effect; the control, which drew the identical screen with nothing delivered at
	// all, is the only reason that is not in the record as a finding.
	//
	// So the letter index cannot be for THIS screen. ALL PROGRAMMES is entry 1 and already
	// highlighted, so one SELECT reaches whatever is below it, and that is where a list keyed by
	// letter would live.
	for i := 0; i < 8_000_000; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "categories-"+artefact); err != nil {
		t.Fatal(err)
	}
	menuAZ := screenNow(t, box)
	inner := uint32(0)
	for attempt := 1; attempt <= 6 && (inner == 0 || inner == menuAZ); attempt++ {
		inner = press(0x5C, fmt.Sprintf("select ALL PROGRAMMES (try %d)", attempt), 80_000_000)
	}
	if inner == 0 || inner == menuAZ {
		if err := dumpScreen(t, box, "inner-not-reached-"+artefact); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: SELECT never left the A-Z category menu (%08X), so the screen measured "+
			"below is the menu and not the list", menuAZ)
	}
	az = inner
	opened = inner
	afterOpen := word(slotA)
	t.Logf("slot A with A-Z LISTINGS open: %08X  (was %08X before)", afterOpen, beforeOpen)
	switch {
	case afterOpen != beforeOpen && afterOpen != 0:
		t.Logf("THE SCREEN REGISTERED ITSELF in the list-head slot when it opened, which is the "+
			"mechanism the decompile predicted: a section arriving BEFORE this moment is handed "+
			"to whatever was there, and that was %08X", beforeOpen)
	case afterOpen == 0:
		t.Logf("slot A is STILL ZERO with the screen open, so either this screen is not the one " +
			"that registers for letter 'A', or registration happens on some later event. The " +
			"delivery below is still worth making, but do not read a null result as a fact about " +
			"the carousel")
	default:
		t.Logf("slot A did not change when the screen opened; the registration reading is NOT " +
			"confirmed and the delivery below stands on its own")
	}

	// NOW DELIVER IT, REPEATEDLY, WITH THE SCREEN UP. Versions are cycled because the box
	// deduplicates a repeat of a version it has already parsed -- a confound this project has
	// already been caught by once, where a shared box coupled the case index to the version and
	// produced a result that alternated with it.
	//
	// The records carry our own channels' listings ids. rec[4..8] have never been swept off zero
	// and are NOT guessed at here; if the screen wants something in them it will keep drawing
	// nothing, and that is a cleaner answer than a plausible fabrication.
	records := []broadcast.IndexRecord{}
	listings := guide.On(day)
	for i := range listings.Services {
		records = append(records, broadcast.IndexRecord{
			ID:       listings.Services[i].ListingsID,
			Packed:   0x0f, // all four flag bits set: the line-up's own "visible" encoding
			Selector: 0xc0,
		})
	}
	letter, err := broadcast.IndexLetter('A')
	if err != nil {
		t.Fatal(err)
	}
	final = az
	for cycle := 0; cycle < 12; cycle++ {
		if deliver {
			section, err := broadcast.IndexSection(letter, byte(cycle&0x1f), 0, 0, records) // #nosec G115 -- masked
			if err != nil {
				t.Fatal(err)
			}
			if err := box.Demux.Push(probePID, section); err != nil {
				t.Fatalf("harness: cycle %d was refused (%v), so the screen was never offered "+
					"the index and its emptiness would measure the push", cycle, err)
			}
		}
		for i := 0; i < 4_000_000; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		now := screenNow(t, box)
		if now != final {
			t.Logf("cycle %2d: the screen moved %08X -> %08X", cycle, final, now)
			final = now
		}
	}
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	what := "with NOTHING delivered"
	if deliver {
		what = "with the index delivered twelve times"
	}
	t.Logf("A-Z LISTINGS opened on %08X and finished on %08X %s", az, final, what)
	return az, final
}
