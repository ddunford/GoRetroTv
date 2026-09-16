package clock_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/platform/clock"
)

// firing records what fired and when, which is the only thing this package promises.
type firing struct {
	name string
	at   uint64
}

func TestOneShotFiresAtItsDueInstruction(t *testing.T) {
	t.Parallel()

	c := clock.New()
	var fired []firing
	if _, err := c.At(100, "vsync", func(now uint64) error {
		fired = append(fired, firing{"vsync", now})
		return nil
	}); err != nil {
		t.Fatalf("At: %v", err)
	}

	if err := c.Advance(99); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(fired) != 0 {
		t.Fatalf("fired early at %d: %v", c.Now(), fired)
	}

	if err := c.Advance(1); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if want := []firing{{"vsync", 100}}; !reflect.DeepEqual(fired, want) {
		t.Errorf("fired = %v, want %v", fired, want)
	}
	if c.Now() != 100 {
		t.Errorf("Now = %d, want 100", c.Now())
	}
	if c.Pending() != 0 {
		t.Errorf("Pending = %d, want 0: a one-shot must not linger", c.Pending())
	}
}

// TestTheHandlerSeesItsOwnDueInstruction is what makes a log line from inside a device honest. If
// the handler saw the end of the batch instead, every event in a 1,000-instruction advance would
// report the same time and the trace would be unreadable.
func TestTheHandlerSeesItsOwnDueInstruction(t *testing.T) {
	t.Parallel()

	c := clock.New()
	var seen []uint64
	for _, due := range []uint64{10, 20, 30} {
		if _, err := c.At(due, "pump", func(now uint64) error {
			seen = append(seen, now)
			return nil
		}); err != nil {
			t.Fatalf("At(%d): %v", due, err)
		}
	}

	if err := c.Advance(1000); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if want := []uint64{10, 20, 30}; !reflect.DeepEqual(seen, want) {
		t.Errorf("handlers saw %v, want %v", seen, want)
	}
	if c.Now() != 1000 {
		t.Errorf("Now = %d, want 1000", c.Now())
	}
}

func TestPeriodicEventsFireEveryPeriod(t *testing.T) {
	t.Parallel()

	c := clock.New()
	var seen []uint64
	if _, err := c.Every(250, "carousel wave", func(now uint64) error {
		seen = append(seen, now)
		return nil
	}); err != nil {
		t.Fatalf("Every: %v", err)
	}

	if err := c.Advance(1000); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if want := []uint64{250, 500, 750, 1000}; !reflect.DeepEqual(seen, want) {
		t.Errorf("fired at %v, want %v", seen, want)
	}
}

// TestPeriodDoesNotDriftWithBatchSize is the wall-clock bug rewritten in instructions: a pump that
// rescheduled from "when it ran" instead of "when it was due" would slip by however much the
// instruction loop happened to overshoot.
func TestPeriodDoesNotDriftWithBatchSize(t *testing.T) {
	t.Parallel()

	run := func(batch uint64) []uint64 {
		c := clock.New()
		var seen []uint64
		if _, err := c.Every(7, "pump", func(now uint64) error {
			seen = append(seen, now)
			return nil
		}); err != nil {
			t.Fatalf("Every: %v", err)
		}
		// Stop exactly at 70 however uneven the batches are, so a run that overshoots is not
		// mistaken for a run that drifted.
		for c.Now() < 70 {
			step := batch
			if remaining := 70 - c.Now(); remaining < step {
				step = remaining
			}
			if err := c.Advance(step); err != nil {
				t.Fatalf("Advance: %v", err)
			}
		}
		return seen
	}

	want := []uint64{7, 14, 21, 28, 35, 42, 49, 56, 63, 70}
	for _, batch := range []uint64{1, 3, 7, 13, 70} {
		if got := run(batch); !reflect.DeepEqual(got, want) {
			t.Errorf("batch %d fired at %v, want %v", batch, got, want)
		}
	}
}

