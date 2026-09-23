package firmwaretests_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE O-CODE THAT CHOOSES "..no listings available", CAUGHT IN THE ACT.
//
// Everything about the DATA is eliminated, each with a control: coverage, the day, the block, the
// service key, the line-up's kind byte, a table 0xC1 index under any extension, tomorrow's block,
// and a channel with nothing on it. The store demonstrably holds the programmes -- the now-and-next
// banner resolves one for the grid's own first row at the grid's own first column, and ALL
// PROGRAMMES A-Z resolves ten across five channels off the same store.
//
// And the grid enters the MIPS listings module ZERO times while it draws. It does not ask and get
// nothing; IT NEVER ASKS. So the decision is a branch in interpreted o-code taken BEFORE any query,
// and o-code cannot be decompiled -- there is no processor module for it in any disassembler.
//
// **BUT IT CAN BE WATCHED.** The interpreter fetches bytecode from FLASH AS DATA, so every flash
// read is an o-code program counter. This keeps a ring of the most recent ones and snapshots it the
// instant the grid first touches the "..no listings available" string in DRAM. What comes back is
// the o-code trail immediately before the string was chosen -- which is the branch, and which
// tools/ocode-disasm.py can then render.
//
// THE STRING IS FOUND IN DRAM ON THIS RUN, not computed from a file offset. Two earlier probes
// watched the FLASH copies -- first one window, then every window on both chips, since U202 and
// U203 both carry the table -- and neither saw a single read while the screen that displays the
// string drew. The message table is copied into DRAM at boot, like the application image itself.
//
// IT ASSERTS ITS OWN SUBJECT twice: the string must be found, and the grid must actually reach for
// it. A run where it never does has measured something other than the failure.
func TestWhatOCodeChoosesNoListingsAvailable(t *testing.T) {
	const (
		ringSize      = 4000
		mainFetchSite = 0x80069298
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

	// FIND THE STRING WHERE THIS BOX PUT IT.
	needle := []byte("..no listings available")
	size := box.RAM.Size()
	ram := make([]byte, size)
	for i := uint32(0); i < size; i++ {
		ram[i] = byte(box.RAM.Read(i, bus.Byte)) // #nosec G115 -- byte read
	}
	var found []uint32
	for at := 0; ; {
		i := bytes.Index(ram[at:], needle)
		if i < 0 {
			break
		}
		found = append(found, uint32(at+i)) // #nosec G115 -- bounded by RAM size
		at += i + 1
	}
	if len(found) == 0 {
		t.Skipf("the box does not hold %q as text anywhere in DRAM, so the message table is kept "+
			"in some coded form and a watch on a guessed address would report a confident zero",
			needle)
	}
	for _, at := range found {
		t.Logf("the string is in DRAM at %08X", 0x80000000|at)
	}
	inString := func(off uint32) bool {
		for _, at := range found {
			if off >= at && off < at+uint32(len(needle)) { // #nosec G115 -- small constant
				return true
			}
		}
		return false
	}

	var (
		watching   bool
		ring       [ringSize]uint32
		ringAt     int
		ringFilled bool
		trail      []uint32
		flashReads int
		caught     bool
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		// The interpreter fetches bytecode from flash as DATA, so a flash read is an o-code PC.
		// Flash is quoted at 0xBFC00000 and mirrored at 0x9FC00000; fold both onto one offset.
		if a.Virtual >= 0x9FC00000 {
			flashReads++
			// ONLY THE MAIN OPCODE FETCH. Every other flash read is an OPERAND byte, and a ring
			// of all of them is mostly operands -- five hundred entries covered barely a hundred
			// instructions, and the tail is the part that matters. 0x80069298 is the
			// interpreter's opcode fetch; the disassembler uses the same address for the same
			// reason, to tell an instruction boundary from the bytes inside one.
			if box.Machine.Core.State().PC&^1 != mainFetchSite {
				return
			}
			ring[ringAt] = 0x9FC00000 | (a.Virtual & 0x00ffffff)
			if ringAt++; ringAt == ringSize {
				ringAt, ringFilled = 0, true
			}
			return
		}
		if caught || !inString(a.Virtual&0x1fffffff) {
			return
		}
		// THE MOMENT OF CHOICE. Snapshot the ring oldest-first and stop.
		caught = true
		if ringFilled {
			trail = append(trail, ring[ringAt:]...)
		}
		trail = append(trail, ring[:ringAt]...)
	}}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching = true
		}
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
	settled := openAllChannels(t, press, ".artifacts/ocode-choice.png", true)
	for i := 0; i < 60_000_000 && !caught; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "ocode-choice.png"); err != nil {
		t.Fatal(err)
	}
	if flashReads == 0 {
		t.Fatal("harness: the grid read flash not once while it drew, so the o-code trail is " +
			"empty for reasons that have nothing to do with the screen")
	}
	if !caught {
		t.Fatalf("harness: the grid never reached for %q at all in this run (%d flash reads "+
			"seen), so whatever it drew, it is not the failure this probe is about. Read "+
			".artifacts/ocode-choice.png", needle, flashReads)
	}
	t.Logf("the grid settled on %08X; %d flash reads seen before it reached for the string",
		settled, flashReads)
	// THE TRANSITIONS, NOT THE ADDRESSES. Four thousand consecutive o-code addresses are almost
	// all straight-line and unreadable; the CONTROL FLOW is what says where a decision was made.
	// A step of one to sixteen bytes forward is the next instruction; anything else is a jump, a
	// call or a return, and those are the only rows worth printing.
	t.Logf("=== the control flow of the last %d o-code instructions before the string was "+
		"touched ===", len(trail))
	type edge struct{ from, to uint32 }
	var edges []edge
	for i := 1; i < len(trail); i++ {
		from, to := trail[i-1], trail[i]
		if to > from && to-from <= 16 {
			continue
		}
		edges = append(edges, edge{from, to})
	}
	// THE LAST ONES, because the decision is at the END of the trail. A first version printed the
	// first hundred and twenty and they were all one loop going round -- true, and not the part
	// that chose anything.
	const tail = 80
	first := 0
	if len(edges) > tail {
		first = len(edges) - tail
		t.Logf("    ... %d earlier transitions elided", first)
	}
	for _, e := range edges[first:] {
		t.Logf("    %08X -> %08X", e.from, e.to)
	}
	shown := len(edges)
	if shown == 0 {
		t.Logf("    the trail is entirely straight-line, so the decision is further back than "+
			"%d instructions", len(trail))
	}
	t.Logf("render these with tools/ocode-disasm.py -- the branch that chose the string is in " +
		"the tail of that trail, and it is taken BEFORE the grid asks the listings module " +
		"anything, because it never asks it anything at all")
}
