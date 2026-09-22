package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT STOPS THE TV GUIDE MENU AT ENTRY 6?
//
// The tab lists ten entries and the highlight will not pass the sixth. Forty
// DOWN presses moved it five times and then nothing, the same screen hash
// across the last thirty-five -- so this is a refusal, not the swallow
// behaviour, which does not absorb thirty-five presses in a row.
//
// THE INSTRUMENT IS A DIFFERENTIAL BETWEEN TWO PRESSES OF THE SAME KEY. One
// down that MOVES the highlight, and one that does not, taken as close together
// as the machine allows: the refusal must execute something the acceptance does
// not, or take a branch it does not, and that difference is the check. Anything
// common to both -- the key path, the event loop, the repaint -- cancels.
//
// It asserts its own subject: if the "failing" press turns out to move the
// screen, or the "working" one does not, the two are not what this claims and
// the diff below would be noise.
func TestWhatStopsTheTvGuideMenuAtEntrySix(t *testing.T) {
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

	var watching bool
	seen := map[uint32]int{}
	hooks := board.StepHooks{}

	press := func(raw uint8, name string, budget int) (uint32, uint32) {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, settled := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if watching {
				seen[box.Machine.Core.State().PC&^1]++
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				settled = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		if name != "" {
			t.Logf("%-26s %08X -> %08X", name, before, settled)
		}
		return before, settled
	}

	// To the tv guide tab, retrying LEFT because box office settles there first.
	const boxOfficeMenu = 0xFE8D1CCC
	_, menu := press(0x7D, "box office", 80_000_000)
	tab := menu
	for attempt := 1; attempt <= 4 && (tab == menu || tab == boxOfficeMenu || tab == 0); attempt++ {
		_, tab = press(0x5A, "left to tv guide", 80_000_000)
	}
	if tab == menu || tab == boxOfficeMenu || tab == 0 {
		t.Fatalf("harness: never reached the tv guide tab (stuck at %08X)", tab)
	}

	// Down until it stops moving, counting the moves that actually happened.
	moves := 0
	for attempt := 0; attempt < 40; attempt++ {
		if _, s := press(0x59, "", 4_000_000); s != 0 {
			moves++
			continue
		}
		// Three refusals in a row is the wall rather than a swallowed press.
		if _, s2 := press(0x59, "", 4_000_000); s2 != 0 {
			moves++
			continue
		}
		if _, s3 := press(0x59, "", 4_000_000); s3 == 0 {
			break
		}
		moves++
	}
	t.Logf("the highlight moved %d times before it stopped", moves)
	if moves == 0 {
		t.Fatal("harness: the highlight never moved at all, so there is no working press to compare")
	}

	// THE REFUSAL, recorded.
	watching = true
	seen = map[uint32]int{}
	before, settled := press(0x59, "down AT THE WALL", 8_000_000)
	refusal := seen
	if settled != 0 {
		t.Fatalf("harness: the press at the wall MOVED the screen (%08X -> %08X), so it is not a "+
			"refusal and this diff would compare two acceptances", before, settled)
	}

	// IS IT THE MENU OR THE BOX? UP is tried first, then a key that leaves the
	// menu altogether. If navigation is dead in both directions but box office
	// still redraws, the refusal is the menu's; if nothing at all responds, the
	// box has stopped taking input and this is gort-slq rather than a gate.
	watching = false
	_, up := press(0x58, "up one", 8_000_000)
	if up == 0 {
		_, escape := press(0x7D, "box office FROM THE WALL", 12_000_000)
		if escape == 0 {
			// THE CHEAPEST HEALTH CHECK ON THE WHOLE SYSTEM, per this project's
			// record: [0x801072B0] is Nucleus's TCD_Execute_Task, and it is
			// non-zero exactly when a task is actually running. Zero means every
			// task is blocked and the machine is sitting in the idle loop at
			// 0x800D35DC -- which is a wedge, not a busy box declining to redraw.
			exec := box.RAM.Read(0x801072B0&0x1fffffff, bus.Word)
			ready := box.RAM.Read(0x801072D8&0x1fffffff, bus.Word)
			retiredBefore := box.Machine.Retired
			for i := 0; i < 2_000_000; i++ {
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("RTOS at the wall: TCD_Execute_Task=[0x801072B0]=%08X ready=[0x801072D8]=%08X",
				exec, ready)
			t.Logf("the box retired %d instructions while unresponsive, so it is running rather "+
				"than halted", box.Machine.Retired-retiredBefore)
			if exec == 0 {
				t.Logf("EVERY TASK IS BLOCKED -- the guest is in the idle loop, so this is a WEDGE " +
					"and not a menu declining to move")
				// WHICH tasks, and in what state. The record's own smartcard
				// section reached its conclusion exactly this way: status 7 is
				// an event wait, and the task list names what the box is
				// waiting for rather than leaving "blocked" as the answer.
				for _, name := range []string{
					"TASK0", "TASK1", "FETask", "SMHKTask", "SMNTask", "SMTTask", "SCTask", "EVTTask",
				} {
					status, runs, found, err := box.TaskState(name)
					if err != nil {
						t.Fatal(err)
					}
					if !found {
						continue
					}
					// ONLY STATUS 7 IS ESTABLISHED. The record says "status 7 means
					// event wait" and says nothing about the others, so the rest
					// are printed as raw numbers rather than given names this
					// project has not measured. Naming them would put a guess in
					// the output where a reader would take it for a reading.
					meaning := "(status not established by the record)"
					if status == 7 {
						meaning = "EVENT WAIT"
					}
					t.Logf("    %-10s status=%d runs=%-6d  %s", name, status, runs, meaning)
				}
			} else {
				t.Logf("a task IS running (%08X), so the guest is doing something and simply not "+
					"acting on keys", exec)
			}
			// REPORTED, NOT FAILED. The wedge is a known defect (gort-slq) and a
			// test that fails on it would red the suite until it is fixed, which
			// is what the tracker is for. What this test asserts is that its own
			// route and presses worked; the wedge itself it characterises. When
			// gort-slq is fixed, the log below becomes the assertion.
			t.Logf("VERDICT: neither UP nor box office moved the screen from %08X. The BOX has "+
				"stopped responding to input, so this is NOT a menu gate -- gort-slq. The "+
				"highlight stopped after %d moves, and at a different entry on another run, "+
				"which fits an unresponsive box rather than a disabled entry.", before, moves)
			return
		}
		t.Logf("VERDICT: UP is dead but box office still redraws (%08X), so navigation specifically "+
			"has stopped while the box still takes keys. The highlight moved %d times. That is a "+
			"menu-level refusal in BOTH directions, not a disabled entry 7.", escape, moves)
		return
	}
	seen = map[uint32]int{}
	watching = true
	_, ok := press(0x59, "down THAT WORKS", 8_000_000)
	acceptance := seen
	watching = false
	if ok == 0 {
		t.Fatal("harness: the press after UP did not move the screen either, so there is no " +
			"working press in this pair")
	}

	var only []uint32
	for pc := range refusal {
		if acceptance[pc] == 0 {
			only = append(only, pc)
		}
	}
	sort.Slice(only, func(a, b int) bool { return only[a] < only[b] })
	t.Logf("refusal executed %d distinct PCs, acceptance %d; %d ran ONLY in the refusal",
		len(refusal), len(acceptance), len(only))
	if len(only) == 0 {
		t.Log("VERDICT: the refusal runs nothing the acceptance does not. The decision is a taken " +
			"branch inside shared code, not a separate path -- a read-watch is the next instrument.")
		return
	}
	for i, pc := range only {
		if i >= 40 {
			t.Logf("    ... and %d more", len(only)-i)
			break
		}
		t.Logf("    %08X  %d", pc, refusal[pc])
	}
}
