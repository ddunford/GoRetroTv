// Package csi models the front-panel microcontroller's synchronous serial link.
package csi

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	// Base is the link's uncached MMIO address.
	Base = 0xB2009000
	// Size is the link's mapped register window.
	Size = 0x100
	// IRQMask selects the CSI dispatcher in the board interrupt controller.
	IRQMask = 0x2000
	// TrafficInstructions is the measured instruction interval for active bytes.
	TrafficInstructions = 2100
	// MaxQueuedBytes bounds both wire queues and trace logs.
	MaxQueuedBytes = 4096
	// IdleInstructions prevents empty wire clocking from starving the guest.
	IdleInstructions = 320000
	// PowerupTicks is the measured board timer delay before the reader starts.
	PowerupTicks = 120
)

// Link is a one-byte receive register with a paced incoming wire. The card
// model can enqueue reply frames through Queue; Key constructs an unsolicited
// type-2 handset frame. Pump is called by the instruction loop.
type Link struct {
	control, interruptEnable  uint32
	data                      uint8
	ready                     bool
	queue                     []byte
	reply                     []byte
	transmitted               []byte
	lastByteAt                uint64
	now                       uint64
	powerAtTick, timerTicks   uint64
	cardLive, boxBusy, txSeen bool
	frame                     []byte
	escaped                   bool
	ack                       [4]uint64
	interrupt                 *irq.Controller
}

// New binds the link to the board interrupt controller.
func New(interrupt *irq.Controller) *Link {
	l := &Link{interrupt: interrupt}
	l.SetAckPolicy([]uint8{0x52, 0x18})
	return l
}

// Name is the snapshot identity for this device.
func (*Link) Name() string { return "csi-link" }

func (l *Link) Read(off uint32, size bus.Size) uint32 {
	if size != bus.Word {
		return 0
	}
	switch off {
	case 0x00:
		return l.control
	case 0x10:
		return uint32(l.data)
	case 0x20:
		if l.ready {
			return 1
		}
		return 0
	case 0x30:
		return l.interruptEnable
	default:
		return 0
	}
}

func (l *Link) Write(off uint32, size bus.Size, value uint32) {
	if size != bus.Word {
		return
	}
	switch off {
	case 0x00:
		l.control = value
	case 0x10:
		if l.enabled() && len(l.transmitted) < MaxQueuedBytes {
			l.transmitted = append(l.transmitted, byte(value))
		}
		if l.enabled() {
			l.boxBusy = byte(value) != 0
			l.txSeen = true
			l.accept(byte(value))
		}
	case 0x20:
		if value&1 == 0 {
			l.ready = false
			l.updateLine()
		}
	case 0x30:
		l.interruptEnable = value
		if value&1 != 0 {
			l.powerAtTick = l.timerTicks + PowerupTicks
			l.cardLive = false
		}
		l.updateLine()
	}
}

func (l *Link) enabled() bool { return l.control&0x80 != 0 && l.interruptEnable&1 != 0 }
func (l *Link) updateLine() {
	if l.interrupt != nil {
		l.interrupt.SetLine(IRQMask, l.ready && l.interruptEnable&1 != 0)
	}
}

// Queue appends encoded wire bytes. It refuses an unbounded host-side backlog.
func (l *Link) Queue(wire []byte) error {
	if len(wire) > MaxQueuedBytes-len(l.queue) {
		return fmt.Errorf("csi: receive queue exceeds %d bytes", MaxQueuedBytes)
	}
	l.queue = append(l.queue, wire...)
	return nil
}

// Key sends the exact six-byte unsolicited type-2 payload used by the handset.
// Source 2 is rejected by guest firmware, but remains representable here so the
// hardware path can be tested without changing the guest's decision.
func (l *Link) Key(raw, source uint8) error {
	if source > 3 {
		return fmt.Errorf("csi: key source %d out of range", source)
	}
	payload := []byte{5, 0x80, 2, 0, source<<4 | raw>>4, raw << 4}
	return l.Queue(Encode(payload))
}

// Encode applies the firmware's 0x1B escaping and zero terminator.
func Encode(payload []byte) []byte {
	wire := make([]byte, 0, len(payload)*2+1)
	for _, b := range payload {
		if b == 0 || b == 0x1b {
			wire = append(wire, 0x1b)
		}
		wire = append(wire, b)
	}
	return append(wire, 0)
}

