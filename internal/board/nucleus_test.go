package board_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

// A guest laid out the way Nucleus lays one out: three tasks on a circular created list, a
// semaphore and an event group, and a suspend block for each of the two waits.
//
// It is built by hand rather than taken from a real boot because the CONTROLS are the point. A
// scan over 32 MB of a running box will find something to say about every task; what says whether
// it is reading the machine is a task that waits on nothing and reports nothing, and a
// created-list link that is not mistaken for a suspension.
func nucleusGuest(t *testing.T) *board.Runtime {
	t.Helper()
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	name := func(off uint32, s string) {
		for i := range s {
			ram.Write(off+0x10+uint32(i), bus.Byte, uint32(s[i]))
		}
	}
	// Every created object opens with its list node -- previous, then next -- and both are
	// written because both are what tells a control block from a run of English text.
	block := func(off, previous, next, magic uint32, who string) {
		ram.Write(off, bus.Word, memory.DRAMBase+previous)
		ram.Write(off+4, bus.Word, memory.DRAMBase+next)
		ram.Write(off+0x0c, bus.Word, magic)
		name(off, who)
	}
	task := func(off, previous, next uint32, who string, status uint8, runs uint32) {
		block(off, previous, next, 0x5441534b, who) // "TASK"
		ram.Write(off+0x18, bus.Byte, uint32(status))
		ram.Write(off+0x1c, bus.Word, runs)
	}
	// The created list: SMTTask -> TASK20 -> IDLE -> SMTTask.
	task(0x1000, 0x8000, 0x2000, "SMTTask", 7, 10)
	task(0x2000, 0x1000, 0x8000, "TASK20", 7, 2583)
	task(0x8000, 0x2000, 0x1000, "IDLE", 0, 99)
	// Unlinked from that list, and so not on it at all.
	task(0x3000, 0x3000, 0x3000, "STALE", 7, 9000)

	// A STALE BYTE PAST THE TERMINATOR, which is what the real box has. Its TASK0 -- the control
	// block the record names for the smartcard stall -- holds "TASK0", a NUL, then a leftover '0'
	// from whatever the field said before. A reader that insists on a clean tail drops that task.
	ram.Write(0x2000+0x10+7, bus.Byte, uint32('9'))

	block(0x4000, 0x4000, 0x4000, 0x53454d41, "Periph")  // "SEMA"
	block(0x6000, 0x6000, 0x6000, 0x4556454e, "SMNEvts") // "EVEN"

	// Suspend blocks, which Nucleus builds on the waiting task's own stack: each names the object
	// it is queued on and the task that is queued, and the object's own suspension-list field
	// points back at it. BOTH HALVES ARE WRITTEN BECAUSE BOTH ARE REQUIRED -- one-way is what a
	// stale stack word looks like, and the control below is exactly that.
	suspend := func(off, object, tcb uint32) {
		ram.Write(object+0x18, bus.Word, memory.DRAMBase+off) // the object's suspension list
		ram.Write(off+0x08, bus.Word, memory.DRAMBase+object)
		ram.Write(off+0x0c, bus.Word, memory.DRAMBase+tcb)
	}
	suspend(0x5000, 0x4000, 0x2000) // TASK20 on Periph
	suspend(0x7000, 0x6000, 0x1000) // SMTTask on SMNEvts

	// THE STALE STACK. A task that has obtained and released a semaphore a thousand times leaves
	// the pointers lying in its stack frame, so a region naming both an object and a task is the
	// commonest thing in DRAM and means nothing at all. Nothing points at this one.
	ram.Write(0xa008, bus.Word, memory.DRAMBase+0x4000)
	ram.Write(0xa00c, bus.Word, memory.DRAMBase+0x8000)
	return &board.Runtime{RAM: ram}
}

