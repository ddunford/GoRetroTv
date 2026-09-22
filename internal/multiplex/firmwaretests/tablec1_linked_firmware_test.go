package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// IS THE TABLE LINKED INTO THE A-Z INDEX, OR THROWN AWAY? Asked of the data
// structure directly, because asking the SCREEN turned out to be impossible:
// the TV GUIDE menu will not move its highlight past entry 6, so A-Z LISTINGS
// at entry 9 cannot be opened at all on this fixture. That is its own finding
// and it is recorded, but it is not a reason to leave this question unanswered.
//
// The address falls straight out of 0x800C4F94:
//
//	lw    s0,0x800C51A0     s0 = *(0x800C51A0) = 0x80120E0C, the head table
//	sltiu a0,65 / sltiu a0,91   'A'..'Z' only
//	lw    v0,0x800C51C0     bias = 0x3FFFFFC0
//	addu  a0,v0 / sll a0,2  (letter + bias) << 2, which truncates to
//	                        (letter - 0x40) * 4 in 32 bits -- 'A' -> +4, 'Z' -> +104
//	addu  s0,a0             s0 = 0x80120E0C + offset
//
// So the head for 'A' is at 0x80120E10. If a table sent under 'A' is retained,
// that word points at its twelve-byte header; if it is discarded, it does not.
//
// BOTH DIRECTIONS ARE MEASURED, because "the pointer matches" is only meaningful
// beside a case where it does not: the same table is also sent under 0x0100,
// which is outside the letter range and is known to take the branch that frees
// the list.
func TestWhetherATableC1SentUnderALetterIsLinkedIntoTheAtoZIndex(t *testing.T) {
	const probePID = 0x52
	const headTable = 0x80120E0C // the value AT 0x800C51A0, read from the image
	const records = 6

	headFor := func(letter uint16) uint32 {
		// (letter + 0x3FFFFFC0) << 2, truncated to 32 bits, exactly as the guest does.
		return headTable + ((uint32(letter)+0x3FFFFFC0)<<2)&0xffffffff
	}
	if got := headFor('A'); got != 0x80120E10 {
		t.Fatalf("harness: the 'A' head computes to %08X, not 0x80120E10; the bias arithmetic here "+
			"does not match the guest's and nothing below would mean anything", got)
	}

	probe := func(t *testing.T, extension uint16) (head uint32, arrayBase uint32, linked bool) {
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

		idBase := uint16(0xE300)
		payload := make([]byte, 0, records*9)
		for i := 0; i < records; i++ {
			id := idBase + uint16(i)
			payload = append(payload, byte(id>>8), byte(id), 0xA0, 0x40, 0, 0, 0, 0, 0)
		}
		if err := box.Demux.Push(probePID, sectionTableVersioned(0xC1, extension, 0, payload)); err != nil {
			t.Fatalf("harness: the box refused the section: %v", err)
		}
		runUntil(t, box, transmitter, censusBudget, func(int) bool { return false })

		size := box.RAM.Size()
		for off := uint32(0); off+records*10 <= size; off += 2 {
			if uint16(box.RAM.Read(off, bus.Half)) == idBase &&
				uint16(box.RAM.Read(off+10, bus.Half)) == idBase+1 {
				arrayBase = off
				break
			}
		}
		if arrayBase == 0 {
			t.Fatalf("harness: the array was not assembled under %04X, so there is nothing to look for",
				extension)
		}
		// The head is read for the LETTER under test where there is one, and for
		// 'A' otherwise, so both runs look at the same word.
		letter := extension
		if letter < 'A' || letter > 'Z' {
			letter = 'A'
		}
		head = box.RAM.Read(headFor(letter)&0x1fffffff, bus.Word)
		wantHeader := 0x80000000 | (arrayBase - 12)
		return head, wantHeader, head == wantHeader
	}

	t.Run("under letter A", func(t *testing.T) {
		head, header, linked := probe(t, 'A')
		t.Logf("head[0x80120E10] = %08X, our header = %08X", head, header)
		if !linked {
			t.Errorf("a table sent under 'A' is NOT the head of the 'A' list -- the head points at "+
				"%08X and our header is at %08X", head, header)
			return
		}
		t.Log("LINKED: the table sent under 'A' is the head of the A-Z index's 'A' list")
	})

	t.Run("under 0x0100, outside the letter range", func(t *testing.T) {
		head, header, linked := probe(t, 0x0100)
		t.Logf("head[0x80120E10] = %08X, our header = %08X", head, header)
		if linked {
			t.Error("a table sent OUTSIDE the letter range was linked into the 'A' list, so the " +
				"letter dispatch is not what distinguishes them and the reading is wrong")
			return
		}
		t.Log("NOT LINKED, as expected: outside 'A'..'Z' the list is freed rather than indexed")
	})
}
