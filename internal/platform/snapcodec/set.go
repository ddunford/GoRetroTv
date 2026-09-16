package snapcodec

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// A set container holds one opaque blob per named member: the shape an aggregate snapshot needs,
// where a bus frames its devices or a machine frames its bus, CPU and clock.
//
// What is inside a member's blob is that member's business. This layer's only job is to make the
// set of blobs match the set of members EXACTLY, because that is the failure spike 003 exists to
// name: a snapshot that restores most of a machine does not fail. It produces a plausible machine
// whose faults read as firmware bugs, and by the time anyone is reading them the snapshot is long
// out of sight. The comparison can only be made here, while both sets are still in hand.
//
// It lives in snapcodec rather than in the package that happens to need it first so that there is
// one aggregate format for ARCH-SNAP-1 to inspect. What does NOT live here is any opinion about
// which members ought to be present: the caller passes its expected set to Require, because "the
// devices attached to this bus" is the bus's knowledge and nothing here should pretend otherwise.
//
// The payload layout is a uint32 count followed by that many (String name, Bytes blob) pairs,
// written in name order so two sets with the same contents encode to the same bytes. Snapshots are
// compared as well as restored, and a container whose byte image depended on insertion order could
// not be compared at all.

// minMemberBytes is the least payload a single member can occupy: a four-byte length prefix for
// its name and another for its blob, both of which may then be empty. It is the multiplier that
// turns a claimed member count into a smallest-possible byte count, which is what makes a forged
// count checkable before anything is allocated on the strength of it.
const minMemberBytes = 8

// Sentinel causes for a set that does not describe what the caller expected.
var (
	// ErrMemberMissing means the set says nothing about a member the caller expected.
	ErrMemberMissing = errors.New("snapshot is missing a member")
	// ErrMemberUnexpected means the set names a member the caller does not have.
	ErrMemberUnexpected = errors.New("snapshot names a member that is not present")
	// ErrDuplicateMember means one name appears twice.
	ErrDuplicateMember = errors.New("snapshot describes a member twice")
)

// SetWriter builds an aggregate container.
type SetWriter struct {
	name    string
	version uint16
	members map[string][]byte
	order   []string
	err     error
}

// NewSetWriter starts an aggregate called name, in that container's own format version.
func NewSetWriter(name string, version uint16) *SetWriter {
	w := &SetWriter{name: name, version: version, members: make(map[string][]byte)}
	if name == "" {
		w.err = fmt.Errorf("snapcodec: set writer name is empty")
	}
	return w
}

// Add stores one member's blob.
//
// A duplicate name is an error rather than an overwrite. Overwriting would mean one of two
// snapshots of the same member silently winning, and which one won would depend on iteration
// order - a difference that would show up much later as a machine that restored to not quite
// either state.
func (w *SetWriter) Add(member string, blob []byte) error {
	if w.err != nil {
		return w.err
	}
	if member == "" {
		w.err = fmt.Errorf("snapcodec: %s: a member with no name cannot be matched on restore", w.name)
		return w.err
	}
	if _, dup := w.members[member]; dup {
		w.err = fmt.Errorf("snapcodec: %s: %w: %q", w.name, ErrDuplicateMember, member)
		return w.err
	}
	stored := make([]byte, len(blob))
	copy(stored, blob)
	w.members[member] = stored
	w.order = append(w.order, member)
	return nil
}

// Len reports how many members have been added.
func (w *SetWriter) Len() int { return len(w.members) }

// Err reports the first error the writer hit, if any.
func (w *SetWriter) Err() error { return w.err }

// Blob finalises the container. Members are written in name order regardless of the order they
// were added, so the encoding is a function of the contents alone.
func (w *SetWriter) Blob() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}

	names := append([]string(nil), w.order...)
	sort.Strings(names)

	count, err := u32len(len(names))
	if err != nil {
		return nil, fmt.Errorf("snapcodec: %s: %w", w.name, err)
	}

	inner := NewWriter(w.name, w.version)
	inner.Uint32(count)
	for _, n := range names {
		inner.String(n)
		inner.Bytes(w.members[n])
	}
	return inner.Blob()
}

// Set is a decoded aggregate container.
type Set struct {
	name    string
	version uint16
	members map[string][]byte
	names   []string
}

