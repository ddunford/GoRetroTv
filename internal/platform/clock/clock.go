// Package clock is the machine's only source of time: the instruction counter.
//
// Every timer, device pump and carousel wave in this emulator is scheduled here, in instructions,
// and nothing anywhere consults the wall. That is a correctness requirement rather than a
// preference. The predecessor scheduled off wall time, and the consequence was that two runs of the
// same firmware produced different event counts, which sent a session hunting a device bug that did
// not exist. Replay, the oracle comparison and the snapshot round-trip all depend on the same
// program producing the same schedule on a loaded machine as on an idle one.
//
// So: no time.Time, no time.Timer, no goroutine. Advance is driven by the instruction loop, and an
// event fires when the counter reaches its due instruction. Ties between events due at the same
// instruction break by registration order, because "whichever the map iterated first" is how a
// scheduler stops being deterministic without anyone editing it.
//
// A Clock is not safe for concurrent use, deliberately: the instruction loop is single-threaded by
// construction (CLAUDE.md -> No goroutine in the instruction loop) and a mutex here would suggest
// otherwise.
//
// Typical use from the instruction loop, which must not pay a heap check per instruction:
//
//	for {
//		budget := clk.Budget(maxBatch)   // instructions until the next event is due
//		ran := cpu.Step(budget)
//		if err := clk.Advance(ran); err != nil {
//			return err
//		}
//	}
package clock

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
)

// Errors returned when a caller asks for a schedule that cannot be honoured. Each one is a bug at
// the call site rather than a runtime condition, so they name the numbers involved.
var (
	// ErrPastDue means an event was scheduled at or before the current instruction. At is
	// rejected rather than fired immediately: a device pump that reschedules itself with a delta
	// of zero would otherwise spin for ever inside one Advance, which looks like a hang rather
	// than like the mistake it is.
	ErrPastDue = errors.New("event is due at or before the current instruction")
	// ErrZeroPeriod means a periodic event was given a period of zero, which is the same spin.
	ErrZeroPeriod = errors.New("periodic event needs a period of at least one instruction")
	// ErrCounterOverflow means the instruction counter would wrap. At one billion instructions a
	// second this is several hundred years away, and it is checked because a silently wrapped
	// counter would make every event in the queue permanently overdue.
	ErrCounterOverflow = errors.New("instruction counter would overflow")
)

// EventID identifies a scheduled event so it can be cancelled.
type EventID uint64

// Handler runs when an event comes due. now is the instruction at which it fired, which is the
// event's due instruction and not wherever the surrounding Advance was heading.
type Handler func(now uint64) error

type event struct {
	id     EventID
	name   string
	due    uint64
	period uint64 // zero for a one-shot
	seq    uint64 // registration order, for deterministic tie-breaking
	fn     Handler
	index  int // position in the heap, maintained by container/heap
}

// Clock counts instructions and fires what is due.
type Clock struct {
	icount  uint64
	nextDue uint64
	queue   eventQueue
	live    map[EventID]*event
	nextID  EventID
	nextSeq uint64
}

// New returns a clock at instruction zero with nothing scheduled.
func New() *Clock {
	return &Clock{nextDue: math.MaxUint64, live: make(map[EventID]*event)}
}

// Now reports the current instruction count.
func (c *Clock) Now() uint64 { return c.icount }

// Pending reports how many events are scheduled.
func (c *Clock) Pending() int { return len(c.live) }

// At schedules fn to run when the counter reaches due.
//
// name appears in errors and in Pending listings; give it something a reader of a failing gate can
// act on, like "demux section pump" rather than "timer".
func (c *Clock) At(due uint64, name string, fn Handler) (EventID, error) {
	if fn == nil {
		return 0, fmt.Errorf("clock: %s: handler is nil", name)
	}
	if due <= c.icount {
		return 0, fmt.Errorf("clock: %s: %w: due %d, now %d", name, ErrPastDue, due, c.icount)
	}
	return c.schedule(name, due, 0, fn), nil
}

// After schedules fn to run delta instructions from now. delta must be at least one.
func (c *Clock) After(delta uint64, name string, fn Handler) (EventID, error) {
	if delta == 0 {
		return 0, fmt.Errorf("clock: %s: %w: delta 0, now %d", name, ErrPastDue, c.icount)
	}
	due, err := add(c.icount, delta)
	if err != nil {
		return 0, fmt.Errorf("clock: %s: %w", name, err)
	}
	return c.At(due, name, fn)
}

// Every schedules fn to run every period instructions, starting period instructions from now.
//
// The event reschedules from its own due instruction rather than from the moment it ran, so a
// pump's cadence cannot drift with the size of the batches the instruction loop happens to run.
func (c *Clock) Every(period uint64, name string, fn Handler) (EventID, error) {
	if period == 0 {
		return 0, fmt.Errorf("clock: %s: %w", name, ErrZeroPeriod)
	}
	if fn == nil {
		return 0, fmt.Errorf("clock: %s: handler is nil", name)
	}
	due, err := add(c.icount, period)
	if err != nil {
		return 0, fmt.Errorf("clock: %s: %w", name, err)
	}
	return c.schedule(name, due, period, fn), nil
}

// Cancel removes a scheduled event and reports whether it was still scheduled.
//
// Calling it from inside a handler is safe, including on the event currently running: a periodic
// event cancelled by its own handler does not reschedule.
func (c *Clock) Cancel(id EventID) bool {
	ev, ok := c.live[id]
	if !ok {
		return false
	}
	delete(c.live, id)
	if ev.index >= 0 {
		heap.Remove(&c.queue, ev.index)
		ev.index = -1
		c.refreshNextDue()
	}
	return true
}

