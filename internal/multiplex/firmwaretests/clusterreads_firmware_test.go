package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE SORTED CHANNEL INDEX, AND WHETHER EITHER SCREEN READS IT.
//
// A sweep of the whole of DRAM for the four channel numbers distinctive enough to mean something --
// 251, 301, 401 and 501 -- found the box keeping them in twenty-eight places, and two of those are
// not scratch. They are ARRAYS, with a constant stride and their entries IN ASCENDING CHANNEL
// ORDER, which is the order an ALL CHANNELS grid draws in and an order nothing else in this box has
// a reason to impose:
//
//	0x802AD9C8  six-byte stride     251, 301, 401, 501
//	0x802A7C2C  twenty-four stride  251, 301, 401, 501
//
// It also settled a lead that had been driving this task and was wrong. The banner was believed to
// resolve channels "from page 0x80493" because a read there returned 0x191, Sky Sports 1; the grid
// reads the page above it eighteen thousand times and never touches 0x80493, and that difference
// was the most concrete thing the task had. But NOT ONE of our channel numbers is stored anywhere on
// either page. They are the o-code interpreter's own working memory, the 0x191 was a value in
// flight through it, and the difference between the two screens was a difference in how far the
// interpreter's stack grew. An address a value passes THROUGH is not where it lives.
//
// So this asks the question directly, of both screens, against every array the sweep found rather
// than against a page guessed at in advance:
//
//	does the screen that WORKS read the sorted index?   does the screen that draws NO ROWS?
//
// **THE BANNER IS THE CONTROL AND IT IS WHAT MAKES A NEGATIVE MEAN ANYTHING.** It draws a real
// programme on a real channel off the same broadcast. If neither screen reads these arrays then
// they are an acquisition-time structure nothing draws from, exactly as the eighteen-byte line-up
// records turned out to be, and the search moves on with two more candidates eliminated instead of
// one more page to stare at. If the banner reads them and the grid does not, the grid is looking
// somewhere else and the PCs that did the reading name where.
//
// **EACH SCREEN GETS ITS OWN BOX, and that is a correction rather than a precaution.** Measuring
// both on one box drew the banner first and then could not find the route to the grid at all: with
// the banner over the picture every screen on the way has a different hash -- box office came back
// 110D5C60 where a clean box draws 1CBD8D51 -- so six presses later the probe was still in the box
// office menu and failed, correctly, rather than measuring whatever it had landed on. Acquisition
// costs a few seconds; a route that starts from a screen nobody pinned costs a wrong finding.
//
// IT ONLY READS. Nothing is written into the guest, and every number it hunts for is one the
// broadcast put there.
func TestWhetherEitherScreenReadsTheSortedChannelIndex(t *testing.T) {
	t.Run("the now-and-next banner", func(t *testing.T) {
		measureChannelArrayReads(t, "banner", func(p *screenProbe) uint32 {
			drew := p.press(keySky, "sky (now-and-next banner)", 60_000_000)
			if drew == 0 {
				t.Fatal("harness: the Sky key drew nothing new, so no banner was measured and " +
					"the control this whole probe depends on does not exist")
			}
			return drew
		})
	})
	t.Run("the ALL CHANNELS grid", func(t *testing.T) {
		measureChannelArrayReads(t, "all-channels", func(p *screenProbe) uint32 {
			p.watching = false
			return openAllChannels(t, func(raw uint8, name string, budget int) uint32 {
				if raw == keySelect {
					runUntil(t, p.box, p.transmitter, 8_000_000, func(int) bool { return false })
					p.reset()
					p.watching = true
				}
				drew := p.press(raw, name, budget)
				p.watching = false
				return drew
			}, ".artifacts/cluster-reads-all-channels.png", false)
		})
	})
}

