package clock

import (
	"container/heap"
	"fmt"
	"sort"

	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

const (
	snapshotName    = "clock"
	snapshotVersion = 1
	// Four uint64 fields and one uint32 string length are the shortest possible event.
	minSnapshotEventBytes = 36
	// snapcodec's fixed header, clock writer name, and four clock-level fields.
	snapshotBaseBytes = 14 + len(snapshotName) + 8 + 8 + 8 + 4
)

// Snapshot records the instruction count and the complete pending schedule. Handlers are
// functions and cannot be encoded; Restore resolves each event name against its handler map.
// Events are written in registration order so equivalent schedules have identical blobs.
func (c *Clock) Snapshot() ([]byte, error) {
	events := make([]*event, 0, len(c.live))
	for _, ev := range c.live {
		events = append(events, ev)
	}
	sort.Slice(events, func(i, j int) bool { return events[i].seq < events[j].seq })
	if uint64(len(events)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("clock: too many events to snapshot: %d", len(events))
	}
	for _, ev := range events {
		if ev.name == "" {
			return nil, fmt.Errorf("clock: event %d has an empty handler name", ev.id)
		}
	}
	w := snapcodec.NewWriter(snapshotName, snapshotVersion)
	w.Uint64(c.icount)
	w.Uint64(uint64(c.nextID))
	w.Uint64(c.nextSeq)
	w.Uint32(uint32(len(events))) // #nosec G115 -- bounded by MaxUint32 above.
	for _, ev := range events {
		w.Uint64(uint64(ev.id))
		w.String(ev.name)
		w.Uint64(ev.due)
		w.Uint64(ev.period)
		w.Uint64(ev.seq)
	}
	return w.Blob()
}

// Restore validates a complete snapshot before changing the clock. Every pending event name must
// resolve to a non-nil handler; the caller rebinds closures to the restored devices. Names may
// repeat when a device has several events handled by the same function.
func (c *Clock) Restore(blob []byte, handlers map[string]Handler) error {
	r, err := snapcodec.Open(blob)
	if err != nil {
		return fmt.Errorf("clock: restore: %w", err)
	}
	if err := r.Expect(snapshotName, snapshotVersion, snapshotVersion); err != nil {
		return fmt.Errorf("clock: restore: %w", err)
	}

	icount := r.Uint64()
	nextID := EventID(r.Uint64())
	nextSeq := r.Uint64()
	count := r.Uint32()
	if err := r.Err(); err != nil {
		return fmt.Errorf("clock: restore: %w", err)
	}
	if int64(count) > int64(len(blob)-snapshotBaseBytes)/minSnapshotEventBytes {
		return fmt.Errorf("clock: restore: event count %d exceeds blob capacity", count)
	}

	queue := make(eventQueue, 0, count)
	live := make(map[EventID]*event, count)
	var previousSeq uint64
	for i := uint32(0); i < count; i++ {
		id := EventID(r.Uint64())
		name := r.String()
		due := r.Uint64()
		period := r.Uint64()
		seq := r.Uint64()
		if err := r.Err(); err != nil {
			return fmt.Errorf("clock: restore event %d: %w", i, err)
		}
		if id == 0 || id > nextID {
			return fmt.Errorf("clock: restore event %d: invalid ID %d (next ID %d)", i, id, nextID)
		}
		if _, exists := live[id]; exists {
			return fmt.Errorf("clock: restore event %d: duplicate ID %d", i, id)
		}
		if seq >= nextSeq || (i > 0 && seq <= previousSeq) {
			return fmt.Errorf("clock: restore event %d: invalid registration sequence %d", i, seq)
		}
		if due <= icount {
			return fmt.Errorf("clock: restore event %d (%s): %w: due %d, now %d", i, name, ErrPastDue, due, icount)
		}
		if name == "" {
			return fmt.Errorf("clock: restore event %d: empty handler name", i)
		}
		fn := handlers[name]
		if fn == nil {
			return fmt.Errorf("clock: restore event %d: no handler for %q", i, name)
		}
		ev := &event{id: id, name: name, due: due, period: period, seq: seq, fn: fn}
		live[id] = ev
		queue = append(queue, ev)
		previousSeq = seq
	}
	if err := r.Done(); err != nil {
		return fmt.Errorf("clock: restore: %w", err)
	}
	heap.Init(&queue)
	c.icount, c.nextID, c.nextSeq, c.queue, c.live = icount, nextID, nextSeq, queue, live
	c.refreshNextDue()
	return nil
}
