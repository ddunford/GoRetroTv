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

// DefaultAckPolicy is the set of command codes the modelled front-panel micro answers.
//
// EVERY CODE HERE IS ON THE LIST FOR A MEASURED REASON, and the two that were added last were
// added because their SILENCE WEDGED THE BOX.
//
//	0x52  card status. The record has the handler at 0x8002A534 reading frame[3], masking it to
//	      three bits and releasing the Periph semaphore unconditionally, so the box genuinely
//	      waits on this one and any value 0-7 satisfies it.
//	0x18  the periodic heartbeat, reached from the descriptor at flash 0x9FC237A0.
//	0x41,
//	0x42  THE TWO THE BOX SENDS ON EVERY KEY PRESS. Left unanswered, SMTTask waits out its
//	      fifty-tick timeout for each one, the event queue EVQP0002 that SMTTask alone drains
//	      backs up, and after eight presses it is full with three tasks suspended on it and the
//	      box stops responding to the handset entirely (gort-slq).
//
// THE LAST TWO ARE MEASURED BOTH WAYS. Across the eight presses that fill the pipe the box sends
// exactly three codes -- 0x18 three times, 0x41 six times, 0x42 six times -- so these are not
// candidates picked from the boot capture, they are the whole of what it asks for while it fails.
// And they were swept one at a time, because two changes at once cannot say which mattered:
//
//	0x52 and 0x18 only       pipe FULL after  8 presses
//	and 0x41                 pipe FULL after 10 presses
//	and 0x42                 pipe FULL after 10 presses
//	and both                 never full in 60, and never more than 4 of its 20 messages deep
//	every code at all        identical to "and both"
//
// Either alone barely moves it; together they recover the ENTIRE effect of answering every code,
// which is why the policy stops here rather than widening to the codes the box sends only at boot.
// Answering a code a real card would refuse makes a machine that is plausibly wrong rather than
// visibly broken, so the list is what the box demonstrably needs and nothing more.
// Arms: internal/multiplex/firmwaretests/cardsilence_firmware_test.go.
var DefaultAckPolicy = []uint8{0x52, 0x18, 0x41, 0x42}

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
	// inFrame and sendingReply say that a frame has begun leaving for the guest and which queue it
	// came from. THE SOURCE MUST NOT CHANGE UNTIL THE TERMINATOR HAS GONE: card replies and
	// handset frames share this wire and share its framing, so a frame interrupted half way
	// through is not a delayed frame, it is two corrupt ones, and the guest's de-framer cannot
	// tell. See Pump.
	inFrame, sendingReply bool
	ack                   [4]uint64
	interrupt             *irq.Controller
}

