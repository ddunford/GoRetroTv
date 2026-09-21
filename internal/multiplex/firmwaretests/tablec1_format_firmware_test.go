package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE 0xC1 PAYLOAD IS AN ARRAY OF NINE-BYTE RECORDS, and this proves it on the
// box rather than on the page.
//
// Read off the parser at 0x800C4C34 with tools/disasm.sh:
//
//	lbu v1,1(s1) / lbu v0,2(s1) / sll v1,8 / addu v1,v0
//	li  a0,4095  / and v1,a0        section_length & 0x0FFF
//	addiu v1,-9                     payload = section_length - 9
//	li  v0,9     / div zero,v1,v0   ÷ 9
//	mflo a0      / sh a0,0(sp+14)   RECORD COUNT
//	lhu a1,0(sp+14) / li v1,10 / mult a1,v1
//	mflo a0      / addiu a0,12 / jalr   alloc(count*10 + 12)
//
// and the walk starting at section+8 reads each record as a 16-bit id in
// rec[0..1], a six-bit field packed out of rec[2]&0xF0 and rec[3]&0xC0>>6, and
// a flag at rec[2]&0x08.
//
// STATIC READINGS OF THIS FIRMWARE HAVE BEEN BACKWARDS TWICE -- tools/ghidra's
// own header says the "CRSL" tag test and the halfword id both looked the other
// way round on the page -- so the arithmetic above is a hypothesis until the
// machine agrees with it. The test: send records carrying ids nothing else
// would contain, and find them in DRAM at the ten-byte stride the allocation
// implies. Their SPACING is the claim; their presence alone is not, because the
// box copies any section it is given into the heap.
func TestTheTableC1PayloadIsNineByteRecords(t *testing.T) {
	const probePID, extension = 0x52, 0x0100
	ids := []uint16{0xBEEF, 0xCAFE, 0xF00D, 0xD00D}

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

	// Nine bytes per record, exactly as the parser's divisor says.
	payload := make([]byte, 0, len(ids)*9)
	for _, id := range ids {
		payload = append(payload,
			byte(id>>8), byte(id), // rec[0..1]: the 16-bit id
			0xA8,          // rec[2]: high nibble 0xA, and bit 3 SET
			0x40,          // rec[3]: top two bits 01
			0, 0, 0, 0, 0) // rec[4..8]: unread by the part decoded so far
	}
	section := sectionTableVersioned(0xC1, extension, 0, payload)

	// The parser's own arithmetic, asserted against the bytes we are sending --
	// if this does not hold, the section is malformed and the box's answer means
	// nothing.
	declared := int(section[1]&0x0f)<<8 | int(section[2])
	if got := (declared - 9) / 9; got != len(ids) {
		t.Fatalf("harness: section_length %d gives %d records by the firmware's own formula, not %d",
			declared, got, len(ids))
	}
	if err := box.Demux.Push(probePID, section); err != nil {
		t.Fatalf("harness: the box refused the section: %v", err)
	}
	runUntil(t, box, transmitter, censusBudget, func(int) bool { return false })

	// Find the first id, then ask whether the rest follow at ten bytes.
	size := box.RAM.Size()
	hits := 0
	for off := uint32(0); off+uint32(len(ids))*10 <= size; off += 2 {
		if uint16(box.RAM.Read(off, bus.Half)) != ids[0] {
			continue
		}
		ok := true
		for i, id := range ids {
			if uint16(box.RAM.Read(off+uint32(i)*10, bus.Half)) != id {
				ok = false
				break
			}
		}
		if ok {
			hits++
			t.Logf("all %d ids found at a TEN-BYTE stride, guest 0x%08X", len(ids), 0x80000000|off)
		}
	}
	if hits == 0 {
		// Distinguish "the ids are nowhere" from "they are there but not at ten".
		found := 0
		for off := uint32(0); off+2 <= size; off += 2 {
			if uint16(box.RAM.Read(off, bus.Half)) == ids[0] {
				found++
			}
		}
		t.Fatalf("no ten-byte-stride array found (the first id appears %d times in DRAM). Either the "+
			"records are not nine bytes on the wire, or not ten in memory, or the walk stores them "+
			"somewhere this scan cannot see", found)
	}
}
