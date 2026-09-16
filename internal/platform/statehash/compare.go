package statehash

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// Kind says what sort of disagreement was found.
type Kind uint8

const (
	// Agree means the two streams matched over every window they share.
	Agree Kind = iota

	// StateDiverged means both machines reached the same instruction count and their states
	// differed. This is the ordinary result: something in the port does not match the oracle.
	StateDiverged

	// CadenceDiverged means the two reached the same window at DIFFERENT instruction counts.
	// Their hashes cannot be compared - they describe the machine after different numbers of
	// instructions - and the disagreement is about how many instructions each retired, not
	// about what it computed. Reporting it as a state divergence would send the reader looking
	// at the wrong thing entirely.
	CadenceDiverged

	// WindowMissing means one stream has a window the other never reported, in a range they
	// otherwise both cover.
	WindowMissing
)

// String names the kind.
func (k Kind) String() string {
	switch k {
	case Agree:
		return "agree"
	case StateDiverged:
		return "state diverged"
	case CadenceDiverged:
		return "cadence diverged"
	case WindowMissing:
		return "window missing"
	default:
		return "unknown"
	}
}

// Finding is one disagreement, at the window it was found in.
type Finding struct {
	Kind   Kind
	Window uint64

	// Lo and Hi are the instructions the window covers - the range tier 2 re-runs with
	// per-instruction tracing.
	Lo, Hi uint64

	// AtA and AtB are the checkpoints involved. For WindowMissing only one is present.
	AtA, AtB   Checkpoint
	HasA, HasB bool
}

// Comparison is what came of comparing two streams.
type Comparison struct {
	Kind Kind

	// A and B name the two streams.
	A, B string

	// Interval is the shared sampling interval.
	Interval uint64

	// Compared is how many windows AGREED outright: both present, both at the same instruction
	// count, both with the same hash. It is what the report says out loud, so it must not
	// include a window that disagreed - counting the failing one made "agreed over the
	// preceding N checkpoints" wrong by one, caught by comparing against a real recorded oracle
	// boot rather than against a fixture written alongside the code.
	//
	// It counts the WHOLE shared range, not the part before the first disagreement. The walk
	// does not stop early: an earlier version did, and then this number meant "agreed before I
	// stopped looking", which is a different and much less useful claim to print in a report.
	Compared int

	// Cadence is how many windows the two reached at different instruction counts. Their hashes
	// could not be compared there, so those windows are neither agreement nor disagreement
	// about the machine - they are windows where nothing could be asked.
	Cadence int

	// The headline finding: the FIRST disagreement in window order. Zero when they agree.
	Window   uint64
	Lo, Hi   uint64
	AtA, AtB Checkpoint
	HasA     bool
	HasB     bool

	// FirstState is the first window where both sides reached the SAME instruction count and
	// their states differed - the question this comparison exists to answer.
	//
	// It is reported even when the headline is a cadence divergence, and that is the whole point
	// of it. The oracle steps over about one boundary in ten (a MIPS32 branch and its delay slot
	// retire together), so a Go loop that samples at a slightly different moment produces a
	// cadence difference within the first handful of checkpoints. Stopping there would mean
	// every live oracle-versus-port run ended at checkpoint four having compared no state at
	// all, and phase 2 would have no instrument to drive the first real divergence to zero.
	FirstState *Finding

	// ReachedA and ReachedB are how far each stream ran, so an "agree" can say over what.
	ReachedA, ReachedB uint64
}

// Agreed reports whether the two matched over everything they share.
func (c Comparison) Agreed() bool { return c.Kind == Agree }

// String renders the comparison the way the command prints it.
func (c Comparison) String() string {
	switch c.Kind {
	case Agree:
		if c.Cadence > 0 {
			return fmt.Sprintf("agree over %d checkpoints, to instruction %d (%d further windows "+
				"could not be compared: the two sampled them at different instruction counts)",
				c.Compared, min64(c.ReachedA, c.ReachedB), c.Cadence)
		}
		return fmt.Sprintf("agree over %d checkpoints, to instruction %d",
			c.Compared, min64(c.ReachedA, c.ReachedB))
	case StateDiverged:
		return fmt.Sprintf("%s at window %d (instructions %d..%d): %s %s, %s %s",
			c.Kind, c.Window, c.Lo, c.Hi, c.A, hexfmt.Word(c.AtA.Hash), c.B, hexfmt.Word(c.AtB.Hash))
	case CadenceDiverged:
		head := fmt.Sprintf("%s at window %d (instructions %d..%d): %s reached instruction %d, "+
			"%s reached %d", c.Kind, c.Window, c.Lo, c.Hi, c.A, c.AtA.ICount, c.B, c.AtB.ICount)
		// The headline alone is misleading here: a cadence difference is usually about the two
		// EMITTERS and says nothing about the machines, so the state answer goes with it.
		if c.FirstState != nil {
			return head + fmt.Sprintf("; the states first differ at window %d (instructions "+
				"%d..%d): %s %s, %s %s", c.FirstState.Window, c.FirstState.Lo, c.FirstState.Hi,
				c.A, hexfmt.Word(c.FirstState.AtA.Hash), c.B, hexfmt.Word(c.FirstState.AtB.Hash))
		}
		return head + fmt.Sprintf("; the states agree in all %d windows both sampled alike",
			c.Compared)
	case WindowMissing:
		missing, present := c.B, c.A
		if !c.HasA {
			missing, present = c.A, c.B
		}
		return fmt.Sprintf("%s: window %d (instructions %d..%d) is in %s and not in %s",
			c.Kind, c.Window, c.Lo, c.Hi, present, missing)
	default:
		return c.Kind.String()
	}
}