// New binds the link to the board interrupt controller.
func New(interrupt *irq.Controller) *Link {
	l := &Link{interrupt: interrupt}
	l.SetAckPolicy(DefaultAckPolicy)
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
		transmitted := byte(value & 0xff)
		if l.enabled() && len(l.transmitted) < MaxQueuedBytes {
			l.transmitted = append(l.transmitted, transmitted)
		}
		if l.enabled() {
			l.boxBusy = transmitted != 0
			l.txSeen = true
			l.accept(transmitted)
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
	// WHICH QUEUE THE NEXT BYTE COMES FROM, AND IT DOES NOT CHANGE MID-FRAME.
	//
	// Card replies used to take priority unconditionally, so a reply generated while a handset
	// frame was part-way out cut straight into it -- the guest received
	// `05 80 02 1b 00 07 | 02 01 41 00 | d0 00` and de-framed two corrupt messages instead of a
	// key and an acknowledgement. It went unnoticed for as long as the modelled card answered
	// almost nothing and therefore almost never had anything to say at the wrong moment.
	//
	// A frame whose source has run dry can only come from a caller queueing a partial one, and
	// stalling the link for ever would be worse than the splice; that case starts afresh.
	const (
		nothing = iota
		fromReply
		fromQueue
	)
	source := nothing
	switch {
	case l.inFrame && l.sendingReply && len(l.reply) > 0:
		source = fromReply
	case l.inFrame && !l.sendingReply && len(l.queue) > 0:
		source = fromQueue
	case len(l.reply) > 0:
		source = fromReply
	case len(l.queue) > 0:
		source = fromQueue
	}
	switch source {
	case fromReply:
		if !l.cardLive || !l.txSeen || now-l.lastByteAt < TrafficInstructions {
			return
		}
		l.data, l.reply = l.reply[0], l.reply[1:]
		l.txSeen, l.sendingReply, l.inFrame = false, true, l.data != 0
	case fromQueue:
		if !l.cardLive || !l.txSeen || now-l.lastByteAt < TrafficInstructions {
			return
		}
		l.data, l.queue = l.queue[0], l.queue[1:]
		l.txSeen, l.sendingReply, l.inFrame = false, false, l.data != 0
	default:
		period := uint64(IdleInstructions)
		if l.boxBusy {
			period = TrafficInstructions
		}
		if !l.cardLive || now-l.lastByteAt < period {
			return
		}
		l.data, l.inFrame = 0, false
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
	l.inFrame, l.sendingReply = false, false
	l.SetAckPolicy(DefaultAckPolicy)
	l.updateLine()
}

// Snapshot captures all device-owned register, queue and timing state.
func (l *Link) Snapshot() ([]byte, error) {
	// v4 DROPPED THE ACK POLICY, which is model configuration rather than machine state -- see
	// Restore. v3 blobs still load; they simply carry four words this build discards.
	w := snapcodec.NewWriter(l.Name(), 4)
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
	w.Bool(l.inFrame)
	w.Bool(l.sendingReply)
	return w.Blob()
}

// Restore validates the complete blob before replacing link state.
func (l *Link) Restore(blob []byte) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("csi: restore: %w", err)
	}
	if err := r.Expect(l.Name(), 3, 4); err != nil {
		return fmt.Errorf("csi: restore: %w", err)
	}
	regs, data, ready := r.Words(), r.Uint8(), r.Bool()
	queue, reply, transmitted := r.Bytes(), r.Bytes(), r.Bytes()
	lastByteAt, now, powerAtTick, timerTicks := r.Uint64(), r.Uint64(), r.Uint64(), r.Uint64()
	cardLive, boxBusy, txSeen, escaped := r.Bool(), r.Bool(), r.Bool(), r.Bool()
	frame := r.Bytes()
	// v4 added the two flags that keep a frame from being cut in half; a v3 blob predates them and
	// restores to a link that is not mid-frame, which is what it was.
	var inFrame, sendingReply bool
	if r.Version() >= 4 {
		inFrame, sendingReply = r.Bool(), r.Bool()
	}
	// THE ACK POLICY IS NOT MACHINE STATE AND IS DELIBERATELY NOT RESTORED. It describes what the
	// modelled peripheral ANSWERS, which belongs to the model the way the link's baud rate does --
	// and the product restores a snapshot on every start, so a policy read back out of one would
	// pin whatever card behaviour was current when that snapshot was taken onto every later build.
	// The two codes added to fix gort-slq would have worked in every test that boots cold and
	// silently done nothing on the demo, which is the worst shape a fix can have.
	//
	// v3 blobs carry four words of it and are still accepted; they are consumed and dropped, so an
	// existing private snapshot keeps loading and only the policy comes from the build. A caller
	// that wants a diagnostic policy sets it after restoring, which is what the firmware tests do.
	if r.Version() == 3 {
		for i := 0; i < 4; i++ {
			r.Uint64()
		}
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
	l.frame = frame
	l.inFrame, l.sendingReply = inFrame, sendingReply
	l.SetAckPolicy(DefaultAckPolicy)
	l.updateLine()
	return nil
}
