package statehash_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

func newRAM(t *testing.T, pages uint32) *memory.RAM {
	t.Helper()
	r, err := memory.NewRAM("dram", pages*memory.DirtyPageLen)
	if err != nil {
		t.Fatalf("NewRAM: %v", err)
	}
	return r
}

func newHasher(t *testing.T, r *memory.RAM) *statehash.Hasher {
	t.Helper()
	h, err := statehash.New(r)
	if err != nil {
		t.Fatalf("statehash.New: %v", err)
	}
	return h
}

// aState is a machine in a recognisable, entirely non-default condition, so that a hash which
// silently ignores one of its fields has something to ignore.
func aState() statehash.State {
	s := statehash.State{PC: 0x80081C58, ISA: 1, HI: 0x0000DEAD, LO: 0x0000BEEF}
	for i := range s.GPR {
		s.GPR[i] = 0x1000_0000 + uint32(i)*0x11
	}
	for i := range s.COP0 {
		s.COP0[i] = 0x2000_0000 + uint32(i)*0x13
	}
	return s
}

// Every field of the machine must reach the hash. A field left out is a divergence the oracle
// comparison cannot see, and the reader hunts for it somewhere else entirely.
func TestEveryFieldOfTheMachineChangesTheHash(t *testing.T) {
	t.Parallel()
	ram := newRAM(t, 4)
	h := newHasher(t, ram)
	base := h.Hash(aState())

	cases := []struct {
		name  string
		alter func(*statehash.State)
	}{
		{"PC", func(s *statehash.State) { s.PC ^= 4 }},
		// The one the test plan singles out. Omit the ISA bit and a MIPS16/MIPS32 mode
		// divergence reads as a wrong instruction, which sends the reader into the decoder.
		{"the ISA mode bit", func(s *statehash.State) { s.ISA ^= 1 }},
		{"HI", func(s *statehash.State) { s.HI ^= 1 }},
		{"LO", func(s *statehash.State) { s.LO ^= 1 }},
		{"GPR 0", func(s *statehash.State) { s.GPR[0] ^= 1 }},
		{"GPR 29, the stack pointer", func(s *statehash.State) { s.GPR[29] ^= 1 }},
		{"GPR 31", func(s *statehash.State) { s.GPR[31] ^= 1 }},
		{"COP0 9, Count", func(s *statehash.State) { s.COP0[9] ^= 1 }},
		{"COP0 12, Status", func(s *statehash.State) { s.COP0[12] ^= 1 }},
		{"COP0 14, EPC", func(s *statehash.State) { s.COP0[14] ^= 1 }},
		{"COP0 31", func(s *statehash.State) { s.COP0[31] ^= 1 }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := aState()
			tc.alter(&s)
			if got := newHasher(t, newRAM(t, 4)).Hash(s); got == base {
				t.Fatalf("changing %s left the hash at %#08x, so the hash does not cover it",
					tc.name, base)
			}
		})
	}
}

// Swapping two registers must change the hash: a hash that folded them order-independently would
// call two different machines the same. Both files are checked, because a hash can easily be
// order-dependent in one and not the other - and COP0 12 and 13 holding each other's value is
// Status and Cause swapped, which is a machine in a completely different condition.
func TestRegisterOrderIsPartOfTheHash(t *testing.T) {
	t.Parallel()
	cases := map[string]func(a statehash.State) statehash.State{
		"two general registers": func(a statehash.State) statehash.State {
			b := a
			b.GPR[4], b.GPR[5] = a.GPR[5], a.GPR[4]
			return b
		},
		"COP0 12 and 13, Status and Cause": func(a statehash.State) statehash.State {
			b := a
			b.COP0[12], b.COP0[13] = a.COP0[13], a.COP0[12]
			return b
		},
		"HI and LO": func(a statehash.State) statehash.State {
			b := a
			b.HI, b.LO = a.LO, a.HI
			return b
		},
	}
	for name, swap := range cases {
		name, swap := name, swap
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := aState()
			ha := newHasher(t, newRAM(t, 4)).Hash(a)
			hb := newHasher(t, newRAM(t, 4)).Hash(swap(a))
			if ha == hb {
				t.Fatalf("swapping %s left the hash at %#08x", name, ha)
			}
		})
	}
}

func TestDramReachesTheHash(t *testing.T) {
	t.Parallel()
	ram := newRAM(t, 4)
	h := newHasher(t, ram)
	before := h.Hash(aState())

	ram.Write(2*memory.DirtyPageLen+16, bus.Byte, 1)
	if after := h.Hash(aState()); after == before {
		t.Fatalf("a byte written to DRAM left the hash at %#08x", before)
	}
}