// TestTheScheduleIsIdenticalOnEveryRun is TC-1.8's determinism clause and the reason this package
// exists rather than time.Timer. The sleep is there on purpose: real elapsed time differs wildly
// between the two runs, and the schedule must not notice.
func TestTheScheduleIsIdenticalOnEveryRun(t *testing.T) {
	t.Parallel()

	run := func(pause time.Duration) []firing {
		c := clock.New()
		var fired []firing
		record := func(name string) clock.Handler {
			return func(now uint64) error {
				fired = append(fired, firing{name, now})
				time.Sleep(pause)
				return nil
			}
		}
		// Three events colliding at instruction 60 and 120: the tie-break has to be the thing
		// that orders them, not map iteration or heap luck.
		if _, err := c.Every(60, "a", record("a")); err != nil {
			t.Fatalf("Every a: %v", err)
		}
		if _, err := c.Every(30, "b", record("b")); err != nil {
			t.Fatalf("Every b: %v", err)
		}
		if _, err := c.Every(20, "c", record("c")); err != nil {
			t.Fatalf("Every c: %v", err)
		}
		for i := 0; i < 13; i++ {
			if err := c.Advance(10); err != nil {
				t.Fatalf("Advance: %v", err)
			}
		}
		return fired
	}

	fast := run(0)
	slow := run(2 * time.Millisecond)
	if !reflect.DeepEqual(fast, slow) {
		t.Errorf("a loaded run scheduled differently from an idle one:\n fast: %v\n slow: %v", fast, slow)
	}
	if len(fast) == 0 {
		t.Fatal("nothing fired; the comparison above is vacuous")
	}
	// Registration order breaks the tie at 60: a, then b, then c.
	want := []firing{{"c", 20}, {"b", 30}, {"c", 40}, {"a", 60}, {"b", 60}, {"c", 60}}
	if !reflect.DeepEqual(fast[:len(want)], want) {
		t.Errorf("first firings = %v, want %v", fast[:len(want)], want)
	}
}

