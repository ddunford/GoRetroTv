package statehash

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// Why a stream can refuse to be compared at all.
//
// These are separated from ordinary divergences on purpose. "The two machines disagreed" and "I
// could not tell whether they disagreed" are different answers, and a gate that treats the second
// as the first reports agreement over a run that never happened - which is the easiest vacuous
// pass in this project and the reason the emitter writes an END line at all.
var (
	// ErrTruncatedStream means the stream has no END line, so it stopped rather than finished.
	ErrTruncatedStream = errors.New("checkpoint stream is truncated")

	// ErrEmptyStream means the stream carries no checkpoints. Every claim about it is vacuously
	// true, including "they agreed".
	ErrEmptyStream = errors.New("checkpoint stream carries no checkpoints")

	// ErrIntervalMismatch means the two streams were sampled at different intervals. They would
	// agree on the checkpoints they happen to share and say nothing about the rest.
	ErrIntervalMismatch = errors.New("checkpoint streams were taken at different intervals")

	// ErrNoCommonWindows means the two streams cover no instruction in common.
	ErrNoCommonWindows = errors.New("checkpoint streams share no window")
)

// Checkpoint is one sample: the machine's state hash after ICount instructions.
type Checkpoint struct {
	ICount uint64
	Hash   uint32
}

// Window is which interval-sized window a checkpoint falls in.
func (c Checkpoint) Window(interval uint64) uint64 { return c.ICount / interval }

// Stream is a parsed tier-1 checkpoint stream.
type Stream struct {
	// Name is where it came from, for error messages.
	Name string

	Interval    uint64
	Checkpoints []Checkpoint

	// Complete is whether a well-formed END line closed the stream. A stream that is not
	// complete stopped; it did not finish, and nothing may be concluded from the part that
	// arrived.
	Complete bool

	// DeclaredCount and DeclaredLast are what the END line claimed, when there was one.
	DeclaredCount uint64
	DeclaredLast  uint64
}

