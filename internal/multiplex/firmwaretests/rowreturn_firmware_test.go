package firmwaretests_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT DOES THE GRID'S O-CODE GET TOLD WHEN THE ENUMERATION RETURNS?
//
// The position is precise and every link in it is measured. The ALL CHANNELS grid has a loop of its
// own at 0x800A4B60 — three other screens that draw rows of text reach it zero times. With the
// transport ready that loop enumerates all six channels, its body does four thousand instructions
// of work per channel, it writes 178 distinct words, and those words are read back 1,622 times by
// the 0x800D* subsystem. And the drawing surface takes exactly the same number of writes as when
// the loop runs once: 42062, 38914, 26893, 20192, identical to the digit.
//
// The body never enters the drawing module at 0x8009xxxx, so it was never going to paint. **It is
// the database side, and the screen that draws is interpreted o-code.** Which makes one question
// the whole thing now turns on:
//
//	when this enumeration finishes and returns, what does its caller receive?
//
// A function that walks six channels and hands back "none" explains the screen completely, and
// explains it without any missing table — which matches the other measurements, because opening the
// grid changes nothing about what the box asks the broadcast for and all sixteen match units are
// byte-identical before and after.
//
// So this waits for the loop to finish its six iterations, then TRACES OUTWARD until control leaves
// the enumeration entirely, and records where it went and what was in v0 when it got there. A
// return value is a fact; reading the function and choosing an exit is how this project got the
// resolver's answer wrong (the literal pool said 0xFFFFFFFB and the box returned 2, because the arm
// that ran was not the arm that looked like the exit).
//
// IT ASSERTS ITS OWN SUBJECT: the loop must complete its six iterations, or this measures the
// one-iteration behaviour; and control must actually leave, or the value reported is a register
// caught mid-function.
//
// IT ONLY READS.
func TestWhatTheGridsEnumerationReturns(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		wantKind    = 14
		readyFrom   = 6

		loopHead  = 0x800A4B60
		bodyStart = 0x800A4BBE
		// The enumeration's own module.
		//
		// **LEAVING IT IS NOT THE SAME AS RETURNING FROM IT, and the first version of this probe
		// got that wrong.** It stopped at the first fetch outside the module, found 0x800A3A10,
		// read v0 as the return value and was about to report that the enumeration "hands the
		// o-code a list with none of our channels in it". Both halves were false: 0x800A4DFC is
		// `lw v1,0x800a501c` followed by `jalr v1`, so control CALLED OUT rather than returned,
		// 0x800A3A10 is a thunk, and v0 was a live register that happened to hold a pointer into
		// the o-code interpreter's own frame -- the dump was full of 0xFFFFFFFD markers and a
		// flash address at 0x9FC77400, which is o-code, not channels.
		//
		// A return is where control leaves the module WITH THE FRAME POPPED. MIPS stacks grow
		// down, so a call from inside pushes sp lower and only a return restores it above the
		// value it held while the loop was running. That is the test below.
		moduleLo = 0x800A4000
		moduleHi = 0x800A5000
		tailLen  = 4000
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if kind := box.RAM.Read(off, bus.Word); kind != wantKind {
		t.Fatalf("harness: %08X holds kind %d, not %d -- the transport object has moved",
			uint32(transportAt), kind, wantKind)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d", state())

	var (
		watching   bool
		iterations int
		bodies     int
		tracing    bool
		settled    bool
		tail       []uint32
		spAtLoop   uint32
		leftAt     uint32
		returnedTo uint32
		returnV0   uint32
		returnV1   uint32
		returnA0   uint32
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch || settled {
			return
		}
		at := a.Virtual &^ 1
		switch at {
		case bodyStart:
			bodies++
		case loopHead:
			iterations++
			if spAtLoop == 0 {
				spAtLoop = box.Machine.Core.State().GPR[29]
			}
			// Six bodies have run and the head has come round to decide against a seventh: the
			// enumeration is finished and everything from here is its return path.
			if bodies >= 6 && !tracing {
				tracing = true
			}
		}
		if !tracing {
			return
		}
		tail = append(tail, at)
		if at < moduleLo || at >= moduleHi {
			// Outside the module AND the frame popped: a return, not a call out.
			if st := box.Machine.Core.State(); st.GPR[29] > spAtLoop {
				leftAt, returnedTo = tail[max(0, len(tail)-2)], at
				returnV0, returnV1, returnA0 = st.GPR[2], st.GPR[3], st.GPR[4]
				settled = true
			}
			return
		}
		if len(tail) >= tailLen {
			settled = true
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching, tracing, settled = true, false, false
			iterations, bodies, tail, spAtLoop = 0, 0, tail[:0], 0
		}
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, drew := 0, before, uint32(0)
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
				drew = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		watching = false
		t.Logf("%-32s drew %08X", name, drew)
		return drew
	}

	grid := openAllChannels(t, press, ".artifacts/row-return.png", false)
	if err := dumpScreen(t, box, "row-return.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid drew %08X, transport state %d", grid, state())

	if bodies < 6 {
		t.Fatalf("harness: the row body ran %d times, not six, so the loop did not complete its "+
			"enumeration and this measured the wrong behaviour", bodies)
	}
	if !settled {
		t.Fatal("harness: control never left the enumeration within the traced tail, so the " +
			"registers below would be caught mid-function rather than at a return")
	}
	t.Logf("the loop head was reached %d times and the body ran %d times", iterations, bodies)
	t.Logf("the enumeration RETURNED (frame popped) from %08X to %08X", leftAt, returnedTo)
	t.Logf("  v0 = %08X (%d)   v1 = %08X (%d)   a0 = %08X",
		returnV0, int32(returnV0), returnV1, int32(returnV1), returnA0) // #nosec G115
	// WHAT IT HANDED BACK, now that this really is a return.
	if at := returnV0 & 0x1fffffff; returnV0 >= 0x80000000 && at+256 <= box.RAM.Size() {
		t.Logf("the returned pointer %08X holds:", returnV0)
		for row := uint32(0); row < 128; row += 16 {
			t.Logf("    +%3d  %08X %08X %08X %08X", row,
				box.RAM.Read(at+row, bus.Word), box.RAM.Read(at+row+4, bus.Word),
				box.RAM.Read(at+row+8, bus.Word), box.RAM.Read(at+row+12, bus.Word))
		}
		// And the announced channel numbers, hunted for across a generous window either side,
		// because a list is as likely to be a vector of pointers as a block of records.
		listings := guide.On(day)
		mine := map[uint16]string{}
		for i := range listings.Services {
			mine[listings.Services[i].Channel] = listings.Services[i].Name
		}
		lo := uint32(0)
		if at > 0x800 {
			lo = at - 0x800
		}
		hi := at + 0x800
		if hi > box.RAM.Size() {
			hi = box.RAM.Size()
		}
		found := 0
		for o := lo; o+2 <= hi; o += 2 {
			v := uint16(box.RAM.Read(o, bus.Half)) // #nosec G115 -- half read
			if name, ours := mine[v]; ours {
				t.Logf("    %08X = %d (%s)%s", 0x80000000|o, v, name,
					map[bool]string{true: "  <- at the returned pointer", false: ""}[o == at])
				found++
			}
		}
		if found == 0 {
			t.Logf("VERDICT: NOT ONE of our six channel numbers is within 2 KB of the pointer the " +
				"enumeration returned. It walked all six channels and handed back a list that does " +
				"not contain them, so the o-code is drawing nothing because it was given nothing.")
		} else {
			t.Logf("VERDICT: %d of our channel numbers are in the returned list's neighbourhood, so "+
				"the enumeration DOES hand the o-code our channels and the fault is in what the "+
				"o-code then does with them.", found)
		}
	}

	t.Logf("the %d instructions from the end of the enumeration to that point:", len(tail))
	var line []string
	for i, at := range tail {
		line = append(line, hexPC(at))
		if len(line) == 8 || i == len(tail)-1 {
			t.Logf("    %s", strings.Join(line, " "))
			line = line[:0]
		}
	}
	t.Logf("the distinct regions it passed through, in order:")
	prev := uint32(0)
	for _, at := range tail {
		if prev != 0 && (at > prev+64 || prev > at+64) {
			t.Logf("    %s -> %s", hexPC(prev), hexPC(at))
		}
		prev = at
	}
}
