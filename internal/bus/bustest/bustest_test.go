package bustest_test

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/bus/bustest"
)

// scratchpad is a stand-in for a real peripheral, shaped around the thing that actually goes wrong.
//
// regs is state a bus read can see. seq is not: it is the position in a multi-write command
// sequence, the same shape as a flash chip's unlock sequence, which spike 003 names as real state
// that no read can observe. A device author who serialises what they can see keeps regs and
// forgets seq, and the restored machine then interprets the next write as a fresh command.
type scratchpad struct {
	regs [4]uint32
	seq  uint8
	hits uint64
}

func newScratchpad() *scratchpad { d := &scratchpad{}; d.Reset(); return d }

func (d *scratchpad) Name() string { return "scratchpad" }

func (d *scratchpad) Read(off uint32, _ bus.Size) uint32 {
	d.hits++
	if i := off >> 2; i < uint32(len(d.regs)) {
		return d.regs[i]
	}
	return 0
}

func (d *scratchpad) Write(off uint32, _ bus.Size, value uint32) {
	d.hits++
	if i := off >> 2; i < uint32(len(d.regs)) {
		d.regs[i] = value
		d.seq = (d.seq + 1) % 3
	}
}

func (d *scratchpad) Reset() { *d = scratchpad{} }

func (d *scratchpad) Snapshot() ([]byte, error) {
	b := make([]byte, 0, 4*4+1+8)
	for _, r := range d.regs {
		b = binary.BigEndian.AppendUint32(b, r)
	}
	b = append(b, d.seq)
	return binary.BigEndian.AppendUint64(b, d.hits), nil
}

func (d *scratchpad) Restore(state []byte) error {
	if len(state) != 4*4+1+8 {
		return fmt.Errorf("scratchpad: snapshot is %d bytes, want %d", len(state), 4*4+1+8)
	}
	*d = scratchpad{}
	for i := range d.regs {
		d.regs[i] = binary.BigEndian.Uint32(state[i*4:])
	}
	d.seq = state[16]
	d.hits = binary.BigEndian.Uint64(state[17:])
	return nil
}

// forgetful is scratchpad with one deliberate defect: its Snapshot omits seq. It is kept rather
// than deleted after being watched to fail once, because a negative control that has been removed
// is a negative control nobody can run again - and the check it guards has to keep working across
// every device phases 2 to 7 add.
type forgetful struct{ scratchpad }

func newForgetful() *forgetful { d := &forgetful{}; d.Reset(); return d }

func (d *forgetful) Name() string { return "forgetful" }

func (d *forgetful) Snapshot() ([]byte, error) {
	b := make([]byte, 0, 4*4+8)
	for _, r := range d.regs {
		b = binary.BigEndian.AppendUint32(b, r)
	}
	return binary.BigEndian.AppendUint64(b, d.hits), nil
}

func (d *forgetful) Restore(state []byte) error {
	if len(state) != 4*4+8 {
		return fmt.Errorf("forgetful: snapshot is %d bytes, want %d", len(state), 4*4+8)
	}
	d.scratchpad = scratchpad{}
	for i := range d.regs {
		d.regs[i] = binary.BigEndian.Uint32(state[i*4:])
	}
	d.hits = binary.BigEndian.Uint64(state[16:])
	return nil
}

// lazyRestore is scratchpad with the other defect, and it is the shape a real one takes: its
// Restore decodes the whole snapshot but only writes the sequence position when the device looks
// uninitialised. Into a fresh device that is indistinguishable from a correct restore, so the
// completeness check cannot see it. Onto a device that has been running, the position it was
// already at survives the restore - and the machine carries on from a state the snapshot never
// described.
type lazyRestore struct{ scratchpad }

func newLazyRestore() *lazyRestore { d := &lazyRestore{}; d.Reset(); return d }

func (d *lazyRestore) Name() string { return "lazyRestore" }

func (d *lazyRestore) Restore(state []byte) error {
	if len(state) != 4*4+1+8 {
		return fmt.Errorf("lazyRestore: snapshot is %d bytes, want %d", len(state), 4*4+1+8)
	}
	for i := range d.regs {
		d.regs[i] = binary.BigEndian.Uint32(state[i*4:])
	}
	if d.seq == 0 {
		d.seq = state[16]
	}
	d.hits = binary.BigEndian.Uint64(state[17:])
	return nil
}

// mutate drives all three fields: regs directly, hits because every access counts, and seq
// because four writes leave the modulo-3 sequence position at 1 rather than back at 0. That last
// one is not incidental - an earlier version of this test wrote three times, seq came back round
// to its fresh value, and the forgotten-field test below passed a device that had forgotten it.
func mutate(d bus.Device) {
	d.Write(0x00, bus.Word, 0xDEADBEEF)
	d.Write(0x04, bus.Word, 0x80081C58)
	d.Write(0x08, bus.Word, 0x0000FFC0)
	d.Write(0x0C, bus.Word, 0x000000A0)
}

