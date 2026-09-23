package firmwaretests_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// A VIEWER'S FIRST KEY PRESS MUST LAND, HOWEVER LONG THE BOX HAS BEEN SITTING THERE.
//
// It did not, and on the live demo that was every visit: press a key, nothing happens, press it
// again, the menu appears (gort-4sx.firstkey). The key REACHED the hardware both times -- the wire
// drained and the guest read the data register -- so every early theory pointed downstream, at a
// task asleep or an event queue not being drained.
//
// IT WAS NONE OF THOSE. It was this port cutting the key frame in half. Card replies and handset
// frames share one wire and one framing, and the rule that stops a reply splicing into a frame
// asked "was the last byte non-zero" -- but a key frame CONTAINS a zero, escaped as `1b 00`, so at
// its fifth byte the guard fell open and a waiting reply walked in:
//
//	immediate   in  05 80 02 1b 00 05 a0 00                  the frame, intact
//	after idle  in  05 80 02 1b 00 | 02 2b 18 00 | 05 a0 00   a heartbeat ack, spliced in
//
// The guest de-framed two corrupt messages and no key at all. A reply only has to be PENDING when
// the frame reaches its escape, which is why a box idle long enough to have exchanged a heartbeat
// lost the press and a busy one kept it. The fix is csi's advanceFrame; the unit-level sweep that
// holds it is TestACardReplyDoesNotSpliceItselfIntoAHandsetFrame.
//
// SO THIS GATE SWEEPS THE IDLE, not because the length matters in itself but because it sets where
// the card's heartbeat falls relative to the press. One idle length is one phase, and one phase is
// how this shipped: the press that was measured worked, and the one viewers made did not.
//
// IT ASSERTS ITS OWN SUBJECT: the box office menu must be reached before each press, or a screen
// that never changed would be a box with no menu rather than a box that dropped a key.
//
// IT ONLY READS the machine; the wire transcript below exists for the failure, because "the screen
// did not change" does not say why and the bytes do.
func TestAFirstPressLandsAfterTheBoxHasBeenIdle(t *testing.T) {
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	// One byte crossing the link and which way it went. Kept in order, because the splice is only
	// visible as a sequence -- every byte here is legitimate on its own.
	type moment struct {
		out bool
		b   byte
	}

	for _, idle := range []int{0, 1_000_000, 4_000_000, 8_000_000, 16_000_000} {
		t.Run(fmt.Sprintf("idle_%dm_instructions", idle/1_000_000), func(t *testing.T) {
			guide := demoGuide(t)
			dict := demoDictionary(t)
			box := restoredBox(t)
			transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day},
				demoSchedule())
			if err != nil {
				t.Fatal(err)
			}
			want := programmesInTheBlock(t, guide, day)
			registered := 0
			if at := runUntil(t, box, transmitter, 120_000_000,
				registeringProgrammes(box, want, &registered)); at < 0 {
				t.Fatalf("harness: only %d of %d programmes registered", registered, want)
			}
			runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

			pump := func() error { return transmitter.Pump(box.Machine.Retired) }
			// Reaching the menu is the harness, so it may retry; the press under test may not.
			menu := uint32(0)
			for attempt := 1; attempt <= attempts && menu != boxOfficeMenu; attempt++ {
				menu = pressAndLetItFinish(t, box, pump, keyBoxOffice, pressBudget)
			}
			if menu != boxOfficeMenu {
				t.Fatalf("harness: never reached the box office menu, drew %08X -- with no menu "+
					"on screen a key that changes nothing proves nothing", menu)
			}
			for i := 0; i < idle; i++ {
				if err := pump(); err != nil {
					t.Fatal(err)
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}

			// ONE PRESS. The whole defect is that the second one worked, so a retry here would
			// reproduce the demo's workaround rather than test the fix.
			var wire []moment
			hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
				if a.Fetch || a.Virtual != 0xB2009010 {
					return
				}
				// +0x10 is the byte register in both directions: read it and you take what the
				// card sent, write it and you send to the card.
				wire = append(wire, moment{out: a.Write, b: byte(a.Value)})
			}}
			drew := pressAndLetItFinishHooked(t, box, pump, hooks, keyLeft, pressBudget)
			if drew == tvGuideMenuScreen {
				return
			}

			var in, sent []byte
			for _, m := range wire {
				if m.out {
					sent = append(sent, m.b)
					continue
				}
				in = append(in, m.b)
			}
			raw := uint8(keyLeft)
			frame := []byte{5, 0x80, 2, 0x1b, 0, raw >> 4, raw << 4, 0}
			verdict := fmt.Sprintf("the key frame %s IS in what the guest read, so the box was "+
				"handed the press and declined it -- the loss is downstream of the link this time",
				hexRun(frame))
			if !strings.Contains(string(in), string(frame)) {
				verdict = fmt.Sprintf("the key frame %s IS NOT in what the guest read -- look for "+
					"it cut in half by a card reply, which is what gort-4sx.firstkey was",
					hexRun(frame))
			}
			t.Fatalf("one press of LEFT after %d instructions of idling drew %08X, not the TV "+
				"GUIDE menu %08X. The viewer's first key was lost.\n  in  %s\n  out %s\n  %s",
				idle, drew, uint32(tvGuideMenuScreen), hexRun(in), hexRun(sent), verdict)
		})
	}
}

// hexRun renders a byte run for a log line, truncated so a long idle stream cannot bury the frame.
func hexRun(b []byte) string {
	const most = 64
	var sb strings.Builder
	for i, v := range b {
		if i == most {
			fmt.Fprintf(&sb, "... (%d more)", len(b)-most)
			break
		}
		fmt.Fprintf(&sb, "%02x ", v)
	}
	if sb.Len() == 0 {
		return "(nothing)"
	}
	return strings.TrimSpace(sb.String())
}
