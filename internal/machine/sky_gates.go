package machine

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// SkyGates is the browser oracle's declared post-boot presentation policy.
// It answers two application gates only after the guest has created 42 tasks.
type SkyGates struct {
	enabled bool
	done    bool
	next    uint64
}

const (
	skyGateCheckInterval = 4_000_000
	skyGateRAMAddress    = 0x80054F84
	skyGateFlashOffset   = 0x72FA1
)

// NewSkyGates chooses whether this optional presentation policy runs.
func NewSkyGates(enabled bool) *SkyGates { return &SkyGates{enabled: enabled} }

// Tick checks at the oracle's four-million-instruction cadence. The guest's
// task list is supplied by the runner so this policy does not own RTOS parsing.
func (g *SkyGates) Tick(icount uint64, handoffDone bool, taskCount func() (int, error), board *bus.Bus, flash *memory.Flash) (bool, error) {
	if !g.enabled || g.done || !handoffDone || icount < g.next {
		return false, nil
	}
	g.next = icount + skyGateCheckInterval
	count, err := taskCount()
	if err != nil {
		return false, fmt.Errorf("sky gates: task census: %w", err)
	}
	if count < 42 {
		return false, nil
	}
	if board.Read(skyGateRAMAddress, bus.Word) != 0xFFFFFFFF {
		return false, nil
	}
	if err := flash.ApplyHostPatch(skyGateFlashOffset, 0x75, 0x76); err != nil {
		return false, err
	}
	board.Write(skyGateRAMAddress, bus.Word, 0)
	g.done = true
	return true, nil
}

// Done reports whether both declared gate answers were applied.
func (g *SkyGates) Done() bool { return g.done }

// Snapshot preserves the policy deadline and one-shot state.
func (g *SkyGates) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter("sky-gates", 1)
	w.Bool(g.enabled)
	w.Bool(g.done)
	w.Uint64(g.next)
	return w.Blob()
}

// Restore validates the policy state before replacing it.
func (g *SkyGates) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("sky gates: restore: %w", err)
	}
	if err := r.Expect("sky-gates", 1, 1); err != nil {
		return fmt.Errorf("sky gates: restore: %w", err)
	}
	enabled, done, next := r.Bool(), r.Bool(), r.Uint64()
	if err := r.Done(); err != nil {
		return fmt.Errorf("sky gates: restore: %w", err)
	}
	g.enabled, g.done, g.next = enabled, done, next
	return nil
}
