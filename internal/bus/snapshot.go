package bus

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// The machine-snapshot container: one opaque blob per attached device, keyed by the device's name.
//
// What is INSIDE a blob is that device's business. This layer's whole job is to make sure the set
// of blobs matches the set of devices EXACTLY, because the failure spike 003 exists to name is a
// snapshot that restores most of a machine. That failure has no symptom - it produces a plausible
// machine whose faults read as firmware bugs - so the only place it can be caught is here, where
// the two sets can still be compared.
//
// The framing is snapcodec's, not this package's, so that the magic, the container version, the
// truncation checks and the not-fully-consumed check are the same ones every device gets.
const (
	snapWriter  = "machine"
	snapVersion = 1
)

// Snapshot encodes the state of every attached device.
//
// Devices are written in name order, so two snapshots of the same machine state are byte-identical
// - which is what lets a snapshot be compared rather than only restored.
func (b *Bus) Snapshot() ([]byte, error) {
	if b.unknownState != nil {
		return nil, fmt.Errorf("bus: snapshot: %w", b.unknownState)
	}

	type entry struct {
		name string
		blob []byte
	}
	entries := make([]entry, 0, len(b.regions))
	for _, r := range b.regions {
		blob, err := r.dev.Snapshot()
		if err != nil {
			return nil, fmt.Errorf("bus: snapshot %q: %w", r.dev.Name(), err)
		}
		entries = append(entries, entry{name: r.dev.Name(), blob: blob})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	// Bounded by the devices actually attached, but a conversion that is silently wrong when it
	// is not is exactly the shape of fault this package exists to make loud.
	total := uint64(len(entries))
	if total > math.MaxUint32 {
		return nil, fmt.Errorf("bus: snapshot: %d devices is more than the format can count", total)
	}
	w := snapcodec.NewWriter(snapWriter, snapVersion)
	w.Uint32(uint32(total))
	for _, e := range entries {
		w.String(e.name)
		w.Bytes(e.blob)
	}
	blob, err := w.Blob()
	if err != nil {
		return nil, fmt.Errorf("bus: snapshot: %w", err)
	}
	return blob, nil
}

// Restore replaces every attached device's state from a snapshot.
//
// It is all or nothing. A snapshot that does not name exactly the devices attached is refused
// before any device is touched, and a device that rejects its own blob part-way through rolls the
// whole machine back to where it was. A half-applied restore is worse than a refused one: the
// caller believes nothing happened, and the machine is a mixture of two states that will diverge
// from both.
func (b *Bus) Restore(state []byte) error {
	if b.unknownState != nil {
		return fmt.Errorf("bus: restore: %w", b.unknownState)
	}

	blobs, err := decodeSnapshot(state)
	if err != nil {
		return err
	}
	if err := b.checkDeviceSet(blobs); err != nil {
		return err
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

// checkDeviceSet refuses a snapshot whose devices are not exactly the devices attached, naming
// what is wrong on each side.
func (b *Bus) checkDeviceSet(blobs map[string][]byte) error {
	var missing, unknown []string
	for name := range blobs {
		if _, attached := b.names[name]; !attached {
			unknown = append(unknown, name)
		}
	}
	for name := range b.names {
		if _, present := blobs[name]; !present {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)

	switch {
	case len(missing) > 0 && len(unknown) > 0:
		return fmt.Errorf("bus: restore: the snapshot does not describe this machine - it is "+
			"missing %s and names %s, which is not attached",
			strings.Join(missing, ", "), strings.Join(unknown, ", "))
	case len(missing) > 0:
		return fmt.Errorf("bus: restore: the snapshot says nothing about %s, and a device left "+
			"holding whatever it happened to hold is a machine that looks right and is not",
			strings.Join(missing, ", "))
	case len(unknown) > 0:
		return fmt.Errorf("bus: restore: the snapshot names %s, which this machine does not have",
			strings.Join(unknown, ", "))
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

func decodeSnapshot(state []byte) (map[string][]byte, error) {
	r, err := snapcodec.Open(state)
	if err != nil {
		return nil, fmt.Errorf("bus: restore: %w", err)
	}
	if err := r.Expect(snapWriter, snapVersion, snapVersion); err != nil {
		return nil, fmt.Errorf("bus: restore: %w", err)
	}

	count := r.Uint32()
	blobs := make(map[string][]byte)
	for i := uint32(0); i < count; i++ {
		name := r.String()
		blob := r.Bytes()
		if r.Err() != nil {
			// The count is the blob's own claim, so a corrupt one must not be looped on: the
			// reader's error is sticky and Done below reports it.
			break
		}
		if _, dup := blobs[name]; dup {
			return nil, fmt.Errorf("bus: restore: the snapshot describes %q twice", name)
		}
		blobs[name] = blob
	}
	if err := r.Done(); err != nil {
		return nil, fmt.Errorf("bus: restore: %w", err)
	}
	if uint64(len(blobs)) != uint64(count) {
		return nil, fmt.Errorf("bus: restore: the snapshot claims %d devices and carries %d",
			count, len(blobs))
	}
	return blobs, nil
}
