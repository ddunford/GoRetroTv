package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE DISPATCH BOUNDARIES, HELD AS PREDICTIONS THE BOX CAN FALSIFY.
//
// 0x800C4C34 was decompiled rather than sampled, and it ends in a four-way branch on the
// table-id extension:
//
//	0x0000                  one list head
//	'A'..'Z' (0x41..0x5A)   twenty-six list heads at 0x800C51A0
//	0x00FF                  one list head, its own pointer at 0x800C51A4
//	0x0100..0x01CF          a table at 0x800C51A8, slot (ext & 0x0F) * 4 + ((ext & 0xC0) >> 6)
//	anything else           falls through -- the record array is built and LEAKED
//
// The sixteen-by-four slot arithmetic is the part worth testing, because it answers a question the
// record has carried open since the first sweep -- what 0x0000 versus 0x0100 select. They are two
// different FAMILIES: 0x0000 and the letters are the alphabetical index behind A-Z LISTINGS, and
// 0x0100..0x01CF is sixteen genres by the four six-hour blocks the title tables' low two bits
// already select. That is the shape of the guide's own menu, which names ALL CHANNELS, MOVIES,
// SPORTS, ENTERTAINMENT, NEWS & DOCUMENTARIES, KIDS, MUSIC & RADIO and SPECIALIST.
//
// A STATIC READING OF THIS FIRMWARE HAS BEEN BACKWARDS TWICE, so it is not filed until the box
// agrees. These three cases are chosen because the decompiled rule can FAIL them, which a case
// re-sampling the middle of an accepted range cannot:
//
//	0x00FF  must be ACCEPTED -- and the earlier sweep's rule, "high byte 0x00 or 0x01 AND low byte
//	        0x00", says it must be rejected. The two readings disagree here, so one dies.
//	0x01CF  must be ACCEPTED -- the top of the genre range, low byte 0xCF, which the earlier rule
//	        also rejects.
//	0x01D0  must be REJECTED -- one past the top. Nothing but a real bound produces a cliff here.
func TestTheIndexDispatchBoundariesAreWhereTheDecompilerSaysTheyAre(t *testing.T) {
	if testing.Short() {
		t.Skip("four acquisitions of a real box")
	}
	control := indexCensusControl(t)
	t.Logf("control: %d distinct PCs with nothing delivered", len(control))

	accepted := func(ext uint16) int {
		n := indexCensus(t, ext, control)
		t.Logf("  extension %04X -> %4d exclusive PCs", ext, n)
		return n
	}
	ff := accepted(0x00ff)
	top := accepted(0x01cf)
	past := accepted(0x01d0)

	// The floor a 0xC1 reaches when it passes the table check and fails the extension one was
	// measured at 22 exclusive PCs; an accepted one reached 492 and 544. The bounds are loose
	// because the exact counts belong to this fixture and this budget -- what must hold is the
	// CLIFF, and that it falls between 0x01CF and 0x01D0 rather than somewhere the older rule put it.
	if ff < 100 {
		t.Errorf("extension 0x00FF woke only %d exclusive PCs, so the decompiled 0x00FF arm is "+
			"wrong and the earlier 'low byte must be 0x00' rule stands", ff)
	}
	if top < 100 {
		t.Errorf("extension 0x01CF woke only %d exclusive PCs, so the genre range does not reach "+
			"the top of 0x0100..0x01CF and the slot arithmetic is not what was decompiled", top)
	}
	if past*4 >= top {
		t.Errorf("extension 0x01D0 woke %d exclusive PCs against 0x01CF's %d -- there is no bound "+
			"at 0x01CF, so the sixteen-by-four reading of the genre range is wrong", past, top)
	}
}

// indexCensusControl and indexCensus are the same differential the table 0xC1 sweep uses: a box
// that is delivered nothing, and a box that is delivered one section, counting the guest PCs the
// section alone causes. A section that is decoded and thrown away still reaches a floor of a
// couple of dozen, so the question is never "did anything happen" but "did it do the work".
func indexCensusControl(t *testing.T) map[uint32]int {
	t.Helper()
	seen, _ := indexCensusRun(t, false, 0, nil)
	return seen
}

func indexCensus(t *testing.T, extension uint16, control map[uint32]int) int {
	t.Helper()
	_, n := indexCensusRun(t, true, extension, control)
	return n
}

func indexCensusRun(t *testing.T, deliver bool, extension uint16,
	control map[uint32]int) (map[uint32]int, int) {
	t.Helper()
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
	if at := runUntil(t, box, transmitter, 15_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if deliver {
		marker := []byte{0xDE, 0xAD, 0xC1, 0x05, 0x5E, 0xC7, 0x10, 0x4E}
		if err := box.Demux.Push(0x52, sectionTableVersioned(0xC1, extension, 0, marker)); err != nil {
			t.Fatalf("harness: extension %04X was not delivered at all (%v), so its count would "+
				"measure the push and not the consumer", extension, err)
		}
	}
	seen := make(map[uint32]int, 8192)
	runUntil(t, box, transmitter, censusBudget, func(int) bool {
		seen[box.Machine.Core.State().PC&^1]++
		return false
	})
	if control == nil {
		return seen, 0
	}
	n := 0
	for pc := range seen {
		if control[pc] == 0 {
			n++
		}
	}
	return seen, n
}
