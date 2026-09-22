package board

import (
	"fmt"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
)

// Everything this port knows about the guest RTOS's own data structures lives here.
//
// The RTOS is Nucleus, and every control block it creates opens the same way: a twelve-byte list
// node, a four-character type magic, then an eight-byte NUL-padded name. One shape finds a task, a
// semaphore, an event group and a queue alike, which is the whole reason this is one file rather
// than a scan improvised wherever a question came up.
//
// IT ONLY EVER READS. The project's binding rule is that the box is driven through its modelled
// inputs; an instrument that writes to the guest to get an answer has changed the thing it was
// measuring. Reading is always allowed and this is what it is for.
//
// WHAT IT BUYS. "Every task is blocked" is not a finding, it is a restatement of the symptom. The
// record's smartcard section got past that by naming the objects -- TASK0 on semaphore "Periph",
// SMNTask on event group "SMNEvts" -- and the names came out of exactly these structures. A
// blocked box that can name what it is waiting for is a box you can go and satisfy.

// The control-block offsets. These are the ones the task census has read since it was written and
// the box answers to; they are not copied out of a Nucleus header this project does not have.
const (
	cbNext    = 0x04       // the created list's forward link, inside the leading list node
	cbID      = 0x0c       // the four-character type magic
	cbName    = 0x10       // eight bytes, NUL-padded, the name the firmware chose
	cbStatus  = 0x18       // a task's tc_status
	cbRuns    = 0x1c       // a task's own schedule count
	cbLen     = 0x70       // enough of a block to require mapped before reading one
	taskMagic = 0x5441534b // "TASK"

	// How much of a suspend block to read. Nucleus builds one on the SUSPENDING TASK'S OWN STACK
	// and links it into the object's suspension list, so it sits wherever that stack happens to be
	// and is found by what it points at rather than by its address. The blocks are a couple of
	// dozen bytes; this is generous rather than tuned, because a window too small reports nothing
	// and says nothing about why.
	suspendBlock = 0x20

	// How far into a control block to look for a suspension list. A task's own block is 0x70 of
	// structure, but the other object types are laid out differently and their list heads sit
	// further in -- a range that stopped at a task's length found the semaphore and event-group
	// waits and silently missed every pipe. The two-way linkage below is what keeps a wider window
	// honest: a field that is not a list head points at nothing that points back.
	objectFields = 0x100

	// How far along a suspension list to walk before deciding it is not one. Nucleus queues are
	// short; a list this long is memory that happens to link up.
	queueDepth = 64

	// The census must find at least this many named objects before it will believe its own scan.
	// A guest with tasks running has dozens.
	censusFloor = 4
)

// Task is one entry of the guest's live created-task list.
type Task struct {
	// TCB is the guest address of the control block this was read from. It is the handle for
	// asking what a blocked task is waiting for, which is why the census returns it.
	TCB uint32
	// Name is the task's own name, as the firmware spelled it.
	Name string
	// Status is Nucleus's tc_status. What the numbers mean was measured off the box rather than
	// taken from a header, by reading the TYPE of the object each suspended task is queued on:
	// 4 is a queue wait, 5 a pipe wait, 6 a semaphore wait, 7 an event-group wait, and 0 is
	// runnable. Four of those had exactly one object type against them across forty-two tasks.
	// A caller that meets a value not on that list prints it raw rather than naming it.
	Status uint8
	// Runs is the guest's own count of how many times the task has been scheduled.
	Runs uint32
}

