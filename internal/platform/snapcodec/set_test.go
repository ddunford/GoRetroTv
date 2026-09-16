package snapcodec_test

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	machineName    = "machine"
	machineVersion = uint16(1)
)

// attached is the set a bus would pass to Require: the members it actually has.
var attached = []string{"blitter", "demux", "dram", "flash"}

func buildSet(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	w := snapcodec.NewSetWriter(machineName, machineVersion)
	for name, blob := range members {
		if err := w.Add(name, blob); err != nil {
			t.Fatalf("Add(%q): %v", name, err)
		}
	}
	blob, err := w.Blob()
	if err != nil {
		t.Fatalf("Blob: %v", err)
	}
	return blob
}

func fullMachine() map[string][]byte {
	return map[string][]byte{
		"dram":    {0x01, 0x02},
		"flash":   {0x03},
		"demux":   {0x04, 0x05, 0x06},
		"blitter": {},
	}
}

func TestSetRoundTrip(t *testing.T) {
	t.Parallel()

	want := fullMachine()
	set, err := snapcodec.OpenSet(buildSet(t, want), machineName, machineVersion, machineVersion)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}
	if set.Len() != len(want) {
		t.Errorf("Len = %d, want %d", set.Len(), len(want))
	}
	if got := set.Names(); !reflect.DeepEqual(got, attached) {
		t.Errorf("Names = %v, want %v (sorted)", got, attached)
	}
	for name, blob := range want {
		got, ok := set.Member(name)
		if !ok {
			t.Errorf("Member(%q) missing", name)
			continue
		}
		if !reflect.DeepEqual(got, blob) {
			t.Errorf("Member(%q) = %v, want %v", name, got, blob)
		}
	}
	if err := set.Require(attached); err != nil {
		t.Errorf("Require: %v", err)
	}
}

// TestRequireRefusesASetMissingAMember is spike 003's finding as an assertion, and the reason this
// container exists at all. A snapshot that restores most of a machine does not fail; it produces a
// plausible machine whose faults read as firmware bugs.
func TestRequireRefusesASetMissingAMember(t *testing.T) {
	t.Parallel()

	partial := fullMachine()
	delete(partial, "demux")

	set, err := snapcodec.OpenSet(buildSet(t, partial), machineName, machineVersion, machineVersion)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}

	// The set itself is perfectly well-formed. Only the comparison against what the caller has
	// can tell that it describes a different machine - which is why the check lives here.
	err = set.Require(attached)
	if err == nil {
		t.Fatal("Require accepted a snapshot with no demux in it; this is the plausible-machine failure")
	}
	if !errors.Is(err, snapcodec.ErrMemberMissing) {
		t.Errorf("error %v does not wrap ErrMemberMissing", err)
	}
	if !strings.Contains(err.Error(), "demux") {
		t.Errorf("error %q does not name the missing member; a refusal that does not say what is missing cannot be acted on", err)
	}
	if errors.Is(err, snapcodec.ErrMemberUnexpected) {
		t.Errorf("error %v also claims an unexpected member; nothing here is unexpected", err)
	}
}

func TestRequireRefusesASetNamingAMemberThatIsNotThere(t *testing.T) {
	t.Parallel()

	extra := fullMachine()
	extra["smartcard"] = []byte{0x09}

	set, err := snapcodec.OpenSet(buildSet(t, extra), machineName, machineVersion, machineVersion)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}

	err = set.Require(attached)
	if err == nil {
		t.Fatal("Require accepted a snapshot of a different machine")
	}
	if !errors.Is(err, snapcodec.ErrMemberUnexpected) {
		t.Errorf("error %v does not wrap ErrMemberUnexpected", err)
	}
	if !strings.Contains(err.Error(), "smartcard") {
		t.Errorf("error %q does not name the unexpected member", err)
	}
}

