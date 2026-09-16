package statehash_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

// emit runs a toy machine to n instructions and returns its checkpoint stream. corrupt, when it
// is not nil, is called at every instruction and may perturb the machine - which is how a
// divergence gets injected at a known instruction rather than at a convenient one.
func emit(t *testing.T, n, interval uint64, corrupt func(i uint64, s *statehash.State)) string {
	t.Helper()
	ram, err := memory.NewRAM("dram", 4*memory.DirtyPageLen)
	if err != nil {
		t.Fatalf("NewRAM: %v", err)
	}
	h, err := statehash.New(ram)
	if err != nil {
		t.Fatalf("statehash.New: %v", err)
	}
	var out bytes.Buffer
	e, err := statehash.NewEmitter(&out, interval, h)
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	s := statehash.State{PC: 0xBFC00008}
	for i := uint64(0); i < n; i++ {
		s.PC += 4
		s.GPR[i%32] = uint32(i) * 7
		s.COP0[9] = uint32(i)
		ram.Write(uint32(i*4)%(4*memory.DirtyPageLen), bus.Word, uint32(i))
		if corrupt != nil {
			corrupt(i, &s)
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

func read(t *testing.T, name, text string) *statehash.Stream {
	t.Helper()
	s, err := statehash.ReadStream(name, strings.NewReader(text))
	if err != nil {
		t.Fatalf("ReadStream(%s): %v", name, err)
	}
	return s
}

// TC-1.6: a divergence injected at instruction 4,500,000 must be reported in the
// 4,500,000..4,500,999 window - localised, not merely detected.
func TestAnInjectedDivergenceIsLocalisedToItsWindow(t *testing.T) {
	t.Parallel()
	const (
		n        = 4_502_000
		interval = 1000
		at       = 4_500_000
	)

	good := emit(t, n, interval, nil)
	bad := emit(t, n, interval, func(i uint64, s *statehash.State) {
		if i >= at {
			// COP0 12 is Status, which the toy program never writes, so the corruption sticks.
			// Two earlier versions of this line did not: an XOR applied every instruction
			// toggles the bit back and forth so half the later checkpoints match by accident,
			// and an OR into a general register the program rewrites every 32 instructions set
			// a bit that was already set. Both are corruptions that do not corrupt, and the
			// assertion below would have passed on luck in the first case and on nothing in
			// the second.
			s.COP0[12] = 0xDEADBEEF
		}
	})

	got, err := statehash.Compare(read(t, "go", good), read(t, "oracle", bad))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Kind != statehash.StateDiverged {
		t.Fatalf("a corrupted register gave %q, want a state divergence: %s", got.Kind, got)
	}
	if got.Lo != at || got.Hi != at+interval-1 {
		t.Fatalf("reported instructions %d..%d, want %d..%d", got.Lo, got.Hi, at, at+interval-1)
	}
	if got.Window != at/interval {
		t.Fatalf("reported window %d, want %d", got.Window, at/interval)
	}
	// And it must have actually walked the stream to get there, not stopped at the first line.
	// Windows 0..4,499 agree and 4,500..4,501 do not, so 4,500 of the 4,502 agree. Asserting
	// the exact number matters: a tool that reported the right window having examined almost
	// none of the run would satisfy anything looser.
	if want := uint64(n-at) / interval; got.Compared != int(at/interval) {
		t.Fatalf("it reports %d windows agreed; windows 0..%d agree and the last %d do not, so "+
			"it should be %d", got.Compared, at/interval-1, want, at/interval)
	}
	t.Logf("caught: %s", got)
}

func TestTwoRunsOfTheSameProgramAgree(t *testing.T) {
	t.Parallel()
	a := emit(t, 50_000, 1000, nil)
	b := emit(t, 50_000, 1000, nil)

	got, err := statehash.Compare(read(t, "go", a), read(t, "oracle", b))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !got.Agreed() {
		t.Fatalf("two runs of the same program disagree: %s", got)
	}
	if got.Compared != 50 {
		t.Fatalf("it compared %d windows of 50; an agreement over fewer is an agreement about "+
			"less than the run", got.Compared)
	}
}

// TC-1.6's other half. A truncated or empty stream must be a HARNESS FAILURE: "they agreed" over
// a run that stopped is the easiest vacuous pass there is.
func TestAnUnusableStreamIsAHarnessFailureAndNotAgreement(t *testing.T) {
	t.Parallel()
	full := emit(t, 10_000, 1000, nil)
	lines := strings.Split(strings.TrimRight(full, "\n"), "\n")

	cases := []struct {
		name   string
		text   string
		cause  error
		expect string
	}{
		{
			name:   "truncated: the run stopped and there is no END line",
			text:   strings.Join(lines[:len(lines)-3], "\n") + "\n",
			cause:  statehash.ErrTruncatedStream,
			expect: "truncated",
		},
		{
			name:   "empty: a header and nothing else",
			text:   lines[0] + "\n",
			cause:  statehash.ErrEmptyStream,
			expect: "no checkpoints",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bad := read(t, "oracle", tc.text)
			good := read(t, "go", full)

			for _, order := range []struct {
				label string
				a, b  *statehash.Stream
			}{{"as the second stream", good, bad}, {"as the first stream", bad, good}} {
				got, err := statehash.Compare(order.a, order.b)
				if err == nil {
					t.Fatalf("%s: comparing against an unusable stream returned %q instead of "+
						"refusing", order.label, got)
				}
				if !instrument.IsHarnessFailure(err) {
					t.Fatalf("%s: it must be a harness failure, so a gate cannot read it as a "+
						"finding of no divergence: %v", order.label, err)
				}
				if !errors.Is(err, tc.cause) {
					t.Fatalf("%s: want cause %v, got: %v", order.label, tc.cause, err)
				}
			}
			t.Logf("caught: %v", func() error { _, e := statehash.Compare(good, bad); return e }())
		})
	}
}

func TestStreamsSampledAtDifferentIntervalsAreRefused(t *testing.T) {
	t.Parallel()
	a := read(t, "go", emit(t, 10_000, 1000, nil))
	b := read(t, "oracle", emit(t, 10_000, 500, nil))

	_, err := statehash.Compare(a, b)
	if err == nil {
		t.Fatal("two streams sampled at different intervals must be refused: they agree on the " +
			"checkpoints they happen to share and say nothing about the rest")
	}
	if !instrument.IsHarnessFailure(err) || !errors.Is(err, statehash.ErrIntervalMismatch) {
		t.Fatalf("want a harness failure naming the interval mismatch, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

func TestStreamsThatOverlapNowhereAreRefused(t *testing.T) {
	t.Parallel()
	early := read(t, "go", "GRTV-CHECKPOINTS 1 interval=1000\n0 0x00000001\n1000 0x00000002\nEND 2 1000\n")
	late := read(t, "oracle", "GRTV-CHECKPOINTS 1 interval=1000\n900000 0x00000003\n901000 0x00000004\nEND 2 901000\n")

	_, err := statehash.Compare(early, late)
	if err == nil {
		t.Fatal("two streams covering no instruction in common must be refused, not called equal")
	}
	if !instrument.IsHarnessFailure(err) || !errors.Is(err, statehash.ErrNoCommonWindows) {
		t.Fatalf("want a harness failure naming the lack of overlap, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// The oracle's MIPS32 path retires a branch and its delay slot in one iteration, so its
// checkpoints land on instruction counts like 300001 - 477 of 4,629 on a real recorded boot. Two
// machines that agree perfectly will produce those; a port whose loop samples a moment earlier
// will not, and that is a difference between the two EMITTERS rather than between the machines.
//
// It is reported, because something is wrong. But it is reported as ITS OWN kind, because the two
// hashes describe the machine after different numbers of instructions: equal ones there would be
// coincidence and unequal ones would prove nothing, and calling either a state divergence would
// manufacture a false finding.
func TestADifferentInstructionCountInTheSameWindowIsItsOwnKindOfDivergence(t *testing.T) {
	t.Parallel()
	a := read(t, "go", "GRTV-CHECKPOINTS 1 interval=1000\n0 0x00000001\n1000 0x0000BEEF\nEND 2 1000\n")
	b := read(t, "oracle", "GRTV-CHECKPOINTS 1 interval=1000\n0 0x00000001\n1001 0x0000BEEF\nEND 2 1001\n")

	got, err := statehash.Compare(a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Kind != statehash.CadenceDiverged {
		t.Fatalf("got %q, want a cadence divergence: the two reached window 1 after a different "+
			"number of instructions", got.Kind)
	}
	if got.Window != 1 || got.Lo != 1000 || got.Hi != 1999 {
		t.Fatalf("reported window %d (%d..%d), want 1 (1000..1999)", got.Window, got.Lo, got.Hi)
	}
	if !strings.Contains(got.String(), "1001") {
		t.Fatalf("the report must give both instruction counts: %s", got)
	}
	// And it must say what it found about the STATES, or the headline is all the reader gets and
	// the headline is about sampling.
	if got.FirstState != nil {
		t.Fatalf("there is no state divergence here to find: %s", got)
	}
	if !strings.Contains(got.String(), "agree") {
		t.Fatalf("with no state divergence, the report must say the states agreed where they "+
			"could be compared: %s", got)
	}
	t.Logf("caught: %s", got)
}

// THE BUG THIS PACKAGE SHIPPED, pinned so it cannot come back.
//
// The first version stopped dead on a cadence difference and returned. Since the oracle steps over
// about one boundary in ten and the FIRST of them is at its fourth checkpoint, every live
// oracle-versus-port comparison would have ended there having compared no state at all - the
// instrument for the project's only independent correctness check, unable to reach a single
// comparison of the two machines. Found by a QA audit, not by any test here.
func TestAStraddledBoundaryDoesNotStopTheComparison(t *testing.T) {
	t.Parallel()
	// Window 1 straddles: different instruction counts, IDENTICAL hashes. Window 3 is a real
	// state divergence, reached only if the walk continues past window 1.
	a := read(t, "go", "GRTV-CHECKPOINTS 1 interval=1000\n"+
		"0 0x00000001\n1000 0x0000BEEF\n2000 0x00000003\n3000 0x0000AAAA\n4000 0x00000005\nEND 5 4000\n")
	b := read(t, "oracle", "GRTV-CHECKPOINTS 1 interval=1000\n"+
		"0 0x00000001\n1001 0x0000BEEF\n2000 0x00000003\n3000 0x0000BBBB\n4000 0x00000005\nEND 5 4000\n")

	got, err := statehash.Compare(a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.FirstState == nil {
		t.Fatal("the comparison stopped at the straddled boundary and never reached the state " +
			"divergence at window 3 - which is every live oracle-versus-port run ending at its " +
			"fourth checkpoint having compared nothing")
	}
	if got.FirstState.Window != 3 || got.FirstState.Lo != 3000 || got.FirstState.Hi != 3999 {
		t.Fatalf("the state divergence is reported at window %d (%d..%d), want 3 (3000..3999)",
			got.FirstState.Window, got.FirstState.Lo, got.FirstState.Hi)
	}
	if got.FirstState.AtA.Hash != 0xAAAA || got.FirstState.AtB.Hash != 0xBBBB {
		t.Fatalf("the state divergence carries the wrong checkpoints: %+v", got.FirstState)
	}
	// The headline is still the first disagreement in order, which is the straddle.
	if got.Kind != statehash.CadenceDiverged || got.Window != 1 {
		t.Fatalf("the headline should still be the first disagreement, the straddle at window 1; "+
			"got %q at %d", got.Kind, got.Window)
	}
	if got.Cadence != 1 {
		t.Fatalf("it counted %d windows it could not compare, want 1", got.Cadence)
	}
	// Windows 0, 2 and 4 agreed; window 1 was not comparable and window 3 differed.
	if got.Compared != 3 {
		t.Fatalf("%d windows agreed, want 3", got.Compared)
	}
	if !strings.Contains(got.String(), "3000..3999") {
		t.Fatalf("the report must name the window worth re-running under tier 2: %s", got)
	}
	t.Logf("caught: %s", got)
}

// A run where the only difference is sampling: the machines agree everywhere they can be
// compared. The report must say so rather than leaving a bare "cadence diverged".
func TestAPureSamplingDifferenceSaysTheMachinesAgree(t *testing.T) {
	t.Parallel()
	a := read(t, "go", "GRTV-CHECKPOINTS 1 interval=1000\n"+
		"0 0x1\n1000 0x2\n2000 0x3\n3000 0x4\nEND 4 3000\n")
	b := read(t, "oracle", "GRTV-CHECKPOINTS 1 interval=1000\n"+
		"0 0x1\n1001 0x2\n2000 0x3\n3001 0x4\nEND 4 3001\n")

	got, err := statehash.Compare(a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.FirstState != nil {
		t.Fatalf("there is no state divergence here: %s", got)
	}
	if got.Cadence != 2 {
		t.Fatalf("it counted %d incomparable windows, want 2", got.Cadence)
	}
	if got.Compared != 2 {
		t.Fatalf("%d windows agreed, want 2", got.Compared)
	}
	t.Logf("caught: %s", got)
}

// A window skipped in the middle of a range both streams cover is a disagreement, not an end.
func TestAWindowMissingFromTheMiddleIsReported(t *testing.T) {
	t.Parallel()
	a := read(t, "go", "GRTV-CHECKPOINTS 1 interval=1000\n0 0x1\n1000 0x2\n2000 0x3\nEND 3 2000\n")
	b := read(t, "oracle", "GRTV-CHECKPOINTS 1 interval=1000\n0 0x1\n2000 0x3\nEND 2 2000\n")

	got, err := statehash.Compare(a, b)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Kind != statehash.WindowMissing || got.Window != 1 {
		t.Fatalf("got %q at window %d, want a missing window at 1: %s", got.Kind, got.Window, got)
	}
	if !strings.Contains(got.String(), "oracle") {
		t.Fatalf("the report must name the stream the window is missing from: %s", got)
	}
	t.Logf("caught: %s", got)
}

// One stream running further than the other is not a disagreement over the part they share - but
// an "agree" must say how far it actually got, or it is an agreement about an unstated range.
func TestAgreementSaysHowFarItCompared(t *testing.T) {
	t.Parallel()
	long := read(t, "go", emit(t, 50_000, 1000, nil))
	short := read(t, "oracle", emit(t, 10_000, 1000, nil))

	got, err := statehash.Compare(long, short)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !got.Agreed() {
		t.Fatalf("the common prefix should agree: %s", got)
	}
	if got.Compared != 10 {
		t.Fatalf("compared %d windows, want the 10 they share", got.Compared)
	}
	if !strings.Contains(got.String(), "9999") && !strings.Contains(got.String(), "10 checkpoints") {
		t.Fatalf("an agreement must state its range: %s", got)
	}
	t.Logf("%s", got)
}

func TestMalformedStreamsAreRefused(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"no header":                    "1000 0x00000001\nEND 1 1000\n",
		"a header from another format": "CHECKPOINTS 1 interval=1000\n1000 0x1\nEND 1 1000\n",
		"a future format version":      "GRTV-CHECKPOINTS 99 interval=1000\n1000 0x1\nEND 1 1000\n",
		"an interval of zero":          "GRTV-CHECKPOINTS 1 interval=0\n1000 0x1\nEND 1 1000\n",
		"a hash that is not a number":  "GRTV-CHECKPOINTS 1 interval=1000\n1000 wat\nEND 1 1000\n",
		"checkpoints out of order":     "GRTV-CHECKPOINTS 1 interval=1000\n2000 0x1\n1000 0x2\nEND 2 1000\n",
		"a repeated instruction count": "GRTV-CHECKPOINTS 1 interval=1000\n1000 0x1\n1000 0x2\nEND 2 1000\n",
		"a trailer that miscounts":     "GRTV-CHECKPOINTS 1 interval=1000\n1000 0x1\nEND 7 1000\n",
		"a trailer naming a wrong end": "GRTV-CHECKPOINTS 1 interval=1000\n1000 0x1\nEND 1 9999\n",
		"content after the END line":   "GRTV-CHECKPOINTS 1 interval=1000\n1000 0x1\nEND 1 1000\n2000 0x2\n",
		"an empty file":                "",
	}
	for name, text := range cases {
		name, text := name, text
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if s, err := statehash.ReadStream("stream", strings.NewReader(text)); err == nil {
				t.Fatalf("a stream with %s must be refused; it parsed to %d checkpoints",
					name, len(s.Checkpoints))
			} else {
				t.Logf("caught: %v", err)
			}
		})
	}
}

// The stream a real emitter writes must be readable by the reader, or the two halves of this
// format have drifted apart inside one package.
func TestTheEmitterAndTheReaderAgreeOnTheFormat(t *testing.T) {
	t.Parallel()
	text := emit(t, 5000, 1000, nil)
	s := read(t, "go", text)

	if !s.Complete {
		t.Fatal("a stream the emitter closed must read as complete")
	}
	if s.Interval != 1000 {
		t.Fatalf("interval read back as %d", s.Interval)
	}
	if len(s.Checkpoints) != 5 {
		t.Fatalf("read %d checkpoints, want 5", len(s.Checkpoints))
	}
	if err := s.Usable(); err != nil {
		t.Fatalf("a complete, non-empty stream must be usable: %v", err)
	}
	if got := fmt.Sprint(s.Checkpoints[0].ICount); got != "0" {
		t.Fatalf("the first checkpoint is at %s, want 0", got)
	}
}