func TestTaskStateReadsOnlyTheLiveCreatedList(t *testing.T) {
	box := nucleusGuest(t)
	status, runs, found, err := box.TaskState("TASK20")
	if err != nil || !found || status != 7 || runs != 2583 {
		t.Fatalf("TASK20 state = status %d runs %d found %v err %v", status, runs, found, err)
	}
	_, _, found, err = box.TaskState("STALE")
	if err != nil || found {
		t.Fatalf("unlinked stale task appeared: found %v err %v", found, err)
	}
}

func TestANameWithStaleBytesPastItsTerminatorIsStillTheName(t *testing.T) {
	_, _, found, err := nucleusGuest(t).TaskState("TASK20")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("a task whose name field carries a leftover byte past the terminator went missing, " +
			"which on the real box loses TASK0 and with it the whole smartcard finding")
	}
}

func TestTasksCarryTheAddressOfTheControlBlockTheyWereReadFrom(t *testing.T) {
	tasks, err := nucleusGuest(t).Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("created list has %d tasks, want 3", len(tasks))
	}
	at := map[string]uint32{}
	for _, task := range tasks {
		at[task.Name] = task.TCB
	}
	if at["TASK20"] != memory.DRAMBase+0x2000 {
		t.Errorf("TASK20's control block is at %08X, want %08X", at["TASK20"], memory.DRAMBase+0x2000)
	}
}

func TestTheObjectCensusReadsTypesRatherThanCheckingForOnesItExpects(t *testing.T) {
	objects, err := nucleusGuest(t).NucleusObjects()
	if err != nil {
		t.Fatal(err)
	}
	kind := map[string]string{}
	for _, o := range objects {
		kind[o.Name] = o.Type
	}
	// The magics here are never written down in the census -- it reports what it read, so these
	// are the values the guest stored coming back out.
	for name, want := range map[string]string{
		"Periph": "SEMA", "SMNEvts": "EVEN", "SMTTask": "TASK", "TASK20": "TASK",
	} {
		if kind[name] != want {
			t.Errorf("census reports %q as type %q, want %q", name, kind[name], want)
		}
	}
	// The unlinked task is a real control block and the census is a scan, so it is found here
	// even though the created-list walk skips it. That is the difference between the two.
	if kind["STALE"] != "TASK" {
		t.Errorf("the scan missed the unlinked control block, so it is not seeing every object")
	}
}

func TestTheObjectCensusRefusesWhenItsShapeIsWrong(t *testing.T) {
	ram, err := memory.NewRAM("dram", memory.DRAMSize)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&board.Runtime{RAM: ram}).NucleusObjects(); err == nil {
		t.Fatal("an empty guest censused clean; an instrument that finds nothing must say so " +
			"rather than report no semaphores")
	}
}

func TestTaskWaitsNamesTheObjectEachBlockedTaskIsQueuedOn(t *testing.T) {
	waits, err := nucleusGuest(t).TaskWaits()
	if err != nil {
		t.Fatal(err)
	}
	on := map[string][]string{}
	for _, w := range waits {
		for _, s := range w.On {
			on[w.Task.Name] = append(on[w.Task.Name], s.Object.Type+" "+s.Object.Name)
		}
	}
	for task, want := range map[string]string{"TASK20": "SEMA Periph", "SMTTask": "EVEN SMNEvts"} {
		if len(on[task]) != 1 || on[task][0] != want {
			t.Errorf("%s is waiting on %v, want exactly [%s]", task, on[task], want)
		}
	}
	// THE CONTROL, and it is the whole reason this walks from the object. IDLE waits on nothing,
	// but its control block is linked into the created list and a stale stack frame elsewhere in
	// DRAM names it beside a semaphore. A reader that scans for pointers to a task finds both and
	// reports a suspension that never happened -- which on the real box put TASK0 on six
	// semaphores at once through a single stack address, and read perfectly plausibly.
	if len(on["IDLE"]) != 0 {
		t.Errorf("IDLE waits on nothing and the census says %v, so stale pointers are being read "+
			"as suspensions", on["IDLE"])
	}
}