// NucleusObject is a named kernel object found in guest RAM.
//
// THE TYPE MAGIC IS READ, NOT ASSUMED. Nothing here knows what a semaphore's magic is; the filter
// is only that the four bytes are upper-case letters and that a name field follows. So a census
// reports the type codes the box actually uses instead of confirming ones this port believes in --
// which is the difference between finding out and checking a guess, and it is how "SEMA", "EVNT",
// "PIPE", "QUEU" and "HISR" were learnt here rather than looked up.
//
// THE MAGIC AND THE NAME ARE NOT ENOUGH ON THEIR OWN, and finding that out is what the filter
// below is. Four upper-case letters followed by printable text is what ORDINARY ENGLISH looks
// like, and this firmware carries a lot of it: "AVAILABLE", "BACKGROUND", "SUBSCRIPTION". The
// census came back with eight hundred types. What separates a control block from a message is the
// twelve-byte list node in front of it -- a created object is ON a created list, so its first two
// words are pointers into DRAM, and no string has those.
type NucleusObject struct {
	// Address is the guest address of the control block.
	Address uint32
	// Type is the four-character magic as the guest stores it, e.g. "TASK".
	Type string
	// Name is the eight-byte name the firmware gave the object.
	Name string
}

// Suspension is one kernel object holding a block that names a particular task.
type Suspension struct {
	// Object is what the task is waiting on.
	Object NucleusObject
	// Block is the guest address of the structure naming both, which is normally inside the
	// waiting task's own stack.
	Block uint32
}

// TaskWait is one task and everything found waiting on its behalf.
type TaskWait struct {
	Task Task
	// On is what the task is suspended on. EMPTY IS A REAL ANSWER and means only that no object
	// names it: a task asleep on a timer, or suspended by hand, is blocked with nothing to find.
	On []Suspension
}

// Tasks returns the guest's live created-task list, in list order.
func (r *Runtime) Tasks() ([]Task, error) {
	var tasks []Task
	err := r.walkTasks(func(t Task) { tasks = append(tasks, t) })
	return tasks, err
}

// TaskState reads a named task from the guest's live created-task list.
// Status is Nucleus's tc_status, as Task documents; runs is the guest's own schedule count.
func (r *Runtime) TaskState(name string) (status uint8, runs uint32, found bool, err error) {
	err = r.walkTasks(func(t Task) {
		if t.Name == name {
			status, runs, found = t.Status, t.Runs, true
		}
	})
	return
}

// taskCount follows the guest's circular created-task list from SMTTask.
// A raw TASK magic scan would include stale or unlinked control blocks.
func (r *Runtime) taskCount() (int, error) {
	count := 0
	err := r.walkTasks(func(Task) { count++ })
	return count, err
}

func (r *Runtime) walkTasks(visit func(Task)) error {
	var seed uint32
	for off := uint32(0); off+0x38 <= r.RAM.Size(); off += 4 {
		if r.RAM.Read(off+cbID, bus.Word) != taskMagic {
			continue
		}
		if who, ok := controlBlockName(r.RAM, off); ok && who == "SMTTask" {
			seed = memory.DRAMBase + off
			break
		}
	}
	if seed == 0 {
		return nil
	}
	seen := make(map[uint32]bool)
	for at := seed; ; {
		if at < memory.DRAMBase || at-memory.DRAMBase+cbLen > r.RAM.Size() || at&3 != 0 {
			return fmt.Errorf("task census: created list left DRAM at %08X", at)
		}
		if seen[at] {
			return fmt.Errorf("task census: created list repeated %08X", at)
		}
		if len(seen) >= 200 {
			return fmt.Errorf("task census: created list exceeds 200 tasks")
		}
		seen[at] = true
		off := at - memory.DRAMBase
		if r.RAM.Read(off+cbID, bus.Word) != taskMagic {
			return fmt.Errorf("task census: created list points to non-task at %08X", at)
		}
		visit(Task{
			TCB:    at,
			Name:   taskNameAt(r.RAM, off),
			Status: uint8(r.RAM.Read(off+cbStatus, bus.Byte)), // #nosec G115 -- byte read.
			Runs:   r.RAM.Read(off+cbRuns, bus.Word),
		})
		next := r.RAM.Read(off+cbNext, bus.Word)
		if next == seed {
			return nil
		}
		at = next
	}
}

