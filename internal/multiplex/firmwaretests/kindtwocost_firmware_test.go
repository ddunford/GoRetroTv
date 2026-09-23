package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT LINE-UP KIND 2 COSTS THE SCREENS THAT READ THE TITLE STORE.
//
// Kind 2 makes the ALL CHANNELS grid draw programmes off the broadcast alone -- the row type is
// map(kind) and only 2 maps to 2. It also empties the title store: with every channel at kind 2,
// ZERO of 21 programmes register, where kind 1 registers all of them.
//
// The now-and-next banner reads that store and is the only other screen that has ever shown a
// programme from this broadcast. So the trade has to be SEEN rather than inferred from a
// registration count: "nothing registered, therefore the banner is empty" is exactly the kind of
// plausible step this project keeps having to withdraw.
//
// BOTH RUNS, ONE PROBE. The same walk with kind 1 and with kind 2, each dumping its banner, so the
// comparison is against this machine on this day rather than against a hash from another run.
//
// IT ONLY READS.
func TestWhatKindTwoCostsTheBanner(t *testing.T) {
	one := bannerAtKind(t, 1, "kindcost-banner-kind1.png")
	two := bannerAtKind(t, 2, "kindcost-banner-kind2.png")
	t.Logf("=== banner with kind 1: %08X", one)
	t.Logf("=== banner with kind 2: %08X", two)
	if one == two {
		t.Logf("the banner is IDENTICAL either way, so kind 2 costs it nothing and the empty " +
			"title store does not reach this screen. Read both artefacts to be sure.")
		return
	}
	t.Logf("THE BANNER DIFFERS. Read .artifacts/kindcost-banner-kind1.png against -kind2.png: if " +
		"the programme is gone, kind 2 buys the grid at the banner's expense and is not the " +
		"shipping answer on its own.")
}

func bannerAtKind(t *testing.T, kind byte, artefact string) uint32 {
	t.Helper()
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	listings := guide.On(day)
	for i := range listings.Services {
		listings.Services[i].Kind = kind
	}
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	// A FIXED WARM-UP: waiting for registrations that never come burns the budget and the first
	// key press lands too late to be taken.
	runUntil(t, box, transmitter, 60_000_000, registeringProgrammes(box, want, &registered))
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })
	t.Logf("kind %d: %d of %d programmes registered", kind, registered, want)

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	drew := pressAndLetItFinish(t, box, pump, keyTVGuide, 80_000_000)
	if drew == 0 {
		t.Fatalf("harness: the banner never settled at kind %d, so there is nothing to compare", kind)
	}
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	t.Logf("kind %d: the banner drew %08X (%s)", kind, drew, artefact)
	return drew
}
