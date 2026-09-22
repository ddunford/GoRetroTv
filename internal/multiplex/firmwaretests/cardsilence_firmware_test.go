package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// IS THE WEDGE OURS? Does answering every card command stop the event pipe filling?
//
// The handset and the viewing card share one wire here -- the front-panel micro's synchronous
// serial link -- and the pipe that wedges is drained by SMTTask, the smartcard module's transmit
// task. This port's modelled card answers exactly two command codes, 0x52 and 0x18, because those
// are the two the record measured; the box sends 0x11, 0x53, 0x54, 0x41, 0x43, 0x42 and 0x17 as
// well and advances past them on SMTTask's fifty-tick timeout. **If each drained event costs
// SMTTask a timeout, twenty outstanding events is a handful of presses.**
//
// THIS IS AN INPUT-SIDE EXPERIMENT AND NOT A POKE. What changes between the two arms is what the
// modelled peripheral ANSWERS -- the same class of change as the link's baud rate, which this
// project already records as ours rather than the firmware's and load-bearing. Nothing is written
// into the guest.
//
// IT IS A DIAGNOSTIC, NOT A FIX, and the difference matters. AckAll answers every code with a
// stock reply, which is not what a viewing card does; a real one answers some and refuses others.
// So a difference here says where the cause is, not what the card should say -- that is a separate
// question and needs its own evidence.
//
// THE CONTROL ARM HAS TO REPRODUCE THE WEDGE or the comparison means nothing, and this asserts it.
func TestWhetherAnsweringEveryCardCommandStopsTheEventPipeFilling(t *testing.T) {
	const watched = "EVQP0002"
	const presses = 60
	const settle = 2_000_000

	// arm presses DOWN until the event pipe is full, and says how many presses that took.
	arm := func(t *testing.T, answerEverything bool) (int, uint32) {
		t.Helper()
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
		// AFTER the acquisition, so both arms acquire identically and the only difference is what
		// the card says once the handset starts being used.
		if answerEverything {
			box.CSI.AckAll()
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
			t.Fatalf("harness: no %s in %d objects, so there is nothing to watch", watched,
				len(objects))
		}
		countAt := (pipe & 0x1fffffff) + uint32(pipeCount)*4 // #nosec G115 -- a small constant index
		var deepest, writes uint32
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if a.Write && a.Virtual&0x1fffffff == countAt {
				writes++
				if a.Value > deepest {
					deepest = a.Value
				}
			}
		}}
		free := func() uint32 {
			return box.RAM.Read((pipe&0x1fffffff)+uint32(pipeAvailable)*4, bus.Word) // #nosec G115
		}

		for press := 1; press <= presses; press++ {
			if err := box.CSI.Key(0x59, 0); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < settle; i++ {
				if err := transmitter.Pump(box.Machine.Retired); err != nil {
					t.Fatal(err)
				}
				if err := box.StepWithHooks(hooks); err != nil {
					t.Fatal(err)
				}
			}
			if free() == 0 {
				return press, deepest
			}
		}
		if writes == 0 {
			t.Fatalf("harness: nothing wrote %s's message count across %d presses, so the link is "+
				"not carrying the keys and neither arm measured anything", watched, presses)
		}
		return 0, deepest
	}

	silent, silentDepth := arm(t, false)
	answering, answeringDepth := arm(t, true)

	t.Logf("card answers 0x52 and 0x18 only: pipe full after %d presses (deepest %d)",
		silent, silentDepth)
	t.Logf("card answers everything:         pipe full after %d presses (deepest %d)",
		answering, answeringDepth)

	if silent == 0 {
		t.Fatalf("harness: the CONTROL arm never filled %s in %d presses, reaching %d messages. It "+
			"is not reproducing the wedge, so the other arm has nothing to be compared against -- "+
			"the route needs to be the menu's, as wedge_firmware_test.go walks it", watched,
			presses, silentDepth)
	}
	switch {
	case answering == 0:
		t.Logf("VERDICT: ANSWERING EVERY COMMAND STOPS IT. The silent card fills %s after %d "+
			"presses; a card that answers never fills it at all in %d, reaching %d of its "+
			"messages. The wedge is THIS PORT'S, not the firmware's -- the modelled card's silence "+
			"is what costs SMTTask the time it cannot spare. What a real card actually answers is "+
			"the next question and needs its own evidence.", watched, silent, presses, answeringDepth)
	case answering > silent*2:
		t.Logf("VERDICT: answering every command DELAYS it markedly -- %d presses against %d. That "+
			"is the same mechanism running slower rather than a different one, so the card's "+
			"silence is a contributor and not the whole cause.", answering, silent)
	default:
		t.Logf("VERDICT: answering every command changes nothing -- %d presses against %d. The "+
			"card's silence is NOT what fills the pipe, and the cost a press puts on SMTTask is "+
			"somewhere else on the link.", answering, silent)
	}
}