// TestRequireReportsBothDirectionsAtOnce spares whoever is holding a snapshot of the wrong machine
// from discovering one half of the mismatch per attempt.
func TestRequireReportsBothDirectionsAtOnce(t *testing.T) {
	t.Parallel()

	wrong := fullMachine()
	delete(wrong, "demux")
	wrong["smartcard"] = []byte{0x09}

	set, err := snapcodec.OpenSet(buildSet(t, wrong), machineName, machineVersion, machineVersion)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}

	err = set.Require(attached)
	if err == nil {
		t.Fatal("Require accepted a snapshot that is wrong in both directions")
	}
	if !errors.Is(err, snapcodec.ErrMemberMissing) || !errors.Is(err, snapcodec.ErrMemberUnexpected) {
		t.Errorf("error %v does not wrap both causes", err)
	}
	for _, want := range []string{"demux", "smartcard"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestAnEmptyMemberBlobIsNotAMissingMember is the distinction Member's second result exists for. A
// member that legitimately holds no state must not read as one the snapshot forgot.
func TestAnEmptyMemberBlobIsNotAMissingMember(t *testing.T) {
	t.Parallel()

	set, err := snapcodec.OpenSet(buildSet(t, fullMachine()), machineName, machineVersion, machineVersion)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}

	blob, ok := set.Member("blitter")
	if !ok {
		t.Fatal("the blitter, whose blob is empty, read as absent")
	}
	if len(blob) != 0 {
		t.Errorf("blitter blob = %v, want empty", blob)
	}
	if _, ok := set.Member("nothing-like-this"); ok {
		t.Error("an absent member reported present")
	}
	if err := set.Require(attached); err != nil {
		t.Errorf("Require: %v; an empty blob is a member that holds no state, not a missing one", err)
	}
}