// The digest is incremental, so the arithmetic that takes a page out and puts it back has to be
// exact: writing a byte and putting the old one back must return the digest it started from.
func TestTheDramDigestIsIncrementalAndExact(t *testing.T) {
	t.Parallel()
	ram := newRAM(t, 8)
	h := newHasher(t, ram)
	ram.Write(100, bus.Word, 0x11223344)
	ram.Write(5*memory.DirtyPageLen, bus.Word, 0x55667788)
	start := h.RAMDigest()

	ram.Write(3*memory.DirtyPageLen+8, bus.Byte, 0xAB)
	changed := h.RAMDigest()
	if changed == start {
		t.Fatal("a write left the DRAM digest unchanged")
	}

	ram.Write(3*memory.DirtyPageLen+8, bus.Byte, 0)
	if back := h.RAMDigest(); back != start {
		t.Fatalf("reverting the write gave %#08x, want the original %#08x - the incremental "+
			"arithmetic does not undo itself", back, start)
	}

	// And an incremental digest must equal one computed from scratch over the same memory.
	if rebuilt := (func() uint32 { h.Rebuild(); return h.RAMDigest() })(); rebuilt != start {
		t.Fatalf("rebuilding from scratch gave %#08x and the incremental digest says %#08x",
			rebuilt, start)
	}
}

// Two pages swapping their contents must change the digest. It would not if a page digest did not
// include the page's index, and XOR would then cancel the two changes exactly.
func TestSwappingTwoPagesChangesTheDigest(t *testing.T) {
	t.Parallel()
	one := newRAM(t, 4)
	one.Write(0, bus.Word, 0xAAAAAAAA)
	one.Write(memory.DirtyPageLen, bus.Word, 0xBBBBBBBB)

	other := newRAM(t, 4)
	other.Write(0, bus.Word, 0xBBBBBBBB)
	other.Write(memory.DirtyPageLen, bus.Word, 0xAAAAAAAA)

	if a, b := newHasher(t, one).RAMDigest(), newHasher(t, other).RAMDigest(); a == b {
		t.Fatalf("two pages with swapped contents both digest to %#08x", a)
	}
}

// The same memory must digest the same whether it was written in one go or over many checkpoints,
// or a run that happened to check in more often would disagree with one that did not.
func TestTheDigestDoesNotDependOnWhenItWasTaken(t *testing.T) {
	t.Parallel()
	writes := []struct {
		off uint32
		val uint32
	}{{0, 1}, {memory.DirtyPageLen, 2}, {3*memory.DirtyPageLen + 40, 3}, {64, 4}}

	all := newRAM(t, 4)
	for _, w := range writes {
		all.Write(w.off, bus.Word, w.val)
	}
	once := newHasher(t, all).RAMDigest()

	stepped := newRAM(t, 4)
	h := newHasher(t, stepped)
	for _, w := range writes {
		stepped.Write(w.off, bus.Word, w.val)
		h.RAMDigest() // fold after every write
	}
	if got := h.RAMDigest(); got != once {
		t.Fatalf("folding after every write gave %#08x and folding once gave %#08x", got, once)
	}
}

func TestNewRefusesAMemoryItCannotCover(t *testing.T) {
	t.Parallel()
	if _, err := statehash.New(nil); err == nil {
		t.Fatal("a nil memory must be refused")
	}
	if _, err := statehash.New(emptyRAM{}); err == nil {
		t.Fatal("a memory reporting no pages must be refused: the hash would cover no DRAM and " +
			"every checkpoint would agree for the wrong reason")
	} else {
		t.Logf("caught: %v", err)
	}
}

type emptyRAM struct{}

func (emptyRAM) Pages() uint32                      { return 0 }
func (emptyRAM) PageLen() uint32                    { return 4096 }
func (emptyRAM) EachDirtyPage(func(uint32, []byte)) {}
func (emptyRAM) ClearDirty()                        {}
func (emptyRAM) MarkAllDirty()                      {}

// ---- the checkpoint stream ----

