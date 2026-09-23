package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT IS BEHIND "Press SELECT to view" ON THE WORKING GRID.
//
// The ALL CHANNELS grid now draws programmes, and it offers the same affordance the A-Z listings
// screen does. Nothing in this project has ever been through it, because until today there was
// never a programme on the grid to select.
//
// It is worth going through for two reasons. It is a SCREEN -- one more thing the demo either does
// or does not do, and nobody has looked. And it is the likeliest home for the 0xB2 descriptor's
// other scalars: +10, the three-bit +9 and the one-bit +11 are all inert to the grid, so whatever
// they carry is read somewhere else, and the detail screen for the very programme they describe is
// the first place to look.
//
// THE SCALARS ARE GIVEN DISTINCT VALUES so that if any of them reaches this screen it arrives
// legible rather than as another zero. Which channel carries which is logged, so a number on the
// screen can be traced back to the field that put it there.
//
// THE ROUTE IS PINNED AT THE SCREEN IT SELECTS FROM, not at the one it lands on -- the destination
// is the thing under test and this project has measured the wrong screen five times.
//
// IT ONLY READS.
func TestWhatIsBehindSelectOnTheGrid(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	listings := guide.On(day)
	for i := range listings.Services {
		// Distinct and small: +9 is three bits and +11 is one, so nothing here may exceed them.
		listings.Services[i].RowAt10 = byte(0x10 + i) // #nosec G115 -- six services
		listings.Services[i].RowAt9 = byte(i % 8)     // #nosec G115 -- three bits
		listings.Services[i].RowAt11 = byte(i % 2)    // #nosec G115 -- one bit
		t.Logf("%-14s  +10=%#02x  +9=%d  +11=%d", listings.Services[i].Name,
			listings.Services[i].RowAt10, listings.Services[i].RowAt9, listings.Services[i].RowAt11)
	}

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
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := azPressFunc(t, box, pump)
	grid := openAllChannelsFinished(t, press, ".artifacts/griddetail-grid.png")
	// PAST THE SETTLE before selecting: the rows paint in bursts that hold still across four
	// samples and carry on afterwards, and a SELECT sent mid-paint is swallowed.
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	filled := screenNow(t, box)
	if err := dumpScreen(t, box, "griddetail-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid opened on %08X and finished on %08X", grid, filled)

	detail := uint32(0)
	for attempt := 1; attempt <= 6 && detail == 0; attempt++ {
		detail = press(keySelect, fmt.Sprintf("select the highlighted programme (%d)", attempt), 80_000_000)
	}
	if detail == 0 || detail == filled {
		if err := dumpScreen(t, box, "griddetail-nothing.png"); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("harness: SELECT never left the grid (%08X), so there is nothing behind it to "+
			"describe. Read .artifacts/griddetail-nothing.png", filled)
	}
	final := detail
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
		if i%1_000_000 == 0 {
			final = screenNow(t, box)
		}
	}
	if err := dumpScreen(t, box, "griddetail-screen.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("SELECT opened %08X and it finished on %08X", detail, final)
	t.Logf("READ .artifacts/griddetail-screen.png -- it is the only thing that says what is there, " +
		"and whether any of the 0xB2's other scalars reached it")
}
