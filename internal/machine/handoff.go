// Package machine contains deterministic policies applied around guest instructions.
package machine

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/cpu"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

// Handoff is the declared browser-oracle policy for starting the application
// after the real bootloader has gone idle. The policy checks the image header.
// The policy changes only PC, ISA and RA; the guest executes the decompressor.
type Handoff struct {
	idleSpin uint64
	done     bool
}

const idleThreshold uint64 = 200_000

// Tick checks one 16-iteration device-pump boundary, as in the browser oracle.
// Count the covered instructions explicitly: counting calls made a batched pump
// turn this 200,000-instruction gate into a 3.2-million-instruction gate.
func (h *Handoff) Tick(core *cpu.Core, board *bus.Bus) (bool, error) {
	if h.done {
		return false, nil
	}
	ready := board.Read(0x800083CC, bus.Word)
	current := board.Read(0x800083D0, bus.Word)
	if ready < 0x100 || current != 0 {
		h.idleSpin = 0
		return false, nil
	}
	h.idleSpin += 16
	if h.idleSpin < idleThreshold || core.HasPendingBranch() {
		return false, nil
	}
	magic := board.Read(0xBFC12584, bus.Word)
	header := board.Read(0xBFC20000, bus.Word)
	if header != magic {
		return false, fmt.Errorf("application handoff: flash header %08X differs from ROM magic %08X", header, magic)
	}
	entry := board.Read(0xBFC2002C, bus.Word)
	// This field in the supplied image is an instruction, not an address.
	// The oracle then starts the flash entry stub, which performs the guest load.
	if entry < memory.FlashU202+0x20000 || entry >= memory.FlashU202+memory.FlashSize {
		entry = 0xBFC2048C
	}
	core.GPR[31] = 0
	core.PC = entry
	core.ISA = false
	h.done = true
	return true, nil
}

// Done reports whether the declared host handoff has fired.
func (h *Handoff) Done() bool { return h.done }

// Snapshot preserves the idle interval and one-shot state across a machine save.
func (h *Handoff) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter("application-handoff", 1)
	w.Uint64(h.idleSpin)
	w.Bool(h.done)
	return w.Blob()
}

// Restore replaces the policy state only after the whole snapshot validates.
func (h *Handoff) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("application handoff: restore: %w", err)
	}
	if err := r.Expect("application-handoff", 1, 1); err != nil {
		return fmt.Errorf("application handoff: restore: %w", err)
	}
	spin, done := r.Uint64(), r.Bool()
	if err := r.Done(); err != nil {
		return fmt.Errorf("application handoff: restore: %w", err)
	}
	h.idleSpin, h.done = spin, done
	return nil
}
