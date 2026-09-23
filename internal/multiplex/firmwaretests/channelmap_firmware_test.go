package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// EVERY PLACE THE BOX KEEPS OUR CHANNELS, AND EVERYTHING IT IS STILL ASKING FOR.
//
// Three separate structures are now known to hold channel data and no two of them are the same
// memory: the eighteen-byte line-up records the BAT builds at 0x802B5340, which neither screen
// reads; whatever the now-and-next banner walks at 0x80493B2C, where the first word it takes is
// 0x191 -- Sky Sports 1; and whatever the ALL CHANNELS grid reads 18,767 times on the page above
// it, 0x80494, having never once touched 0x80493. Knowing three structures exist is not the same as
// knowing how many there are, and every one of those was found by accident while asking something
// else.
//
// So this stops guessing at pages and SWEEPS THE WHOLE OF RAM for the numbers the fixture chose.
// Four of the six channel numbers -- 501, 401, 301 and 251 -- are above the threshold at which a
// small integer stops being ordinary, and a halfword anywhere in DRAM holding one of them is the
// box keeping that channel somewhere. The map that comes back is the thing this task has been
// missing: not "does the grid read the line-up" (it does not) but "what channel lists exist at all,
// and is the one the grid walks EMPTY or merely different".
//
// IT DUMPS ALL SIXTEEN MATCH UNITS UNCONDITIONALLY, because the project's own rule says so and it
// is cheap to do here: "the box is not asking for it" has had four distinct causes on this port and
// three of them were the instrument. The unit rules are also the only honest way to ask what the
// box wants on PID 0x52 -- the one PID it arms that nothing has ever fed -- without guessing a
// format, which phase 7 forbids.
//
// IT TAKES THE MAP TWICE: once with the box idle after acquisition, and again with ALL CHANNELS on
// screen. A structure that appears only in the second is one the grid builds; one that shrinks is
// one it filters. Either is the answer.
//
// IT ONLY READS. Nothing is written into the guest, and every number it looks for is one the
// broadcast put there.
func TestWhereTheBoxKeepsOurChannels(t *testing.T) {
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

	listings := guide.On(day)
	// THE DISTINCTIVE ONES ONLY. A value carries information here only where the firmware has no
	// other reason to hold it: watching every identifier once returned 2,532 reads of "101" from a
	// single instruction, which is a loop counter and not a channel.
	const distinctive = 250
	names := map[uint16]string{}
	for i := range listings.Services {
		s := &listings.Services[i]
		if s.Channel >= distinctive {
			names[s.Channel] = s.Name
		}
	}
	if len(names) < 2 {
		t.Fatalf("harness: only %d announced channel numbers are above %d, and a map built from one "+
			"value cannot show a stride, so this probe could not tell an array from a coincidence",
			len(names), distinctive)
	}
	wanted := make([]uint16, 0, len(names))
	for n := range names {
		wanted = append(wanted, n)
	}
	sort.Slice(wanted, func(a, b int) bool { return wanted[a] < wanted[b] })
	t.Logf("sweeping RAM for channel numbers %v", wanted)

	sweep := func(label string) map[uint32]uint16 {
		t.Helper()
		size := box.RAM.Size()
		hits := map[uint32]uint16{}
		for off := uint32(0); off+2 <= size; off += 2 {
			v := uint16(box.RAM.Read(off, bus.Half)) // #nosec G115 -- half read
			if _, mine := names[v]; mine {
				hits[off] = v
			}
		}
		if len(hits) == 0 {
			t.Fatalf("harness: %s -- not one of our channel numbers is anywhere in %d bytes of RAM, "+
				"which cannot be true of a box that has just drawn one of them in a banner. The "+
				"sweep is wrong, not the box", label, size)
		}
		return hits
	}

	// Clusters, not addresses. A channel number on its own is a coincidence; two or more of them
	// close together at one stride is an ARRAY, and the stride and the count are what this task
	// actually needs. 512 bytes is wide enough to hold six records at any plausible stride and
	// narrow enough that two unrelated structures do not merge into one.
	report := func(label string, hits map[uint32]uint16) {
		t.Helper()
		offs := make([]uint32, 0, len(hits))
		for off := range hits {
			offs = append(offs, off)
		}
		sort.Slice(offs, func(a, b int) bool { return offs[a] < offs[b] })
		t.Logf("--- %s: %d halfwords in RAM hold one of our channel numbers ---", label, len(offs))
		i := 0
		for i < len(offs) {
			j := i + 1
			for j < len(offs) && offs[j]-offs[j-1] <= 512 {
				j++
			}
			group := offs[i:j]
			seen := map[uint16]bool{}
			for _, off := range group {
				seen[hits[off]] = true
			}
			stride := "n/a"
			if len(group) > 1 {
				deltas := map[uint32]int{}
				for k := 1; k < len(group); k++ {
					deltas[group[k]-group[k-1]]++
				}
				best, n := uint32(0), 0
				for d, c := range deltas {
					if c > n || (c == n && d < best) {
						best, n = d, c
					}
				}
				stride = fmt.Sprintf("%d bytes (%d of %d gaps)", best, n, len(group)-1)
			}
			t.Logf("  %08X..%08X  %d hits, %d of our %d channels, commonest stride %s",
				0x80000000|group[0], 0x80000000|group[len(group)-1], len(group), len(seen),
				len(names), stride)
			for _, off := range group {
				t.Logf("        %08X = %d (%s)", 0x80000000|off, hits[off], names[hits[off]])
			}
			i = j
		}
	}

	report("box idle after acquisition", sweep("idle"))

	// WHAT IS THE BOX STILL ASKING FOR? All sixteen units, printed whether or not they look
	// programmed, beside the PID channels they cannot be paired with by index.
	dumpDemux := func(label string) {
		t.Helper()
		filters := box.Demux.ArmedFilters()
		t.Logf("--- %s: %d armed section filters ---", label, len(filters))
		for _, f := range filters {
			t.Logf("  filter %2d  PID %#05x  register %08X", f.Filter, f.PID, f.Word)
		}
		t.Logf("--- %s: all sixteen match units ---", label)
		for unit := uint8(0); unit < 16; unit++ {
			line := fmt.Sprintf("  unit %2d ", unit)
			any := false
			for b := uint8(0); b < 8; b++ {
				m, ok := box.Demux.Match(unit, b)
				if !ok {
					line += "  --  "
					continue
				}
				any = true
				line += fmt.Sprintf(" %02X/%02X", m.Value, m.Mask)
			}
			if !any {
				line += "  (no rule set)"
			}
			t.Logf("%s", line)
		}
	}
	dumpDemux("box idle after acquisition")

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		settled := pressAndLetItFinish(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, raw, budget)
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}

	grid := openAllChannels(t, press, ".artifacts/channel-map-all-channels.png", false)
	_ = grid
	if err := dumpScreen(t, box, "channel-map-all-channels.png"); err != nil {
		t.Fatal(err)
	}

	report("ALL CHANNELS on screen", sweep("grid"))
	dumpDemux("ALL CHANNELS on screen")
}
