package firmwaretests_test

import (
	"testing"
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
	sig := acceptanceSignature(t)
	t.Logf("the acceptance path is %d addresses; a refused delivery hits %d of them by accident",
		len(sig.pcs), sig.floor)

	// EVERY CASE IS READ AGAINST THE SAME MEASURED FLOOR. This used to difference each delivery
	// against a box that had been delivered NOTHING, which counts the whole cost of a section
	// ARRIVING as though it were the dispatcher running -- and that cost went up by a factor of
	// forty when the 0xB2 descriptor shipped, taking the cliff at 0x01CF with it. The three
	// extensions here are the three the decompiled rule can FAIL; the signature they are measured
	// against is built from extensions no probe is trying to decide.
	reached := func(ext uint16) int {
		t.Helper()
		n := sig.overlap(indexRun(t, indexPID, ext, true))
		t.Logf("  extension %04X -> %4d of %d addresses of the acceptance path", ext, n, len(sig.pcs))
		return n
	}
	ff := reached(0x00ff)
	top := reached(0x01cf)
	past := reached(0x01d0)

	if ff < len(sig.pcs)/2 {
		t.Errorf("extension 0x00FF reproduced only %d of the acceptance path's %d addresses, so "+
			"the decompiled 0x00FF arm is wrong and the earlier 'low byte must be 0x00' rule "+
			"stands", ff, len(sig.pcs))
	}
	if top < len(sig.pcs)/2 {
		t.Errorf("extension 0x01CF reproduced only %d of the acceptance path's %d addresses, so "+
			"the genre range does not reach the top of 0x0100..0x01CF and the slot arithmetic is "+
			"not what was decompiled", top, len(sig.pcs))
	}
	if past > sig.floor*2 {
		t.Errorf("extension 0x01D0 reproduced %d of the acceptance path against a floor of %d -- "+
			"there is no bound at 0x01CF, so the sixteen-by-four reading of the genre range is "+
			"wrong", past, sig.floor)
	}
}
