package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// DOES THE EVENT PIPE FILL ON A BOX NOBODY TOUCHES?
//
// The wedge next door is reached by pressing the handset, so it has always looked like something
// the keys do. It may not be. EVQP0002 has one producer and one consumer, and BOTH ARE ALREADY
// BUSY ON AN IDLE BOX -- over two million instructions with the handset untouched, EVTTask sends
// to it sixteen times and SMTTask receives from it fifteen. A queue with traffic in it either
// keeps up or it does not, and the keys are then incidental to which.
//
// THE TWO ANSWERS SEND THE WORK SOMEWHERE COMPLETELY DIFFERENT, which is why this is worth its own
// run. If an untouched box fills the pipe, the handset is a red herring and the cause is upstream
// of it -- and the first suspects are this port's own, because the record already says the
// smartcard link's pacing is "ours, not the firmware's, and load-bearing". If an untouched box
// never fills it, the presses really are doing something and the question goes back to what a
// press costs the consumer.
//
// IT WATCHES THE COUNT BEING WRITTEN RATHER THAN SAMPLING IT. The first version of this read the
// pipe every two million instructions and reported that it never held a single message in five
// hundred million -- which was true of every sample and false of the box. The queue is drained as
// fast as it is filled, so its depth is a transient, and this file's own retracted HISR finding
// says the same thing in the same words: POLLING A TRANSIENT IS NOT OBSERVING AN EVENT. The depth
// is now taken from the writes to the count field, which is every change it ever has.
//
// It asserts its own subject twice. The pipe must be found by name, or the watch below is on
// address zero; and the count field must actually be WRITTEN, or this watched a queue nobody uses
// and learnt nothing about the one that wedges.
func TestWhetherTheEventPipeFillsOnABoxNobodyTouches(t *testing.T) {
	const watched = "EVQP0002"
	// Enough to cover what the wedge run spends getting to the wall, which is the acquisition plus
	// the press budgets -- roughly three hundred million. Going well past that is what makes "it
	// never filled" mean something.
	const budget = 500_000_000
	const slice = 2_000_000

	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}

	pipe := uint32(0)
	objects, err := box.NucleusObjects()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range objects {
		if o.Name == watched {
			pipe = o.Address
		}
	}
	if pipe == 0 {
		t.Fatalf("harness: the census found %d objects and no %s, so there is nothing to watch",
			len(objects), watched)
	}

	field := func(index int) uint32 {
		return box.RAM.Read((pipe&0x1fffffff)+uint32(index)*4, bus.Word) // #nosec G115 -- small index
	}
	size := field(pipeSize)
	if size == 0 {
		t.Fatalf("harness: %s reports a zero-byte buffer, so it is not the pipe", watched)
	}
	t.Logf("%s at %08X: %d-byte buffer, and not one key will be pressed", watched, pipe, size)

	started := box.Machine.Retired
	countAt := (pipe & 0x1fffffff) + uint32(pipeCount)*4  // #nosec G115 -- a small constant index
	waitAt := (pipe & 0x1fffffff) + uint32(pipeWaiting)*4 // #nosec G115 -- a small constant index
	var writes, mostHeld, mostWaiting uint32
	var firstSuspend, firstFull uint64
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write {
			return
		}
		switch a.Virtual & 0x1fffffff {
		case countAt:
			writes++
			if a.Value > mostHeld {
				mostHeld = a.Value
			}
		case waitAt:
			if a.Value > mostWaiting {
				mostWaiting = a.Value
			}
			if a.Value > 0 && firstSuspend == 0 {
				firstSuspend = box.Machine.Retired - started
			}
		}
	}}
	for box.Machine.Retired-started < budget {
		for i := 0; i < slice; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		if field(pipeAvailable) == 0 && firstFull == 0 {
			firstFull = box.Machine.Retired - started
			t.Logf("after %d instructions: %s is FULL, holding %d messages", firstFull, watched,
				field(pipeCount))
			break
		}
	}

	// THE SUBJECT CHECK, and it is not a formality: a count field nobody ever writes belongs to a
	// pipe nobody uses, and "it never filled" would then be a statement about the instrument.
	if writes == 0 {
		t.Fatalf("harness: nothing wrote %s's message count in %d instructions, so this watched an "+
			"idle queue and its silence says nothing about the one that wedges", watched, budget)
	}
	t.Logf("%s carried traffic throughout: its message count was written %d times, and the deepest "+
		"it ever got was %d of %d messages with at most %d tasks suspended",
		watched, writes, mostHeld, size/8, mostWaiting)

	reportEventPipeVerdict(t, box, watched, mostHeld, size/8, firstFull, firstSuspend, budget)
}

// reportEventPipeVerdict says which of the two answers the run gave, and what each one means for
// where the wedge is chased next.
func reportEventPipeVerdict(t *testing.T, box *board.Runtime, watched string,
	mostHeld, capacity uint32, firstFull, firstSuspend uint64, budget int,
) {
	t.Helper()
	waits, err := box.TaskWaits()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range waits {
		for _, s := range w.On {
			if s.Object.Name == watched {
				t.Logf("    %-10s status=%d runs=%-6d is queued on it", w.Task.Name, w.Task.Status,
					w.Task.Runs)
			}
		}
	}
	if firstFull != 0 {
		t.Logf("VERDICT: %s FILLED WITH NO INPUT AT ALL, after %d instructions. The handset is "+
			"incidental -- the presses were never the cause, they were what made it visible. The "+
			"first task suspended on it at %d. Chase this upstream of the keys, and this port's "+
			"own smartcard link pacing is the first suspect, because the record already calls that "+
			"ours rather than the firmware's and load-bearing.", watched, firstFull, firstSuspend)
		return
	}
	t.Logf("VERDICT: %s did NOT fill in %d instructions with nothing touched. It got to %d of its "+
		"%d messages at the busiest. So the presses really do cost the consumer something, and the "+
		"question is what a key press makes SMTTask do that it cannot keep up with -- not why the "+
		"box drifts into it on its own, because it does not.", watched, budget, mostHeld, capacity)
}
