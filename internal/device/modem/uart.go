// Package modem models the box's 16550-style internal modem UART.
package modem

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the uncached UART0 register window.
	Base = 0xB2001000
	// Size is the register window length.
	Size = 0x100
	// IRQMask selects the modem UART in the board interrupt controller.
	IRQMask  = 0x200
	maxBytes = 4096
)

// UART owns both the UART registers and the small soldered modem's AT reply.
type UART struct {
	ier, lcr, mcr, dll, dlm, msr, delta uint8
	rx, tx                              []byte
	lastTick                            uint64
	interrupt                           *irq.Controller
}

// New connects the modem UART to the board interrupt controller.
func New(interrupt *irq.Controller) *UART {
	return &UART{interrupt: interrupt, lastTick: ^uint64(0)}
}

// Name is the device's snapshot identity.
func (*UART) Name() string { return "modem-uart0" }

func (u *UART) Read(off uint32, size bus.Size) uint32 {
	if size != bus.Word && size != bus.Byte {
		return 0
	}
	switch off &^ 3 {
	case 0x00:
		if u.lcr&0x80 != 0 {
			return uint32(u.dll)
		}
		if len(u.rx) == 0 {
			return 0
		}
		b := u.rx[0]
		u.rx = u.rx[1:]
		return uint32(b)
	case 0x10:
		if u.lcr&0x80 != 0 {
			return uint32(u.dlm)
		}
		return uint32(u.ier)
	case 0x20:
		if u.ier&1 != 0 && len(u.rx) != 0 {
			return 0x04
		}
		if u.ier&8 != 0 && u.delta != 0 {
			return 0x00
		}
		if u.ier&2 != 0 {
			return 0x02
		}
		return 0x01
	case 0x30:
		return uint32(u.lcr)
	case 0x40:
		return uint32(u.mcr)
	case 0x50:
		v := uint32(0x60)
		if len(u.rx) != 0 {
			v |= 1
		}
		return v
	case 0x60:
		v := u.msr | u.delta
		u.delta = 0
		return uint32(v)
	}
	return 0
}

func (u *UART) Write(off uint32, size bus.Size, value uint32) {
	if size != bus.Word && size != bus.Byte {
		return
	}
	v := uint8(value & 0xff) // #nosec G115 -- UART registers are eight bits.
	switch off &^ 3 {
	case 0x00:
		if u.lcr&0x80 != 0 {
			u.dll = v
		} else {
			u.accept(v)
		}
	case 0x10:
		if u.lcr&0x80 != 0 {
			u.dlm = v
		} else {
			u.ier = v & 0x0f
		}
	case 0x30:
		u.lcr = v
	case 0x40:
		u.mcr = v
		before := u.msr
		u.msr = 0x80
		if v&2 != 0 {
			u.msr |= 0x10
		}
		if v&1 != 0 {
			u.msr |= 0x20
		}
		if (before^u.msr)&0x10 != 0 {
			u.delta |= 1
		}
		if (before^u.msr)&0x20 != 0 {
			u.delta |= 2
		}
		if (before^u.msr)&0x80 != 0 {
			u.delta |= 8
		}
	}
}

func (u *UART) accept(b byte) {
	if len(u.tx) < maxBytes {
		u.tx = append(u.tx, b)
	}
	n := len(u.tx)
	if n >= 2 && u.tx[n-2] == 'A' && u.tx[n-1] == 'T' && len(u.rx)+6 <= maxBytes {
		u.rx = append(u.rx, '\r', '\n', 'O', 'K', '\r', '\n')
	}
}

func (u *UART) wantsIRQ() bool {
	return u.ier&1 != 0 && len(u.rx) != 0 || u.ier&8 != 0 && u.delta != 0 || u.ier&2 != 0
}

// Pump updates pending status at the oracle's device cadence and requests IP2
// at most once per board timer tick while an enabled UART cause remains.
func (u *UART) Pump(boardTicks uint64) {
	want := u.wantsIRQ()
	if u.interrupt != nil {
		u.interrupt.SetStatus(IRQMask, want)
	}
	if want && boardTicks != u.lastTick {
		u.lastTick = boardTicks
		if u.interrupt != nil {
			u.interrupt.Pulse(IRQMask)
		}
	}
}

// Reset clears the volatile UART state and its interrupt line.
func (u *UART) Reset() {
	interrupt := u.interrupt
	*u = UART{interrupt: interrupt, lastTick: ^uint64(0)}
	if interrupt != nil {
		interrupt.SetStatus(IRQMask, false)
	}
}

// Snapshot captures the modem, registers, pending reply, and control-line changes.
func (u *UART) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(u.Name(), 2)
	w.Bytes([]byte{u.ier, u.lcr, u.mcr, u.dll, u.dlm, u.msr, u.delta})
	w.Bytes(u.rx)
	w.Bytes(u.tx)
	w.Uint64(u.lastTick)
	return w.Blob()
}

// Restore validates and replaces the complete UART state.
func (u *UART) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("modem: restore: %w", err)
	}
	if err = r.Expect(u.Name(), 2, 2); err != nil {
		return fmt.Errorf("modem: restore: %w", err)
	}
	regs, rx, tx, lastTick := r.Bytes(), r.Bytes(), r.Bytes(), r.Uint64()
	if err = r.Done(); err != nil {
		return fmt.Errorf("modem: restore: %w", err)
	}
	if len(regs) != 7 || len(rx) > maxBytes || len(tx) > maxBytes {
		return fmt.Errorf("modem: restore: invalid state")
	}
	u.ier, u.lcr, u.mcr, u.dll, u.dlm, u.msr, u.delta = regs[0], regs[1], regs[2], regs[3], regs[4], regs[5], regs[6]
	u.rx, u.tx = append([]byte(nil), rx...), append([]byte(nil), tx...)
	u.lastTick = lastTick
	if u.interrupt != nil {
		u.interrupt.SetStatus(IRQMask, u.wantsIRQ())
	}
	return nil
}

var _ bus.Device = (*UART)(nil)
