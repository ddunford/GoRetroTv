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

// Comparison is what came of comparing two streams.
type Comparison struct {
	Kind Kind

	// A and B name the two streams.
	A, B string

	// Interval is the shared sampling interval.
	Interval uint64

	// Compared is how many windows AGREED. On a divergence it is the count before the one that
	// disagreed, not including it, because that is what the report says out loud: "agreed over
	// the preceding N checkpoints". Counting the failing window too made that sentence wrong by
	// one, which was caught by comparing against a real recorded oracle boot rather than
	// against a fixture written alongside the code.
	Compared int

	// Window is where the disagreement is, when there is one; Lo and Hi are the instructions it
	// covers, which is the range tier 2 re-runs with per-instruction tracing.
	Window uint64
	Lo, Hi uint64

	// AtA and AtB are the checkpoints that disagreed. For WindowMissing only one is present.
	AtA, AtB Checkpoint
	HasA     bool
	HasB     bool

	// ReachedA and ReachedB are how far each stream ran, so an "agree" can say over what.
	ReachedA, ReachedB uint64
}

// Agreed reports whether the two matched over everything they share.
func (c Comparison) Agreed() bool { return c.Kind == Agree }

// String renders the comparison the way the command prints it.
func (c Comparison) String() string {
	switch c.Kind {
	case Agree:
		return fmt.Sprintf("agree over %d checkpoints, to instruction %d",
			c.Compared, min64(c.ReachedA, c.ReachedB))
	case StateDiverged:
		return fmt.Sprintf("%s at window %d (instructions %d..%d): %s %s, %s %s",
			c.Kind, c.Window, c.Lo, c.Hi, c.A, hexfmt.Word(c.AtA.Hash), c.B, hexfmt.Word(c.AtB.Hash))
	case CadenceDiverged:
		return fmt.Sprintf("%s at window %d (instructions %d..%d): %s reached instruction %d, "+
			"%s reached %d", c.Kind, c.Window, c.Lo, c.Hi, c.A, c.AtA.ICount, c.B, c.AtB.ICount)
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
// Windows, not instruction counts, are what get paired. The two implementations retire
// instructions at different granularities - the oracle's MIPS32 path runs a branch and its delay
// slot in one iteration, so its checkpoints land on counts like 300001 - and demanding equal
// instruction counts would report a divergence at the first straddling branch between two machines
// that agree perfectly. A window whose counts DO differ is reported, but as a cadence divergence,
// because their hashes describe the machine after different numbers of instructions and comparing
// them would be meaningless.
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

	for w := lo; w <= hi; w++ {
		ca, okA := wa[w]
		cb, okB := wb[w]
		if !okA || !okB {
			// A window either side skipped inside the shared range. The machine did not stop,
			// so this is a real disagreement about what happened, not an end of stream.
			if !okA && !okB {
				continue // neither reported it; nothing to say
			}
			out.Kind = WindowMissing
			out.Window, out.Lo, out.Hi = w, w*interval, (w+1)*interval-1
			out.AtA, out.HasA = ca, okA
			out.AtB, out.HasB = cb, okB
			return out, nil
		}
		out.HasA, out.HasB = true, true
		if ca.ICount != cb.ICount {
			out.Kind = CadenceDiverged
			out.Window, out.Lo, out.Hi = w, w*interval, (w+1)*interval-1
			out.AtA, out.AtB = ca, cb
			return out, nil
		}
		if ca.Hash != cb.Hash {
			out.Kind = StateDiverged
			out.Window, out.Lo, out.Hi = w, w*interval, (w+1)*interval-1
			out.AtA, out.AtB = ca, cb
			return out, nil
		}
		out.Compared++
	}

	if out.Compared == 0 {
		return Comparison{}, &instrument.HarnessError{
			Instrument: "checkpoint comparison",
			Subject:    a.Name + " against " + b.Name,
			Detail: "the two streams overlap but no window is present in both, so nothing was " +
				"actually compared",
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
