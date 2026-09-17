// Package hwtimer models the board timer that drives the firmware event loop.
package hwtimer

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the timer's MMIO window.
	Base = 0xB000D000
	// Size is the timer's register window length.
	Size = 0xE4
	// IRQMask selects timer channel zero in the board interrupt controller.
	IRQMask = 0x40
	// PeriodInstructions is the measured oracle's instruction-clock divider.
	PeriodInstructions = 20000
)

// Timer has channel-zero status and acknowledge registers and an icount clock.
type Timer struct {
	regs      [Size / 4]uint32
	now, next uint64
	armed     bool
	interrupt *irq.Controller
}

// New binds the timer to the board interrupt controller.
func New(interrupt *irq.Controller) *Timer { return &Timer{interrupt: interrupt} }

// Name is the timer's snapshot identity.
func (*Timer) Name() string { return "board-timer" }

func (t *Timer) Read(off uint32, size bus.Size) uint32 {
	if size != bus.Word || off >= Size || off%4 != 0 {
		return 0
	}
	return t.regs[off/4]
}

func (t *Timer) Write(off uint32, size bus.Size, value uint32) {
	if size != bus.Word || off >= Size || off%4 != 0 {
		return
	}
	if off == 0xE0 {
		t.regs[0xD0/4] &^= value
		t.updateLine()
		return
	}
	t.regs[off/4] = value
	if !t.armed {
		t.armed = true
		t.next = t.now + PeriodInstructions
	}
}

// Pump raises channel zero at each instruction-clock deadline after programming.
func (t *Timer) Pump(now uint64) {
	t.now = now
	if !t.armed || now < t.next {
		return
	}
	t.next += ((now-t.next)/PeriodInstructions + 1) * PeriodInstructions
	t.regs[0xD0/4] |= 1
	t.updateLine()
}

func (t *Timer) updateLine() {
	if t.interrupt != nil {
		t.interrupt.SetLine(IRQMask, t.regs[0xD0/4]&1 != 0)
	}
}

// Reset disarms the timer and clears its pending line.
func (t *Timer) Reset() {
	*t = Timer{interrupt: t.interrupt}
	t.updateLine()
}

// Snapshot captures registers, the instruction clock, and the next deadline.
func (t *Timer) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(t.Name(), 1)
	words := make([]uint32, 0, len(t.regs)+5)
	words = append(words, t.regs[:]...)
	// #nosec G115 -- splitting uint64 into its intentional high and low words.
	words = append(words, uint32(t.now>>32), uint32(t.now), uint32(t.next>>32), uint32(t.next))
	if t.armed {
		words = append(words, 1)
	} else {
		words = append(words, 0)
	}
	w.Words(words)
	return w.Blob()
}

// Restore validates the snapshot before replacing live timer state.
func (t *Timer) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("hwtimer: restore: %w", err)
	}
	if err := r.Expect(t.Name(), 1, 1); err != nil {
		return fmt.Errorf("hwtimer: restore: %w", err)
	}
	words := r.Words()
	if err := r.Done(); err != nil {
		return fmt.Errorf("hwtimer: restore: %w", err)
	}
	if len(words) != len(t.regs)+5 || words[len(words)-1] > 1 {
		return fmt.Errorf("hwtimer: restore: invalid state")
	}
	copy(t.regs[:], words[:len(t.regs)])
	t.now = uint64(words[len(t.regs)])<<32 | uint64(words[len(t.regs)+1])
	t.next = uint64(words[len(t.regs)+2])<<32 | uint64(words[len(t.regs)+3])
	t.armed = words[len(words)-1] == 1
	t.updateLine()
	return nil
}

var _ bus.Device = (*Timer)(nil)