// screenProbe is one box, acquired and quiet, with a read watch over the channel arrays.
type screenProbe struct {
	t           *testing.T
	box         *board.Runtime
	transmitter *multiplex.Multiplex
	regions     []channelArray
	hits        []map[reader]int
	anyRead     int
	watching    bool
}

// reader is one (instruction, caller) pair that touched a channel array.
type reader struct{ pc, ra, via uint32 }

type channelArray struct {
	lo, hi uint32
	stride uint32
	held   int
}

func (p *screenProbe) reset() {
	p.anyRead = 0
	for i := range p.hits {
		p.hits[i] = map[reader]int{}
	}
}

func (p *screenProbe) press(raw uint8, name string, budget int) uint32 {
	p.t.Helper()
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !p.watching || a.Fetch || a.Write {
			return
		}
		p.anyRead++
		at := a.Virtual & 0x1fffffff
		for i, r := range p.regions {
			if at < r.lo || at >= r.hi {
				continue
			}
			// THE CALLER, NOT JUST THE READER, AND THEN ONE HOP FURTHER. The instruction that
			// reads the sorted index in the working screen is the byte load inside memmove, a leaf
			// library routine the whole application shares, whose address names nothing. ra at the
			// read gave 0x8001DB22 -- and that is not a caller either, it is the return slot of a
			// THUNK: 0x8001DB18 pushes ra, loads 0x800F99ED out of its literal pool, jalrs it and
			// returns. The application's C library is reached through a table of these, so every
			// memmove in the box reports the same ra.
			//
			// The thunk stores the REAL caller's return address at 4(sp) and memmove never touches
			// sp, so that word is still there at the read. Reading it is what turns "something
			// copied it" into a function.
			st := p.box.Machine.Core.State()
			r := reader{pc: st.PC &^ 1, ra: st.GPR[31] &^ 1}
			if sp := st.GPR[29] & 0x1fffffff; sp+8 <= p.box.RAM.Size() {
				r.via = p.box.RAM.Read(sp+4, bus.Word) &^ 1
			}
			p.hits[i][r]++
		}
	}}
	settled := pressAndLetItFinishHooked(p.t, p.box,
		func() error { return p.transmitter.Pump(p.box.Machine.Retired) }, hooks, raw, budget)
	p.t.Logf("%-32s drew %08X", name, settled)
	return settled
}

// measureChannelArrayReads acquires a fresh box, finds the channel arrays by sweeping for them, runs
// the caller's route with the watch on, and reports.
func measureChannelArrayReads(t *testing.T, artefact string, route func(*screenProbe) uint32) {
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
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	regions := findChannelArrays(t, box, guide.On(day))
	p := &screenProbe{t: t, box: box, transmitter: transmitter, regions: regions}
	p.hits = make([]map[reader]int, len(regions))
	p.reset()

	p.watching = true
	drew := route(p)
	p.watching = false
	if err := dumpScreen(t, box, "cluster-reads-"+artefact+".png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the screen settled on %08X over %d data reads", drew, p.anyRead)

	if p.anyRead == 0 {
		t.Fatal("harness: the observer saw no data read at all while the screen drew, so its zeros " +
			"below are the instrument and not the screen")
	}
	total := 0
	for i, r := range regions {
		n := 0
		for _, c := range p.hits[i] {
			n += c
		}
		total += n
		if n == 0 {
			t.Logf("    %08X (stride %d): NOT READ ONCE", 0x80000000|r.lo, r.stride)
			continue
		}
		pcs := make([]reader, 0, len(p.hits[i]))
		for pc := range p.hits[i] {
			pcs = append(pcs, pc)
		}
		sort.Slice(pcs, func(a, b int) bool { return p.hits[i][pcs[a]] > p.hits[i][pcs[b]] })
		t.Logf("    %08X (stride %d): %d reads from %d (instruction, caller) pairs",
			0x80000000|r.lo, r.stride, n, len(pcs))
		for _, pc := range pcs {
			t.Logf("          read at %08X  via %08X  caller %08X  %d reads",
				pc.pc, pc.ra, pc.via, p.hits[i][pc])
		}
	}
	if total == 0 {
		t.Logf("VERDICT: this screen reads NONE of the sorted channel arrays, in %d data reads.",
			p.anyRead)
		return
	}
	t.Logf("VERDICT: this screen READS the channel arrays -- %d reads across %d arrays.",
		total, len(regions))
}

