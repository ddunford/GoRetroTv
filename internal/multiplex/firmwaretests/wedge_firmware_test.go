package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/device/csi"
	"github.com/ddunford/goretrotv/internal/device/hwtimer"
	"github.com/ddunford/goretrotv/internal/device/irq"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT IS THE BOX WAITING FOR WHEN IT WEDGES?
//
// Press the TV GUIDE menu a few times and it stops moving. That was filed as a menu gate -- "the
// highlight will not pass entry 6" -- and the filing was wrong. UP stops working too; so does box
// office, which leaves the menu altogether; and the stop point varies between runs, which a
// disabled entry would not do.
//
// So this walks the box to the wall and then asks the RTOS rather than the screen. [0x801072B0] is
// Nucleus's TCD_Execute_Task and is non-zero exactly when a task is actually running, so zero
// means every task is blocked and the guest is sitting in the idle loop at 0x800D35DC. That
// separates a wedge from a busy box declining to redraw, and it is a reading rather than an
// inference.
//
// "EVERY TASK IS BLOCKED" IS STILL ONLY THE SYMPTOM RESTATED. The finding is what they are blocked
// ON, and that is reachable: Nucleus builds a suspend block naming both the waiting task and the
// object it is queued on, so the semaphores and event groups can be found and named. The record
// reached its smartcard conclusion exactly this way -- "TASK0 blocked obtaining semaphore Periph",
// "SMNTask waits on event group SMNEvts" -- and a box that can name what it is waiting for is a
// box you can go and satisfy through its inputs.
//
// IT REPORTS RATHER THAN FAILS. The wedge is tracked as gort-slq and a test that failed on it
// would red the suite until it is fixed, which is what the tracker is for. What this asserts is
// its own route and its own presses; the wedge itself it characterises, and when gort-slq is fixed
// the log below becomes the assertion.
func TestWhatTheBoxIsWaitingForWhenTheMenuWedges(t *testing.T) {
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

	// THE CONTROL, TAKEN BEFORE ANYTHING IS PRESSED. "Every task is blocked" is only damning if a
	// working box does not look like that, and a box between events looks EXACTLY like that: every
	// task sits on its own event group or its command queue waiting for work, which is what idle
	// is. Without this reading the headline below would be an over-read dressed as a measurement.
	baseline := rtosStateAt(t, box, "idle, before any key")

	// THE PIPE THE WEDGE LANDS ON, found by name before anything is pressed so the same address is
	// watched on both sides of the comparison. It asserts its own subject: EVQP0002 is where all
	// three moving tasks end up, and a run that cannot find it is watching nothing.
	const watched = "EVQP0002"
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
		t.Fatalf("harness: the census found %d objects and no %s, so the watch below would be on "+
			"address zero and every count it reports would be a fiction", len(objects), watched)
	}
	t.Logf("watching %s at %08X", watched, pipe)

	idleLink := csiTraffic(t, box, transmitter, csiWindow, pipe)
	idleAfter := readRTOS(t, box)
	t.Logf("hardware while idle and working: %s", idleLink)
	t.Logf("tasks scheduled over %d idle instructions: %v", csiWindow, scheduled(baseline, idleAfter))

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

	// IS IT THE MENU OR THE BOX? UP is tried first, then a key that leaves the menu altogether. If
	// navigation is dead in both directions but box office still redraws, the refusal is the
	// menu's; if nothing at all responds, the box has stopped taking input.
	watching = false
	_, up := press(0x58, "up one", 8_000_000)
	if up == 0 {
		_, escape := press(0x7D, "box office FROM THE WALL", 12_000_000)
		if escape == 0 {
			reportTheWedge(t, box, transmitter, baseline, idleLink, watched, pipe, before, moves)
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

// rtosState is what the guest's scheduler is doing and what every task is waiting for.
//
// THE OBJECTS ARE NAMED, NOT COUNTED. "Every task is blocked" is the symptom restated; the finding
// is WHICH object each one is queued on, and Nucleus makes that readable because a suspend block
// names both the task and the object. The record reached its smartcard conclusion this way --
// "TASK0 blocked obtaining semaphore Periph", "SMNTask waits on event group SMNEvts" -- and a box
// that names what it wants is a box that can be given it through its inputs.
type rtosState struct {
	exec, ready uint32
	// running is the task that held the CPU at the instant of the sample, where there was one.
	// A snapshot of a working box catches SOMEBODY mid-run, and that task then reads as having
	// "changed" against any later sample -- which is the sample instant, not a finding.
	running string
	on      map[string]string
	// runs is each task's own schedule count. Comparing two samples says which tasks ACTUALLY RAN
	// in between, which is a different and blunter question from what they are queued on -- and
	// the one that separates a task frozen in a state from a task cycling through it.
	runs map[string]uint32
}

// scheduled names the tasks whose own run count moved between two samples.
func scheduled(before, after rtosState) []string {
	var ran []string
	for name, n := range after.runs {
		if n != before.runs[name] {
			ran = append(ran, fmt.Sprintf("%s+%d", name, n-before.runs[name]))
		}
	}
	sort.Strings(ran)
	return ran
}

func readRTOS(t *testing.T, box *board.Runtime) rtosState {
	t.Helper()
	state := rtosState{
		// THE CHEAPEST HEALTH CHECK ON THE WHOLE SYSTEM, per this project's record: [0x801072B0]
		// is Nucleus's TCD_Execute_Task, non-zero exactly when a task is actually running.
		exec:  box.RAM.Read(0x801072B0&0x1fffffff, bus.Word),
		ready: box.RAM.Read(0x801072D8&0x1fffffff, bus.Word),
		on:    map[string]string{},
		runs:  map[string]uint32{},
	}
	waits, err := box.TaskWaits()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range waits {
		if w.Task.TCB == state.exec {
			state.running = w.Task.Name
		}
		// THE STATUS IS PRINTED RAW BESIDE THE OBJECT'S TYPE, which is what lets the two be
		// correlated further down instead of one of them being asserted. A number named here
		// would be a name this instrument imported rather than measured.
		where := "nothing names it"
		for i, s := range w.On {
			if i > 0 {
				where += " + "
			} else {
				where = ""
			}
			where += s.Object.Type + " " + s.Object.Name
		}
		state.on[w.Task.Name] = fmt.Sprintf("status=%d %s", w.Task.Status, where)
		state.runs[w.Task.Name] = w.Task.Runs
	}
	return state
}

func rtosStateAt(t *testing.T, box *board.Runtime, when string) rtosState {
	t.Helper()
	state := readRTOS(t, box)
	t.Logf("RTOS %s: TCD_Execute_Task=%08X ready=%08X, %d tasks", when, state.exec, state.ready,
		len(state.on))
	return state
}

// reportTheWedge asks the RTOS what it is doing and what it is waiting for, once the box has
// stopped answering the handset altogether, and says what CHANGED since it was working.
func reportTheWedge(t *testing.T, box *board.Runtime, transmitter *multiplex.Multiplex,
	baseline rtosState, idleLink csiLink, watched string, pipe uint32, screen uint32, moves int,
) {
	t.Helper()

	wallBefore := readRTOS(t, box)
	waitsAtTheWall, err := box.TaskWaits()
	if err != nil {
		t.Fatal(err)
	}
	wallLink := csiTraffic(t, box, transmitter, csiWindow, pipe)
	wall := readRTOS(t, box)
	t.Logf("RTOS at the wall: TCD_Execute_Task=[0x801072B0]=%08X ready=[0x801072D8]=%08X",
		wall.exec, wall.ready)
	t.Logf("the box retired %d instructions while unresponsive, so it is running rather than halted",
		csiWindow)

	// THE LINK THE HANDSET ARRIVES ON. The card is the clock master on this wire and the model
	// follows the firmware: a queued byte is presented only once the guest has written one back,
	// because that is how the guest shifts its own frame out. So if the guest ever stops writing
	// to the data register while something is queued, the link goes silent in BOTH directions --
	// no key byte, and no idle byte either, because a waiting queue takes priority over idle. That
	// is a stall this port could cause rather than a firmware defect, so it is measured before
	// anything upstream is blamed.
	t.Logf("hardware while idle and working: %s", idleLink)
	t.Logf("hardware at the wall:            %s", wallLink)

	t.Logf("%s at %08X, working window against the wall:", watched, pipe)
	reportTheWatchedBlock(t, watched, pipe, idleLink, wallLink)

	// WHICH TASKS STILL RUN. Status is a state and a state can be one a task passes through every
	// few thousand instructions; the run count says whether it is passing through at all. A task
	// frozen in status 5 and a task cycling through status 5 look identical in one sample and
	// nothing alike in two.
	t.Logf("tasks scheduled over %d instructions AT THE WALL: %v", csiWindow,
		scheduled(wallBefore, wall))
	if wallLink.queued > 0 && wallLink.writes == 0 {
		t.Logf("THE LINK IS STALLED, AND IT IS THIS PORT'S WIRE: %d bytes are queued for the guest "+
			"and the guest wrote to the data register %d times in %d instructions. The model only "+
			"clocks a queued byte out when the guest writes one in, so neither side moves again.",
			wallLink.queued, wallLink.writes, csiWindow)
	}

	// THE COMPARISON IS THE FINDING. An idle box has every task blocked too, so the reading that
	// matters is whether any task is waiting on something DIFFERENT from what it waited on while
	// the handset still worked.
	moved := 0
	for name, now := range wall.on {
		was, known := baseline.on[name]
		switch {
		case !known:
			t.Logf("    NEW  %-10s %s", name, now)
			moved++
		case was == now:
		case name == baseline.running:
			t.Logf("    %-10s %s  ->  %s  (it held the CPU when the control was taken, so this "+
				"line is the sample instant rather than the wedge)", name, was, now)
		default:
			t.Logf("    %-10s %s  ->  %s", name, was, now)
			moved++
		}
	}
	for name := range baseline.on {
		if _, still := wall.on[name]; !still {
			t.Logf("    GONE %-10s %s", name, baseline.on[name])
			moved++
		}
	}
	if moved == 0 {
		t.Logf("NOT ONE TASK MOVED. All %d are queued on exactly what they were queued on while the "+
			"handset still worked, and the scheduler was idle then too (exec=%08X). So this is NOT "+
			"a deadlock the box fell into: it is the resting state, and what stopped is the arrival "+
			"of the EVENT a key press is supposed to produce. The question is the input path, not "+
			"the RTOS.", len(wall.on), baseline.exec)
	} else {
		t.Logf("%d of %d tasks are waiting on something different from when the handset worked, "+
			"which is where the wedge is.", moved, len(wall.on))
	}

	objects, err := box.NucleusObjects()
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string]int{}
	at := map[string]uint32{}
	for _, o := range objects {
		byType[o.Type]++
		at[o.Type+" "+o.Name] = o.Address
	}
	// IT ASSERTS ITS OWN SUBJECT, against the record rather than against itself. These three are
	// named in the measured record's smartcard section -- the semaphore TASK0 blocks on, the event
	// group SMNTask waits on, and the HISR that wakes it -- so a census that cannot find them is
	// not reading the machine, and its silence about everything else would mean nothing.
	for _, known := range []string{"SEMA Periph", "EVNT SMNEvts", "HISR CSIHISR"} {
		if at[known] == 0 {
			t.Fatalf("harness: the census found %d objects and not %q, which the record names with "+
				"an address -- so it is not finding kernel objects and nothing below is a reading",
				len(objects), known)
		}
	}
	t.Logf("census: %d kernel objects, including %s at %08X and %s at %08X", len(objects),
		"SEMA Periph", at["SEMA Periph"], "EVNT SMNEvts", at["EVNT SMNEvts"])
	types := make([]string, 0, len(byType))
	for kind := range byType {
		types = append(types, kind)
	}
	sort.Slice(types, func(a, b int) bool { return byType[types[a]] > byType[types[b]] })
	for _, kind := range types {
		t.Logf("    %-6q %d", kind, byType[kind])
	}

	// WHAT THE STATUS NUMBERS MEAN, TAKEN OFF THIS BOX RATHER THAN OUT OF A HEADER. The record
	// established one value -- 7 is an event wait -- and printed the rest raw for want of
	// evidence. The evidence is right here: every suspended task is queued on an object whose TYPE
	// is read from the guest, so if every status-4 task is on a QUEU and every status-6 task on a
	// SEMA, the numbers name themselves. A status with more than one type against it is NOT
	// established, and the line says so rather than picking the commonest.
	byStatus := map[uint8]map[string]int{}
	for _, w := range waitsAtTheWall {
		if byStatus[w.Task.Status] == nil {
			byStatus[w.Task.Status] = map[string]int{}
		}
		if len(w.On) == 0 {
			byStatus[w.Task.Status]["(nothing names it)"]++
		}
		for _, s := range w.On {
			byStatus[w.Task.Status][s.Object.Type]++
		}
	}
	statuses := make([]int, 0, len(byStatus))
	for status := range byStatus {
		statuses = append(statuses, int(status))
	}
	sort.Ints(statuses)
	for _, status := range statuses {
		kinds := byStatus[uint8(status)] // #nosec G115 -- the keys came from a uint8
		parts := make([]string, 0, len(kinds))
		for kind, n := range kinds {
			parts = append(parts, fmt.Sprintf("%s x%d", kind, n))
		}
		sort.Strings(parts)
		verdict := ""
		if len(parts) == 1 {
			verdict = "  <- one type only, so the number names itself"
		}
		t.Logf("    status %d: %v%s", status, parts, verdict)
	}

	names := make([]string, 0, len(wall.on))
	for name := range wall.on {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Logf("    %-10s %s", name, wall.on[name])
	}

	t.Logf("VERDICT: neither UP nor box office moved the screen from %08X. The BOX has stopped "+
		"responding to input, so this is NOT a menu gate -- gort-slq. The highlight stopped after "+
		"%d moves, and at a different entry on another run, which fits an unresponsive box rather "+
		"than a disabled entry.", screen, moves)
}

// csiWindow is how long to watch the link for. It is the same budget on both sides of the
// comparison, because a count taken over two different windows is not a comparison.
const csiWindow = 2_000_000

// csiLink is what the hardware did over one window: the front-panel serial link the handset
// arrives on, the interrupt controller and timer that drive the guest's clock, and how many times
// the CPU actually entered its exception vector.
//
// THE EXCEPTION COUNT IS THE ONE THAT MATTERS and it is why this is not just a link counter. A
// task that stops being scheduled has stopped being woken, and on this box waking is an interrupt.
// Counting entries at 0x80000180 says whether interrupts are still arriving at all, which
// separates "the guest stopped handling them" from "they stopped coming".
type csiLink struct {
	reads, writes, dataIn int
	queued                int
	irq, timer            int
	exceptions            int
	// watchRead and watchWrite are the guest PCs that touched the watched control block, and how
	// often. A blocked pipe's producer and consumer are code, and this is what names them.
	watchRead, watchWrite map[uint32]int
	// block is the watched control block's words at the end of the window.
	block []uint32
}

func (l csiLink) String() string {
	return fmt.Sprintf("%d reads, %d writes (%d to the data register), %d queued; "+
		"%d interrupt-controller and %d timer accesses; %d exception entries",
		l.reads, l.writes, l.dataIn, l.queued, l.irq, l.timer, l.exceptions)
}

// csiTraffic runs the box for one window and counts what crosses the CSI's registers.
//
// A COUNT OF ZERO HERE IS A READING, NOT A BROKEN INSTRUMENT, so it says what it examined: if the
// observer saw no bus access at all then the hook is not installed and the zero is the harness.
func csiTraffic(t *testing.T, box *board.Runtime, transmitter *multiplex.Multiplex, window int,
	watch uint32,
) csiLink {
	t.Helper()
	link := csiLink{watchRead: map[uint32]int{}, watchWrite: map[uint32]int{}}
	var anyAccess int
	inWatch := func(a uint32) bool {
		return watch != 0 && a >= watch && a < watch+controlBlockWords*4
	}
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		// THE EXCEPTION VECTOR IS COUNTED AS A FETCH, not by sampling the PC between steps. The
		// PC-sampling version read zero on a box that was plainly taking interrupts, because the
		// vector is entered and left inside one step and is never the PC a caller sees.
		if a.Fetch && a.Virtual != 0x80000180 {
			return
		}
		anyAccess++
		switch {
		case a.Virtual == 0x80000180:
			link.exceptions++
		case inWatch(a.Virtual | 0x80000000):
			// The firmware quotes the same DRAM cached and uncached, so the watch has to match
			// both windows onto it or half the traffic is invisible.
			if a.Write {
				link.watchWrite[box.Machine.Core.State().PC&^1]++
				return
			}
			link.watchRead[box.Machine.Core.State().PC&^1]++
		case a.Virtual&^uint32(csi.Size-1) == csi.Base:
			if a.Write {
				link.writes++
				if a.Virtual&0xff == 0x10 {
					link.dataIn++
				}
				return
			}
			link.reads++
		case a.Virtual&^uint32(irq.Size-1) == irq.Base:
			link.irq++
		case a.Virtual >= hwtimer.Base && a.Virtual < hwtimer.Base+hwtimer.Size:
			link.timer++
		}
	}}
	for i := 0; i < window; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if anyAccess == 0 {
		t.Fatal("harness: the observer saw no bus access at all over the window, so its zero for " +
			"the link is the instrument rather than the link")
	}
	link.queued = box.CSI.Pending()
	if watch != 0 {
		for i := uint32(0); i < controlBlockWords; i++ {
			link.block = append(link.block, box.RAM.Read((watch&0x1fffffff)+i*4, bus.Word))
		}
	}
	return link
}

