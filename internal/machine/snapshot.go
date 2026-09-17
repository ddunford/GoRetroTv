package machine

import (
	"errors"
	"fmt"
	"io"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/platform/clock"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	machineSnapshotName    = "goretrotv-machine"
	machineSnapshotVersion = 1
	maxMachineSnapshotSize = 128 << 20
)

var machineMembers = []string{"bus", "clock", "cpu", "handoff", "retired", "sky-gates"}

// Machine owns the state that must move together when an emulation run is saved.
// All attached memory and peripherals are included by the bus's own exact-set snapshot.
type Machine struct {
	Core     *cpu.Core
	Bus      *bus.Bus
	Clock    *clock.Clock
	Handoff  *Handoff
	SkyGates *SkyGates
	Retired  uint64

	clockHandlers map[string]clock.Handler
	unknownState  error
}

// New assembles the parts of one machine. Clock handlers are keyed by their stable event names
// so a restored schedule can bind to this machine's devices instead of stale closures.
func New(core *cpu.Core, board *bus.Bus, clk *clock.Clock, handoff *Handoff, sky *SkyGates, handlers map[string]clock.Handler) (*Machine, error) {
	if core == nil || board == nil || clk == nil || handoff == nil || sky == nil {
		return nil, fmt.Errorf("machine: every state owner must be present")
	}
	bound := make(map[string]clock.Handler, len(handlers))
	for name, fn := range handlers {
		if name == "" || fn == nil {
			return nil, fmt.Errorf("machine: clock handler %q is missing a name or function", name)
		}
		bound[name] = fn
	}
	return &Machine{Core: core, Bus: board, Clock: clk, Handoff: handoff, SkyGates: sky, clockHandlers: bound}, nil
}

// Snapshot writes one versioned image containing the CPU, all bus devices, clock and host policies.
func (m *Machine) Snapshot(w io.Writer) error {
	if m.unknownState != nil {
		return fmt.Errorf("machine: snapshot: %w", m.unknownState)
	}
	if w == nil {
		return fmt.Errorf("machine: snapshot: writer is nil")
	}
	set := snapcodec.NewSetWriter(machineSnapshotName, machineSnapshotVersion)
	owners := []struct {
		name string
		save func() ([]byte, error)
	}{
		{"bus", m.Bus.Snapshot},
		{"clock", m.Clock.Snapshot},
		{"cpu", m.Core.Snapshot},
		{"handoff", m.Handoff.Snapshot},
		{"retired", m.snapshotRetired},
		{"sky-gates", m.SkyGates.Snapshot},
	}
	for _, owner := range owners {
		blob, err := owner.save()
		if err != nil {
			return fmt.Errorf("machine: snapshot %s: %w", owner.name, err)
		}
		if err := set.Add(owner.name, blob); err != nil {
			return fmt.Errorf("machine: snapshot %s: %w", owner.name, err)
		}
	}
	blob, err := set.Blob()
	if err != nil {
		return fmt.Errorf("machine: snapshot: %w", err)
	}
	if len(blob) > maxMachineSnapshotSize {
		return fmt.Errorf("machine: snapshot is %d bytes, above %d-byte limit", len(blob), maxMachineSnapshotSize)
	}
	n, err := w.Write(blob)
	if err != nil {
		return fmt.Errorf("machine: write snapshot: %w", err)
	}
	if n != len(blob) {
		return fmt.Errorf("machine: write snapshot: %w", io.ErrShortWrite)
	}
	return nil
}

