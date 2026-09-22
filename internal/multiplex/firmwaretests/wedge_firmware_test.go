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

// THE BOX MUST NOT WEDGE WHEN SOMEBODY USES THE MENU.
//
// It used to. Three or four presses of the TV GUIDE menu and the box stopped responding to the
// handset ALTOGETHER -- not the menu, the box: UP died too, and so did `box office`, which leaves
// the menu entirely. The cause is recorded in full and was this port's own: the event queue
// EVQP0002 is drained by one task, SMTTask, and every key press made the box send two commands the
// modelled card did not answer, so SMTTask waited out its fifty-tick timeout for each one, the
// queue filled, and SMTTask ended up suspended trying to add to the very pipe it alone drains.
// Answering those two codes (internal/device/csi.DefaultAckPolicy) fixed it.
//
// SO THIS IS NOW A GUARD, AND IT IS DELIBERATELY NOT A NARROW ONE. It presses the menu forty times
// and asserts the box is still answering at the end, because the failure it exists to catch is not
// "the ack policy changed" -- it is "something, anywhere, starved the task that drains the event
// queue". A test that only checked the policy would pass while the box wedged for a new reason.
//
// ON FAILURE IT DIAGNOSES RATHER THAN JUST REPORTING A COUNT. Everything that was needed to find
// this the first time is kept and runs when the guard trips: what every task is waiting on, which
// of them moved since the box was idle, the pipe's own counters, and which code and which TASK
// last touched it. That is a day's work the next person does not have to repeat, and it is the
// reason this file is long.
func TestTheBoxDoesNotWedgeWhenTheMenuIsUsed(t *testing.T) {
	const watched = "EVQP0002"
	// Far more than the three or four the wedge used to survive, and more than the ten entries the
	// menu has, so the highlight wraps and keeps going. Not more than that, because this package
	// runs under the race detector against a thirty-minute cap it has hit before.
	const presses = 20
	// THE MOVE COUNT IS A BACKSTOP AND NOT THE MAIN CHECK, because it measures repaint timing as
	// much as health: a press that lands while the menu is painting is swallowed, which is an
	// ordinary and separately tracked behaviour, and a healthy box scores around eighteen of
	// forty here rather than forty. The wedge allowed three or four and then NOTHING, so ten
	// separates them cleanly without the guard becoming a test of the settle budget.
	const mustMove = 8

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

	// THE CONTROL, TAKEN BEFORE ANYTHING IS PRESSED, and it is kept even though the box no longer
	// wedges: "every task is blocked" is what an IDLE box looks like too, and without this reading
	// beside it the diagnosis below would call a resting box a deadlock.
	baseline := rtosStateAt(t, box, "idle, before any key")
	pipe := pipeNamed(t, box, watched)
	t.Logf("watching %s at %08X", watched, pipe)
	idleLink := csiTraffic(t, box, transmitter, csiWindow, pipe)
	idleAfter := readRTOS(t, box)
	t.Logf("hardware while idle: %s", idleLink)
	t.Logf("tasks scheduled over %d idle instructions: %v", csiWindow, scheduled(baseline, idleAfter))

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

	// THE PIPE IS WATCHED THROUGHOUT, and this is the real check. The wedge is not "the menu
	// stopped" -- that was the symptom that misled the first three readings of it -- it is the
	// event queue filling because the one task that drains it got starved. Watching the queue's
	// own counters catches that at its cause, and catches it even on a run where the screen
	// happens to keep up.
	countAt := (pipe & 0x1fffffff) + uint32(pipeCount)*4  // #nosec G115 -- a small constant index
	waitAt := (pipe & 0x1fffffff) + uint32(pipeWaiting)*4 // #nosec G115 -- a small constant index
	var deepest, everWaiting, countWrites uint32
	hooks = board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write {
			return
		}
		switch a.Virtual & 0x1fffffff {
		case countAt:
			countWrites++
			if a.Value > deepest {
				deepest = a.Value
			}
		case waitAt:
			if a.Value > everWaiting {
				everWaiting = a.Value
			}
		}
	}}
	moves := 0
	for attempt := 0; attempt < presses; attempt++ {
		if _, s := press(0x59, "", 4_000_000); s != 0 {
			moves++
		}
	}
	capacity := box.RAM.Read((pipe&0x1fffffff)+uint32(pipeSize)*4, bus.Word) // #nosec G115
	t.Logf("the highlight moved on %d of %d presses; %s reached %d messages deep with at most %d "+
		"tasks suspended on it, its count written %d times", moves, presses, watched, deepest,
		everWaiting, countWrites)
	// A COUNT OF ZERO WOULD MEAN THE WATCH MISSED, not that the queue was quiet: every press puts
	// messages through this queue by construction, so a silent counter is the instrument failing.
	if countWrites == 0 {
		t.Fatalf("harness: nothing wrote %s's message count across %d presses, so the watch is on "+
			"the wrong address and every zero below is the instrument", watched, presses)
	}

	// AND THE BOX MUST STILL TAKE A KEY THAT LEAVES THE MENU, which is the check that separates a
	// menu declining to redraw from a box that has stopped running. It was the reading that
	// withdrew "the menu will not pass entry 6" in the first place.
	//
	// RETRIED, BECAUSE ONE PRESS IS NOT THE PROPERTY. A press landing while the menu is painting is
	// swallowed -- that is why the highlight moves on roughly three presses in five -- so a single
	// box-office press has a real chance of doing nothing on a perfectly healthy box, and asserting
	// on it makes this guard flaky in the one direction a guard must never be: crying wolf about a
	// wedge that is not there. It cost one false failure before it was noticed. The property is
	// that the box still TAKES such a key, and three tries is far short of the "nothing at all
	// responds, ever again" the wedge produced.
	escape := uint32(0)
	for attempt := 1; attempt <= 3 && escape == 0; attempt++ {
		_, escape = press(0x7D, fmt.Sprintf("box office, after all that (try %d)", attempt), 12_000_000)
	}

	full, waiting := pipeStateNow(box, pipe)
	switch {
	case everWaiting > 0 || full:
		t.Errorf("%s had %d tasks suspended on it during the presses and is %sfull now, reaching "+
			"%d of its %d messages. That is the wedge at its cause, whether or not the screen "+
			"kept up: the one task that drains this queue is being starved", watched, everWaiting,
			map[bool]string{true: "", false: "not "}[full], deepest, capacity/8)
	case escape == 0:
		t.Error("the menu still moves but box office no longer redraws, so the box has stopped " +
			"taking keys that leave the menu entirely -- which is how the wedge first showed")
	case moves < mustMove:
		t.Errorf("the highlight moved on only %d of %d presses, and the queue looks healthy. The "+
			"box used to stop after three or four; something is swallowing presses well beyond "+
			"the repaint window", moves, presses)
	default:
		t.Logf("the box was still answering after %d presses, and %s never went past %d messages "+
			"with %d tasks suspended on it", presses, watched, deepest, waiting)
		return
	}
	diagnoseTheWedge(t, box, transmitter, baseline, idleLink, watched, pipe)
}

