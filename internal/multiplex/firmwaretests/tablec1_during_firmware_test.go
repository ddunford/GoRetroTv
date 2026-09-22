package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// IS THE 0xC1 TABLE READ WHILE THE BOX IS ACQUIRING?
//
// Every probe so far pushed the section at a box that had finished acquiring
// and then watched five screens, and all five read nothing. That rules out a
// top-level screen as the consumer; it does not rule out a consumer that runs
// DURING acquisition and has nothing to say afterwards. This file's own note
// that PID 0x52 "opens in response to our NIT and closes again" is what makes
// that the most interesting of the remaining possibilities.
//
// The window exists in every one of these tests and was simply never used: a
// restored box still acquires the line-up and the day's titles off the
// transmitter, which is what registeringProgrammes waits for. So the section
// goes in BEFORE that finishes, and the observer runs across the whole of it.
//
// It asserts its own subject twice over. The push must be accepted, or the
// filter was not armed and nothing was measured; and the acquisition must
// actually complete inside the window, or the box was not doing the thing this
// is watching it do.
func TestWhetherTheTableC1ArrayIsReadDuringAcquisition(t *testing.T) {
	const probePID, extension = 0x52, 0x0100
	const idBase = 0xE100
	const records = 8

	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	payload := make([]byte, 0, records*9)
	for i := 0; i < records; i++ {
		id := uint16(idBase + i)
		payload = append(payload, byte(id>>8), byte(id), 0xA0, 0x40, 0, 0, 0, 0, 0)
	}
	section := sectionTableVersioned(0xC1, extension, 0, payload)

	// Straight in, before anything has been acquired off the transmitter.
	if err := box.Demux.Push(probePID, section); err != nil {
		t.Fatalf("harness: PID %#x is not armed on a freshly restored box (%v), so this measured "+
			"nothing -- the whole point is to be inside the acquisition, not after it", probePID, err)
	}
	// Just enough to let the parser build the array, and no more.
	runUntil(t, box, transmitter, 2_000_000, func(int) bool { return false })

	var bases []uint32
	size := box.RAM.Size()
	for off := uint32(0); off+records*10 <= size; off += 2 {
		if uint16(box.RAM.Read(off, bus.Half)) == idBase &&
			uint16(box.RAM.Read(off+10, bus.Half)) == idBase+1 {
			bases = append(bases, off)
		}
	}
	if len(bases) == 0 {
		t.Fatal("harness: the array was not assembled, so there is nothing to watch")
	}
	for _, b := range bases {
		t.Logf("watching guest 0x%08X..0x%08X", 0x80000000|(b-12), 0x80000000|(b+records*10))
	}
	inWindow := func(p uint32) bool {
		for _, b := range bases {
			if p >= b-12 && p < b+records*10 {
				return true
			}
		}
		return false
	}

	reads := map[uint32]int{}
	offsets := map[int]int{}
	var anyReads int
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch || a.Write {
			return
		}
		anyReads++
		if p := a.Virtual & 0x1fffffff; inWindow(p) {
			reads[box.Machine.Core.State().PC&^1]++
			// Relative to the HEADER, so header+4 and record[0]+n are
			// distinguishable -- the difference decides whether the box linked
			// our table into a list or walked into the records themselves.
			offsets[int(p)-int(bases[0])+12] += 1
		}
	}}

	// Now the acquisition itself, with the observer on throughout.
	want := programmesInTheBlock(t, guide, day)
	registered, done := 0, false
	for i := 0; i < 120_000_000 && !done; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			registered++
			if registered >= want {
				done = true
			}
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if !done {
		t.Fatalf("harness: only %d of %d programmes registered, so the box did not finish acquiring "+
			"inside the window and this did not watch what it claims to", registered, want)
	}
	if anyReads == 0 {
		t.Fatal("harness: the observer saw no data read at all, so its zero is the instrument")
	}
	t.Logf("acquisition completed: %d programmes registered, observer saw %d data reads in total",
		registered, anyReads)

	total := 0
	for _, n := range reads {
		total += n
	}
	if total == 0 {
		t.Log("VERDICT: the array is NOT read during acquisition either. The consumer is not a " +
			"top-level screen and not the acquisition path, on this fixture.")
		return
	}
	pcs := make([]uint32, 0, len(reads))
	for pc := range reads {
		pcs = append(pcs, pc)
	}
	sort.Slice(pcs, func(a, b int) bool { return reads[pcs[a]] > reads[pcs[b]] })
	t.Logf("VERDICT: THE ARRAY IS READ DURING ACQUISITION -- %d reads from %d PCs", total, len(pcs))
	for i, pc := range pcs {
		if i >= 10 {
			break
		}
		t.Logf("    %08X  %d reads", pc, reads[pc])
	}
	var offs []int
	for o := range offsets {
		offs = append(offs, o)
	}
	sort.Ints(offs)
	for _, o := range offs {
		where := "inside the records"
		if o < 12 {
			where = "IN THE 12-BYTE HEADER"
		}
		t.Logf("    offset +%d from the header start: %d reads  (%s)", o, offsets[o], where)
	}
}