// controlBlockWords is how much of a watched control block to read and compare.
//
// IT IS TRIMMED TO THE BLOCK, and the first version was not. Reading 0x40 words past a pipe whose
// control block is under 0x50 bytes ran the dump into whatever sits after it, and printed twenty
// lines of a neighbouring structure's contents as though they were the pipe's -- plausible,
// orderly, and nothing to do with the subject.
const controlBlockWords = 0x14

// reportTheWatchedBlock says which words of a control block moved between the two windows, and
// which guest code touched it.
//
// THE COUNTERS AND THE CODE ANSWER DIFFERENT HALVES. A pipe nobody drains has counters that stop
// moving, which says the state is frozen; the PCs say WHO froze it, because a producer that is
// still running and a producer that has gone away look identical in the counters alone.
// The pipe control-block fields this port has established, by offset from the block.
//
// NOTHING HERE IS TAKEN FROM A NUCLEUS HEADER, and the fields NOT in this list matter as much as
// the ones in it. +0x30 holds 8011B80C, which the object census independently names as the
// semaphore EVQS0002, and +0x48 holds SMTEvts -- so this block carries pointers to its sibling
// objects where a bare control block would carry its buffer's end, and a reader that assumed the
// Nucleus layout would have called the semaphore a buffer pointer and got a length out of it.
const (
	pipeSize      = 0x1C / 4 // the buffer length in bytes; available equals it when nothing is queued
	pipeCount     = 0x20 / 4 // messages held: zero when available equals size, non-zero when it does not
	pipeUnit      = 0x24 / 4 // UNIDENTIFIED. It reads 32 and never moves, and it is not the message size
	pipeAvailable = 0x28 / 4 // bytes still free
	pipeRead      = 0x34 / 4 // the two move together and by the same step
	pipeWrite     = 0x38 / 4
	pipeWaiting   = 0x3C / 4 // tasks suspended on it
)

