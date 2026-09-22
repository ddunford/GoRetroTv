package firmwaretests_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH CARD COMMANDS HAVE TO BE ANSWERED TO KEEP THE EVENT PIPE DRAINED?
//
// The handset and the viewing card share one wire here -- the front-panel micro's synchronous
// serial link -- and the pipe that wedges (gort-slq) is drained by SMTTask, the smartcard module's
// transmit task. This port's card answered exactly two command codes, 0x52 and 0x18, because those
// are the two the record measured. The box also sends others and advances past them on SMTTask's
// fifty-tick timeout: **if each drained event costs a timeout, twenty outstanding events is a
// handful of presses.**
//
// WHICH others is not a matter of opinion. Across the eight presses that fill the pipe the box
// sends exactly three codes -- 0x18 three times, 0x41 six times and 0x42 six times
// (cardcodes_firmware_test.go) -- so the whole question is what answering 0x41 and 0x42 does. The
// arms below take them one at a time, because two changes at once cannot tell you which mattered.
//
// AckAll IS INCLUDED AS AN UPPER BOUND AND IS NOT A CANDIDATE POLICY. It answers every code with a
// stock frame, which is not what a viewing card does -- a real one answers some and refuses others,
// and a reply the box accepts where a card would have refused makes a machine that is plausibly
// wrong rather than visibly broken. It is here to say how much of the effect a narrower policy
// recovers.
//
// THIS IS INPUT-SIDE AND NOT A POKE. What changes between arms is what the modelled peripheral
// answers, the same class of thing as the link's baud rate, which this project already records as
// ours rather than the firmware's and load-bearing. Nothing is written into the guest.
//
// THE CONTROL ARM HAS TO REPRODUCE THE WEDGE or nothing else here means anything, and it asserts it.
func TestWhichCardCommandsMustBeAnsweredToKeepTheEventPipeDrained(t *testing.T) {
	const watched = "EVQP0002"
	// TWENTY, NOT SIXTY. The control fills after eight, so twenty is two and a half times the
	// distance it survives -- enough separation to mean something, and the arms that DO fill stop
	// at their own count anyway. The larger number cost four hundred million extra instructions
	// under the race detector and bought no extra confidence: this package has a thirty-minute cap
	// and has hit it before.
	const presses = 20
	const settle = 2_000_000

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

	// ONE ACQUISITION, MANY ARMS. Restoring the machine puts every device back INCLUDING the link's
	// ack policy, so each arm sets its policy after the restore or it silently runs the last one's.
	var snapshot bytes.Buffer
	if err := box.Machine.Snapshot(&snapshot); err != nil {
		t.Fatal(err)
	}

	// arm presses DOWN until the event pipe is full, and says how many presses that took and how
	// deep it ever got. A zero press count means it never filled.
	arm := func(t *testing.T, policy []uint8, everything bool) (int, uint32) {
		t.Helper()
		if err := box.Restore(bytes.NewReader(snapshot.Bytes())); err != nil {
			t.Fatal(err)
		}
		if everything {
			box.CSI.AckAll()
		} else {
			box.CSI.SetAckPolicy(policy)
		}
		pipe := pipeNamed(t, box, watched)
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
				"not carrying the keys and this arm measured nothing", watched, presses)
		}
		return 0, deepest
	}

	measured := []struct {
		name       string
		policy     []uint8
		everything bool
	}{
		{"0x52 and 0x18 only (the control)", []uint8{0x52, 0x18}, false},
		{"and 0x41", []uint8{0x52, 0x18, 0x41}, false},
		{"and 0x42", []uint8{0x52, 0x18, 0x42}, false},
		{"and both 0x41 and 0x42", []uint8{0x52, 0x18, 0x41, 0x42}, false},
		{"every code (the upper bound, NOT a candidate)", nil, true},
	}
	filled := make([]int, len(measured))
	for i, m := range measured {
		at, deepest := arm(t, m.policy, m.everything)
		filled[i] = at
		outcome := "never filled"
		if at > 0 {
			outcome = "FULL"
		}
		t.Logf("%-46s %s after %2d presses, deepest %2d of 20", m.name, outcome, at, deepest)
	}

	control, both, all := filled[0], filled[3], filled[4]
	if control == 0 {
		t.Fatalf("harness: the CONTROL arm never filled %s in %d presses. It is not reproducing "+
			"the wedge, so no other arm here has anything to be compared against", watched, presses)
	}
	if both != 0 {
		t.Errorf("answering 0x41 and 0x42 still fills %s, after %d presses against the control's "+
			"%d. Those are the only codes the box sends while it fills, so either they are not "+
			"the whole cause or the replies are not being accepted", watched, both, control)
		return
	}
	verdict := "and that is the whole of it -- a policy answering only what the box actually asks " +
		"during the presses is as good as answering every code there is"
	if all != 0 {
		verdict = fmt.Sprintf("and it does BETTER than answering every code, which filled after "+
			"%d. Two codes beating the superset is not a result to build on until it is "+
			"understood", all)
	}
	t.Logf("VERDICT: the control fills %s after %d presses; answering 0x41 and 0x42 never fills it "+
		"in %d, %s", watched, control, presses, verdict)
}
