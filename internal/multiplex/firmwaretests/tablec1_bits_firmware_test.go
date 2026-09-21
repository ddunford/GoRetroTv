package firmwaretests_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT EVERY BIT OF rec[2] AND rec[3] DOES, mapped by setting one at a time.
//
// The parser at 0x800C4C34 is known to read three things out of those two
// bytes: (rec[2] & 0xF0) and (rec[3] & 0xC0) >> 6 are OR-ed into one output
// byte, and rec[2] & 0x08 is a flag. That leaves rec[2] & 0x07 and rec[3] & 0x3F
// unaccounted for, and "the decoded part is all there is" is an assumption
// rather than a reading -- the walk continues past where it was followed.
//
// SEVENTEEN CASES IN ONE SECTION, not seventeen boxes. The payload is an array,
// so each case can be its own RECORD: one id per case, one bit set per case,
// and the box assembles them all into a contiguous array this then reads at the
// ten-byte stride already established. That turns what would be seventeen
// acquisitions into one, which matters because this package is the expensive
// one and has about five minutes of headroom against its timeout.
//
// The baseline record with both bytes zero is the control: every other case is
// read as a DIFFERENCE from it, so an output byte that is nonzero for reasons
// of its own cannot be mistaken for a bit's effect.
func TestWhatEachBitOfRecordBytes2And3Does(t *testing.T) {
	const probePID, extension = 0x52, 0x0100
	const idBase = 0xB000

	type probe struct {
		name         string
		byte2, byte3 byte
	}
	cases := []probe{{"baseline (both zero)", 0x00, 0x00}}
	for bit := 0; bit < 8; bit++ {
		cases = append(cases, probe{fmt.Sprintf("rec[2] bit %d (0x%02X)", bit, 1<<bit), byte(1 << bit), 0x00})
	}
	for bit := 0; bit < 8; bit++ {
		cases = append(cases, probe{fmt.Sprintf("rec[3] bit %d (0x%02X)", bit, 1<<bit), 0x00, byte(1 << bit)})
	}
	// COMBINATIONS, AS PREDICTIONS. Single bits give a rule that FITS; these are
	// chosen so the rule can be wrong. If out[3] is four 2-bit fields packed
	// MSB-first from rec[2] bits 0..3, each 1 when set and 2 when clear, then
	// 0x0F must give 01 01 01 01 = 0x55 and 0x05 must give 01 10 01 10 = 0x66.
	// A rule that merely counted set bits, or OR-ed them, gives neither.
	cases = append(cases,
		probe{"rec[2]=0x0F predicts out[3]=0x55", 0x0F, 0x00},
		probe{"rec[2]=0x05 predicts out[3]=0x66", 0x05, 0x00},
		probe{"rec[2]=0xF0|rec[3]=0xC0 -> out[2]=0xF3", 0xF0, 0xC0},
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

	payload := make([]byte, 0, len(cases)*9)
	for i, c := range cases {
		id := uint16(idBase + i)
		payload = append(payload, byte(id>>8), byte(id), c.byte2, c.byte3, 0, 0, 0, 0, 0)
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

	// Locate the assembled array by its first two ids, so a stray copy of one id
	// cannot be mistaken for the array itself.
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
	t.Logf("array at guest 0x%08X, %d records at a ten-byte stride", 0x80000000|base, len(cases))

	read := func(i int) []byte {
		out := make([]byte, 10)
		for b := range out {
			out[b] = byte(box.RAM.Read(base+uint32(i)*10+uint32(b), bus.Byte))
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

	baseline := read(0)
	t.Logf("%-24s in[2,3]=%02x,%02x  out=%s", cases[0].name, cases[0].byte2, cases[0].byte3, hex(baseline))
	for i := 1; i < len(cases); i++ {
		got := read(i)
		var diff []string
		for b := 2; b < 10; b++ { // bytes 0..1 are the id and differ by construction
			if got[b] != baseline[b] {
				diff = append(diff, fmt.Sprintf("out[%d] %02x->%02x", b, baseline[b], got[b]))
			}
		}
		effect := "no change"
		if len(diff) > 0 {
			effect = strings.Join(diff, ", ")
		}
		t.Logf("%-38s in[2,3]=%02x,%02x  out=%s  %s",
			cases[i].name, cases[i].byte2, cases[i].byte3, hex(got), effect)
	}

	// The predictions, asserted. A fitted rule that cannot fail is not a finding.
	for _, want := range []struct {
		name  string
		index int
		at    int
		value byte
	}{
		{"rec[2]=0x0F -> out[3]", len(cases) - 3, 3, 0x55},
		{"rec[2]=0x05 -> out[3]", len(cases) - 2, 3, 0x66},
		{"rec[2]=0xF0,rec[3]=0xC0 -> out[2]", len(cases) - 1, 2, 0xF3},
	} {
		if got := read(want.index)[want.at]; got != want.value {
			t.Errorf("%s: predicted %02x, measured %02x -- the decode is wrong",
				want.name, want.value, got)
		} else {
			t.Logf("PREDICTION HELD  %s = %02x", want.name, got)
		}
	}
}