// readPipe states what a pipe control block says about itself.
//
// THE MESSAGE SIZE IS DERIVED, NOT LOOKED UP. Two fields sit beside the 160-byte size -- 32, which
// never moves, and one that reads 0 when the pipe is empty and 20 when it is full. A field that is
// exactly zero with nothing queued and exactly N with the buffer full is a COUNT of what is in it,
// which makes the messages 160/20 = 8 bytes and leaves the 32 unidentified. Taking the 32 for the
// message size instead gives five slots and no explanation of the other field at all, and it was
// the first reading here until the empty sample was put beside the full one.
//
// The finding does not rest on that either way: FULL is read from available reaching zero.
func readPipe(t *testing.T, name string, block []uint32, when string) {
	t.Helper()
	if len(block) <= pipeWaiting {
		t.Fatalf("harness: %s was read %d words, too few to hold the fields below", name, len(block))
	}
	if block[pipeAvailable] > block[pipeSize] || block[pipeSize] == 0 {
		t.Logf("    %s %s: %d bytes free of a declared %d, which cannot be -- read as UNIDENTIFIED "+
			"rather than named", name, when, block[pipeAvailable], block[pipeSize])
		return
	}
	state := fmt.Sprintf("%d of %d bytes free", block[pipeAvailable], block[pipeSize])
	switch block[pipeAvailable] {
	case 0:
		state = fmt.Sprintf("FULL -- not one of its %d bytes free", block[pipeSize])
	case block[pipeSize]:
		state = "empty"
	}
	held := "nothing"
	if block[pipeCount] > 0 {
		held = fmt.Sprintf("%d messages of %d bytes", block[pipeCount], block[pipeSize]/block[pipeCount])
	}
	if (block[pipeCount] == 0) != (block[pipeAvailable] == block[pipeSize]) {
		held = fmt.Sprintf("%d, which does not track the free count -- so it is NOT a message count",
			block[pipeCount])
	}
	t.Logf("    %s %s: %s, holding %s, %d tasks suspended on it, read %08X write %08X",
		name, when, state, held, block[pipeWaiting], block[pipeRead], block[pipeWrite])
}

