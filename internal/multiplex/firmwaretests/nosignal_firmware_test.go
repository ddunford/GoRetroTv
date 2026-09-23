package firmwaretests_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT DECIDES "No satellite signal is being received".
//
// It sits over the picture whenever the demo tunes to a channel. The record establishes what the
// message IS -- a channel-level error, one entry in a table beside "There is a technical fault with
// this channel" -- and that it is drawn with ZERO demodulator traffic, so it is not read off the
// front end. What it has never established is what SETS it.
//
// The obvious next move was to send a transport stream, and measuring first killed it: the packet
// path is gated on the demux register at 0x140, the box never writes that register ONCE across 673
// other demux accesses, and the only code in the image that touches it manipulates bits in the
// upper half rather than the enable. So the stream would have gone nowhere and the code that would
// ask for one never runs.
//
// So ask the box instead, the way the grid's "..no listings available" was cracked: find the string
// where THIS box put it in DRAM, watch for the moment it is first reached for, and keep a ring of
// the o-code addresses leading up to it. The interpreter fetches bytecode from flash as data, so a
// flash read is an o-code program counter, and the tail of that ring is the decision.
//
// THE ROUTE IS THE WORKING GRID AND A SELECT, because that is where the message appears: SELECT on
// a drawn programme tunes to the channel and the banner comes up with the message over it.
//
// IT ASSERTS ITS OWN SUBJECT TWICE: the string must be found in DRAM, and the box must actually
// reach for it. A run where it never does has measured something other than the failure.
//
// IT ONLY READS.
func TestWhatDecidesNoSatelliteSignal(t *testing.T) {
	const (
		ringSize      = 3000
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

	needle := []byte("No satellite signal is being received")
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
		t.Skipf("the box does not hold %q as text in DRAM, so the message table is kept coded and "+
			"a watch on a guessed address would report a confident zero", needle)
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
		caught     bool
		flashReads int
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch {
			return
		}
		if a.Virtual >= 0x9FC00000 {
			flashReads++
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
		caught = true
		if ringFilled {
			trail = append(trail, ring[ringAt:]...)
		}
		trail = append(trail, ring[:ringAt]...)
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
	watching = true
	openAllChannelsFinished(t, press, ".artifacts/nosignal-grid.png")
	for i := 0; i < 40_000_000 && !caught; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	before := screenNow(t, box)
	viewing := uint32(0)
	for attempt := 1; attempt <= 6 && (viewing == 0 || viewing == before); attempt++ {
		viewing = press(keySelect, "select to view the channel", 80_000_000)
	}
	for i := 0; i < 40_000_000 && !caught; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if err := dumpScreen(t, box, "nosignal-viewing.png"); err != nil {
		t.Fatal(err)
	}
	if flashReads == 0 {
		t.Fatal("harness: no flash was read at all while this ran, so the o-code trail is empty " +
			"for reasons that have nothing to do with the message")
	}
	if !caught {
		t.Fatalf("harness: the box never reached for %q in this run (%d flash reads, viewing "+
			"%08X), so whatever it drew, it is not the message. Read "+
			".artifacts/nosignal-viewing.png", needle, flashReads, viewing)
	}
	t.Logf("the box is viewing %08X; %d o-code instructions were captured before it reached for "+
		"the string", viewing, len(trail))

	type edge struct{ from, to uint32 }
	var edges []edge
	for i := 1; i < len(trail); i++ {
		from, to := trail[i-1], trail[i]
		if to > from && to-from <= 16 {
			continue
		}
		edges = append(edges, edge{from, to})
	}
	const tail = 40
	first := 0
	if len(edges) > tail {
		first = len(edges) - tail
		t.Logf("    ... %d earlier transitions elided", first)
	}
	t.Logf("=== the control flow immediately before the message was chosen ===")
	for _, e := range edges[first:] {
		t.Logf("    %08X -> %08X", e.from, e.to)
	}
	if len(trail) > 0 {
		t.Logf("the last o-code instruction before the string was touched is %08X -- disassemble "+
			"around it with tools/ocode-disasm.py", trail[len(trail)-1])
	}
}
