package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH PID CARRIES TABLE 0xC1, ASKED OF THE BOX RATHER THAN DEDUCED.
//
// The box holds ninety-two subscriptions for table 0xC1 -- 0x0000, every letter 'A'..'Z', 0x00FF
// and the sixty-four genre extensions -- and this port has never transmitted one of them. 0xC1 is
// the index behind the A-Z LISTINGS screens and the genre screens, so it is the largest single
// thing in the signal that nobody has fed.
//
// THE PID CANNOT BE READ OFF THE SUBSCRIPTION TREE, and finding that out was most of the work. Each
// subscription's PID node carries a field in 0x10..0x1F that looks exactly like a PID on a box
// whose PIDs are 0x10, 0x11 and 0x14 -- and it is not one. The tree walk reports table 0xA3 under
// "PID 0x17" while the box's own match unit asks for that table on PID 0x33, which is the check
// that caught it; the demux's armed channels are numbered 19..24, so it is not a filter handle
// either. What that field IS remains unestablished.
//
// Eliminating the five tables whose PIDs are known (0x40 on 0x10, 0x42/0x4A on 0x11, 0x73 on 0x14,
// 0xA3 on 0x33, 0xA0 on 0x34) leaves 0xC1 on the sixth armed PID, 0x52 -- but that is deduction,
// and deduction is what produces a plausible wrong answer on this project. **So this asks the box.**
// One section is pushed at each armed PID in turn and the question is simply whether the 0xC1
// parser runs. The parser either executes or it does not; there is nothing to interpret.
//
// IT ASSERTS ITS OWN SUBJECT TWICE OVER. The section is built by the shipping builder, so a run
// where NO pid reaches the parser is reported as a harness failure rather than as "the box does not
// want 0xC1" -- and the parser's address is proved live by the run before any verdict is read off
// it.
func TestWhichPIDCarriesTheIndexTable(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		// The function decompiled at 0x800C4C34: it reads the count, allocates count*10+12, fills
		// ten-byte records and then branches on the extension. The window stops short of
		// 0x800C4F94, which is the separate free-the-list routine.
		parserFrom  = 0x800C4C34
		parserTo    = 0x800C4F00
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered, so the box has not acquired and "+
			"its subscriptions are not built yet", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}

	// A section the consumer would dispatch, built by the shipping builder so this also proves the
	// builder produces something the box's own parser accepts.
	letter, err := broadcast.IndexLetter('A')
	if err != nil {
		t.Fatal(err)
	}
	section, err := broadcast.IndexSection(letter, 0, 0, 0, []broadcast.IndexRecord{
		{ID: 0x0065, Packed: 0x0f, Selector: 0xc0},
		{ID: 0x01f5, Packed: 0x0f, Selector: 0xc0},
	})
	if err != nil {
		t.Fatal(err)
	}

	filters := box.Demux.ArmedFilters()
	if len(filters) == 0 {
		t.Fatal("harness: the box has no armed filter channels at all, so there is nowhere to push")
	}
	pids := make([]uint16, 0, len(filters))
	seen := map[uint16]bool{}
	for _, f := range filters {
		if !seen[f.PID] {
			seen[f.PID] = true
			pids = append(pids, f.PID)
		}
	}
	sort.Slice(pids, func(a, b int) bool { return pids[a] < pids[b] })
	t.Logf("the box has %d armed PIDs: %#04x", len(pids), pids)

	inParser := 0
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Fetch {
			return
		}
		if pc := a.Virtual &^ 1; pc >= parserFrom && pc < parserTo {
			inParser++
		}
	}}

	reached := map[uint16]int{}
	for _, pid := range pids {
		if err := box.Demux.Push(pid, section); err != nil {
			t.Logf("  PID %#04x  push refused: %v", pid, err)
			continue
		}
		inParser = 0
		for i := 0; i < 8_000_000; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
		}
		reached[pid] = inParser
		verdict := "the 0xC1 parser did not run"
		if inParser > 0 {
			verdict = "THE 0xC1 PARSER RAN"
		}
		t.Logf("  PID %#04x  %6d instructions inside the parser  -- %s", pid, inParser, verdict)
	}

	var carriers []uint16
	for pid, n := range reached {
		if n > 0 {
			carriers = append(carriers, pid)
		}
	}
	sort.Slice(carriers, func(a, b int) bool { return carriers[a] < carriers[b] })
	if len(carriers) == 0 {
		t.Fatalf("harness: a section the consumer dispatches was pushed at every one of the %d "+
			"armed PIDs %#04x and the parser at %#08X never executed. Either the parser is not at "+
			"that address or the section never reached the guest -- EITHER WAY this says nothing "+
			"about which PID carries table 0xC1, and must not be filed as one",
			len(pids), pids, uint32(parserFrom))
	}
	t.Logf("TABLE 0xC1 IS CARRIED ON PID(S) %#04x -- measured, not deduced", carriers)
	if len(carriers) > 1 {
		t.Logf("more than one PID reached the parser, so the box accepts 0xC1 on several; the " +
			"transmitter should use the lowest unless something else distinguishes them")
	}
}