// Compare finds the first window in which two checkpoint streams disagree.
//
// It refuses, with a harness failure, anything that would make a green result meaningless: a
// stream with no checkpoints, a stream that was cut short, two streams sampled at different
// intervals, or two streams that share no window at all. Those are not findings of agreement.
// They are the instrument saying it could not look, and a gate that cannot tell the two apart is
// the gate this project has shipped before.
//
// Windows, not instruction counts, are what get paired. The oracle's MIPS32 path retires a branch
// and its delay slot in one iteration and steps over the boundary, so about one checkpoint in ten
// lands on a count like 300001 rather than 300000 - measured, on a real boot, at 477 of 4,629.
//
// Where the two counts differ the hashes are NOT compared, and that is not fastidiousness: they
// describe the machine after different numbers of instructions, so equal hashes there would be a
// coincidence and unequal ones would prove nothing. Reporting either as a state divergence would
// be a false finding, which is the one thing this project must not manufacture.
//
// But such a window does not stop the comparison either. It is counted, noted, and the walk
// continues, because a cadence difference is usually a difference between the two EMITTERS rather
// than between the two machines - and an instrument that halts on it answers a question nobody
// asked while leaving the one they did ask unasked. Both are reported: the first disagreement of
// any kind, and the first STATE divergence among the windows where a state comparison was
// actually possible.
func Compare(a, b *Stream) (Comparison, error) {
	if a == nil || b == nil {
		return Comparison{}, fmt.Errorf("statehash: Compare needs two streams")
	}
	if err := a.Usable(); err != nil {
		return Comparison{}, err
	}
	if err := b.Usable(); err != nil {
		return Comparison{}, err
	}
	if a.Interval != b.Interval {
		return Comparison{}, &instrument.HarnessError{
			Instrument: "checkpoint comparison",
			Subject:    a.Name + " against " + b.Name,
			Detail: fmt.Sprintf("%s was sampled every %d instructions and %s every %d; they "+
				"would agree on the checkpoints they happen to share and say nothing at all "+
				"about the rest", a.Name, a.Interval, b.Name, b.Interval),
			Err: ErrIntervalMismatch,
		}
	}

	interval := a.Interval
	byWindow := func(s *Stream) map[uint64]Checkpoint {
		m := make(map[uint64]Checkpoint, len(s.Checkpoints))
		for _, cp := range s.Checkpoints {
			m[cp.Window(interval)] = cp
		}
		return m
	}
	wa, wb := byWindow(a), byWindow(b)

	out := Comparison{
		A: a.Name, B: b.Name, Interval: interval,
		ReachedA: a.Checkpoints[len(a.Checkpoints)-1].ICount,
		ReachedB: b.Checkpoints[len(b.Checkpoints)-1].ICount,
	}

	// Only the windows both streams cover can say anything. Beyond the shorter one's end there
	// is nothing to compare, and silence there is not disagreement.
	lastA := a.Checkpoints[len(a.Checkpoints)-1].Window(interval)
	lastB := b.Checkpoints[len(b.Checkpoints)-1].Window(interval)
	firstA := a.Checkpoints[0].Window(interval)
	firstB := b.Checkpoints[0].Window(interval)
	lo, hi := max64(firstA, firstB), min64(lastA, lastB)
	if firstA > lastB || firstB > lastA {
		return Comparison{}, &instrument.HarnessError{
			Instrument: "checkpoint comparison",
			Subject:    a.Name + " against " + b.Name,
			Detail: fmt.Sprintf("%s covers windows %d..%d and %s covers %d..%d; they overlap "+
				"nowhere, so there is nothing to compare",
				a.Name, firstA, lastA, b.Name, firstB, lastB),
			Err: ErrNoCommonWindows,
		}
	}

	var first, firstState *Finding
	note := func(f Finding) {
		if first == nil {
			c := f
			first = &c
		}
		if f.Kind == StateDiverged && firstState == nil {
			c := f
			firstState = &c
		}
	}

	for w := lo; w <= hi; w++ {
		ca, okA := wa[w]
		cb, okB := wb[w]
		at := Finding{Window: w, Lo: w * interval, Hi: (w+1)*interval - 1}

		switch {
		case !okA && !okB:
			// Neither reported it. Both emitters stepped over this boundary, which is the
			// ordinary consequence of a branch pair straddling it - nothing to say.
			continue

		case !okA || !okB:
			// One side has a window the other never reported, inside a range they both cover.
			// The machine did not stop, so this is a disagreement about what happened.
			at.Kind = WindowMissing
			at.AtA, at.HasA = ca, okA
			at.AtB, at.HasB = cb, okB
			note(at)

		case ca.ICount != cb.ICount:
			at.Kind = CadenceDiverged
			at.AtA, at.AtB, at.HasA, at.HasB = ca, cb, true, true
			out.Cadence++
			note(at)

		case ca.Hash != cb.Hash:
			at.Kind = StateDiverged
			at.AtA, at.AtB, at.HasA, at.HasB = ca, cb, true, true
			note(at)

		default:
			out.Compared++
		}
	}

	if first != nil {
		out.Kind = first.Kind
		out.Window, out.Lo, out.Hi = first.Window, first.Lo, first.Hi
		out.AtA, out.AtB, out.HasA, out.HasB = first.AtA, first.AtB, first.HasA, first.HasB
		out.FirstState = firstState
		return out, nil
	}

	if out.Compared == 0 {
		return Comparison{}, &instrument.HarnessError{
			Instrument: "checkpoint comparison",
			Subject:    a.Name + " against " + b.Name,
			Detail: "the two streams overlap but no window had a state that could be compared in " +
				"both, so nothing was actually compared",
			Err: ErrNoCommonWindows,
		}
	}
	out.Kind = Agree
	return out, nil
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
