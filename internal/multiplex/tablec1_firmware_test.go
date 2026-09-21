package multiplex_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/dvb"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// MATCH UNIT 10 ASKS FOR A TABLE NOBODY SENDS, and this finds out whether
// answering it wakes anything.
//
// At rest the box's filters are annotated in the record as NIT, SDT, BAT and
// TOT -- and one more is listed with no annotation at all:
//
//	unit 10: c1/ff 01/fe ff/00 ...
//
// A DVB match unit skips section_length, so byte 0 is the table id and bytes
// 1..2 are the extension -- which is how unit 3 reads as "BAT, bouquet 0x1000"
// and unit 1 as "NIT, network 0x0020", exactly as the record labels them. Unit
// 10 therefore asks for a LONG-FORM SECTION WITH TABLE ID 0xC1 and an extension
// whose high byte is 0x00 or 0x01. We have never transmitted a 0xC1 section:
// the builders emit 0x40, 0x42, 0x4A, 0x70, 0x73 and 0xA0..0xA3 and nothing
// else.
//
// THE INSTRUMENT IS A DIFFERENTIAL, because it needs no prior knowledge of
// which guest code would consume this. Two boxes are restored from the same
// snapshot, acquire the same listings and run the same budget; one of them is
// given the 0xC1 section. The emulator is deterministic by construction, so any
// guest PC executed in the test run and never in the control is something the
// section caused. That is the same technique the record used to establish that
// a key press "runs far more, over addresses the idle loop never touches".
//
// It asserts its own subject: if the push is refused, or the control and test
// runs do not execute the same number of instructions before the push, the
// instrument is broken and says so rather than reporting a quiet zero.
func TestWhetherAnythingWantsTableC1(t *testing.T) {
	const (
		tableC1   = 0xC1
		probePID  = 0x52 // the only armed PID this transmitter never feeds
		extension = 0x0100
	)

	run := func(t *testing.T, push bool) (map[uint32]bool, uint64) {
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
		if push {
			if err := box.Demux.Push(probePID, sectionC1(tableC1, extension)); err != nil {
				t.Fatalf("harness: the box refused the probe section: %v", err)
			}
		}
		seen := make(map[uint32]bool, 8192)
		start := box.Machine.Retired
		runUntil(t, box, transmitter, 20_000_000, func(int) bool {
			seen[box.Machine.Core.State().PC&^1] = true
			return false
		})
		return seen, box.Machine.Retired - start
	}

	control, controlRan := run(t, false)
	test, testRan := run(t, true)
	if controlRan != testRan {
		t.Fatalf("harness: control ran %d instructions and the test ran %d; the runs are not comparable",
			controlRan, testRan)
	}
	t.Logf("control executed %d distinct PCs, test %d, over %d instructions each",
		len(control), len(test), controlRan)

	var only []uint32
	for pc := range test {
		if !control[pc] {
			only = append(only, pc)
		}
	}
	sort.Slice(only, func(a, b int) bool { return only[a] < only[b] })

	if len(only) == 0 {
		t.Log("VERDICT: the 0xC1 section woke NOTHING. It was delivered to an armed filter and no guest")
		t.Log("         code ran that does not run without it. Unit 10 wants a 0xC1 section, but either")
		t.Log("         not on PID 0x52, or not with this extension, or not in this state.")
		return
	}
	t.Logf("VERDICT: %d guest PCs ran ONLY when the 0xC1 section was delivered:", len(only))
	for i, pc := range only {
		if i >= 40 {
			t.Logf("    ... and %d more", len(only)-i)
			break
		}
		t.Logf("    %08X", pc)
	}
}

// sectionC1 builds a minimal long-form private section: a real header, a real
// CRC, and a payload of zeroes. The payload FORMAT is deliberately not guessed
// -- this asks only whether the box looks, which is the question the phase's
// "do not guess a format" rule leaves open.
func sectionC1(tableID byte, extension uint16) []byte {
	body := []byte{
		byte(extension >> 8), byte(extension & 0xff),
		0xC1, // current, version 0
		0x00, // section_number
		0x00, // last_section_number
		0, 0, 0, 0,
	}
	length := len(body) + 4 // + CRC
	section := append([]byte{tableID, 0xB0 | byte(length>>8), byte(length)}, body...)
	crc := dvb.MPEGCRC32(section)
	return append(section, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}