// NucleusObjects censuses every named kernel object in guest DRAM.
//
// IT CHECKS ITSELF AGAINST A KNOWN ANSWER. The created-task list is walked independently, by
// following links rather than by scanning, and every task it names must appear in the census. If
// one does not, the scan's shape is wrong and its silence about semaphores would be meaningless --
// so it returns a harness failure rather than a short list. With no tasks running there is no
// control to check against, and that is said rather than papered over.
func (r *Runtime) NucleusObjects() ([]NucleusObject, error) {
	var found []NucleusObject
	at := map[uint32]bool{}
	size := r.RAM.Size()
	inDRAM := func(v uint32) bool {
		return v >= memory.DRAMBase && v&3 == 0 && v-memory.DRAMBase+cbLen <= size
	}
	for off := uint32(0); off+cbName+8 <= size; off += 4 {
		// The created-list node first, because it is the cheap test and it rejects almost
		// everything: two DRAM pointers, which prose does not have.
		if !inDRAM(r.RAM.Read(off, bus.Word)) || !inDRAM(r.RAM.Read(off+cbNext, bus.Word)) {
			continue
		}
		id, ok := controlBlockType(r.RAM, off)
		if !ok {
			continue
		}
		name, ok := controlBlockName(r.RAM, off)
		if !ok {
			continue
		}
		addr := memory.DRAMBase + off
		found = append(found, NucleusObject{Address: addr, Type: id, Name: name})
		at[addr] = true
	}
	if len(found) < censusFloor {
		return nil, fmt.Errorf("nucleus census: only %d named objects in %d MB of DRAM, so the "+
			"control-block shape this scans for is wrong and its findings mean nothing",
			len(found), size>>20)
	}
	tasks, err := r.Tasks()
	if err != nil {
		return nil, fmt.Errorf("nucleus census: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("nucleus census: found %d objects but the guest has no tasks, so "+
			"there is no independent answer to check the scan against", len(found))
	}
	for _, t := range tasks {
		if !at[t.TCB] {
			return nil, fmt.Errorf("nucleus census: the created list names task %q at %08X and the "+
				"scan did not find it, so the scan is missing objects", t.Name, t.TCB)
		}
	}
	return found, nil
}

