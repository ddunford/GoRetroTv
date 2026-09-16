package bus

import (
	"errors"
	"fmt"

	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// The machine-snapshot container: one opaque blob per attached device, keyed by the device's name.
//
// The FRAMING and the completeness refusal are snapcodec's. What stays here is the policy that
// makes them mean anything on a bus: that the set of members a snapshot must describe IS the set
// of devices attached. "What is attached to this bus" is this package's knowledge, and nothing in
// snapcodec should have a view on it.
//
// That division matters because the failure being guarded against has no symptom. A snapshot that
// restores most of a machine does not fail - it produces a plausible machine whose faults read as
// firmware bugs (spike 003), and by the time anyone is reading those the snapshot is long out of
// sight. The comparison can only be made here, while both sets are still in hand.
//
// This file framed its own set until snapcodec grew one. Two ways to frame a device set, both
// tested and both looking authoritative, is worse than either alone: the next person reaches for
// whichever they find first.
const (
	snapWriter  = "machine"
	snapVersion = 1
)

// Snapshot encodes the state of every attached device.
//
// Members come back in name order whatever order they were added in, so two snapshots of the same
// machine state are byte-identical - which is what lets a snapshot be compared rather than only
// restored.
func (b *Bus) Snapshot() ([]byte, error) {
	if b.unknownState != nil {
		return nil, fmt.Errorf("bus: snapshot: %w", b.unknownState)
	}

	w := snapcodec.NewSetWriter(snapWriter, snapVersion)
	for _, r := range b.regions {
		blob, err := r.dev.Snapshot()
		if err != nil {
			return nil, fmt.Errorf("bus: snapshot %q: %w", r.dev.Name(), err)
		}
		if err := w.Add(r.dev.Name(), blob); err != nil {
			return nil, fmt.Errorf("bus: snapshot: %w", err)
		}
	}
	blob, err := w.Blob()
	if err != nil {
		return nil, fmt.Errorf("bus: snapshot: %w", err)
	}
	return blob, nil
}

// Restore replaces every attached device's state from a snapshot.
//
// It is all or nothing. A snapshot that does not describe exactly the devices attached is refused
// before any device is touched, and a device that rejects its own blob part-way through rolls the
// whole machine back to where it was. A half-applied restore is worse than a refused one: the
// caller believes nothing happened, and the machine is a mixture of two states that will diverge
// from both.
func (b *Bus) Restore(state []byte) error {
	if b.unknownState != nil {
		return fmt.Errorf("bus: restore: %w", b.unknownState)
	}

	set, err := snapcodec.OpenSet(state, snapWriter, snapVersion, snapVersion)
	if err != nil {
		return fmt.Errorf("bus: restore: %w", err)
	}

	// The policy, and the only part of this a bus owns: what the snapshot must describe is
	// exactly what is attached.
	attached := make([]string, 0, len(b.regions))
	for _, r := range b.regions {
		attached = append(attached, r.dev.Name())
	}
	if err := set.Require(attached); err != nil {
		// snapcodec says which members are missing and which are unexpected, and why that
		// matters; what it cannot know is that the members here are DEVICES.
		return fmt.Errorf("bus: restore: the snapshot does not describe the devices on this "+
			"bus: %w", err)
	}

	blobs := make(map[string][]byte, len(attached))
	for _, name := range attached {
		blob, ok := set.Member(name)
		if !ok {
			// Require has just established that this cannot happen. Saying so, rather than
			// restoring a device from nothing, is the difference between a refusal and the
			// plausible machine this file exists to prevent.
			return fmt.Errorf("bus: restore: %q passed the completeness check and then had no "+
				"blob, which cannot happen and means this package and snapcodec disagree", name)
		}
		blobs[name] = blob
	}

	// The rollback copy, taken before anything is written, so a device that refuses its blob
	// half way through the loop below cannot leave the machine part-restored.
	before := make(map[string][]byte, len(b.regions))
	for _, r := range b.regions {
		blob, err := r.dev.Snapshot()
		if err != nil {
			return fmt.Errorf("bus: restore: could not save %q first, so the restore was not "+
				"attempted: %w", r.dev.Name(), err)
		}
		before[r.dev.Name()] = blob
	}

	for i, r := range b.regions {
		if err := r.dev.Restore(blobs[r.dev.Name()]); err != nil {
			failed := fmt.Errorf("bus: restore %q: %w", r.dev.Name(), err)
			if undo := rollback(b.regions[:i+1], before); undo != nil {
				b.unknownState = fmt.Errorf("the machine is in no known state: a restore failed "+
					"part way through and could not be undone; Reset it before using it again "+
					"(%w)", undo)
				return errors.Join(failed, b.unknownState)
			}
			return failed
		}
	}
	return nil
}

// rollback puts back the devices a failed restore had already reached.
//
// It should not be able to fail: the blobs it writes came out of these same devices moments
// earlier. If one refuses anyway then the machine is neither the state it was in nor the state
// that was being restored, and the only honest thing left is to say so - which is why the caller
// turns this error into a standing refusal rather than letting the machine be used again. It does
// not take the process down: a run that cannot report what it found is worse than one that reports
// bad news.
func rollback(reached []region, before map[string][]byte) error {
	var failures []error
	for _, r := range reached {
		if err := r.dev.Restore(before[r.dev.Name()]); err != nil {
			failures = append(failures, fmt.Errorf("%q rejected the snapshot taken from it "+
				"moments earlier: %w", r.dev.Name(), err))
		}
	}
	return errors.Join(failures...)
}
