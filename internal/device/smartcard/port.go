// Package smartcard models the framed controller behind the second card slot.
package smartcard

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the second card slot's uncached MMIO address.
	Base = 0xB2002000
	// Size is the register window length.
	Size = 0x100
	// IRQMask selects the smartcard UART dispatcher on the board.
	IRQMask = 0x00010000
	// ByteInstructions is one transmitted byte interval.
	ByteInstructions = 8000
	// ReplyGapInstructions spaces receive bytes so the guest HISR can advance.
	ReplyGapInstructions = 20000
	maxFrameBytes        = 1024
)

// Port owns the UART1 status, partial frame and paced reply bytes.
type Port struct {
	control, pair30, pair40, status, enable uint32
	rxByte                                  uint8
	frame, reply                            []byte
	txDelay, rxDelay                        uint64
	interrupt                               *irq.Controller
}

// New binds the port to the board IRQ controller.
func New(interrupt *irq.Controller) *Port { return &Port{interrupt: interrupt} }

// Name is the snapshot identity.
func (*Port) Name() string { return "smartcard-uart1" }

// Read exposes error flags, received byte, status and enabled interrupt bits.
func (p *Port) Read(off uint32, size bus.Size) uint32 {
	reg := off &^ 3
	switch reg {
	case 0x20:
		return 0
	case 0x50:
		if off == 0x53 && size == bus.Byte || off == 0x50 && size == bus.Word {
			return uint32(p.rxByte)
		}
	case 0x60:
		if size == bus.Word {
			return p.status
		}
	case 0x70:
		if size == bus.Word {
			return p.enable
		}
	}
	return 0
}

// Write starts a TX byte, acknowledges interrupt status, or changes IRQ enable.
func (p *Port) Write(off uint32, size bus.Size, value uint32) {
	if size != bus.Word {
		return
	}
	switch off {
	case 0x00:
		p.control = value & 0xff
	case 0x30:
		p.pair30 = value
	case 0x40:
		p.pair40 = value
	case 0x50:
		p.txDelay = ByteInstructions
		p.accept(byte(value & 0xff))
	case 0x60:
		p.status = 0
	case 0x70:
		p.enable = value & 7
	}
}

// Pump advances the oracle's byte delays by one device batch.
func (p *Port) Pump(batch uint64) {
	if p.txDelay > 0 {
		if p.txDelay > batch {
			p.txDelay -= batch
		} else {
			p.txDelay = 0
			p.status |= 2
		}
	}
	if len(p.reply) > 0 && p.status&4 == 0 && p.enable&2 == 0 {
		if p.rxDelay > 0 {
			if p.rxDelay > batch {
				p.rxDelay -= batch
			} else {
				p.rxDelay = 0
			}
		} else {
			p.rxByte, p.reply = p.reply[0], p.reply[1:]
			p.status |= 4
			p.rxDelay = ReplyGapInstructions
		}
	}
	p.updateLine()
}

func (p *Port) updateLine() {
	if p.interrupt != nil {
		p.interrupt.SetLine(IRQMask, p.status&p.enable != 0)
	}
}

func (p *Port) accept(b byte) {
	if len(p.frame) == 0 && b != 0x60 {
		return
	}
	if len(p.frame) >= maxFrameBytes {
		p.frame = nil
		return
	}
	p.frame = append(p.frame, b)
	if len(p.frame) < 3 {
		return
	}
	need := int(p.frame[1])<<8 | int(p.frame[2])
	need += 5
	if need > maxFrameBytes {
		p.frame = nil
		return
	}
	if len(p.frame) != need {
		return
	}
	command := p.frame[3]
	p.frame = nil
	reply := []byte{0xe0, 0, 1, command, 0xc0}
	var checksum byte
	for _, v := range reply {
		checksum ^= v
	}
	if len(p.reply)+len(reply)+1 > maxFrameBytes {
		return
	}
	p.reply = append(p.reply, reply...)
	p.reply = append(p.reply, checksum)
	p.rxDelay = 2 * ReplyGapInstructions
}

// Reset clears all volatile controller state; the modelled slot stays empty.
func (p *Port) Reset() {
	p.control, p.pair30, p.pair40, p.status, p.enable, p.rxByte = 0, 0, 0, 0, 0, 0
	p.frame, p.reply, p.txDelay, p.rxDelay = nil, nil, 0, 0
	p.updateLine()
}

// Snapshot captures the complete byte-level link state.
func (p *Port) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(p.Name(), 2)
	w.Words([]uint32{p.control, p.pair30, p.pair40, p.status, p.enable})
	w.Uint8(p.rxByte)
	w.Bytes(p.frame)
	w.Bytes(p.reply)
	w.Uint64(p.txDelay)
	w.Uint64(p.rxDelay)
	return w.Blob()
}

// Restore validates complete state before replacing the current link.
func (p *Port) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("smartcard: restore: %w", err)
	}
	if err := r.Expect(p.Name(), 2, 2); err != nil {
		return fmt.Errorf("smartcard: restore: %w", err)
	}
	regs, rxByte := r.Words(), r.Uint8()
	frame, reply := r.Bytes(), r.Bytes()
	txDelay, rxDelay := r.Uint64(), r.Uint64()
	if err := r.Done(); err != nil {
		return fmt.Errorf("smartcard: restore: %w", err)
	}
	if len(regs) != 5 || len(frame) > maxFrameBytes || len(reply) > maxFrameBytes {
		return fmt.Errorf("smartcard: restore: invalid state")
	}
	p.control, p.pair30, p.pair40, p.status, p.enable = regs[0], regs[1], regs[2], regs[3], regs[4]
	p.rxByte, p.frame, p.reply, p.txDelay, p.rxDelay = rxByte, frame, reply, txDelay, rxDelay
	p.updateLine()
	return nil
}
