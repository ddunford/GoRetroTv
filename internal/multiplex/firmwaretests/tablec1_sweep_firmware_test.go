package firmwaretests_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/dvb"
)

// WHICH PID AND WHICH EXTENSION DOES THE 0xC1 CONSUMER ACTUALLY WANT?
//
// A 0xC1 section pushed on PID 0x52 with extension 0x0100 is parsed and its
// payload copied into the heap. Neither of those two values is established as
// correct -- they are the ones that happened to work. This sweeps both.
//
// It is also a test of this project's own reading of match unit 10.
// `c1/ff 01/fe ff/00` decodes as "table id 0xC1 exactly, extension high byte
// masked with 0xFE against 0x01 -- so 0x00 or 0x01 -- extension low byte
// ignored". THE DEMUX MODEL DOES NOT ENFORCE THAT: pushFilter writes any
// section delivered to an armed PID straight into the ring, and the match units
// are the GUEST's own filtering. So if the guest accepts exactly the extensions
// the unit describes and refuses the rest, the decode is confirmed by the
// firmware rather than by argument.
//
// THE SIGNAL IS THE DIFFERENTIAL, NOT A MARKER IN MEMORY, and that correction
// cost two wrong readings worth recording. A marker payload DOES reach the heap
// -- but it reaches the heap for table 0xA5, 0x9E, 0xC2 and 0x42 as well, none
// of which the box filters for and none of which this transmitter sends. So the
// box copies whatever arrives on an armed PID out of the ring, and "the payload
// was stored" measures that copy rather than acceptance. The count of guest PCs
// that run ONLY when a section is delivered does discriminate: 0xC1 wakes 544
// and 0xA5 wakes 8, and those 8 are a subset of the 544.
//
// Each case therefore runs on its OWN acquired box against one shared control,
// because sharing a box couples the case index to the section version, and the
// first version of this sweep came back alternating exactly with that index.
func TestWhichPIDAndExtensionTheTableC1ConsumerWants(t *testing.T) {
	sig := acceptanceSignature(t)
	t.Logf("the acceptance path is %d addresses; a refused delivery hits %d of them by accident",
		len(sig.pcs), sig.floor)

	report := func(label string, pid, extension uint16) int {
		t.Helper()
		n := sig.overlap(indexRun(t, pid, extension, true))
		t.Logf("  PID %#04x ext %04X -> %4d of %d   %s", pid, extension, n, len(sig.pcs), label)
		return n
	}

	// THE EXTENSION IS A LETTER. 0x800C4F94 compares it against 65 and 91 -- 'A' and one past 'Z'
	// -- and uses (extension + bias) * 4 to index a table of list heads at 0x800C51A0, freeing the
	// list outright for anything outside that range. The earlier reading here, "low byte must be
	// 0x00", was an artefact of which values this sweep happened to sample: 0x0001 and 0x01FF were
	// tried and the letters never were.
	//
	// 'A' is left out: it is one of the two deliveries the signature is built from, so its overlap
	// is a tautology rather than a measurement.
	for _, c := range []struct {
		ext    uint16
		letter bool
	}{{0x0040, false}, {0x004D, true}, {0x005A, true}, {0x005B, false}} {
		label := "outside 'A'..'Z'"
		if c.letter {
			label = "a LETTER: " + string(rune(c.ext))
		}
		n := report(label, indexPID, c.ext)
		if c.letter && n < len(sig.pcs)/2 {
			t.Errorf("extension %04X is a letter and reproduced only %d of the acceptance path's "+
				"%d addresses, so the 'A'..'Z' arm is not where the decompiler puts it",
				c.ext, n, len(sig.pcs))
		}
		if !c.letter && n > sig.floor*2 {
			t.Errorf("extension %04X is outside 'A'..'Z' and still reproduced %d of the "+
				"acceptance path against a floor of %d, so the letter bound is wrong",
				c.ext, n, sig.floor)
		}
	}

	onTarget := report("the target", indexPID, 0x0100)
	wrongPID := report("a PID nothing dispatches 0xC1 from", 0x11, 0x0100)
	wrongExt := report("an extension no arm claims", indexPID, 0x0200)

	// A SUB-FINDING WORTH LOGGING RATHER THAN ASSERTING. The letters reproduce the acceptance path
	// WHOLE and 0x0100 reproduces all but about thirty of it, every time. The signature is built
	// from 0x0000 and 'A' -- the dispatcher's first two arms -- so those thirty addresses are work
	// those two share and the genre arm does not, which is exactly the family split the record
	// already argues for on other grounds: 0x0000 and the letters are the alphabetical index, and
	// 0x0100..0x01CF is sixteen genres by four blocks. It is not asserted because the exact number
	// belongs to this fixture, and pinning it would turn an unrelated change into a false finding.
	t.Logf("the letters reproduce the path whole (%d of %d) and the genre arm reproduces %d, so "+
		"about %d addresses belong to the alphabetical family rather than to dispatch itself",
		len(sig.pcs), len(sig.pcs), onTarget, len(sig.pcs)-onTarget)

	// THE VERDICTS ARE STATED AGAINST THE MEASURED FLOOR, not against constants. An accepted
	// delivery reproduces most of the acceptance path; a refused one reproduces about as much of
	// it as a refused delivery that helped define nothing. The old form of this test compared
	// every case against a box that had been delivered NOTHING, which counted the cost of a
	// section ARRIVING as evidence of acceptance and stopped discriminating the day that cost
	// went up.
	if onTarget < len(sig.pcs)/2 {
		t.Errorf("0xC1 on PID %#04x extension 0x0100 reproduced only %d of the acceptance path's "+
			"%d addresses, so either the consumer is gone or this instrument is broken",
			uint16(indexPID), onTarget, len(sig.pcs))
	}
	if wrongPID > sig.floor*2 {
		t.Errorf("the same section on PID 0x11 reproduced %d of the acceptance path against a "+
			"floor of %d -- the PID does not discriminate", wrongPID, sig.floor)
	}
	if wrongExt > sig.floor*2 {
		t.Errorf("extension 0x0200 reproduced %d of the acceptance path against a floor of %d -- "+
			"the extension does not discriminate", wrongExt, sig.floor)
	}
}

// sectionC1Versioned varies the version field so consecutive cases are not
// taken for a repeat of one the box has already parsed.
func sectionTableVersioned(table byte, extension uint16, version byte, payload []byte) []byte {
	body := make([]byte, 0, 5+len(payload))
	body = append(body, byte(extension>>8), byte(extension&0xff),
		0xC1|(version&0x1f)<<1, 0x00, 0x00)
	body = append(body, payload...)
	length := len(body) + 4
	section := append([]byte{table, 0xB0 | byte(length>>8), byte(length)}, body...)
	crc := dvb.MPEGCRC32(section)
	return append(section, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}