// OpenSet decodes an aggregate container and asserts what it claims to be.
//
// The name and version checks are the same refusal Reader.Expect makes, for the same reason: an
// aggregate from a different writer, or from a layout this build has never seen, must be refused
// by name rather than decoded into whatever the fields happen to line up with.
func OpenSet(blob []byte, name string, minVersion, maxVersion uint16) (*Set, error) {
	r, err := Open(blob)
	if err != nil {
		return nil, err
	}
	if err := r.Expect(name, minVersion, maxVersion); err != nil {
		return nil, err
	}

	count := r.Uint32()

	// The count is the blob's own claim, so it is checked against what the blob could possibly
	// hold BEFORE it is used to size anything. Every member costs at least eight payload bytes -
	// a four-byte length prefix for its name and another for its blob - so a container claiming
	// more members than remaining/8 is describing a payload that is not there.
	//
	// Without this, `make([]string, 0, count)` asked the allocator for whatever a corrupt or
	// hostile header named: a forged uint32 in a 40-byte file requested a 64 GB block. That is a
	// runtime THROW rather than a panic, so no recover() anywhere can catch it - it takes the
	// process down, which is the one outcome "errors are values; it must never take the process
	// down" forbids, and on a host with generous overcommit it reads as a hang instead. Reader.Words
	// already guards its own length the same way; this was the one place that trusted the header.
	// Widened to int64 rather than uint64: both conversions are then widening and cannot
	// misrepresent their input, and the largest product possible here - 0xFFFFFFFF members at
	// eight bytes - is about 34 billion, which int64 holds with room to spare. An unsigned
	// comparison would need remaining() proved non-negative first, which is a second thing to
	// get right for no benefit.
	if need := int64(count) * minMemberBytes; need > int64(r.remaining()) {
		return nil, fmt.Errorf("snapcodec: %s: %w: header claims %d members, which need at least "+
			"%d payload bytes, and %d remain",
			name, ErrTruncated, count, need, r.remaining())
	}

	members := make(map[string][]byte)
	names := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		member := r.String()
		body := r.Bytes()
		if r.Err() != nil {
			// The count is the blob's own claim, so a corrupt one must not be looped on. The
			// reader's error is sticky and Done reports it below.
			break
		}
		if _, dup := members[member]; dup {
			return nil, fmt.Errorf("snapcodec: %s: %w: %q", name, ErrDuplicateMember, member)
		}
		members[member] = body
		names = append(names, member)
	}
	// Done is the whole check, and deliberately the only one.
	//
	// A header count that overstates what follows runs the reader off the end, and a count that
	// understates it leaves bytes unread; those are ErrShortRead and ErrTrailingBytes, both of
	// which Done reports. An explicit len(members) != count comparison here looks like a third
	// guard and is in fact unreachable - which was demonstrated rather than assumed: deleting it
	// killed no test, because nothing can reach it. A branch that cannot execute is not
	// defence in depth, it is a claim of safety nobody can check, so it is gone. What replaces it
	// is TestAHeaderCountThatDisagreesWithItsContentsIsRefused, which asserts the property
	// through the mechanism that genuinely enforces it.
	if err := r.Done(); err != nil {
		return nil, err
	}

	sort.Strings(names)
	return &Set{name: name, version: r.Version(), members: members, names: names}, nil
}

// Name reports the aggregate's writer name.
func (s *Set) Name() string { return s.name }

// Version reports the aggregate's format version, so a caller can branch on an older layout.
func (s *Set) Version() uint16 { return s.version }

// Len reports how many members the set carries.
func (s *Set) Len() int { return len(s.members) }

// Names lists the members, sorted.
func (s *Set) Names() []string { return append([]string(nil), s.names...) }

// Member returns one member's blob.
//
// The second result distinguishes an absent member from one whose blob is empty. A caller reading
// only the first would restore an absent member from nothing, which is the whole failure.
func (s *Set) Member(name string) ([]byte, bool) {
	b, ok := s.members[name]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, true
}

// Require refuses unless the set describes exactly the expected members.
//
// This is the completeness instrument. Both directions are errors and both are named: a member the
// snapshot forgot would otherwise be restored from nothing or left holding whatever it happened to
// hold, and a member the snapshot carries that the caller does not have means the snapshot is of a
// different machine - which, silently ignored, restores a subset and calls it a success.
//
// Expected names are the caller's to supply; this package has no view on what ought to be present.
func (s *Set) Require(expected []string) error {
	want := make(map[string]bool, len(expected))
	for _, name := range expected {
		want[name] = true
	}

	var missing, unexpected []string
	for name := range want {
		if _, present := s.members[name]; !present {
			missing = append(missing, name)
		}
	}
	for name := range s.members {
		if !want[name] {
			unexpected = append(unexpected, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)

	var problems []error
	if len(missing) > 0 {
		problems = append(problems, fmt.Errorf(
			"snapcodec: %s: %w: says nothing about %s, and a member left holding whatever it "+
				"happened to hold is a machine that looks right and is not",
			s.name, ErrMemberMissing, strings.Join(missing, ", ")))
	}
	if len(unexpected) > 0 {
		problems = append(problems, fmt.Errorf("snapcodec: %s: %w: %s",
			s.name, ErrMemberUnexpected, strings.Join(unexpected, ", ")))
	}
	return errors.Join(problems...)
}
