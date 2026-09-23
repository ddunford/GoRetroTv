package firmwaretests_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE OBJECT EACH GRID ROW IS BUILT FROM, AND WHAT ITS TYPE FIELD SAYS.
//
// The row's state halfword is not computed, it is COPIED, at MIPS 0x800CB138:
//
//	local_68 = resolve(local_dc);   // an object reached from a0
//	*param_3 = local_68[2];         // the row record's first halfword IS that object's field
//
// So "nothing ever writes 2" was the wrong frame. The value is whatever the object's type says, and
// the grid is enumerating a list whose elements are type 1 where a programme row wants another
// type. Every call passes a0 = 0x4001 and a1 = the row index, so a0 is a list handle.
//
// WHICH OBJECT, THOUGH. Nothing here knows which register holds local_68, and guessing one would be
// a reading of a listing -- wrong twice in one day on this project. So this takes it from the
// machine: the state write at 0x800CB138 stores a halfword that was LOADED a moment earlier, so the
// most recent halfword read whose value matches what gets written is that load, and its address is
// the object's type field. The object is that address minus four, because local_68 is a short* and
// the field is [2].
//
// Then it dumps the object. Six rows, six objects: if they are consecutive, that is the list, and
// the list is the thing that has to contain something else.
//
// IT ASSERTS ITS OWN SUBJECT: a state write must be seen, and the load that fed it must be found,
// or the addresses below are guesses wearing a measurement's clothes.
//
// IT ONLY READS.
func TestWhatObjectEachGridRowIsBuiltFrom(t *testing.T) {
	const (
		// THE EXACT LOAD, not a heuristic. Disassembled:
		//     800cb134  lw   v1,128(sp)    ; the object pointer
		//     800cb136  lhu  v1,4(v1)      ; <- reads object+4, the TYPE
		//     800cb138  sh   v1,0(s0)      ; stores it into the row record
		// so a read at 0x800CB136 is the type field and its address minus four is the object.
		// A first version instead took "the most recent halfword load carrying the value being
		// stored", which happened to be right only because that load is the instruction before the
		// store -- with a value as common as 1, that was luck rather than method.
		typeLoadAt   = 0x800CB136
		nextOffset   = 20         // the object's last word points just past it: a chain
		stateWriteAt = 0x800CB138 // the instruction that stores the row's type halfword
		rowStride    = 336
		expectedBase = 0x8048ADB4
		rows         = 6
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
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	type recent struct {
		at    uint32
		value uint32
	}
	type found struct {
		row    uint32
		state  uint32
		field  uint32 // the address the state was loaded from
		object uint32 // ...minus four: local_68 itself
	}
	var ring [64]recent
	ringAt := 0
	var hits []found
	writes := 0
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			return
		}
		at := a.Virtual | 0x80000000
		if !a.Write {
			if a.Size == bus.Half && box.Machine.Core.State().PC&^1 == typeLoadAt {
				ring[ringAt] = recent{at: at, value: a.Value}
				ringAt = (ringAt + 1) % len(ring)
			}
			return
		}
		if a.Size != bus.Half || box.Machine.Core.State().PC&^1 != stateWriteAt {
			return
		}
		writes++
		row := (at - expectedBase) / rowStride
		if at < expectedBase || row >= rows {
			return
		}
		// The type load immediately precedes this store, so the newest entry in the ring is it.
		r := ring[(ringAt-1+len(ring))%len(ring)]
		if r.at == 0 {
			hits = append(hits, found{row: row, state: a.Value})
			return
		}
		hits = append(hits, found{row: row, state: a.Value, field: r.at, object: r.at - 4})
	}}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
	settled := openAllChannelsFinished(t, press, ".artifacts/rowobject-grid.png")
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "rowobject-grid.png"); err != nil {
		t.Fatal(err)
	}
	if writes == 0 {
		t.Fatalf("harness: %#08X never stored a halfword while the grid drew, so the state write "+
			"is not at that instruction any more", uint32(stateWriteAt))
	}
	if len(hits) == 0 {
		t.Fatalf("harness: %d state writes were seen and none could be traced to the load that "+
			"fed it, so the object addresses below would be guesses", writes)
	}
	t.Logf("the grid settled on %08X; %d state writes, %d traced to their source",
		settled, writes, len(hits))

	byteAt := func(at uint32) byte {
		return byte(box.RAM.Read(at&0x1fffffff, bus.Byte)) // #nosec G115 -- byte read
	}
	t.Logf("=== the object each row's type was copied from ===")
	for _, h := range hits {
		if h.object == 0 {
			t.Logf("    row %d  state %d  (source not found)", h.row, h.state)
			continue
		}
		line := ""
		for off := uint32(0); off < 24; off++ {
			line += fmt.Sprintf("%02X", byteAt(h.object+off))
			if off%2 == 1 {
				line += " "
			}
		}
		t.Logf("    row %d  state %d  object %08X  type at %08X  |  %s",
			h.row, h.state, h.object, h.field, line)
	}
	// WALK THE CHAIN. The object's last word points just past itself, so these are linked; what
	// matters is what ELSE is on the chain and what types those carry, because a programme row
	// wants a type this one does not have.
	if len(hits) > 0 && hits[0].object != 0 {
		t.Logf("=== the chain from row 0's object ===")
		at := hits[0].object
		seen := map[uint32]bool{}
		for n := 0; n < 24 && at != 0 && !seen[at]; n++ {
			seen[at] = true
			kind := box.RAM.Read((at+4)&0x1fffffff, bus.Half)
			next := box.RAM.Read((at+nextOffset)&0x1fffffff, bus.Word)
			note := ""
			if kind == 2 {
				note = "   <- TYPE 2: the type a programme row wants"
			}
			t.Logf("    %2d  %08X  type %d  next %08X%s", n, at, kind, next, note)
			if next < 0x80000000 || next >= 0x80800000 || next&1 != 0 {
				t.Logf("        (next is not a guest pointer; the chain ends here)")
				break
			}
			at = next
		}
	}

	// Are they one list? Consecutive objects a fixed distance apart would say so.
	if len(hits) >= 2 && hits[0].object != 0 && hits[1].object != 0 {
		step := int64(hits[1].object) - int64(hits[0].object)
		same := true
		for i := 2; i < len(hits); i++ {
			if hits[i].object == 0 || int64(hits[i].object)-int64(hits[i-1].object) != step {
				same = false
				break
			}
		}
		if same {
			t.Logf("the objects are %d bytes apart, every time -- so they ARE one array, and the "+
				"list handle 0x4001 names it", step)
		} else {
			t.Logf("the objects are not evenly spaced, so they are linked or separately " +
				"allocated rather than an array")
		}
	}
}