func reportTheWatchedBlock(t *testing.T, name string, at uint32, idle, wall csiLink) {
	t.Helper()
	if len(idle.block) != len(wall.block) || len(wall.block) == 0 {
		t.Fatalf("harness: the watched block was read %d words while working and %d at the wall, so "+
			"there is nothing to compare", len(idle.block), len(wall.block))
	}
	// THE WHOLE BLOCK IS PRINTED, not only the words that moved. A field's meaning comes from what
	// it holds beside its neighbours -- a size against a count against a pointer pair -- and a diff
	// alone hands a reader four changed numbers with nothing to read them against.
	changed := 0
	for i := range wall.block {
		mark := "  "
		if idle.block[i] != wall.block[i] {
			mark, changed = "->", changed+1
		}
		t.Logf("    %s+0x%02X  %08X %s %08X", name, i*4, idle.block[i], mark, wall.block[i])
	}
	if changed == 0 {
		t.Logf("    NOT ONE WORD of %s at %08X changed between the working window and the wall, so "+
			"nothing moved through it in either direction", name, at)
	}
	readPipe(t, name, idle.block, "while working")
	readPipe(t, name, wall.block, "at the wall")
	for _, side := range []struct {
		when string
		link csiLink
	}{{"while working", idle}, {"at the wall", wall}} {
		touched := map[uint32]bool{}
		for pc := range side.link.watchRead {
			touched[pc] = true
		}
		for pc := range side.link.watchWrite {
			touched[pc] = true
		}
		pcs := make([]uint32, 0, len(touched))
		for pc := range touched {
			pcs = append(pcs, pc)
		}
		sort.Slice(pcs, func(a, b int) bool { return pcs[a] < pcs[b] })
		if len(pcs) == 0 {
			t.Logf("    %s %s: NOTHING touched it", name, side.when)
			continue
		}
		t.Logf("    %s %s: %d distinct PCs", name, side.when, len(pcs))
		for i, pc := range pcs {
			if i >= 12 {
				t.Logf("        ... and %d more", len(pcs)-i)
				break
			}
			t.Logf("        %08X  %d reads  %d writes", pc, side.link.watchRead[pc],
				side.link.watchWrite[pc])
		}
	}
}