// TestNothingConsultsTheWallClock runs the same schedule either side of a real pause and asserts
// the counter is unmoved by it.
func TestNothingConsultsTheWallClock(t *testing.T) {
	t.Parallel()

	c := clock.New()
	if _, err := c.Every(100, "pump", func(uint64) error { return nil }); err != nil {
		t.Fatalf("Every: %v", err)
	}
	if err := c.Advance(150); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	before := c.Now()
	time.Sleep(10 * time.Millisecond)
	if c.Now() != before {
		t.Errorf("Now moved from %d to %d across a sleep; something here is reading the wall", before, c.Now())
	}
	if due, ok := c.NextDue(); !ok || due != 200 {
		t.Errorf("NextDue = %d, %v; want 200, true", due, ok)
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()

	c := clock.New()
	fired := 0
	id, err := c.Every(10, "pump", func(uint64) error {
		fired++
		return nil
	})
	if err != nil {
		t.Fatalf("Every: %v", err)
	}

	if err := c.Advance(25); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if fired != 2 {
		t.Fatalf("fired %d times before cancel, want 2", fired)
	}

	if !c.Cancel(id) {
		t.Error("Cancel reported the event was not scheduled")
	}
	if c.Cancel(id) {
		t.Error("Cancel reported success twice for the same event")
	}
	if c.Pending() != 0 {
		t.Errorf("Pending = %d after cancel, want 0", c.Pending())
	}

	if err := c.Advance(1000); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if fired != 2 {
		t.Errorf("a cancelled event fired again: %d times, want 2", fired)
	}
}

func TestAPeriodicEventCanCancelItselfFromItsOwnHandler(t *testing.T) {
	t.Parallel()

	c := clock.New()
	var id clock.EventID
	fired := 0
	var err error
	id, err = c.Every(10, "self-cancelling pump", func(uint64) error {
		fired++
		if fired == 3 {
			c.Cancel(id)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Every: %v", err)
	}

	if err := c.Advance(1000); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if fired != 3 {
		t.Errorf("fired %d times, want 3", fired)
	}
	if c.Pending() != 0 {
		t.Errorf("Pending = %d, want 0", c.Pending())
	}
}

func TestAHandlerCanScheduleIntoTheSameAdvance(t *testing.T) {
	t.Parallel()

	c := clock.New()
	var seen []uint64
	var chain clock.Handler
	chain = func(now uint64) error {
		seen = append(seen, now)
		if now >= 50 {
			return nil
		}
		_, err := c.After(10, "chain", chain)
		return err
	}
	if _, err := c.After(10, "chain", chain); err != nil {
		t.Fatalf("After: %v", err)
	}

	if err := c.Advance(1000); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if want := []uint64{10, 20, 30, 40, 50}; !reflect.DeepEqual(seen, want) {
		t.Errorf("seen = %v, want %v", seen, want)
	}
}

// TestSchedulingInThePastIsRefused is the guard against the spin. A pump that reschedules itself
// at its own due instruction would otherwise fire for ever inside one Advance, and a hang is a far
// worse diagnosis than an error.
func TestSchedulingInThePastIsRefused(t *testing.T) {
	t.Parallel()

	// Each case gets its own clock: a shared one could not be checked for stray registrations
	// without the subtests interfering with each other.
	atNow100 := func(t *testing.T) *clock.Clock {
		t.Helper()
		c := clock.New()
		if err := c.Advance(100); err != nil {
			t.Fatalf("Advance: %v", err)
		}
		return c
	}

	tests := []struct {
		name    string
		attempt func(*clock.Clock) error
		want    error
	}{
		{name: "At before now", want: clock.ErrPastDue, attempt: func(c *clock.Clock) error {
			_, err := c.At(50, "late pump", func(uint64) error { return nil })
			return err
		}},
		{name: "At exactly now", want: clock.ErrPastDue, attempt: func(c *clock.Clock) error {
			_, err := c.At(100, "late pump", func(uint64) error { return nil })
			return err
		}},
		{name: "At instruction zero", want: clock.ErrPastDue, attempt: func(c *clock.Clock) error {
			_, err := c.At(0, "late pump", func(uint64) error { return nil })
			return err
		}},
		{name: "After zero delta", want: clock.ErrPastDue, attempt: func(c *clock.Clock) error {
			_, err := c.After(0, "late pump", func(uint64) error { return nil })
			return err
		}},
		{name: "Every zero period", want: clock.ErrZeroPeriod, attempt: func(c *clock.Clock) error {
			_, err := c.Every(0, "late pump", func(uint64) error { return nil })
			return err
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := atNow100(t)
			err := tt.attempt(c)
			if err == nil {
				t.Fatalf("%s with now=%d succeeded; this is the reschedule-at-zero spin", tt.name, c.Now())
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("error %v does not wrap %v", err, tt.want)
			}
			if !strings.Contains(err.Error(), "late pump") {
				t.Errorf("error %q does not name the event", err)
			}
			if c.Pending() != 0 {
				t.Errorf("Pending = %d, want 0: a refused schedule must not be half-registered", c.Pending())
			}
		})
	}
}

func TestANilHandlerIsRefused(t *testing.T) {
	t.Parallel()

	c := clock.New()
	if _, err := c.At(10, "nil one-shot", nil); err == nil {
		t.Error("At accepted a nil handler")
	}
	if _, err := c.Every(10, "nil pump", nil); err == nil {
		t.Error("Every accepted a nil handler")
	}
	if c.Pending() != 0 {
		t.Errorf("Pending = %d, want 0", c.Pending())
	}
}

// TestAHandlerErrorStopsTheAdvanceWhereItFailed keeps "errors are values" true of the scheduler:
// a device pump that fails must not be swallowed, and the counter must report where the machine
// actually got to.
func TestAHandlerErrorStopsTheAdvanceWhereItFailed(t *testing.T) {
	t.Parallel()

	boom := errors.New("the demux refused a section")
	c := clock.New()
	later := 0
	if _, err := c.At(50, "failing pump", func(uint64) error { return boom }); err != nil {
		t.Fatalf("At: %v", err)
	}
	if _, err := c.At(60, "later pump", func(uint64) error {
		later++
		return nil
	}); err != nil {
		t.Fatalf("At: %v", err)
	}

	err := c.Advance(100)
	if err == nil {
		t.Fatal("Advance swallowed a handler error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error %v does not wrap the handler's error", err)
	}
	for _, want := range []string{"failing pump", "50"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if c.Now() != 50 {
		t.Errorf("Now = %d, want 50: the counter must say how far the machine got", c.Now())
	}
	if later != 0 {
		t.Error("an event after the failure still fired")
	}
}

func TestBudget(t *testing.T) {
	t.Parallel()

	c := clock.New()
	if got := c.Budget(1000); got != 1000 {
		t.Errorf("Budget with nothing scheduled = %d, want 1000", got)
	}
	if _, err := c.At(250, "vsync", func(uint64) error { return nil }); err != nil {
		t.Fatalf("At: %v", err)
	}
	if got := c.Budget(1000); got != 250 {
		t.Errorf("Budget = %d, want 250", got)
	}
	if got := c.Budget(100); got != 100 {
		t.Errorf("Budget capped = %d, want 100", got)
	}
	if got := c.Budget(0); got != 0 {
		t.Errorf("Budget(0) = %d, want 0", got)
	}
}

// TestBudgetNeverOvershootsAnEvent drives the loop the way the instruction loop will, and asserts
// the budget never lets it run past something that was due.
func TestBudgetNeverOvershootsAnEvent(t *testing.T) {
	t.Parallel()

	c := clock.New()
	var seen []uint64
	if _, err := c.Every(37, "pump", func(now uint64) error {
		seen = append(seen, now)
		return nil
	}); err != nil {
		t.Fatalf("Every: %v", err)
	}

	for c.Now() < 370 {
		budget := c.Budget(1000)
		if budget == 0 {
			t.Fatalf("budget went to zero at instruction %d; the loop would spin", c.Now())
		}
		before := c.Now()
		if err := c.Advance(budget); err != nil {
			t.Fatalf("Advance: %v", err)
		}
		if c.Now() != before+budget {
			t.Fatalf("Advance(%d) from %d landed at %d", budget, before, c.Now())
		}
	}
	if want := []uint64{37, 74, 111, 148, 185, 222, 259, 296, 333, 370}; !reflect.DeepEqual(seen, want) {
		t.Errorf("fired at %v, want %v", seen, want)
	}
}

func TestRestoreToClearsTheSchedule(t *testing.T) {
	t.Parallel()

	c := clock.New()
	fired := 0
	if _, err := c.Every(10, "stale pump", func(uint64) error {
		fired++
		return nil
	}); err != nil {
		t.Fatalf("Every: %v", err)
	}
	if err := c.Advance(100); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	before := fired

	c.RestoreTo(4_500_000)

	if c.Now() != 4_500_000 {
		t.Errorf("Now = %d, want 4500000", c.Now())
	}
	if c.Pending() != 0 {
		t.Errorf("Pending = %d after RestoreTo, want 0: a handler closed over pre-restore state must not survive", c.Pending())
	}
	if err := c.Advance(1000); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if fired != before {
		t.Errorf("a pre-restore event fired %d more times after RestoreTo", fired-before)
	}
}

func TestReset(t *testing.T) {
	t.Parallel()

	c := clock.New()
	if _, err := c.Every(10, "pump", func(uint64) error { return nil }); err != nil {
		t.Fatalf("Every: %v", err)
	}
	if err := c.Advance(100); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	c.Reset()
	if c.Now() != 0 || c.Pending() != 0 {
		t.Errorf("after Reset: Now = %d, Pending = %d; want 0, 0", c.Now(), c.Pending())
	}
}

func TestOverflowIsRefusedRatherThanWrapped(t *testing.T) {
	t.Parallel()

	c := clock.New()
	c.RestoreTo(^uint64(0) - 10)

	if err := c.Advance(100); !errors.Is(err, clock.ErrCounterOverflow) {
		t.Errorf("Advance error is %v, want it to wrap ErrCounterOverflow", err)
	}
	if _, err := c.After(100, "far future", func(uint64) error { return nil }); !errors.Is(err, clock.ErrCounterOverflow) {
		t.Errorf("After error is %v, want it to wrap ErrCounterOverflow", err)
	}
	if _, err := c.Every(100, "far future pump", func(uint64) error { return nil }); !errors.Is(err, clock.ErrCounterOverflow) {
		t.Errorf("Every error is %v, want it to wrap ErrCounterOverflow", err)
	}
}

func TestManyEventsStayOrdered(t *testing.T) {
	t.Parallel()

	c := clock.New()
	var seen []uint64
	// Registered out of order on purpose: a heap that is not actually a heap passes a sorted
	// registration and fails this.
	for _, due := range []uint64{900, 100, 700, 300, 500, 200, 800, 400, 600, 1000} {
		due := due
		if _, err := c.At(due, fmt.Sprintf("event %d", due), func(now uint64) error {
			seen = append(seen, now)
			return nil
		}); err != nil {
			t.Fatalf("At(%d): %v", due, err)
		}
	}

	if err := c.Advance(1000); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	want := []uint64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("fired at %v, want %v", seen, want)
	}
}