// Restore replaces the whole machine from a snapshot, rolling back if any member refuses it.
func (m *Machine) Restore(r io.Reader) error {
	if m.unknownState != nil {
		return fmt.Errorf("machine: restore: %w", m.unknownState)
	}
	if r == nil {
		return fmt.Errorf("machine: restore: reader is nil")
	}
	blob, err := io.ReadAll(io.LimitReader(r, maxMachineSnapshotSize+1))
	if err != nil {
		return fmt.Errorf("machine: read snapshot: %w", err)
	}
	if len(blob) > maxMachineSnapshotSize {
		return fmt.Errorf("machine: snapshot exceeds %d-byte limit", maxMachineSnapshotSize)
	}
	set, err := snapcodec.OpenSet(blob, machineSnapshotName, machineSnapshotVersion, machineSnapshotVersion)
	if err != nil {
		return fmt.Errorf("machine: restore: %w", err)
	}
	if err := set.Require(machineMembers); err != nil {
		return fmt.Errorf("machine: restore: %w", err)
	}
	before, err := m.saveNonBusState()
	if err != nil {
		return fmt.Errorf("machine: restore: save rollback state: %w", err)
	}
	if err := m.restoreSet(set); err != nil {
		if undo := m.restoreNonBusState(before); undo != nil {
			m.unknownState = fmt.Errorf("machine state is unknown after failed rollback: %w", undo)
			return errors.Join(err, m.unknownState)
		}
		return err
	}
	return nil
}

func (m *Machine) restoreSet(set *snapcodec.Set) error {
	owners := []struct {
		name    string
		restore func([]byte) error
	}{
		{"cpu", m.Core.Restore},
		{"handoff", m.Handoff.Restore},
		{"retired", m.restoreRetired},
		{"sky-gates", m.SkyGates.Restore},
		{"clock", func(blob []byte) error { return m.Clock.Restore(blob, m.clockHandlers) }},
		// Bus.Restore is atomic by itself. Keep it last: if another owner refuses its
		// blob, the 32 MB RAM and both flash chips have never been touched.
		{"bus", m.Bus.Restore},
	}
	for _, owner := range owners {
		blob, ok := set.Member(owner.name)
		if !ok {
			return fmt.Errorf("machine: restore: missing %s after completeness check", owner.name)
		}
		if err := owner.restore(blob); err != nil {
			return fmt.Errorf("machine: restore %s: %w", owner.name, err)
		}
	}
	return nil
}

type nonBusState struct {
	cpu, clock, handoff, sky []byte
	retired                  uint64
}

func (m *Machine) saveNonBusState() (nonBusState, error) {
	var state nonBusState
	var err error
	if state.cpu, err = m.Core.Snapshot(); err != nil {
		return state, err
	}
	if state.clock, err = m.Clock.Snapshot(); err != nil {
		return state, err
	}
	if state.handoff, err = m.Handoff.Snapshot(); err != nil {
		return state, err
	}
	if state.sky, err = m.SkyGates.Snapshot(); err != nil {
		return state, err
	}
	state.retired = m.Retired
	return state, nil
}

func (m *Machine) restoreNonBusState(state nonBusState) error {
	var failures []error
	if err := m.Core.Restore(state.cpu); err != nil {
		failures = append(failures, fmt.Errorf("cpu: %w", err))
	}
	if err := m.Handoff.Restore(state.handoff); err != nil {
		failures = append(failures, fmt.Errorf("handoff: %w", err))
	}
	if err := m.SkyGates.Restore(state.sky); err != nil {
		failures = append(failures, fmt.Errorf("sky gates: %w", err))
	}
	if err := m.Clock.Restore(state.clock, m.clockHandlers); err != nil {
		failures = append(failures, fmt.Errorf("clock: %w", err))
	}
	m.Retired = state.retired
	return errors.Join(failures...)
}

func (m *Machine) snapshotRetired() ([]byte, error) {
	w := snapcodec.NewWriter("retired-instructions", 1)
	w.Uint64(m.Retired)
	return w.Blob()
}

func (m *Machine) restoreRetired(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return err
	}
	if err := r.Expect("retired-instructions", 1, 1); err != nil {
		return err
	}
	retired := r.Uint64()
	if err := r.Done(); err != nil {
		return err
	}
	m.Retired = retired
	return nil
}
