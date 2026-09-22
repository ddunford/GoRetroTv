package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHAT THE GRID'S ROW CALLBACK ANSWERS.
//
// The enumeration does not ignore its row callback's result — it tests it:
//
//	local_60 = (**(code **)(DAT_800a4c90 + local_5c*0x2c + 0x24))(...)   the row callback
//	iVar2    = (*DAT_800a4ca0)(local_1c[0]);
//	if      ((local_60 == DAT_800a4ca4) && (iVar2 == 4))  { ...call it AGAIN... }
//	else if ((local_60 == DAT_800a4ca4) && (iVar2 != 4))  { ...give up on this row... }
//
// And `DAT_800a4ca4` is `0xFFFFFFFE`, minus two. So **a row callback that answers -2 is a row the
// enumeration abandons**, and the callback for the grid's selector is `0x800CB7B8`, invoked once per
// channel with the transport ready and producing no pixels.
//
// That makes one number decisive, and it is a number rather than a reading: what does `0x800CB7B8`
// return each time the grid calls it? Six answers of -2 would say the callback itself is refusing
// every row, and the question becomes what it tests. Six answers of something else would say the
// rows are accepted and lost later, which is a different search entirely.
//
// **It is measured at the RETURN, not at the call site, and that matters.** Reading a register when
// control merely leaves a range catches calls the function makes on its way and reports a live
// register as a result — this project filed exactly that mistake earlier today and had to withdraw
// it. So the callback's own `ra` is captured at its first instruction and the answer is read when
// control arrives back at that address.
//
// IT ASSERTS ITS OWN SUBJECT: the callback must actually be entered, or the absence of answers is
// the instrument and not the screen.
//
// IT ONLY READS.
func TestWhatTheGridsRowCallbackAnswers(t *testing.T) {
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
		readyFrom   = 6

		rowCallback = 0x800CB7B8 // entry 1 of the table at 0x80164978, the mode the grid selects
		abandonRow  = 0xFFFFFFFE // DAT_800a4ca4
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d", state())

	var (
		watching bool
		entered  int
		returnTo uint32
		answers  = map[uint32]int{}
		callers  = map[uint32]int{}
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		at := a.Virtual &^ 1
		if at == rowCallback {
			entered++
			st := box.Machine.Core.State()
			returnTo = st.GPR[31] &^ 1
			callers[returnTo]++
			return
		}
		// Back at the address the callback was told to return to: v0 is its answer.
		if returnTo != 0 && at == returnTo {
			answers[box.Machine.Core.State().GPR[2]]++
			returnTo = 0
		}
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching, entered, returnTo = true, 0, 0
			answers, callers = map[uint32]int{}, map[uint32]int{}
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
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	grid := openAllChannels(t, press, ".artifacts/callback-return-grid.png", false)
	if err := dumpScreen(t, box, "callback-return-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid drew %08X", grid)

	if entered == 0 {
		t.Fatalf("harness: the row callback at %08X was never entered while the grid drew, so the "+
			"absence of answers below is the instrument and not the screen", uint32(rowCallback))
	}
	t.Logf("the row callback at %08X was entered %d times", uint32(rowCallback), entered)
	for site, n := range callers {
		t.Logf("    called from %08X, %d times", site, n)
	}

	keys := make([]uint32, 0, len(answers))
	for v := range answers {
		keys = append(keys, v)
	}
	sort.Slice(keys, func(a, b int) bool { return answers[keys[a]] > answers[keys[b]] })
	abandoned := 0
	for _, v := range keys {
		note := ""
		if v == abandonRow {
			note = "  <- -2: THE ENUMERATION ABANDONS THIS ROW"
			abandoned += answers[v]
		}
		t.Logf("    answered %08X (%d), %d times%s", v, int32(v), answers[v], note) // #nosec G115
	}
	switch {
	case abandoned == entered:
		t.Logf("VERDICT: EVERY ONE of the %d calls answered -2, so the callback refuses every row "+
			"it is given. The rows are not lost after the callback -- the callback itself declines "+
			"them, and what it tests before declining is the whole remaining question.", entered)
	case abandoned > 0:
		t.Logf("VERDICT: %d of %d calls answered -2 and the rest did not, so the callback accepts "+
			"some rows and refuses others. What distinguishes the two sets is the next question.",
			abandoned, entered)
	default:
		t.Logf("VERDICT: NOT ONE call answered -2, so the callback accepts every row it is given " +
			"and the rows are lost somewhere after it. The enumeration is not the place to look.")
	}
}
