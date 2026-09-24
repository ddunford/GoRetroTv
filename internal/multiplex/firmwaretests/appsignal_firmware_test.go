package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE PROBE FOR "WHAT MAKES THE BOX ASK FOR A CAROUSEL", run against the real
// firmware rather than reasoned about.
//
// The documented trigger is not DSM-CC. OpenTV 1.x carries module "flows", and
// "an OpenTV flow always contains a directory module, which is automatically
// downloaded (before any other module) by a STB when an application is
// signalled in the programme stream" (Fagerqvist & Marcussen, LTU 2000:075,
// sections 3.3-3.6). We signal no application anywhere. The later firmware
// trace established that this box runs its EPG from flash and never calls
// getCodeModule at 0x8008E7C8; that is the application-module marker.
// 0x800BECF0 is not one: the same Huffman decoder also expands ordinary title
// and 0xB2 guide-row text, and those paths now execute in the shipped signal.
//
// On a standard DVB receiver an application is signalled in the service's PMT,
// which the receiver finds through the PAT on PID 0x00. WE BROADCAST NEITHER.
// But the box wanting them is UNPROVEN -- the measured record mentions PAT and
// PMT nowhere in six thousand lines, and no observation has shown a PID 0x00
// filter. So the honest first question is not "build a PMT" but "does this box
// ever ask for one", and that is answered by dumping what it has armed.
//
// The project's own rule for this class of question: "the box is not asking for
// it" is a SYMPTOM, not a finding -- go and dump all sixteen match units
// unconditionally. That is what this does. It asserts its own subject: a box
// that armed nothing at all is a harness failure, not a result.
func TestWhatTheBoxHasArmedAfterAcquiring(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	// Let it acquire and take the block's listings, so the filters are the ones
	// a working box holds rather than a mid-boot snapshot of them.
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered, so these filters are not a working box's",
			registered, want)
	}

	armed := box.Demux.ArmedPIDs()
	if len(armed) == 0 {
		t.Fatal("harness: the box has no PID armed at all after acquiring, which is a broken instrument rather than a finding")
	}
	t.Logf("armed PIDs after acquisition: %v", armed)

	var sawPAT bool
	for _, pid := range armed {
		if pid == 0x00 {
			sawPAT = true
		}
	}
	t.Logf("PID 0x00 (PAT) armed: %v", sawPAT)

	// All sixteen match units, unconditionally -- units 8..15 carry their rule
	// in the HIGH half of the word they share, and a reader that took only the
	// low halfword is precisely how this project once concluded the box was
	// asking for nothing.
	for unit := uint8(0); unit < 16; unit++ {
		var row string
		any := false
		for b := uint8(0); b < 8; b++ {
			m, ok := box.Demux.Match(unit, b)
			if !ok {
				break
			}
			any = true
			row += " " + hexByte(m.Value) + "/" + hexByte(m.Mask)
		}
		if any {
			t.Logf("match unit %2d:%s", unit, row)
		}
	}
}

func hexByte(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0xf]})
}
