package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH DMA CHANNELS THE BOX PROGRAMS, AND WHETHER ANY OF THEM IS ASKING FOR A STREAM.
//
// "No satellite signal is being received" is the box saying it has tuned to a service and never
// seen a stream carrying it. The obvious answer -- send one -- was blocked on Demux.PushTransport,
// which does nothing unless bit 0 of the demux register at 0x140 is set, and the box never writes
// that register once.
//
// **THAT GATE IS PROBABLY AN INVENTION, AND THAT MATTERS MORE THAN THE MEASUREMENT.** The record
// describes demux +0x140, +0x144 and +0x148 as the section-filter MATCH AND MASK programming,
// written select-value-commit; this model already treats +0x144 and +0x148 as exactly that, and
// +0x140 alone became a "control" whose bit 0 gates the transport path. Nothing measured says it
// is an enable. So "the box never enables the transport path" may be a fact about our gate rather
// than about the box.
//
// PushTransport's own comment says what the real path is: packets "moved by the MEDIA DMA". The
// record puts that DMA at 0xB0009000 -- thirteen channels, descriptors at 0x80108A60 + 40*ch, the
// physical address written to +0x040 + 0x10*ch, then +0x220 = 1, then bit ch set in +0x010, with
// completion in +0x120 and the LISR clearing the enable itself.
//
// So a box asking for a stream would be a box PROGRAMMING A DMA CHANNEL for it. This watches every
// DMA register write from boot, through acquisition, through the guide, and through TUNING -- and
// reports which channels are armed and when, because a channel that only appears once the box is
// viewing is the media path asking.
//
// IT ASSERTS ITS OWN SUBJECT: the DMA must be written at all, or a tally of zero is the instrument.
//
// IT ONLY READS.
func TestWhichDMAChannelsTheBoxProgramsWhenViewing(t *testing.T) {
	const (
		dmaBase   = 0xB0009000
		dmaSize   = 0x1000
		enableReg = 0x010
		goReg     = 0x220
	)
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	type hit struct {
		reg   uint32
		value uint32
		phase string
	}
	var enables []hit
	byReg := map[uint32]int{}
	writes := 0
	phase := "boot and acquisition"
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write || a.Fetch {
			return
		}
		at := a.Virtual
		if at < dmaBase || at >= dmaBase+dmaSize {
			return
		}
		writes++
		reg := (at - dmaBase) &^ 3
		byReg[reg]++
		if reg == enableReg || reg == goReg {
			enables = append(enables, hit{reg: reg, value: a.Value, phase: phase})
		}
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntilHooked(t, box, transmitter, hooks, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntilHooked(t, box, transmitter, hooks, 20_000_000, func(int) bool { return false })
	acquisitionWrites := writes

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
	openAllChannelsFinished(t, press, ".artifacts/mediadma-grid.png")
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	guideWrites := writes

	phase = "VIEWING a channel"
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
	if err := dumpScreen(t, box, "mediadma-viewing.png"); err != nil {
		t.Fatal(err)
	}
	if writes == 0 {
		t.Fatal("harness: the guest never wrote a DMA register in this whole run, so a tally of " +
			"zero is the instrument and not the box")
	}
	t.Logf("the box is viewing %08X", viewing)
	t.Logf("DMA writes: %d during boot and acquisition, %d more opening the guide, %d more while "+
		"viewing", acquisitionWrites, guideWrites-acquisitionWrites, writes-guideWrites)

	regs := make([]uint32, 0, len(byReg))
	for reg := range byReg {
		regs = append(regs, reg)
	}
	sort.Slice(regs, func(a, b int) bool { return byReg[regs[a]] > byReg[regs[b]] })
	t.Logf("=== every DMA register the box writes ===")
	for i, reg := range regs {
		if i >= 16 {
			t.Logf("    ... and %d more", len(regs)-16)
			break
		}
		note := ""
		switch {
		case reg == enableReg:
			note = "   <- the per-channel ENABLE bitmap"
		case reg == goReg:
			note = "   <- the go bit"
		case reg >= 0x040 && reg < 0x040+0x10*13:
			note = "   <- descriptor address for channel " +
				string(rune('0'+(reg-0x040)/0x10)) // #nosec G115 -- thirteen channels
		}
		t.Logf("    +%03X  %d writes%s", reg, byReg[reg], note)
	}
	t.Logf("=== the channels armed, in order ===")
	seen := map[uint32]bool{}
	for _, h := range enables {
		if h.reg != enableReg {
			continue
		}
		for ch := uint32(0); ch < 13; ch++ {
			if h.value&(1<<ch) == 0 || seen[ch] {
				continue
			}
			seen[ch] = true
			t.Logf("    channel %2d first armed during %s", ch, h.phase)
		}
	}
	if len(seen) == 0 {
		t.Logf("    NONE. The box arms no DMA channel at any point, so nothing is asking for a " +
			"stream through this path either.")
	}
}