// TestEncodingDoesNotDependOnInsertionOrder is what lets a snapshot be compared rather than only
// restored. It is also the property the oracle comparison and the replay gate rest on.
func TestEncodingDoesNotDependOnInsertionOrder(t *testing.T) {
	t.Parallel()

	forward := snapcodec.NewSetWriter(machineName, machineVersion)
	names := make([]string, 0, len(fullMachine()))
	for n := range fullMachine() {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		blob := fullMachine()[n]
		if err := forward.Add(n, blob); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	backward := snapcodec.NewSetWriter(machineName, machineVersion)
	for i := len(names) - 1; i >= 0; i-- {
		blob := fullMachine()[names[i]]
		if err := backward.Add(names[i], blob); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	a, err := forward.Blob()
	if err != nil {
		t.Fatalf("Blob: %v", err)
	}
	b, err := backward.Blob()
	if err != nil {
		t.Fatalf("Blob: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("two sets with identical contents encoded differently; a snapshot whose bytes depend on insertion order cannot be compared")
	}
}

func TestSetWriterRefusesADuplicateMember(t *testing.T) {
	t.Parallel()

	w := snapcodec.NewSetWriter(machineName, machineVersion)
	if err := w.Add("dram", []byte{1}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	err := w.Add("dram", []byte{2})
	if err == nil {
		t.Fatal("Add accepted a duplicate; one of the two would have won silently, and which one would depend on iteration order")
	}
	if !errors.Is(err, snapcodec.ErrDuplicateMember) {
		t.Errorf("error %v does not wrap ErrDuplicateMember", err)
	}
}

func TestSetWriterRefusesAnUnnamedMember(t *testing.T) {
	t.Parallel()

	w := snapcodec.NewSetWriter(machineName, machineVersion)
	if err := w.Add("", []byte{1}); err == nil {
		t.Error("Add accepted a member with no name; it could never be matched on restore")
	}
}

// TestASetFromAnotherWriterOrVersionIsRefusedByName carries the primitive codec's refusal up to
// the aggregate, so an aggregate cannot be mis-read either.
func TestASetFromAnotherWriterOrVersionIsRefusedByName(t *testing.T) {
	t.Parallel()

	blob := buildSet(t, fullMachine())

	if _, err := snapcodec.OpenSet(blob, "something-else", machineVersion, machineVersion); !errors.Is(err, snapcodec.ErrWrongWriter) {
		t.Errorf("error %v does not wrap ErrWrongWriter", err)
	}
	if _, err := snapcodec.OpenSet(blob, machineName, machineVersion+1, machineVersion+2); !errors.Is(err, snapcodec.ErrUnsupportedVersion) {
		t.Errorf("error %v does not wrap ErrUnsupportedVersion", err)
	}
}

func TestOpenSetRefusesACorruptContainer(t *testing.T) {
	t.Parallel()

	good := buildSet(t, fullMachine())
	tests := []struct {
		name string
		blob []byte
	}{
		{"empty", nil},
		{"truncated", good[:len(good)-3]},
		{"wrong magic", append([]byte("XXXX"), good[4:]...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if set, err := snapcodec.OpenSet(tt.blob, machineName, machineVersion, machineVersion); err == nil {
				t.Fatalf("OpenSet accepted a %s container with %d members", tt.name, set.Len())
			}
		})
	}
}

// TestRequireOnAnEmptySetStillRefuses guards the vacuous case: a container carrying nothing must
// not satisfy a Require for four members just because there is nothing to disagree with.
func TestRequireOnAnEmptySetStillRefuses(t *testing.T) {
	t.Parallel()

	set, err := snapcodec.OpenSet(buildSet(t, map[string][]byte{}), machineName, machineVersion, machineVersion)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}
	if set.Len() != 0 {
		t.Fatalf("Len = %d, want 0", set.Len())
	}
	err = set.Require(attached)
	if err == nil {
		t.Fatal("an empty set satisfied a Require for four members")
	}
	for _, want := range attached {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestTheLayoutMatchesTheOneTheBusAlreadyWrites keeps this container a drop-in for the hand-rolled
// framing in internal/bus, so migrating it is a refactor rather than a format change and snapshots
// written before the migration stay readable. If this fails, the migration is no longer free and
// whoever changed the layout needs to say so out loud.
func TestTheLayoutMatchesTheOneTheBusAlreadyWrites(t *testing.T) {
	t.Parallel()

	members := fullMachine()

	// Written the long way, exactly as internal/bus/snapshot.go does it today.
	names := make([]string, 0, len(members))
	for n := range members {
		names = append(names, n)
	}
	sort.Strings(names)
	manual := snapcodec.NewWriter(machineName, machineVersion)
	manual.Uint32(uint32(len(names)))
	for _, n := range names {
		manual.String(n)
		manual.Bytes(members[n])
	}
	handRolled, err := manual.Blob()
	if err != nil {
		t.Fatalf("hand-rolled Blob: %v", err)
	}

	if !reflect.DeepEqual(buildSet(t, members), handRolled) {
		t.Error("SetWriter no longer produces the layout internal/bus writes; the migration is not a drop-in and existing snapshots would not be readable")
	}

	// And the other direction: the hand-rolled bytes must open as a set.
	set, err := snapcodec.OpenSet(handRolled, machineName, machineVersion, machineVersion)
	if err != nil {
		t.Fatalf("OpenSet on hand-rolled bytes: %v", err)
	}
	if err := set.Require(attached); err != nil {
		t.Errorf("Require on hand-rolled bytes: %v", err)
	}
}

// TestAHeaderCountThatDisagreesWithItsContentsIsRefused asserts the invariant that an explicit
// count comparison in OpenSet used to claim. That comparison was removed once a mutant proved it
// unreachable; this is the same property, checked through the mechanism that actually enforces it.
func TestAHeaderCountThatDisagreesWithItsContentsIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count uint32
		pairs int
		want  error
	}{
		{name: "count overstates the members", count: 4, pairs: 2, want: snapcodec.ErrShortRead},
		{name: "count understates the members", count: 1, pairs: 3, want: snapcodec.ErrTrailingBytes},
		{name: "count claims members that are not there at all", count: 1, pairs: 0, want: snapcodec.ErrShortRead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := snapcodec.NewWriter(machineName, machineVersion)
			w.Uint32(tt.count)
			for i := 0; i < tt.pairs; i++ {
				w.String(string(rune('a' + i)))
				w.Bytes([]byte{byte(i)})
			}
			blob, err := w.Blob()
			if err != nil {
				t.Fatalf("Blob: %v", err)
			}

			set, err := snapcodec.OpenSet(blob, machineName, machineVersion, machineVersion)
			if err == nil {
				t.Fatalf("OpenSet accepted a container claiming %d members with %d present, giving a set of %d",
					tt.count, tt.pairs, set.Len())
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("error %v does not wrap %v", err, tt.want)
			}
		})
	}
}
