package clock_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/clock"
	"github.com/ddunford/goretrotv/internal/platform/snapcodec"
)

func TestSnapshotRestoresDeadlinesIdentityAndTieOrder(t *testing.T) {
	t.Parallel()

	original := clock.New()
	noop := func(uint64) error { return nil }
	if _, err := original.Every(10, "periodic", noop); err != nil {
		t.Fatal(err)
	}
	if err := original.Advance(7); err != nil {
		t.Fatal(err)
	}
	firstID, err := original.At(10, "first", noop)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.At(10, "second", noop); err != nil {
		t.Fatal(err)
	}
	cancelled, err := original.At(11, "cancelled", noop)
	if err != nil {
		t.Fatal(err)
	}
	if !original.Cancel(cancelled) {
		t.Fatal("failed to cancel setup event")
	}

	blob, err := original.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var fired []firing
	record := func(name string) clock.Handler {
		return func(now uint64) error {
			fired = append(fired, firing{name, now})
			return nil
		}
	}
	restored := clock.New()
	if err := restored.Restore(blob, map[string]clock.Handler{
		"periodic": record("periodic"), "first": record("first"), "second": record("second"),
	}); err != nil {
		t.Fatal(err)
	}
	if restored.Now() != 7 || restored.Pending() != 3 || restored.Budget(100) != 3 {
		t.Fatalf("restored state: now=%d pending=%d budget=%d", restored.Now(), restored.Pending(), restored.Budget(100))
	}
	if again, err := restored.Snapshot(); err != nil || !bytes.Equal(blob, again) {
		t.Fatalf("snapshot bytes changed after restore: err=%v", err)
	}
	if !restored.Cancel(firstID) {
		t.Fatal("restored event ID does not identify its event")
	}
	// Rebind that event from the same blob to check all three registration-order ties.
	if err := restored.Restore(blob, map[string]clock.Handler{
		"periodic": record("periodic"), "first": record("first"), "second": record("second"),
	}); err != nil {
		t.Fatal(err)
	}
	newID, err := restored.At(12, "later", record("later"))
	if err != nil || newID <= cancelled {
		t.Fatalf("new event ID = %d, err=%v; should follow prior IDs through cancellation", newID, err)
	}
	if err := restored.Advance(23); err != nil {
		t.Fatal(err)
	}
	want := []firing{{"periodic", 10}, {"first", 10}, {"second", 10}, {"later", 12}, {"periodic", 20}, {"periodic", 30}}
	if !reflect.DeepEqual(fired, want) {
		t.Fatalf("firings = %v, want %v", fired, want)
	}
}

func TestRestoreRejectsIncompleteOrCorruptScheduleWithoutMutation(t *testing.T) {
	t.Parallel()
	type pending struct {
		id, due, period, seq uint64
		name                 string
	}
	encode := func(events []pending, count uint32, trailing bool) []byte {
		w := snapcodec.NewWriter("clock", 1)
		w.Uint64(5) // icount
		w.Uint64(3) // next ID
		w.Uint64(3) // next sequence
		w.Uint32(count)
		for _, ev := range events {
			w.Uint64(ev.id)
			w.String(ev.name)
			w.Uint64(ev.due)
			w.Uint64(ev.period)
			w.Uint64(ev.seq)
		}
		if trailing {
			w.Uint8(99)
		}
		blob, err := w.Blob()
		if err != nil {
			t.Fatal(err)
		}
		return blob
	}
	base := pending{1, 10, 0, 0, "pump"}
	cases := []struct {
		name, want string
		blob       []byte
		handlers   map[string]clock.Handler
	}{
		{"missing handler", "no handler", encode([]pending{base}, 1, false), nil},
		{"truncated event", "exceeds blob capacity", encode(nil, 1, false), map[string]clock.Handler{"pump": func(uint64) error { return nil }}},
		{"duplicate ID", "duplicate ID", encode([]pending{base, {1, 12, 0, 1, "pump"}}, 2, false), map[string]clock.Handler{"pump": func(uint64) error { return nil }}},
		{"past deadline", "due at or before", encode([]pending{{1, 5, 0, 0, "pump"}}, 1, false), map[string]clock.Handler{"pump": func(uint64) error { return nil }}},
		{"wrong sequence order", "registration sequence", encode([]pending{{1, 10, 0, 1, "pump"}, {2, 12, 0, 0, "pump"}}, 2, false), map[string]clock.Handler{"pump": func(uint64) error { return nil }}},
		{"trailing data", "fully consumed", encode([]pending{base}, 1, true), map[string]clock.Handler{"pump": func(uint64) error { return nil }}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := clock.New()
			fired := 0
			if _, err := c.At(8, "existing", func(uint64) error { fired++; return nil }); err != nil {
				t.Fatal(err)
			}
			before, err := c.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Restore(tc.blob, tc.handlers); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Restore error = %v, want %q", err, tc.want)
			}
			after, err := c.Snapshot()
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("failed Restore mutated clock: err=%v", err)
			}
			if err := c.Advance(8); err != nil || fired != 1 {
				t.Fatalf("old handler not intact: fired=%d err=%v", fired, err)
			}
		})
	}
}