// TaskWaits names, for every task on the created list, the kernel objects it is queued on.
//
// IT WALKS FROM THE OBJECT, NOT FROM THE TASK, and the first version of this did the opposite.
// Scanning DRAM for words equal to a task's control block finds that task's suspend block -- and
// also every stale copy of the pointer left lying on a stack, of which a task that has been
// running for millions of instructions has dozens. It reported TASK0 queued on six semaphores at
// once, all of them through a single address inside a stack, which is not what a suspension looks
// like and was an artefact from end to end. It was also completely plausible, which is the point:
// the instrument has to rule that out, because the reader cannot.
//
// So the linkage is closed on both sides. The object must POINT AT the block, and the block must
// name BOTH the object and the task. A stale stack word satisfies neither half.
//
// THE WHOLE QUEUE IS READ, NOT JUST ITS HEAD. Only the first block is reachable directly from the
// object; the rest hang off it on the same list node the created lists use. An earlier version
// stopped at the head, and the cost was not a rounding error -- the event task was queued SECOND
// on a pipe and came back as "status 5, nothing names it", which reads as a task waiting on
// something unknown rather than as the second half of a pair on one object.
//
// The chain walk is bounded and stops at the first node that does not name a task, because a list
// that has been walked into the wrong memory does not announce itself.
func (r *Runtime) TaskWaits() ([]TaskWait, error) {
	tasks, err := r.Tasks()
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("task waits: the guest has no tasks, so there is nothing suspended")
	}
	objects, err := r.NucleusObjects()
	if err != nil {
		return nil, err
	}
	waiterAt := make(map[uint32]int, len(tasks))
	for i, t := range tasks {
		waiterAt[t.TCB] = i
	}

	size := r.RAM.Size()
	word := func(addr uint32) (uint32, bool) {
		if addr < memory.DRAMBase || addr&3 != 0 || addr-memory.DRAMBase+4 > size {
			return 0, false
		}
		return r.RAM.Read(addr-memory.DRAMBase, bus.Word), true
	}

	// namedIn is the index of the task a suspend block names, or -1 for a block that names none --
	// which is what stops a chain walk the moment it leaves the list.
	namedIn := func(block uint32) int {
		for b := uint32(0); b <= suspendBlock; b += 4 {
			v, ok := word(block + b)
			if !ok {
				return -1
			}
			if i, isTask := waiterAt[v]; isTask {
				return i
			}
		}
		return -1
	}

	on := make([][]Suspension, len(tasks))
	seen := make([]map[uint32]bool, len(tasks))
	for i := range seen {
		seen[i] = map[uint32]bool{}
	}
	for _, o := range objects {
		// A task is not something a task waits on, and every control block's leading node points
		// at its neighbours on the created list.
		if o.Type == "TASK" {
			continue
		}
		for field := uint32(cbName + 8); field < objectFields; field += 4 {
			head, ok := word(o.Address + field)
			if !ok {
				continue
			}
			// The head has to close the loop: it names the object AND a task.
			waiter, back := namedIn(head), false
			for b := uint32(0); b <= suspendBlock && !back; b += 4 {
				v, ok := word(head + b)
				back = ok && v == o.Address
			}
			if !back || waiter < 0 {
				continue
			}
			// Then the rest of the queue, which inherits the object from the head it hangs off.
			walked := map[uint32]bool{}
			for node := head; waiter >= 0 && !walked[node] && len(walked) < queueDepth; {
				walked[node] = true
				if !seen[waiter][o.Address] {
					seen[waiter][o.Address] = true
					on[waiter] = append(on[waiter], Suspension{Object: o, Block: node})
				}
				next, ok := word(node + cbNext)
				if !ok || next == head {
					break
				}
				node, waiter = next, namedIn(next)
			}
		}
	}

	waits := make([]TaskWait, 0, len(tasks))
	for i, t := range tasks {
		waits = append(waits, TaskWait{Task: t, On: on[i]})
	}
	return waits, nil
}

// controlBlockType reads the four-character type magic, or reports that there is not one there.
func controlBlockType(ram *memory.RAM, off uint32) (string, bool) {
	var id [4]byte
	for i := range id {
		c := byte(ram.Read(off+cbID+uint32(i), bus.Byte)) // #nosec G115 -- byte read
		if c < 'A' || c > 'Z' {
			return "", false
		}
		id[i] = c
	}
	return string(id[:]), true
}

// controlBlockName reads the eight-byte name field, and says whether what is there is one.
//
// A name is printable up to its terminator. THE BYTES AFTER THE TERMINATOR ARE NOT CHECKED, AND
// THAT IS MEASURED RATHER THAN LENIENT: the box's own TASK0, at the control block the record names
// for the smartcard stall, holds `54 41 53 4b 30 00 30 00` -- "TASK0", then a stale '0' left over
// from whatever the field held before. Requiring a clean tail dropped it, and with it the task the
// whole smartcard finding is about.
//
// An empty field is a name of "" and still a name: an object the firmware did not bother to name
// is an object, and refusing it here would make the census disagree with the created-task list,
// which is the one thing it checks itself against.
func controlBlockName(ram *memory.RAM, off uint32) (string, bool) {
	var name [8]byte
	for i := range name {
		c := byte(ram.Read(off+cbName+uint32(i), bus.Byte)) // #nosec G115 -- byte read
		if c == 0 {
			return string(name[:i]), true
		}
		if c < 0x20 || c > 0x7e {
			return "", false
		}
		name[i] = c
	}
	return string(name[:]), true
}

// taskNameAt is the name of a control block already known to be a task, for the created-list walk
// where the block's identity is established by its magic and its place on the list.
func taskNameAt(ram *memory.RAM, off uint32) string {
	name, _ := controlBlockName(ram, off)
	return name
}
