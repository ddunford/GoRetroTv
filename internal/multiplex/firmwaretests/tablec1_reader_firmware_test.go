package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHO READS THE ARRAY AFTER IT IS BUILT? This is the bridge question. Being able
// to build a 0xC1 section is only worth something if the box then USES what it
// stored, and nothing so far has shown it doing that -- the census proved the
// parser runs, the sweeps proved the bytes arrive, and neither watched what
// happens next.
//
// A bus observer answers it directly: run with one installed, filter to DATA
// reads (never instruction fetches, which pass through the same bus) inside the
// array and its twelve-byte header, and record the guest PC doing the reading.
//
// THREE WINDOWS, because "nothing read it" means different things depending on
// when you looked:
//
//	idle       does anything touch it unprompted?
//	tv guide   does opening the guide read it?
//	box office does opening the other menu read it?
//
// A screen that reads this array names what the table is FOR, which is the one
// thing the format work cannot say on its own.
func TestWhatReadsTheAssembledTableC1Array(t *testing.T) {
	const probePID, extension = 0x52, 0x0100
	const idBase = 0xE000
	const records = 8

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

	payload := make([]byte, 0, records*9)
	for i := 0; i < records; i++ {
		id := uint16(idBase + i)
		payload = append(payload, byte(id>>8), byte(id), 0xA0, 0x40, 0, 0, 0, 0, 0)
	}
	if err := box.Demux.Push(probePID, sectionTableVersioned(0xC1, extension, 0, payload)); err != nil {
		t.Fatalf("harness: the box refused the section: %v", err)
	}
	runUntil(t, box, transmitter, censusBudget, func(int) bool { return false })

	// EVERY copy, not the first. The marker test found a payload twice in DRAM,
	// so watching one instance could watch the dead one and report a confident
	// nothing while the live array is read a megabyte away.
	var bases []uint32
	size := box.RAM.Size()
	for off := uint32(0); off+records*10 <= size; off += 2 {
		if uint16(box.RAM.Read(off, bus.Half)) == idBase &&
			uint16(box.RAM.Read(off+10, bus.Half)) == idBase+1 {
			bases = append(bases, off)
		}
	}
	if len(bases) == 0 {
		t.Fatal("harness: no assembled array found, so this measured nothing")
	}
	inWindow := func(p uint32) bool {
		for _, b := range bases {
			if p >= b-12 && p < b+records*10 {
				return true
			}
		}
		return false
	}
	for _, b := range bases {
		t.Logf("watching guest 0x%08X..0x%08X (12-byte header + %d records)",
			0x80000000|(b-12), 0x80000000|(b+records*10), records)
	}

	// reads[pc] = how many data reads that PC made inside the window.
	watch := func(label string, budget int, before func()) {
		t.Helper()
		reads := map[uint32]int{}
		offsets := map[uint32]int{}
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if a.Write || a.Fetch {
				return
			}
			p := a.Virtual & 0x1fffffff
			if !inWindow(p) {
				return
			}
			reads[box.Machine.Core.State().PC&^1]++
			offsets[p-bases[0]+12]++
		}}
		if before != nil {
			before()
		}
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		total := 0
		for _, n := range reads {
			total += n
		}
		if total == 0 {
			t.Logf("%-12s NOTHING read the array in %d instructions", label, budget)
			return
		}
		pcs := make([]uint32, 0, len(reads))
		for pc := range reads {
			pcs = append(pcs, pc)
		}
		sort.Slice(pcs, func(a, b int) bool { return reads[pcs[a]] > reads[pcs[b]] })
		t.Logf("%-12s %d reads from %d distinct PCs", label, total, len(pcs))
		for i, pc := range pcs {
			if i >= 8 {
				break
			}
			t.Logf("               %08X  %d reads", pc, reads[pc])
		}
		var offs []uint32
		for o := range offsets {
			offs = append(offs, o)
		}
		sort.Slice(offs, func(a, b int) bool { return offs[a] < offs[b] })
		var shown []string
		for i, o := range offs {
			if i >= 12 {
				shown = append(shown, "…")
				break
			}
			shown = append(shown, fmt.Sprintf("+%d", int(o)-12))
		}
		t.Logf("               offsets read (relative to the array): %v", shown)
	}

	// THE OBSERVER MUST BE PROVED BEFORE ANY ZERO FROM IT MEANS ANYTHING. A hook
	// that never fires reports "nothing read the array" exactly as convincingly
	// as one that fires and sees nothing, and this project's rule is that an
	// instrument which cannot find its subject is a harness failure rather than
	// a count of zero.
	var anyReads, anyWrites, inDRAM int
	proof := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			return
		}
		if a.Write {
			anyWrites++
			return
		}
		anyReads++
		if p := a.Virtual & 0x1fffffff; p < box.RAM.Size() {
			inDRAM++
		}
	}}
	for i := 0; i < 200_000; i++ {
		if err := box.StepWithHooks(proof); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("observer proof over 200,000 instructions: %d data reads (%d in DRAM), %d writes",
		anyReads, inDRAM, anyWrites)
	if anyReads == 0 {
		t.Fatal("harness: the access observer saw no data read at all, so every zero below is the " +
			"instrument rather than the box")
	}

	watch("idle", 4_000_000, nil)
	watch("tv guide", 8_000_000, func() {
		if err := box.CSI.Key(0x80, 0); err != nil {
			t.Fatal(err)
		}
	})
	watch("box office", 8_000_000, func() {
		if err := box.CSI.Key(0x7D, 0); err != nil {
			t.Fatal(err)
		}
	})
	watch("interactive", 8_000_000, func() {
		if err := box.CSI.Key(0xF5, 0); err != nil {
			t.Fatal(err)
		}
	})
	// SERVICES, the screen this could not reach until gort-p5x was fixed. 0x7E is
	// the Sky menu's Services tab (lessons.md, frame 64AF0A8D) and the product
	// now delivers it -- handsetRaw accepts it and a page/server agreement test
	// keeps the two from drifting apart again. So pressing it here is the same
	// key a viewer presses, which is what makes the answer worth anything.
	watch("services", 8_000_000, func() {
		if err := box.CSI.Key(0x7E, 0); err != nil {
			t.Fatal(err)
		}
	})
}