// run drives a toy machine for n instructions, writing a checkpoint stream. alter is called before
// each instruction so a test can perturb exactly one of them.
func run(t *testing.T, n uint64, interval uint64, alter func(i uint64, s *statehash.State, ram *memory.RAM)) string {
	t.Helper()
	ram := newRAM(t, 4)
	h := newHasher(t, ram)
	var out bytes.Buffer
	e, err := statehash.NewEmitter(&out, interval, h)
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	s := statehash.State{PC: 0xBFC00008}
	for i := uint64(0); i < n; i++ {
		// A deterministic toy program: it moves the PC, touches a register and writes DRAM.
		s.PC += 4
		s.GPR[uint32(i)%32] = uint32(i) * 7
		s.COP0[9] = uint32(i) // Count advances, as it must
		ram.Write(uint32(i*4)%(4*memory.DirtyPageLen), bus.Word, uint32(i))
		if alter != nil {
			alter(i, &s, ram)
		}
		if err := e.Observe(i, s); err != nil {
			t.Fatalf("Observe at %d: %v", i, err)
		}
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return out.String()
}

// TC-1.5, first half: two identical runs produce byte-identical streams.
func TestTwoIdenticalRunsProduceIdenticalStreams(t *testing.T) {
	t.Parallel()
	a := run(t, 5000, 1000, nil)
	b := run(t, 5000, 1000, nil)
	if a != b {
		t.Fatalf("two runs of the same program disagree:\n%s\nversus\n%s", a, b)
	}
	if strings.Count(a, "\n") < 5 {
		t.Fatalf("the stream has %d lines, so this comparison proves almost nothing:\n%s",
			strings.Count(a, "\n"), a)
	}
}

// TC-1.5, second half: flipping the ISA mode bit at a known point must diverge the stream, and
// must diverge it from that point onwards rather than at the end.
func TestFlippingTheIsaBitDivergesTheStream(t *testing.T) {
	t.Parallel()
	plain := run(t, 5000, 1000, nil)
	flipped := run(t, 5000, 1000, func(i uint64, s *statehash.State, _ *memory.RAM) {
		if i >= 2500 {
			s.ISA = 1
		}
	})
	if plain == flipped {
		t.Fatal("flipping the ISA mode bit half way through left the stream identical, so a " +
			"MIPS16/MIPS32 divergence would be invisible")
	}

	p, f := strings.Split(plain, "\n"), strings.Split(flipped, "\n")
	first := -1
	for i := range p {
		if i < len(f) && p[i] != f[i] {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("the streams differ in length but share every line they have")
	}
	// Line 0 is the header; checkpoints are at 0, 1000, 2000, 3000..., so the flip at 2500
	// must first show at the 3000 checkpoint, which is line 4.
	if got := p[first]; !strings.HasPrefix(got, "3000 ") {
		t.Fatalf("the first differing line is %q; the flip at instruction 2500 should first "+
			"show at the 3000 checkpoint", got)
	}
	t.Logf("diverged at %q versus %q", p[first], f[first])
}

func TestTheStreamHasAHeaderAndATrailer(t *testing.T) {
	t.Parallel()
	s := run(t, 3001, 1000, nil)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")

	want := fmt.Sprintf("%s %d interval=%d", statehash.StreamMagic, statehash.StreamVersion, 1000)
	if lines[0] != want {
		t.Fatalf("the header is %q, want %q", lines[0], want)
	}
	// Checkpoints at 0, 1000, 2000, 3000.
	if n := len(lines); n != 6 {
		t.Fatalf("the stream has %d lines, want a header, four checkpoints and a trailer:\n%s", n, s)
	}
	for i, at := range []string{"0", "1000", "2000", "3000"} {
		if !strings.HasPrefix(lines[1+i], at+" 0x") {
			t.Fatalf("checkpoint line %d is %q, want it to start %q and an 0x-prefixed hash",
				i, lines[1+i], at)
		}
	}
	if got, want := lines[5], "END 4 3000"; got != want {
		t.Fatalf("the trailer is %q, want %q - a stream without an exact count and final "+
			"instruction cannot be told apart from one that was cut short", got, want)
	}
}

// The interval is a dial. 1,000 costs 3.6 MB for a cold boot and 10,000 costs 0.4 MB, and a run
// that needs finer resolution must be able to ask for it.
func TestTheIntervalIsADial(t *testing.T) {
	t.Parallel()
	counts := map[uint64]int{}
	for _, interval := range []uint64{100, 1000, 5000} {
		s := run(t, 10000, interval, nil)
		counts[interval] = strings.Count(s, "\n") - 2 // less the header and the trailer
		if !strings.Contains(s, fmt.Sprintf("interval=%d", interval)) {
			t.Fatalf("the header does not record interval %d:\n%s", interval, s[:60])
		}
	}
	if counts[100] != 100 || counts[1000] != 10 || counts[5000] != 2 {
		t.Fatalf("checkpoint counts are %v, want 100, 10 and 2", counts)
	}
}

// The hash is uppercase hex with an 0x prefix and eight digits, because the oracle's own hex32 is
// and the two streams have to be byte-identical. A lower-cased or unpadded hash is the casing
// mismatch that reported a plausible zero for every address with a hex letter in it.
func TestTheHashIsWrittenInTheCanonicalForm(t *testing.T) {
	t.Parallel()
	s := run(t, 2000, 1000, nil)
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		at := strings.Index(line, "0x")
		if at < 0 {
			continue // the header and the trailer
		}
		hash := line[at:]
		if len(hash) != 10 {
			t.Fatalf("%q: the hash is %d characters, want 0x and eight digits", line, len(hash))
		}
		if hash[:2] != "0x" || hash[2:] != strings.ToUpper(hash[2:]) {
			t.Fatalf("%q: the hash is not 0x followed by upper-case digits", line)
		}
	}
}

func TestNewEmitterRefusesAnIntervalOfZero(t *testing.T) {
	t.Parallel()
	h := newHasher(t, newRAM(t, 4))
	if _, err := statehash.NewEmitter(&bytes.Buffer{}, 0, h); err == nil {
		t.Fatal("an interval of zero must be refused")
	} else {
		t.Logf("caught: %v", err)
	}
	if _, err := statehash.NewEmitter(&bytes.Buffer{}, 1000, nil); err == nil {
		t.Fatal("a nil hasher must be refused")
	}
}

func TestObserveAfterCloseIsRefused(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	e, err := statehash.NewEmitter(&out, 1000, newHasher(t, newRAM(t, 4)))
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	if err := e.Observe(0, aState()); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := e.Observe(1000, aState()); err == nil {
		t.Fatal("a checkpoint written after the trailer must be refused: it would sit after END " +
			"where no reader looks for it")
	} else {
		t.Logf("caught: %v", err)
	}
}

// A writer that fails must stop the stream rather than leave a hole in it, because a stream with a
// missing checkpoint compares as a divergence at an instruction where nothing happened.
func TestAFailingWriterStopsTheStream(t *testing.T) {
	t.Parallel()
	// Enough instructions to push the emitter's buffer out to the writer: a failure that never
	// reaches the writer is not a failure this test can see.
	e, err := statehash.NewEmitter(&failAfter{n: 100}, 10, newHasher(t, newRAM(t, 4)))
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	var seen error
	for i := uint64(0); i < 100000 && seen == nil; i += 10 {
		seen = e.Observe(i, aState())
	}
	if seen == nil {
		t.Fatal("a writer that fails must be reported")
	}
	if err := e.Observe(2000, aState()); err == nil {
		t.Fatal("the failure must be sticky: a stream that resumes after a gap is worse than one " +
			"that stops")
	}
	t.Logf("caught: %v", seen)
}

type failAfter struct {
	n       int
	written int
}

func (f *failAfter) Write(p []byte) (int, error) {
	if f.written+len(p) > f.n {
		return 0, fmt.Errorf("no room")
	}
	f.written += len(p)
	return len(p), nil
}

// A caller that does not observe every instruction must still put its checkpoints on the same
// boundaries, because the other implementation's cadence is its own. If the next boundary were
// computed from the instruction observed rather than the boundary it crossed, the two streams
// would drift apart a little at every checkpoint and the divergence would look like the machine.
func TestTheCadenceDoesNotDriftWhenAnObservationIsLate(t *testing.T) {
	t.Parallel()
	ram := newRAM(t, 4)
	h := newHasher(t, ram)
	var out bytes.Buffer
	// A short interval against a step that does not divide it, so the drift a per-observation
	// cadence would introduce accumulates past a whole window inside this run rather than only
	// over a 447,000-checkpoint boot nobody can run in a unit test.
	e, err := statehash.NewEmitter(&out, 100, h)
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	s := statehash.State{PC: 0xBFC00008}
	for i := uint64(0); i < 10000; i += 7 { // never lands on a boundary after the first
		s.PC += 28
		if err := e.Observe(i, s); err != nil {
			t.Fatalf("Observe at %d: %v", i, err)
		}
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	windows := make([]uint64, 0, len(lines))
	for _, line := range lines {
		var at uint64
		if _, err := fmt.Sscanf(line, "%d ", &at); err != nil {
			continue // the header and the trailer
		}
		windows = append(windows, at/100)
	}
	if len(windows) == 0 {
		t.Fatalf("no checkpoints were emitted at all:\n%s", out.String())
	}
	for i, w := range windows {
		if w != uint64(i) {
			t.Fatalf("checkpoint %d fell in window %d; the windows must be 0,1,2,... with no "+
				"gaps and no repeats, or the cadence has drifted: %v", i, w, windows)
		}
	}
	if len(windows) != 100 {
		t.Fatalf("%d checkpoints over 10,000 instructions at interval 100: %v",
			len(windows), windows)
	}
}
