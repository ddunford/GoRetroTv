package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT WOULD MAKE THE BOX ASK FOR A TRANSPORT STREAM.
//
// The 188-byte packet path is gated on bit 0 of the demux register at 0x140, the box never sets it,
// and "No satellite signal is being received" is the box correctly reporting that it has tuned to a
// service it has never seen a stream for.
//
// There are two very different situations and they want different work, so the first thing to
// establish is which one this is:
//
//   - THE REGISTER IS WRITTEN, with bit 0 clear. Then the code that writes it exists and runs, and
//     what it decides the value from is the whole question -- one instruction to find and read.
//   - THE REGISTER IS NEVER TOUCHED AT ALL. Then the enabling code never runs, and the question is
//     what would call it, which is a different and larger hunt.
//
// Guessing between them is how a week goes missing, so this watches every access to 0xB000A140 --
// reads as well as writes, because a read-modify-write would show as both -- from boot, through
// acquisition, through opening the guide, and through TUNING to a channel, which is the moment the
// box would want a stream.
//
// IT ASSERTS ITS OWN SUBJECT: the demux must be accessed at all, or a tally of zero is the
// instrument rather than the box.
//
// IT ONLY READS.
func TestWhatWouldMakeTheBoxAskForATransportStream(t *testing.T) {
	const (
		demuxBase   = 0xB000A000
		transportAt = demuxBase + 0x140
	)
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	type touch struct {
		write bool
		value uint32
		pc    uint32
		phase string
	}
	var touches []touch
	demuxAny := 0
	phase := "boot and acquisition"
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			return
		}
		at := a.Virtual
		if at >= demuxBase && at < demuxBase+0x1000 {
			demuxAny++
		}
		if at&^3 != transportAt {
			return
		}
		touches = append(touches, touch{write: a.Write, value: a.Value,
			pc: box.Machine.Core.State().PC &^ 1, phase: phase})
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntilHooked(t, box, transmitter, hooks, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntilHooked(t, box, transmitter, hooks, 20_000_000, func(int) bool { return false })

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
	phase = "opening the guide"
	openAllChannelsFinished(t, press, ".artifacts/transportenable-grid.png")
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	phase = "tuning to a channel"
	before := screenNow(t, box)
	viewing := uint32(0)
	for attempt := 1; attempt <= 6 && (viewing == 0 || viewing == before); attempt++ {
		viewing = press(keySelect, "select to view the channel", 80_000_000)
	}
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "transportenable-viewing.png"); err != nil {
		t.Fatal(err)
	}
	if demuxAny == 0 {
		t.Fatal("harness: the guest did not touch the demux ONCE in this whole run, so a tally of " +
			"zero at 0x140 is the instrument and not the box")
	}
	t.Logf("the box is viewing %08X; the demux was accessed %d times in all", viewing, demuxAny)

	if len(touches) == 0 {
		t.Logf("=== THE REGISTER AT %08X IS NEVER TOUCHED ===", uint32(transportAt))
		t.Logf("    Not read, not written, at any point -- through boot, acquisition, opening the "+
			"guide and tuning to a channel, across %d other demux accesses. So the code that "+
			"would enable the transport path NEVER RUNS, and the question is what would call it. "+
			"That is a different and larger hunt than reading one instruction's inputs.", demuxAny)
		return
	}
	byPC := map[uint32]int{}
	for _, w := range touches {
		byPC[w.pc]++
	}
	pcs := make([]uint32, 0, len(byPC))
	for pc := range byPC {
		pcs = append(pcs, pc)
	}
	sort.Slice(pcs, func(a, b int) bool { return byPC[pcs[a]] > byPC[pcs[b]] })
	t.Logf("=== %d accesses to %08X, from %d instructions ===",
		len(touches), uint32(transportAt), len(pcs))
	for _, pc := range pcs {
		t.Logf("    MIPS %08X  %d", pc, byPC[pc])
	}
	for i, w := range touches {
		if i >= 24 {
			t.Logf("    ... and %d more", len(touches)-24)
			break
		}
		kind := "read "
		if w.write {
			kind = "WROTE"
		}
		t.Logf("    %s %08X   MIPS %08X   during %s", kind, w.value, w.pc, w.phase)
	}
}