// ReadStream parses a checkpoint stream.
//
// A malformed stream is an error. A stream that is merely INCOMPLETE is not - it parses, and
// Complete reports what it is, because a caller comparing two streams needs to say which of them
// was cut short rather than be handed a parse failure that names neither.
func ReadStream(name string, r io.Reader) (*Stream, error) {
	s := &Stream{Name: name}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	line := 0
	ended := false
	for sc.Scan() {
		line++
		text := strings.TrimRight(sc.Text(), "\r")
		if text == "" {
			continue
		}
		if ended {
			return nil, fmt.Errorf("%s line %d: %q follows the %s line, and a stream has one end",
				name, line, text, StreamEnd)
		}

		switch {
		case line == 1:
			if err := s.parseHeader(name, text); err != nil {
				return nil, err
			}
		case strings.HasPrefix(text, StreamEnd+" "):
			if err := s.parseEnd(name, line, text); err != nil {
				return nil, err
			}
			ended = true
		default:
			cp, err := parseCheckpoint(name, line, text)
			if err != nil {
				return nil, err
			}
			if n := len(s.Checkpoints); n > 0 && cp.ICount <= s.Checkpoints[n-1].ICount {
				return nil, fmt.Errorf("%s line %d: instruction count %d does not advance on the "+
					"previous checkpoint's %d, so the stream is not in order",
					name, line, cp.ICount, s.Checkpoints[n-1].ICount)
			}
			s.Checkpoints = append(s.Checkpoints, cp)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if line == 0 {
		return nil, fmt.Errorf("%s: the file is empty, so it is not a checkpoint stream at all",
			name)
	}
	if s.Interval == 0 {
		return nil, fmt.Errorf("%s: no %s header", name, StreamMagic)
	}

	// A trailer that disagrees with the body is worse than no trailer: it asserts completeness
	// that is not there, which is the one thing the trailer exists to establish.
	if ended {
		if got := uint64(len(s.Checkpoints)); got != s.DeclaredCount {
			return nil, fmt.Errorf("%s: the %s line claims %d checkpoints and the stream carries "+
				"%d", name, StreamEnd, s.DeclaredCount, got)
		}
		if n := len(s.Checkpoints); n > 0 && s.Checkpoints[n-1].ICount != s.DeclaredLast {
			return nil, fmt.Errorf("%s: the %s line claims it ended at instruction %d and the "+
				"last checkpoint is at %d", name, StreamEnd, s.DeclaredLast,
				s.Checkpoints[n-1].ICount)
		}
		s.Complete = true
	}
	return s, nil
}

func (s *Stream) parseHeader(name, text string) error {
	fields := strings.Fields(text)
	if len(fields) != 3 || fields[0] != StreamMagic {
		return fmt.Errorf("%s: the first line is %q; a checkpoint stream begins %q",
			name, text, fmt.Sprintf("%s %d interval=N", StreamMagic, StreamVersion))
	}
	version, err := strconv.ParseUint(fields[1], 10, 16)
	if err != nil {
		return fmt.Errorf("%s: header version %q is not a number: %w", name, fields[1], err)
	}
	if version != StreamVersion {
		return fmt.Errorf("%s: stream format version %d, and this build reads version %d",
			name, version, StreamVersion)
	}
	interval, ok := strings.CutPrefix(fields[2], "interval=")
	if !ok {
		return fmt.Errorf("%s: header field %q is not interval=N", name, fields[2])
	}
	n, err := strconv.ParseUint(interval, 10, 64)
	if err != nil || n == 0 {
		return fmt.Errorf("%s: header interval %q is not a positive number", name, interval)
	}
	s.Interval = n
	return nil
}

func (s *Stream) parseEnd(name string, line int, text string) error {
	fields := strings.Fields(text)
	if len(fields) != 3 {
		return fmt.Errorf("%s line %d: %q is not %s <count> <last instruction>",
			name, line, text, StreamEnd)
	}
	count, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return fmt.Errorf("%s line %d: %s count %q is not a number: %w",
			name, line, StreamEnd, fields[1], err)
	}
	last, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return fmt.Errorf("%s line %d: %s instruction count %q is not a number: %w",
			name, line, StreamEnd, fields[2], err)
	}
	s.DeclaredCount, s.DeclaredLast = count, last
	return nil
}

func parseCheckpoint(name string, line int, text string) (Checkpoint, error) {
	fields := strings.Fields(text)
	if len(fields) != 2 {
		return Checkpoint{}, fmt.Errorf("%s line %d: %q is not <instruction count> <hash>",
			name, line, text)
	}
	icount, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("%s line %d: instruction count %q is not a number: %w",
			name, line, fields[0], err)
	}
	// Through hexfmt, so a hash written in the other casing is read rather than rejected - and
	// so there is one place that knows what this project's hex looks like.
	hash, err := hexfmt.ParseAddr(fields[1])
	if err != nil {
		return Checkpoint{}, fmt.Errorf("%s line %d: hash %q: %w", name, line, fields[1], err)
	}
	return Checkpoint{ICount: icount, Hash: hash}, nil
}

// Usable reports a harness failure if nothing may be concluded from this stream.
//
// It is separate from parsing because both conditions are about what a COMPARISON may say, not
// about whether the bytes were well formed: an empty stream and a truncated one both parse
// perfectly and both make "they agreed" vacuously true.
func (s *Stream) Usable() error {
	if len(s.Checkpoints) == 0 {
		return &instrument.HarnessError{
			Instrument: "checkpoint comparison",
			Subject:    s.Name,
			Detail: "it carries no checkpoints, so every claim about it is vacuously true - " +
				"including that it agreed with the other stream",
			Err: ErrEmptyStream,
		}
	}
	if !s.Complete {
		return &instrument.HarnessError{
			Instrument: "checkpoint comparison",
			Subject:    s.Name,
			Detail: fmt.Sprintf("it has no %s line, so it stopped rather than finished; it "+
				"reaches instruction %d and there is no way to tell that from a run that was "+
				"meant to", StreamEnd, s.Checkpoints[len(s.Checkpoints)-1].ICount),
			Err: ErrTruncatedStream,
		}
	}
	return nil
}
