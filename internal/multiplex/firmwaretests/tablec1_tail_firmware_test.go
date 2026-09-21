package firmwaretests_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE LAST FIVE BYTES OF A RECORD DO -- rec[4..8], the part the earlier
// sweep deliberately held at zero and therefore said nothing about.
//
// Forty-five cases: every bit of the five bytes on its own, plus each whole
// byte set to 0xFF to catch anything that only reacts to a multi-bit value. All
// of them ride in ONE section as separate records, so the sweep costs a single
// acquisition.
//
// TWO PLACES ARE WATCHED, because the earlier sweep could only have seen an
// effect that landed in the record itself:
//
//   - the ten-byte in-memory record, as before;
//   - the TWELVE BYTES OF HEADER in front of the array, which the parser
//     allocates (count*10 + 12) and is known to fill with the section number at
//     +0 and the record count at +2. The other eight are unaccounted for.
//
// If a byte changes neither, it is still not proved unread -- it could steer
// control flow without being stored, which the exclusive-PC differential would
// show and this would not. That limit is stated in the output rather than left
// for a reader to assume.
func TestWhatTheLastFiveBytesOfARecordDo(t *testing.T) {
	const probePID, extension = 0x52, 0x0100
	const idBase = 0xC000

	type probe struct {
		name string
		tail [5]byte // rec[4..8]
	}
	cases := []probe{{"baseline (all zero)", [5]byte{}}}
	for b := 0; b < 5; b++ {
		for bit := 0; bit < 8; bit++ {
			var tail [5]byte
			tail[b] = byte(1 << bit)
			cases = append(cases, probe{fmt.Sprintf("rec[%d] bit %d (0x%02X)", b+4, bit, 1<<bit), tail})
		}
	}
	for b := 0; b < 5; b++ {
		var tail [5]byte
		tail[b] = 0xFF
		cases = append(cases, probe{fmt.Sprintf("rec[%d] = 0xFF", b+4), tail})
	}

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

	payload := make([]byte, 0, len(cases)*9)
	for i, c := range cases {
		id := uint16(idBase + i)
		payload = append(payload, byte(id>>8), byte(id), 0x00, 0x00)
		payload = append(payload, c.tail[:]...)
	}
	section := sectionTableVersioned(0xC1, extension, 0, payload)
	declared := int(section[1]&0x0f)<<8 | int(section[2])
	if got := (declared - 9) / 9; got != len(cases) {
		t.Fatalf("harness: the firmware's own formula makes this %d records, not %d", got, len(cases))
	}
	if err := box.Demux.Push(probePID, section); err != nil {
		t.Fatalf("harness: the box refused the section: %v", err)
	}
	runUntil(t, box, transmitter, censusBudget, func(int) bool { return false })

	base, found := uint32(0), false
	size := box.RAM.Size()
	for off := uint32(0); off+uint32(len(cases))*10 <= size && !found; off += 2 {
		if uint16(box.RAM.Read(off, bus.Half)) != idBase {
			continue
		}
		if uint16(box.RAM.Read(off+10, bus.Half)) == idBase+1 {
			base, found = off, true
		}
	}
	if !found {
		t.Fatal("harness: no assembled array found, so this measured nothing")
	}

	readAt := func(off uint32, n int) []byte {
		out := make([]byte, n)
		for b := range out {
			out[b] = byte(box.RAM.Read(off+uint32(b), bus.Byte))
		}
		return out
	}
	hex := func(b []byte) string {
		parts := make([]string, len(b))
		for i, v := range b {
			parts[i] = fmt.Sprintf("%02x", v)
		}
		return strings.Join(parts, " ")
	}

	// The twelve bytes the allocation reserves in front of the records.
	if base >= 12 {
		t.Logf("header (12 bytes before the array): %s", hex(readAt(base-12, 12)))
	}
	t.Logf("array at guest 0x%08X, %d records", 0x80000000|base, len(cases))

	baseline := readAt(base, 10)
	t.Logf("%-22s tail=%s  out=%s", cases[0].name, hex(cases[0].tail[:]), hex(baseline))

	changed := 0
	for i := 1; i < len(cases); i++ {
		got := readAt(base+uint32(i)*10, 10)
		var diff []string
		for b := 2; b < 10; b++ {
			if got[b] != baseline[b] {
				diff = append(diff, fmt.Sprintf("out[%d] %02x->%02x", b, baseline[b], got[b]))
			}
		}
		if len(diff) == 0 {
			continue
		}
		changed++
		t.Logf("%-22s tail=%s  out=%s  %s",
			cases[i].name, hex(cases[i].tail[:]), hex(got), strings.Join(diff, ", "))
	}
	if changed == 0 {
		t.Logf("NO CASE CHANGED THE STORED RECORD: all %d single-bit and whole-byte variations of "+
			"rec[4..8] leave out[2..9] exactly as the baseline.", len(cases)-1)
		t.Log("That means these five bytes do not reach the ten-byte record. It does NOT mean they " +
			"are unread -- a byte can steer control flow without being stored, and only the " +
			"exclusive-PC differential would see that.")
	} else {
		t.Logf("%d of %d cases changed the stored record", changed, len(cases)-1)
	}
}