// findChannelArrays locates the uniform-stride arrays holding our channel numbers.
//
// IT FINDS THEM RATHER THAN HARD-CODING THEM. Addresses in this box's heap are stable enough to
// quote in a finding and not stable enough to pin a test to: they move when the broadcast changes,
// and a probe watching an address the array has left reports a confident zero.
func findChannelArrays(t *testing.T, box *board.Runtime, listings *multiplex.Listings) []channelArray {
	t.Helper()
	// A value carries information here only where the firmware has no other reason to hold it:
	// watching every identifier once returned 2,532 reads of "101" from a single instruction, which
	// is a loop counter and not a channel.
	const distinctive = 250
	names := map[uint16]string{}
	for i := range listings.Services {
		if s := &listings.Services[i]; s.Channel >= distinctive {
			names[s.Channel] = s.Name
		}
	}
	if len(names) < 3 {
		t.Fatalf("harness: only %d announced channel numbers are above %d, and three are needed "+
			"before a run of them can be told from a coincidence", len(names), distinctive)
	}
	size := box.RAM.Size()
	var offs []uint32
	at := map[uint32]uint16{}
	for off := uint32(0); off+2 <= size; off += 2 {
		v := uint16(box.RAM.Read(off, bus.Half)) // #nosec G115 -- half read
		if _, mine := names[v]; mine {
			offs = append(offs, off)
			at[off] = v
		}
	}
	if len(offs) == 0 {
		t.Fatal("harness: not one of our channel numbers is anywhere in DRAM, which cannot be true " +
			"of a box that has just acquired our broadcast. The sweep is wrong, not the box")
	}
	sort.Slice(offs, func(a, b int) bool { return offs[a] < offs[b] })

	var out []channelArray
	for i := 0; i < len(offs); {
		j := i + 1
		for j < len(offs) && offs[j]-offs[j-1] <= 512 {
			j++
		}
		group := offs[i:j]
		i = j
		seen := map[uint16]bool{}
		for _, off := range group {
			seen[at[off]] = true
		}
		// An array, not a coincidence: three or more of the four distinctive numbers, and a single
		// stride that explains every gap. Two-of-four with one stride is one coincidence away from
		// being nothing.
		if len(seen) < 3 || len(group) < 3 {
			continue
		}
		stride := group[1] - group[0]
		uniform := true
		for k := 2; k < len(group); k++ {
			if group[k]-group[k-1] != stride {
				uniform = false
				break
			}
		}
		if !uniform {
			continue
		}
		// Widen by four strides each way: the two non-distinctive channels (101 and 121) sit
		// outside the hits and are still part of the array.
		lo, hi := group[0], group[len(group)-1]+stride
		if lo > 4*stride {
			lo -= 4 * stride
		}
		out = append(out, channelArray{lo: lo, hi: hi + 4*stride, stride: stride, held: len(seen)})
	}
	if len(out) == 0 {
		t.Fatal("harness: the sweep found no uniform-stride array holding three or more of our " +
			"channels, so there is nothing for this probe to watch and its zeros would be its own")
	}
	for _, r := range out {
		t.Logf("watching %08X..%08X, stride %d, holding %d of our %d distinctive channels",
			0x80000000|r.lo, 0x80000000|r.hi, r.stride, r.held, len(names))
	}
	return out
}

// keySky draws the now-and-next search-and-scan banner over the picture. TV Guide is 0xCC and
// opens the full TV GUIDE menu directly.
const keySky = 0x80