// disturb moves all three again: one further write takes seq from 1 to 2 and hits from 4 to 5.
func disturb(d bus.Device) {
	d.Write(0x00, bus.Word, 0x00000001)
}

func check(newDevice func() bus.Device) bustest.Check {
	return bustest.Check{New: newDevice, Mutate: mutate, Disturb: disturb}
}

// TC-1.2: a device round-trips through snapshot.
func TestSnapshotRoundTripsACompleteDevice(t *testing.T) {
	t.Parallel()
	if err := bustest.CheckSnapshot(check(func() bus.Device { return newScratchpad() })); err != nil {
		t.Fatalf("a device that serialises all of its state should pass: %v", err)
	}
}

// TC-1.2, the negative control: the check must catch a device that forgets a field. Without this
// the passing case above proves only that the check runs, not that it examines anything.
func TestSnapshotCatchesAForgottenField(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(check(func() bus.Device { return newForgetful() }))
	if err == nil {
		t.Fatal("a device whose Snapshot omits its command-sequence position must fail the check; " +
			"it passed, so the check is examining nothing")
	}
	if !strings.Contains(err.Error(), "seq") {
		t.Fatalf("the failure must name the forgotten field so it is actionable, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// The defect this package was rewritten for, pinned so it cannot come back: a mutator that drives
// the visible fields but happens to leave an invisible one at its fresh value. The round trip then
// compares that field against itself and the forgotten-field test above passes a broken device.
// Three writes take scratchpad's modulo-3 sequence position right back round to zero, which is
// exactly how this was shipped the first time.
func TestSnapshotReportsAMutatorThatMissesOneField(t *testing.T) {
	t.Parallel()
	visibleOnly := func(d bus.Device) {
		d.Write(0x00, bus.Word, 0xDEADBEEF)
		d.Write(0x04, bus.Word, 0x80081C58)
		d.Write(0x08, bus.Word, 0x0000FFC0)
	}
	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return newForgetful() }, Mutate: visibleOnly, Disturb: disturb,
	})
	if err == nil {
		t.Fatal("a mutator that leaves one field at its fresh value must be reported; it passed, " +
			"and it passed a device that forgets that very field")
	}
	if !strings.Contains(err.Error(), "harness failure") || !strings.Contains(err.Error(), "seq") {
		t.Fatalf("it must be reported as a harness failure naming the undriven field, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// A Constant exemption that names no field silently stops checking nothing, and hides the day
// somebody renames the field it used to exempt.
func TestSnapshotReportsAStaleConstantExemption(t *testing.T) {
	t.Parallel()
	c := check(func() bus.Device { return newScratchpad() })
	c.Constant = []string{"partNumber"}
	err := bustest.CheckSnapshot(c)
	if err == nil {
		t.Fatal("an exemption for a field the device does not have must be reported")
	}
	if !strings.Contains(err.Error(), "harness failure") {
		t.Fatalf("it must be reported as a harness failure, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// TC-1.2, the mirror defect: a Restore that merges rather than replaces.
func TestSnapshotCatchesARestoreThatDoesNotReplace(t *testing.T) {
	t.Parallel()
	err := bustest.CheckSnapshot(check(func() bus.Device { return newLazyRestore() }))
	if err == nil {
		t.Fatal("a Restore that leaves a field at the value the running device had must fail the check")
	}
	if !strings.Contains(err.Error(), "survived") {
		t.Fatalf("the completeness check cannot see this defect - only the replacement check can, "+
			"so the failure must come from that one: %v", err)
	}
	if !strings.Contains(err.Error(), "seq") {
		t.Fatalf("the failure must name the surviving field, got: %v", err)
	}
	t.Logf("caught: %v", err)
}

// The subject assertion: a mutator that does nothing makes every other check vacuous, so it is a
// harness failure rather than a pass.
func TestSnapshotReportsAnIneffectiveMutator(t *testing.T) {
	t.Parallel()
	nothing := func(bus.Device) {}

	err := bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return newScratchpad() }, Mutate: nothing, Disturb: disturb,
	})
	if err == nil {
		t.Fatal("a mutate that leaves the device fresh must be reported, not passed")
	}
	if !strings.Contains(err.Error(), "harness failure") {
		t.Fatalf("it must be reported as a harness failure, got: %v", err)
	}

	err = bustest.CheckSnapshot(bustest.Check{
		New: func() bus.Device { return newScratchpad() }, Mutate: mutate, Disturb: nothing,
	})
	if err == nil {
		t.Fatal("a disturb that changes nothing must be reported, not passed")
	}
	if !strings.Contains(err.Error(), "harness failure") {
		t.Fatalf("it must be reported as a harness failure, got: %v", err)
	}
}