// pipeStateNow reads whether a pipe is full and how many tasks are suspended on it.
func pipeStateNow(box *board.Runtime, pipe uint32) (full bool, waiting uint32) {
	at := func(index int) uint32 {
		return box.RAM.Read((pipe&0x1fffffff)+uint32(index)*4, bus.Word) // #nosec G115 -- small index
	}
	return at(pipeAvailable) == 0, at(pipeWaiting)
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

// diagnoseTheWedge asks the RTOS what it is doing and what it is waiting for, and says what
// CHANGED since the box was idle and working.
//
// It runs only when the guard above trips, and it is the whole of how the original wedge was
// found: the task census against its idle control, the pipe's own counters, which code touched it
// and -- the reading that named the deadlock -- which TASK was running at each of those accesses.
func diagnoseTheWedge(t *testing.T, box *board.Runtime, transmitter *multiplex.Multiplex,
	baseline rtosState, idleLink csiLink, watched string, pipe uint32,
) {
	t.Helper()

	wallBefore := readRTOS(t, box)
	waitsAtTheWall, err := box.TaskWaits()
	if err != nil {
		t.Fatal(err)
	}
	wallLink := csiTraffic(t, box, transmitter, csiWindow, pipe)
	wall := readRTOS(t, box)
	t.Logf("RTOS now: TCD_Execute_Task=[0x801072B0]=%08X ready=[0x801072D8]=%08X",
		wall.exec, wall.ready)
	t.Logf("the box retired %d further instructions, so it is running rather than halted", csiWindow)

	// THE LINK THE HANDSET ARRIVES ON. The card is the clock master on this wire and the model
	// follows the firmware: a queued byte is presented only once the guest has written one back,
	// because that is how the guest shifts its own frame out. So if the guest ever stops writing
	// to the data register while something is queued, the link goes silent in BOTH directions --
	// no key byte, and no idle byte either, because a waiting queue takes priority over idle. That
	// is a stall this port could cause rather than a firmware defect, so it is measured before
	// anything upstream is blamed.
	t.Logf("hardware while idle:    %s", idleLink)
	t.Logf("hardware at the moment: %s", wallLink)

	t.Logf("%s at %08X, the idle window against this one:", watched, pipe)
	reportTheWatchedBlock(t, watched, pipe, idleLink, wallLink)

	// WHICH TASKS STILL RUN. Status is a state and a state can be one a task passes through every
	// few thousand instructions; the run count says whether it is passing through at all. A task
	// frozen in status 5 and a task cycling through status 5 look identical in one sample and
	// nothing alike in two.
	t.Logf("tasks scheduled over %d instructions NOW: %v", csiWindow, scheduled(wallBefore, wall))
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
	// watchBy is which TASK was running at each of those accesses. The PCs say what the code does;
	// only this says who runs it, and "which task drains this pipe" is the question the PCs cannot
	// answer -- a send and a receive of the same pipe from the same task is a self-deadlock, and
	// from two tasks it is an ordinary one. They need different fixes.
	watchBy map[string]int
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
	link := csiLink{
		watchRead: map[uint32]int{}, watchWrite: map[uint32]int{}, watchBy: map[string]int{},
	}
	var anyAccess int
	// TCD_Execute_Task points at the control block of the task on the CPU, and the created-list
	// walk already turns a control block into a name.
	tasks, err := box.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	named := make(map[uint32]string, len(tasks))
	for _, task := range tasks {
		named[task.TCB] = task.Name
	}
	whoIsRunning := func() string {
		exec := box.RAM.Read(0x801072B0&0x1fffffff, bus.Word)
		if name, ok := named[exec]; ok {
			return name
		}
		if exec == 0 {
			return "(no task running)"
		}
		return fmt.Sprintf("(unknown TCB %08X)", exec)
	}
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
			pc := box.Machine.Core.State().PC &^ 1
			if a.Write {
				link.watchWrite[pc]++
			} else {
				link.watchRead[pc]++
			}
			link.watchBy[whoIsRunning()+" "+pipeSide(pc)]++
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

// The two halves of the guest's pipe driver, read out of the firmware image with tools/disasm.sh
// and named by what they do to the message count rather than by any symbol.
//
//	0x800CEBA0  SEND     ...  at 0x800CED50 it loads the count, adds one, stores it back, and
//	                          every store to the free-byte field in it SUBTRACTS
//	0x800CED68  RECEIVE  ...  at 0x800CEEDE it loads the count, subtracts one, stores it back, and
//	                          every store to the free-byte field in it ADDS
//
// Both are reached through checking wrappers -- 0x800CF1A4 tail-calls the first and 0x800CF218 the
// second, each via a word in its own literal pool -- and those wrappers validate the caller's
// pointer against the four-character "PIPE" id at 0x800CF294, which is the same magic the object
// census finds by scanning. Two independent routes to the same constant.
//
// WHY THE SPLIT MATTERS MORE THAN IT LOOKS. Counting control-block reads against writes does NOT
// separate sending from receiving: both halves read most of the block and write a few words of it,
// so a task that only ever receives still shows up "WRITING" the block. The first attribution here
// did exactly that and made every task look like it did both.
const (
	pipeSendWorker    = 0x800CEBA0
	pipeReceiveWorker = 0x800CED68
	pipeDriverEnd     = 0x800CF180
)

// pipeSide says which half of the driver a program counter is in.
func pipeSide(pc uint32) string {
	switch {
	case pc >= pipeSendWorker && pc < pipeReceiveWorker:
		return "SENDING to"
	case pc >= pipeReceiveWorker && pc < pipeDriverEnd:
		return "RECEIVING from"
	default:
		return "otherwise touching"
	}
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
// The pipe control-block fields, every one of them read off the driver's own instructions rather
// than out of a Nucleus header.
//
//	+0x0C  the "PIPE" id, which the wrapper at 0x800CF1BC compares against 0x800CF294
//	+0x18  first byte: 0 for variable-size messages, which this pipe is
//	+0x1C  buffer length in bytes
//	+0x20  messages held      send adds one at 0x800CED50, receive takes one at 0x800CEEDE
//	+0x24  the message size   the wrapper checks the caller's length against it at 0x800CF1E4
//	+0x28  bytes free         send subtracts, receive adds
//	+0x2C  buffer start       the wrap-around target in both halves
//	+0x30  buffer end         the bound both halves compare against
//	+0x34  read pointer       written only by receive, at 0x800CEED4
//	+0x38  write pointer      written only by send, at 0x800CED46
//	+0x3C  tasks suspended    send adds one at 0x800CEC0C when there is no room
//	+0x44  the suspension list, whose address send passes at 0x800CEC32
//
// A CORRECTION LIVES HERE, because it was a confident wrong reading and the shape of it recurs.
// This comment previously said the block carries pointers to its sibling objects where a bare
// control block carries its buffer's end, on the evidence that +0x30 holds 8011B80C and the object
// census names that the semaphore EVQS0002. Both halves were true and the conclusion was not.
// +0x30 IS the buffer's end; the buffer simply runs up to the composite structure that ENCLOSES
// the pipe -- semaphore at +0x00, pipe control block at +0x28 -- and the fields past +0x3C in the
// dump belong to that structure rather than to the pipe, because the window is wider than the
// block. Two addresses that coincide are not a pointer, and adjacency is the commonest way for a
// memory dump to look like a design.
const (
	pipeSize      = 0x1C / 4
	pipeCount     = 0x20 / 4
	pipeUnit      = 0x24 / 4
	pipeAvailable = 0x28 / 4
	pipeRead      = 0x34 / 4
	pipeWrite     = 0x38 / 4
	pipeWaiting   = 0x3C / 4
)

// readPipe states what a pipe control block says about itself.
//
// THE TWO SIZE-LOOKING FIELDS ARE BOTH REAL AND MEAN DIFFERENT THINGS, which is why reading one
// of them as "the message size" kept giving an arithmetic that did not close. +0x24 is the
// message size the API validates against -- and this pipe is VARIABLE-SIZE, its flag byte zero, so
// that is a MAXIMUM rather than a stride. +0x20 is how many messages are in it. The messages
// actually being sent are four bytes stored in eight, so twenty of them fill the 160-byte buffer
// exactly, and the 32 is the largest one that would have been allowed rather than the size of any
// that were sent.
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
		held = fmt.Sprintf("%d messages averaging %d bytes, against a %d-byte maximum",
			block[pipeCount], block[pipeSize]/block[pipeCount], block[pipeUnit])
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
	readPipe(t, name, idle.block, "while idle")
	readPipe(t, name, wall.block, "now")
	for _, side := range []struct {
		when string
		link csiLink
	}{{"while idle", idle}, {"now", wall}} {
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
		by := make([]string, 0, len(side.link.watchBy))
		for who := range side.link.watchBy {
			by = append(by, who)
		}
		sort.Strings(by)
		for _, who := range by {
			t.Logf("    %s %s: %s it, %d accesses", name, side.when, who, side.link.watchBy[who])
		}
		t.Logf("    %s %s: %d distinct PCs", name, side.when, len(pcs))
		for i, pc := range pcs {
			if i >= 200 {
				t.Logf("        ... and %d more", len(pcs)-i)
				break
			}
			t.Logf("        %08X  %d reads  %d writes", pc, side.link.watchRead[pc],
				side.link.watchWrite[pc])
		}
	}
}
