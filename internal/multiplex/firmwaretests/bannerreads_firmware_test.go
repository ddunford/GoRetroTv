package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DOES THE NOW-AND-NEXT BANNER READ THE LINE-UP THE GRID IGNORES?
//
// The ALL CHANNELS grid draws no rows and never touches the box's own service records -- zero reads
// of them in over half a million during its draw. That is a negative about the grid, and a negative
// alone cannot say whether those records are the WRONG place to look or simply a place the grid
// forgot.
//
// **THE BANNER IS THE CONTROL THIS QUESTION HAS BEEN MISSING.** Press `tv guide` and the box draws
// a now-and-next bar carrying a real programme off our broadcast -- so that screen demonstrably
// resolves a channel to its listings, which is exactly what the grid fails to do. If the banner
// reads the line-up records, they are the channel source and the grid's silence about them is the
// fault. If the banner ignores them too, they are an acquisition-time structure nothing draws from
// and the real channel list is somewhere neither screen has led us to yet.
//
// EITHER ANSWER MOVES THE WORK, which is what makes it worth a run.
//
// IT ONLY READS, and the screen it measures is pinned so it cannot quietly measure another.
func TestWhetherTheNowAndNextBannerReadsTheLineUp(t *testing.T) {
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
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	listings := guide.On(day)
	records := serviceRecords(t, box, listings.Services)
	lo, hi := records[0].base, records[len(records)-1].base+18
	t.Logf("the line-up is %d records at guest %08X..%08X", len(records), 0x80000000|lo, 0x80000000|hi)

	// The distinctive identifiers, on the same footing as the grid probe: a value only carries
	// information where the firmware has no other reason to hold it.
	const distinctive = 250
	ours := map[uint32]string{}
	for i := range listings.Services {
		s := &listings.Services[i]
		if uint32(s.Channel) >= distinctive {
			ours[uint32(s.Channel)] = s.Name + " channel number"
		}
		if uint32(s.ListingsID) >= distinctive {
			ours[uint32(s.ListingsID)] = s.Name + " listings id"
		}
	}

	var anyRead int
	inArray := map[uint32]int{}
	valueHits := map[string]int{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Fetch || a.Write {
			return
		}
		anyRead++
		pc := box.Machine.Core.State().PC &^ 1
		if p := a.Virtual & 0x1fffffff; p >= lo && p < hi {
			inArray[pc]++
		}
		if what, mine := ours[a.Value]; mine {
			valueHits[hexPC(pc)+" "+what]++
		}
	}}

	before := screenNow(t, box)
	if err := box.CSI.Key(0x80, 0); err != nil { // tv guide: the now-and-next banner
		t.Fatal(err)
	}
	watching = true
	stable, last, banner := 0, before, uint32(0)
	for i := 0; i < 80_000_000; i++ {
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
			banner = now
			if stable >= 4 {
				break
			}
			continue
		}
		stable, last = 0, now
	}
	watching = false
	if banner == 0 || banner == before {
		t.Fatalf("harness: the tv guide key drew nothing new (%08X), so this measured no banner",
			before)
	}
	if err := dumpScreen(t, box, "banner-now-and-next.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the banner drew %08X over %d data reads", banner, anyRead)
	if anyRead == 0 {
		t.Fatal("harness: the observer saw no data read at all, so its zeros are the instrument")
	}

	total := 0
	for _, n := range inArray {
		total += n
	}
	if total > 0 {
		pcs := make([]uint32, 0, len(inArray))
		for pc := range inArray {
			pcs = append(pcs, pc)
		}
		sort.Slice(pcs, func(a, b int) bool { return inArray[pcs[a]] > inArray[pcs[b]] })
		t.Logf("VERDICT: THE BANNER READS THE LINE-UP -- %d reads from %d PCs, where the grid read "+
			"it zero times. The service records ARE the channel source, and the grid not touching "+
			"them is the fault rather than a red herring.", total, len(pcs))
		for i, pc := range pcs {
			if i >= 10 {
				break
			}
			t.Logf("    %08X  %d reads", pc, inArray[pc])
		}
	} else {
		t.Logf("VERDICT: the banner does NOT read the line-up either, in %d data reads -- and it is "+
			"drawing a real programme for a real channel while doing so. So the service records "+
			"are an acquisition-time structure that NO screen draws from, and the channel list the "+
			"grid wants is somewhere neither screen has led us to.", anyRead)
	}

	if len(valueHits) == 0 {
		t.Log("and not one read returned a distinctive channel number or listings id of ours, on a " +
			"screen that is displaying one of our programmes -- so the identifiers are resolved " +
			"before this draw, not during it.")
		return
	}
	keys := make([]string, 0, len(valueHits))
	for k := range valueHits {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool { return valueHits[keys[a]] > valueHits[keys[b]] })
	t.Logf("%d (PC, identifier) pairs read one of our numbers while the banner drew:", len(keys))
	for i, k := range keys {
		if i >= 12 {
			break
		}
		t.Logf("    %-52s %d", k, valueHits[k])
	}
}
