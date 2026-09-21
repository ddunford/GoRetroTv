package multiplex_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
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

	run := func(t *testing.T, table byte) (map[uint32]int, uint64) {
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
			if err := box.Demux.Push(probePID, sectionC1(table, extension)); err != nil {
				t.Fatalf("harness: the box refused the probe section: %v", err)
			}
		}
		seen := make(map[uint32]int, 8192)
		start := box.Machine.Retired
		runUntil(t, box, transmitter, 20_000_000, func(int) bool {
			seen[box.Machine.Core.State().PC&^1]++
			return false
		})
		return seen, box.Machine.Retired - start
	}

	control, controlRan := run(t, 0)
	test, testRan := run(t, tableC1)
	if controlRan != testRan {
		t.Fatalf("harness: control ran %d instructions and the test ran %d; the runs are not comparable",
			controlRan, testRan)
	}
	t.Logf("control executed %d distinct PCs, test %d, over %d instructions each",
		len(control), len(test), controlRan)

	var only []uint32
	for pc := range test {
		if control[pc] == 0 {
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
	t.Logf("VERDICT: %d guest PCs ran ONLY when the 0xC1 section was delivered", len(only))

	// The whole census goes to an artefact so it can be cross-referenced against
	// the measured record instead of skimmed in a terminal. Clustered into
	// contiguous runs, because a run's LOWEST address is where a function starts
	// and that is the thing worth naming.
	var out []string
	runLo, runPrev := only[0], only[0]
	flush := func(hi uint32) {
		total := 0
		for pc := runLo; pc <= hi; pc += 2 {
			total += test[pc]
		}
		out = append(out, fmt.Sprintf("%08X %08X %5d %6d", runLo, hi, (hi-runLo)/2+1, total))
	}
	for _, pc := range only[1:] {
		if pc-runPrev > 8 {
			flush(runPrev)
			runLo = pc
		}
		runPrev = pc
	}
	flush(runPrev)
	if err := os.MkdirAll(filepath.Join("..", "..", ".artifacts"), 0o750); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join("..", "..", ".artifacts", "table-c1-exclusive-pcs.txt")
	body := "# lo       hi       pcs  executions   (PCs run ONLY when a 0xC1 section was delivered)\n" +
		strings.Join(out, "\n") + "\n"
	if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d contiguous runs written to .artifacts/table-c1-exclusive-pcs.txt", len(out))

	// THE CONTROL: a table id the box filters for nothing and we never send. If
	// it wakes the same code, then none of the above is about 0xC1 -- it is what
	// ANY unexpected section on an armed PID does, and the finding is generic.
	other, _ := run(t, 0xA5)
	var otherOnly []uint32
	for pc := range other {
		if control[pc] == 0 {
			otherOnly = append(otherOnly, pc)
		}
	}
	shared := 0
	for _, pc := range only {
		if other[pc] != 0 {
			shared++
		}
	}
	t.Logf("CONTROL table 0xA5: %d exclusive PCs, sharing %d of 0xC1's %d",
		len(otherOnly), shared, len(only))
	if len(only) > 0 && shared*100/len(only) >= 90 {
		t.Logf("VERDICT: 0xC1 IS NOT SPECIAL -- %d%% of its exclusive PCs also run for 0xA5.",
			shared*100/len(only))
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

// THIS TEST EXISTS TO SHOW THAT STORAGE IS NOT AN ACCEPTANCE SIGNAL, which is
// the opposite of why it was written.
//
// It sends a payload nothing else in the machine contains and searches all of
// DRAM for it. The marker IS found -- twice, in ordinary heap -- and the
// control without the push does not contain it, so the box really does copy our
// bytes out of the ring. The reading taken from that, "so the box accepted our
// 0xC1 section", was WRONG, and the sweep's table-id control is what killed it:
// the box keeps the payload of 0xA5, 0x9E, 0xC2 and 0x42 as readily, none of
// which it filters for. Something copies every section delivered on an armed
// PID, so this measures that copy.
//
// It is kept because it is the evidence for that, and because it is the shape
// of mistake worth leaving a marker on: a positive result with no negative
// control. The signal that DOES discriminate is the exclusive-PC differential
// in TestWhetherAnythingWantsTableC1 -- 544 for 0xC1 against 8 for 0xA5.
func TestWhetherTheBoxKeepsTheTableC1Payload(t *testing.T) {
	const probePID, extension = 0x52, 0x0100
	marker := []byte{0xDE, 0xAD, 0xC1, 0x05, 0x5E, 0xC7, 0x10, 0x4E}

	run := func(t *testing.T, push bool) int {
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
			section := sectionC1WithBody(0xC1, extension, marker)
			if err := box.Demux.Push(probePID, section); err != nil {
				t.Fatalf("harness: the box refused the probe section: %v", err)
			}
		}
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

		// Every 32-bit-aligned position in DRAM. The marker is eight bytes, so a
		// copy that kept alignment is found wherever it landed.
		found, size := 0, box.RAM.Size()
		first := uint32(0x100000000 - 1)
		for off := uint32(0); off+8 <= size; off += 4 {
			if box.RAM.Read(off, bus.Word) != 0xDEADC105 {
				continue
			}
			if box.RAM.Read(off+4, bus.Word) == 0x5EC7104E {
				found++
				if off < first {
					first = off
				}
			}
		}
		if found > 0 {
			t.Logf("marker found %d time(s), first at DRAM offset %08X", found, first)
		}
		return found
	}

	if n := run(t, false); n != 0 {
		t.Fatalf("harness: the control run already contains the marker %d time(s), so it is not distinctive", n)
	}
	t.Log("control: marker absent, as it must be")

	if n := run(t, true); n == 0 {
		t.Log("VERDICT: the box did NOT keep the payload. It ran 544 addresses and allocated, but our")
		t.Log("         bytes are nowhere in DRAM -- so it inspected the section and did not store it.")
	} else {
		t.Logf("the box kept the payload -- %d copies in DRAM. This is NOT acceptance: it keeps any", n)
		t.Log("table id delivered on an armed PID. See the table-id control in the sweep.")
	}
}

// sectionC1WithBody is sectionC1 with a caller-supplied payload.
func sectionC1WithBody(tableID byte, extension uint16, payload []byte) []byte {
	body := make([]byte, 0, 5+len(payload))
	body = append(body, byte(extension>>8), byte(extension&0xff), 0xC1, 0x00, 0x00)
	body = append(body, payload...)
	length := len(body) + 4
	section := append([]byte{tableID, 0xB0 | byte(length>>8), byte(length)}, body...)
	crc := dvb.MPEGCRC32(section)
	return append(section, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}