// Pump presents at most one byte per link interval. An acknowledged register
// cannot be overwritten by a queued byte before the next instruction interval.
func (l *Link) Pump(now, boardTicks uint64) {
	l.now = now
	l.timerTicks = boardTicks
	if l.powerAtTick != 0 && !l.cardLive && boardTicks >= l.powerAtTick {
		l.cardLive = true
	}
	if !l.enabled() || l.ready || now < l.lastByteAt {
		return
	}
	switch {
	case len(l.reply) > 0:
		if !l.cardLive || !l.txSeen || now-l.lastByteAt < TrafficInstructions {
			return
		}
		l.data, l.reply = l.reply[0], l.reply[1:]
		l.txSeen = false
	case len(l.queue) > 0:
		if now-l.lastByteAt < TrafficInstructions {
			return
		}
		l.data, l.queue = l.queue[0], l.queue[1:]
	default:
		period := uint64(IdleInstructions)
		if l.boxBusy {
			period = TrafficInstructions
		}
		if !l.cardLive || now-l.lastByteAt < period {
			return
		}
		l.data = 0
	}
	l.ready, l.lastByteAt = true, now
	l.updateLine()
}

// Pending reports bytes waiting to enter the guest receive register.
func (l *Link) Pending() int { return len(l.queue) }

// Transmitted returns the bounded log of guest transmit bytes.
func (l *Link) Transmitted() []byte { return append([]byte(nil), l.transmitted...) }

// Reset clears link registers, queues and instruction timing state.
func (l *Link) Reset() {
	l.control, l.interruptEnable, l.data, l.ready = 0, 0, 0, false
	l.queue, l.transmitted, l.lastByteAt, l.now = nil, nil, 0, 0
	l.reply, l.frame, l.powerAtTick, l.timerTicks = nil, nil, 0, 0
	l.cardLive, l.boxBusy, l.txSeen, l.escaped = false, false, false, false
	l.SetAckPolicy([]uint8{0x52, 0x18})
	l.updateLine()
}

// Snapshot captures all device-owned register, queue and timing state.
func (l *Link) Snapshot() ([]byte, error) {
	w := snapcodec.NewWriter(l.Name(), 3)
	w.Words([]uint32{l.control, l.interruptEnable})
	w.Uint8(l.data)
	w.Bool(l.ready)
	w.Bytes(l.queue)
	w.Bytes(l.reply)
	w.Bytes(l.transmitted)
	w.Uint64(l.lastByteAt)
	w.Uint64(l.now)
	w.Uint64(l.powerAtTick)
	w.Uint64(l.timerTicks)
	w.Bool(l.cardLive)
	w.Bool(l.boxBusy)
	w.Bool(l.txSeen)
	w.Bool(l.escaped)
	w.Bytes(l.frame)
	for _, bits := range l.ack {
		w.Uint64(bits)
	}
	return w.Blob()
}

// Restore validates the complete blob before replacing link state.
func (l *Link) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("csi: restore: %w", err)
	}
	if err := r.Expect(l.Name(), 3, 3); err != nil {
		return fmt.Errorf("csi: restore: %w", err)
	}
	regs, data, ready := r.Words(), r.Uint8(), r.Bool()
	queue, reply, transmitted := r.Bytes(), r.Bytes(), r.Bytes()
	lastByteAt, now, powerAtTick, timerTicks := r.Uint64(), r.Uint64(), r.Uint64(), r.Uint64()
	cardLive, boxBusy, txSeen, escaped := r.Bool(), r.Bool(), r.Bool(), r.Bool()
	frame := r.Bytes()
	var ack [4]uint64
	for i := range ack {
		ack[i] = r.Uint64()
	}
	if err := r.Done(); err != nil {
		return fmt.Errorf("csi: restore: %w", err)
	}
	if len(regs) != 2 || len(queue) > MaxQueuedBytes || len(reply) > MaxQueuedBytes || len(transmitted) > MaxQueuedBytes || len(frame) > 32 {
		return fmt.Errorf("csi: restore: invalid state")
	}
	l.control, l.interruptEnable, l.data, l.ready = regs[0], regs[1], data, ready
	l.queue, l.reply, l.transmitted = queue, reply, transmitted
	l.lastByteAt, l.now, l.powerAtTick, l.timerTicks = lastByteAt, now, powerAtTick, timerTicks
	l.cardLive, l.boxBusy, l.txSeen, l.escaped = cardLive, boxBusy, txSeen, escaped
	l.frame, l.ack = frame, ack
	l.updateLine()
	return nil
}
