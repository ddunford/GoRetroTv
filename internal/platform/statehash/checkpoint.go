package statehash

import (
	"bufio"
	"fmt"
	"io"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// The tier-1 checkpoint stream format.
//
// Spike 002 measured the naive per-instruction alternative at 1.8-59 GB per run. A 32-bit hash
// every thousand instructions is 447,000 checkpoints and 3.6 MB for a full cold boot, which is
// cheap enough for CI; on the first differing checkpoint, tier 2 re-runs that one window with full
// per-instruction tracing. So this stream is the thing that has to be small, and the window it
// localises to is the thing that has to be exact.
//
// The format is line-oriented text because the other implementation is a browser page and has to
// produce it byte-identically:
//
//	GRTV-CHECKPOINTS 1 interval=1000
//	0 0x1A2B3C4D
//	1000 0x5E6F7081
//	...
//	END 447001 447000000
//
// The header carries the interval so that two streams taken at different intervals are refused
// rather than compared - they would agree on the checkpoints they happen to share and say nothing
// about the rest. The END line carries the checkpoint count and the final instruction count, which
// is what makes a TRUNCATED stream detectable: without it, a run that died half way through is
// indistinguishable from a short one that finished, and the comparison would report agreement over
// the part that exists. That is the easiest vacuous pass in this project.
const (
	// StreamMagic heads every checkpoint stream.
	StreamMagic = "GRTV-CHECKPOINTS"

	// StreamVersion is the format version in the header.
	StreamVersion = 1

	// StreamEnd heads the trailer line.
	StreamEnd = "END"

	// DefaultInterval is the interval spike 002 sized the tier-1 stream around. It is a DIAL,
	// not a constant: 1,000 costs 3.6 MB for a cold boot and 10,000 costs 0.4 MB.
	DefaultInterval = 1000
)

// Emitter writes a tier-1 checkpoint stream.
type Emitter struct {
	w        *bufio.Writer
	hasher   *Hasher
	interval uint64

	next   uint64
	count  uint64
	last   uint64
	closed bool
	err    error
}

// NewEmitter starts a checkpoint stream on w, writing its header at once.
//
// interval is how many instructions pass between checkpoints. Nothing here treats DefaultInterval
// as special: a phase-7 investigation may want 100 and CI may want 10,000, and the header records
// which was used so a comparison cannot mix them.
func NewEmitter(w io.Writer, interval uint64, h *Hasher) (*Emitter, error) {
	if w == nil {
		return nil, fmt.Errorf("statehash: NewEmitter: writer is nil")
	}
	if h == nil {
		return nil, fmt.Errorf("statehash: NewEmitter: hasher is nil")
	}
	if interval == 0 {
		return nil, fmt.Errorf("statehash: NewEmitter: interval is zero, which would emit a " +
			"checkpoint for every instruction - the 1.8-59 GB per run spike 002 measured and " +
			"rejected")
	}

	e := &Emitter{w: bufio.NewWriter(w), hasher: h, interval: interval}
	if _, err := fmt.Fprintf(e.w, "%s %d interval=%d\n", StreamMagic, StreamVersion, interval); err != nil {
		return nil, fmt.Errorf("statehash: writing the checkpoint header: %w", err)
	}
	return e, nil
}

// Observe writes a checkpoint if icount has reached the next interval boundary.
//
// The instruction loop calls this every instruction, so it is a comparison rather than a modulo:
// a division per instruction is real money against the 15M instructions/s floor.
//
// The icount written is the one observed, not the boundary it crossed. A caller that skips
// instructions therefore produces a stream whose instants are visible rather than one that silently
// claims to have sampled the boundary.
func (e *Emitter) Observe(icount uint64, s State) error {
	// The sticky error comes first, before the not-due shortcut. A broken stream must report
	// itself on EVERY call, not only on the ones where a checkpoint happened to be due: the
	// instruction loop drives this per instruction, and a caller told "fine" for the nine
	// hundred instructions after a failed write will carry on to the end and produce a stream
	// with a hole in it. A hole compares as a divergence at an instruction where nothing
	// happened.
	if e.err != nil {
		return e.err
	}
	if e.closed {
		e.err = fmt.Errorf("statehash: checkpoint at %d written after the stream was closed", icount)
		return e.err
	}
	if icount < e.next {
		return nil
	}

	hash := e.hasher.Hash(s)
	if err := e.hasher.Err(); err != nil {
		e.err = err
		return e.err
	}
	if _, err := fmt.Fprintf(e.w, "%d %s\n", icount, hexfmt.Word(hash)); err != nil {
		e.err = fmt.Errorf("statehash: writing the checkpoint at %d: %w", icount, err)
		return e.err
	}

	e.count++
	e.last = icount
	// From the boundary that was crossed, not from the icount observed, so a caller that skips
	// does not drift the cadence away from the other implementation's.
	e.next = (icount/e.interval + 1) * e.interval
	return nil
}

// Close writes the trailer and flushes. A stream without it is a truncated stream, and the
// comparison is required to say so rather than report agreement over the part that arrived.
func (e *Emitter) Close() error {
	if e.err != nil {
		return e.err
	}
	if e.closed {
		return fmt.Errorf("statehash: the checkpoint stream is already closed")
	}
	e.closed = true

	if _, err := fmt.Fprintf(e.w, "%s %d %d\n", StreamEnd, e.count, e.last); err != nil {
		e.err = fmt.Errorf("statehash: writing the checkpoint trailer: %w", err)
		return e.err
	}
	if err := e.w.Flush(); err != nil {
		e.err = fmt.Errorf("statehash: flushing the checkpoint stream: %w", err)
		return e.err
	}
	return nil
}

// Count is how many checkpoints have been written.
func (e *Emitter) Count() uint64 { return e.count }

// Interval is how many instructions pass between checkpoints.
func (e *Emitter) Interval() uint64 { return e.interval }
