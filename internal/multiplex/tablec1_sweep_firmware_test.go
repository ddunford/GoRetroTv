package multiplex_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/dvb"
	"github.com/ddunford/goretrotv/internal/multiplex"
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
	// exclusive counts the guest PCs a delivered section causes that do not run
	// without it. table 0 means deliver nothing, which is the control.
	exclusive := func(t *testing.T, table byte, pid, extension uint16, control map[uint32]int) (map[uint32]int, int) {
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
		if at := runUntil(t, box, transmitter, 120_000_000,
			registeringProgrammes(box, want, &registered)); at < 0 {
			t.Fatalf("harness: only %d of %d programmes registered", registered, want)
		}
		if table != 0 {
			marker := []byte{0xDE, 0xAD, 0xC1, 0x05, 0x5E, 0xC7, 0x10, 0x4E}
			if err := box.Demux.Push(pid, sectionTableVersioned(table, extension, 0, marker)); err != nil {
				t.Logf("    (not delivered: %v)", err)
				return nil, -1
			}
		}
		seen := make(map[uint32]int, 8192)
		runUntil(t, box, transmitter, 20_000_000, func(int) bool {
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

	control, _ := exclusive(t, 0, 0, 0, nil)
	t.Logf("control: %d distinct PCs with nothing delivered", len(control))

	t.Log("=== which armed PID carries a 0xC1 the consumer reacts to? (exclusive PCs) ===")
	for _, c := range []struct {
		name string
		pid  uint16
	}{
		{"0x10 (NIT)", 0x10}, {"0x11 (SDT/BAT)", 0x11}, {"0x14 (TDT/TOT)", 0x14},
		{"0x33 (titles)", 0x33}, {"0x34 (titles)", 0x34}, {"0x52", 0x52},
		{"0x00 (PAT, not armed)", 0x00},
	} {
		if _, n := exclusive(t, 0xC1, c.pid, 0x0100, control); n >= 0 {
			t.Logf("  PID %-22s -> %4d exclusive PCs", c.name, n)
		}
	}

	t.Log("=== which extension, on PID 0x52? (exclusive PCs) ===")
	for _, ext := range []uint16{0x0000, 0x0001, 0x0100, 0x01FF, 0x0200, 0x8000, 0xFF00} {
		if _, n := exclusive(t, 0xC1, 0x52, ext, control); n >= 0 {
			t.Logf("  extension %04X -> %4d exclusive PCs", ext, n)
		}
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