// NextDue reports the instruction at which the next event fires.
func (c *Clock) NextDue() (uint64, bool) {
	if c.queue.Len() == 0 {
		return 0, false
	}
	return c.nextDue, true
}

// Budget reports how many instructions may be run before the next event is due, capped at max.
//
// This is what keeps the instruction loop off the heap: run the budget, then Advance by however
// many instructions actually executed. With nothing scheduled it returns max.
func (c *Clock) Budget(max uint64) uint64 {
	if max == 0 {
		return 0
	}
	due, ok := c.NextDue()
	if !ok {
		return max
	}
	// due is always strictly greater than icount: At refuses anything else, and Advance fires
	// everything that has come due before it returns.
	if gap := due - c.icount; gap < max {
		return gap
	}
	return max
}

// Tick advances exactly one instruction. The instruction loop uses this instead of Advance(1):
// while no event is due it is only an overflow check, an increment and a cached-deadline compare.
// Keeping the heap out of this path matters because it runs once or twice for every guest
// instruction the emulator retires.
func (c *Clock) Tick() error {
	if c.icount == ^uint64(0) {
		return ErrCounterOverflow
	}
	c.icount++
	if c.icount != c.nextDue {
		return nil
	}
	return c.fireTick()
}

//go:noinline
func (c *Clock) fireTick() error { return c.fireDue(c.icount) }

// Advance moves the counter on by n instructions, firing everything that comes due on the way.
//
// Events fire in due order, and the counter reads as the event's own due instruction while its
// handler runs — so a handler logging Now reports where the machine was, not where this Advance was
// heading. A handler may schedule and cancel freely; anything it schedules within the remaining
// window fires in this same call.
//
// A handler error stops the advance and is returned wrapped with the event's name. The counter is
// left at the instruction where it failed rather than at the target, because that is the honest
// answer to "how far did the machine get".
func (c *Clock) Advance(n uint64) error {
	target, err := add(c.icount, n)
	if err != nil {
		return fmt.Errorf("clock: advance by %d from %d: %w", n, c.icount, err)
	}

	return c.fireDue(target)
}

func (c *Clock) fireDue(target uint64) error {
	for c.queue.Len() > 0 && c.nextDue <= target {
		ev := heap.Pop(&c.queue).(*event)
		ev.index = -1
		c.icount = ev.due

		if ev.period > 0 {
			// Reschedule before running, so the handler can cancel it or reschedule around it
			// and have that stick. add cannot overflow here without target having done so first.
			next, err := add(ev.due, ev.period)
			if err != nil {
				return fmt.Errorf("clock: %s: %w", ev.name, err)
			}
			ev.due = next
			heap.Push(&c.queue, ev)
		} else {
			delete(c.live, ev.id)
		}
		c.refreshNextDue()

		if err := ev.fn(c.icount); err != nil {
			return fmt.Errorf("clock: %s at instruction %d: %w", ev.name, c.icount, err)
		}
	}

	c.icount = target
	return nil
}

// RestoreTo puts the counter back to a snapshotted instruction and clears the schedule.
//
// Clearing is not a shortcut, it is the only correct answer: an event is a closure over a device's
// live state and cannot be serialised, so a restored clock holding the queue from before the
// restore would fire handlers bound to the wrong objects. Every device re-registers its own pumps
// as part of its Restore, which is also what makes a missing registration show up as a device that
// never ticks rather than as one that ticks against stale state.
func (c *Clock) RestoreTo(icount uint64) {
	c.queue = nil
	c.live = make(map[EventID]*event)
	c.icount = icount
	c.nextDue = math.MaxUint64
}

// Reset returns the clock to instruction zero with nothing scheduled.
func (c *Clock) Reset() { c.RestoreTo(0) }

func (c *Clock) schedule(name string, due, period uint64, fn Handler) EventID {
	c.nextID++
	ev := &event{
		id:     c.nextID,
		name:   name,
		due:    due,
		period: period,
		seq:    c.nextSeq,
		fn:     fn,
	}
	c.nextSeq++
	c.live[ev.id] = ev
	heap.Push(&c.queue, ev)
	c.refreshNextDue()
	return ev.id
}

func (c *Clock) refreshNextDue() {
	c.nextDue = math.MaxUint64
	if c.queue.Len() > 0 {
		c.nextDue = c.queue[0].due
	}
}

func add(a, b uint64) (uint64, error) {
	sum := a + b
	if sum < a {
		return 0, fmt.Errorf("%w: %d + %d", ErrCounterOverflow, a, b)
	}
	return sum, nil
}

// eventQueue is a min-heap ordered by due instruction, then by registration order.
//
// The second key is what makes the schedule deterministic: without it, two events due at the same
// instruction fire in whatever order the heap's internal swaps happened to leave them, and the
// order changes with unrelated scheduling elsewhere.
type eventQueue []*event

func (q eventQueue) Len() int { return len(q) }

func (q eventQueue) Less(i, j int) bool {
	if q[i].due != q[j].due {
		return q[i].due < q[j].due
	}
	return q[i].seq < q[j].seq
}

func (q eventQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *eventQueue) Push(x any) {
	ev, ok := x.(*event)
	if !ok {
		// Unreachable: this heap is private to the package and only ever holds *event. The
		// branch exists so the type assertion cannot panic in library code.
		return
	}
	ev.index = len(*q)
	*q = append(*q, ev)
}

func (q *eventQueue) Pop() any {
	old := *q
	n := len(old)
	ev := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return ev
}
