package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE BOX DOES WITH THE EVENT INFORMATION IT ASKED FOR.
//
// It asked by name. The moment it tunes it arms filter 18 on PID 0x0012 with match unit 4 carrying
// 4e/fe 00/ff 64/ff -- table 0x4E or 0x4F, table_id_extension 0x0064, which is service_id 100,
// Sky One, the channel just selected. Until now this port sent nothing on that PID at all, so the
// filter sat open and empty for the whole of the box's life.
//
// SENDING IT IS NOT THE SAME AS IT BEING READ, and the difference is the entire point of this
// probe. Four distinct causes have produced "the box is not asking for it" on this project, and
// the mirror of that is a section the hardware accepts and no task ever looks at: the demux writes
// it into the ring, nothing errors, and from outside that is indistinguishable from a box that
// parsed it and had nothing to show. So this runs the SAME route twice -- acquire, open the grid,
// tune to a channel -- once with the EIT rung and once without, and compares the two by the only
// thing that separates those cases: WHICH CODE RUNS.
//
// A parser that exists wakes hundreds of instructions that the silent run never reaches. A section
// that is filed and forgotten wakes the ring-drain and nothing beyond it.
//
// IT ASSERTS ITS OWN SUBJECT TWICE OVER: the box must arm PID 0x0012, or there was no filter to
// answer and the comparison is between two silent runs; and the transmitter must actually have put
// sections on air, or the "with" run is the "without" run under another name.
//
// IT ONLY READS.
func TestWhatTheBoxDoesWithThePresentFollowingEIT(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	type run struct {
		pcs     map[uint32]bool
		armed   bool
		onAir   int
		viewing uint32
	}
	watch := func(t *testing.T, name string, schedule broadcast.Schedule) run {
		t.Helper()
		guide := demoGuide(t)
		dict := demoDictionary(t)
		box := restoredBox(t)
		transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
			schedule)
		if err != nil {
			t.Fatal(err)
		}
		want := programmesInTheBlock(t, guide, day)
		registered := 0
		if at := runUntil(t, box, transmitter, 120_000_000,
			registeringProgrammes(box, want, &registered)); at < 0 {
			t.Fatalf("harness: %s registered only %d of %d programmes", name, registered, want)
		}
		runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

		pump := func() error { return transmitter.Pump(box.Machine.Retired) }
		press := azPressFunc(t, box, pump)
		openAllChannelsFinished(t, press, fmt.Sprintf(".artifacts/eit-%s-grid.png", name))

		// THE GRID MUST FILL BEFORE SELECT MEANS ANYTHING. It opens on "Searching for listings"
		// and fills a few tens of millions of instructions later, and a SELECT pressed at the
		// searching screen is consumed by the fill: the press returns the FILLED GRID, which is a
		// screen change and reads exactly like a tune. Measured -- .artifacts/eit-silent-viewing.png
		// from the first run of this probe is the grid, with the banner never drawn and PID 0x0012
		// never armed.
		for i := 0; i < 60_000_000; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		filled := screenNow(t, box)
		if err := dumpScreen(t, box, fmt.Sprintf("eit-%s-filled.png", name)); err != nil {
			t.Fatal(err)
		}
		viewing := uint32(0)
		for attempt := 1; attempt <= 6 && (viewing == 0 || viewing == filled); attempt++ {
			viewing = press(keySelect, "select to view the channel", 80_000_000)
		}
		if viewing == 0 || viewing == filled {
			t.Fatalf("harness: %s never left the filled grid (%08X), so it never tuned and PID "+
				"0x0012 was never armed. Read .artifacts/eit-%s-filled.png", name, filled, name)
		}

		// ONLY NOW DOES THE WATCH GO ON. Everything up to here is identical between the two runs
		// by construction, and counting it would bury the difference under four hundred million
		// instructions of boot.
		pcs := map[uint32]bool{}
		out := run{pcs: pcs, viewing: viewing}
		for i := 0; i < 80_000_000; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			pcs[box.Machine.Core.State().PC&^1] = true
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}
		for _, pid := range box.Demux.ArmedPIDs() {
			if pid == 0x0012 {
				out.armed = true
			}
		}
		out.onAir = int(transmitter.Counts().Events)
		if err := dumpScreen(t, box, fmt.Sprintf("eit-%s-viewing.png", name)); err != nil {
			t.Fatal(err)
		}
		t.Logf("%-8s viewing %08X, PID 0x0012 armed=%v, %d event sections on air, %d distinct PCs",
			name, out.viewing, out.armed, out.onAir, len(pcs))
		return out
	}

	// A CONTROL RUN, BECAUSE "MORE CODE RAN" IS NOT A FINDING ON ITS OWN. Two runs of this box
	// are not identical even with identical input: the carousel, the RTOS and the draw interleave
	// differently, and a set difference between one run and one other run counts that divergence
	// as though it were the EIT. So the SAME schedule is run twice and the difference between the
	// two silent runs is the noise floor everything below is measured against. This project has
	// shipped a number without its control before and had to withdraw it.
	silentA := watch(t, "silent", demoSchedule())
	silentB := watch(t, "control", demoSchedule())
	fed := watch(t, "fed", demoScheduleWithEvents())
	silent := silentA

	if !fed.armed {
		t.Fatal("harness: PID 0x0012 is not armed while viewing, so there was no filter to answer " +
			"and neither run says anything about the EIT")
	}
	if fed.onAir == 0 {
		t.Fatal("harness: the transmitter put no event section on air, so the 'fed' run is the " +
			"'silent' run under another name. Read the wave gate in eventWave before believing " +
			"anything below")
	}

	only := func(run run, others ...map[uint32]bool) []uint32 {
		var out []uint32
	next:
		for pc := range run.pcs {
			for _, other := range others {
				if other[pc] {
					continue next
				}
			}
			out = append(out, pc)
		}
		sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
		return out
	}
	noise := only(silentB, silentA.pcs)
	exclusive := only(fed, silentA.pcs, silentB.pcs)
	t.Logf("NOISE FLOOR: two runs of the SAME silent schedule differ by %d PCs", len(noise))
	t.Logf("%d event sections reached the box, and woke %d PCs neither silent run executed",
		fed.onAir, len(exclusive))
	if len(exclusive) <= len(noise) {
		t.Fatalf("=== THE DIFFERENCE IS INSIDE THE NOISE ===\n    %d exclusive PCs against a %d "+
			"floor from two identical runs. This measurement says nothing about the EIT, and a "+
			"number reported from it would be run-to-run divergence wearing a finding's clothes.",
			len(exclusive), len(noise))
	}
	// CONTIGUOUS RUNS, NOT A LIST OF ADDRESSES. Nineteen hundred addresses printed one per line
	// is a wall nobody reads; the same set as runs of consecutive instructions is a list of the
	// FUNCTIONS that woke, which is what the next session has to disassemble.
	type span struct{ lo, hi uint32 }
	var spans []span
	for _, pc := range exclusive {
		if n := len(spans); n > 0 && pc-spans[n-1].hi <= 8 {
			spans[n-1].hi = pc
			continue
		}
		spans = append(spans, span{lo: pc, hi: pc})
	}
	sort.SliceStable(spans, func(a, b int) bool {
		return spans[a].hi-spans[a].lo > spans[b].hi-spans[b].lo
	})
	t.Logf("they fall in %d contiguous runs; the widest are where the work is:", len(spans))
	for i, sp := range spans {
		if i >= 25 {
			t.Logf("    ... %d narrower runs", len(spans)-i)
			break
		}
		t.Logf("    %08X..%08X  %d bytes", sp.lo, sp.hi, sp.hi-sp.lo+2)
	}
	if len(exclusive) == 0 {
		t.Logf("=== THE SECTIONS ARE ACCEPTED AND NOTHING READS THEM ===")
		t.Logf("    The hardware filed %d sections into filter 18's ring and the guest executed "+
			"not one instruction it would not have executed anyway. So the filter is armed by "+
			"code that never drains it, and the next question is what arms it.", fed.onAir)
		return
	}
	t.Logf("=== SOMETHING READ THEM ===")
	t.Logf("    %d addresses ran only in the fed run, against a %d-address floor from two "+
		"identical silent runs. That margin is the EIT consumer, and it is the thing to "+
		"disassemble.", len(exclusive), len(noise))
	if fed.viewing != silent.viewing {
		t.Logf("    AND THE PICTURE CHANGED: %08X fed against %08X silent. Read "+
			".artifacts/eit-fed-viewing.png against .artifacts/eit-silent-viewing.png",
			fed.viewing, silent.viewing)
	}
}
