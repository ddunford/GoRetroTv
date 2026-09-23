package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHY IDLING BEFORE A KEY PRESS LOSES THE KEY.
//
// Measured on 2026-09-23 and not explained. A probe that ran eight million instructions of nothing
// between a settled screen and the next press had every LEFT swallowed -- four in a row, from a box
// office menu that stayed on screen throughout:
//
//	box office (six entries)      A6A21DC5 -> FE8D1CCC   over 25707 addresses
//	left to the tv guide menu     FE8D1CCC -> 00000000   over  6688 addresses
//	left to the tv guide menu     FE8D1CCC -> 00000000   over  4736 addresses
//
// Delete the idle and the same press lands first time, FE8D1CCC -> 43779DC8 over 24957 addresses.
// The "from" hash is what proves it is the key and not the settle detector: the screen before the
// SECOND press was still the box office menu, so the first had not moved it either.
//
// TWO CAUSES ARE POSSIBLE AND THEY WANT OPPOSITE WORK:
//
//   - THE CSI MODEL. The link is a CLOCKED serial interface and Pump only hands a queued byte to
//     the guest once the guest has written the transmit register since the last one -- l.txSeen.
//     A box that has gone quiet writes nothing, so nothing is clocked, so the key sits in the
//     queue for ever and the box never learns there is a reason to wake. That would be OUR bug,
//     in the one device standing between a viewer and everything else.
//   - THE FIRMWARE. A real Digibox may genuinely ignore a key once its menu task is quiescent, in
//     which case the behaviour is right and the lesson is only about how probes drive it.
//
// THE TWO ARE DISTINGUISHED BY WATCHING THE WIRE, not by arguing about it. This runs the same
// route twice -- once pressing straight after the previous screen settles, once after eight
// million instructions of idling -- and counts every guest access to the link's four registers
// either side of the press, plus what is left in the queue at the end.
//
//	the queue drains and the screen does not change  -> the box was told and ignored it: FIRMWARE
//	the queue never drains                           -> the box was never told: THE MODEL
//
// IT ASSERTS ITS OWN SUBJECT: the immediate press must land, or there is no working case to
// compare the idle one against and both halves are just a box that does nothing.
//
// IT ONLY READS. The key goes in through the ordinary handset path; nothing is poked.
//
// ANSWERED 2026-09-23, AND IT IS THE FIRMWARE:
//
//	immediate   FE8D1CCC -> 43779DC8   landed
//	idle  1M    FE8D1CCC -> 43779DC8   landed
//	idle  4M    FE8D1CCC -> 43779DC8   landed
//	idle  8M    FE8D1CCC -> 00000000   SWALLOWED -- 8 wire bytes queued, 0 left afterwards
//	press again FE8D1CCC -> 43779DC8   landed
//
// The key REACHES THE GUEST -- every byte leaves the wire and the guest reads the data register
// 678 times during the press -- and the screen does not move until it is pressed again. So the
// delivery is not the problem and internal/device/csi is not the place to look; the box loses ONE
// key and goes on listening. The threshold is between four and eight million instructions, which
// is a bound somebody can match against a timer rather than a warning about idling.
//
// WHY the firmware declines that one key is NOT established, and must not be inferred from this
// run. The obvious theory -- that a quiescent menu task is not waiting on its event queue -- is a
// theory; this project has a record of those surviving next to real measurements as though they
// had been measured too.
func TestWhyIdlingBeforeAKeyPressLosesTheKey(t *testing.T) {
	const (
		csiBase = 0xB2009000
		csiSize = 0x100
	)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)

	type wire struct {
		reads, writes map[uint32]int
		// queued is the wire depth the instant the key goes in, sampled from a hook rather than
		// before the press: pressAndLetItFinish sends the key itself, so a reading taken before
		// calling it can only ever be zero and would read as "nothing was ever queued".
		queued  int
		pending int
		before  uint32
		after   uint32
		// retry is a SECOND press sent only when the first was swallowed. A box that answers it is
		// one that lost a key; a box that does not is one that has stopped listening, and the two
		// want different work.
		retry        uint32
		retryPending int
	}

	press := func(t *testing.T, idle int) wire {
		t.Helper()
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
		menu := pressAndLetItFinish(t, box, pump, keyBoxOffice, 80_000_000)
		if menu != boxOfficeMenu {
			t.Fatalf("harness: box office drew %08X, not %08X, so this run starts from a screen "+
				"it cannot name", menu, uint32(boxOfficeMenu))
		}
		for i := 0; i < idle; i++ {
			if err := pump(); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
		}

		out := wire{reads: map[uint32]int{}, writes: map[uint32]int{}, before: screenNow(t, box)}
		first := true
		hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
			if first {
				// The first step after the key is queued, so this is the deepest the wire gets.
				out.queued, first = box.CSI.Pending(), false
			}
			if a.Fetch || a.Virtual < csiBase || a.Virtual >= csiBase+csiSize {
				return
			}
			if a.Write {
				out.writes[a.Virtual-csiBase]++
				return
			}
			out.reads[a.Virtual-csiBase]++
		}}
		out.after = pressAndLetItFinishHooked(t, box, pump, hooks, keyLeft, 80_000_000)
		out.pending = box.CSI.Pending()
		// AND THEN PRESS AGAIN, because "the key was lost" and "the box is deaf" are different
		// findings and the routes in this package already work around one of them by retrying.
		if out.after == 0 || out.after == out.before {
			out.retry = pressAndLetItFinishHooked(t, box, pump, hooks, keyLeft, 80_000_000)
			out.retryPending = box.CSI.Pending()
		}
		return out
	}

	show := func(name string, w wire) {
		t.Logf("%-10s %08X -> %08X   the key queued %d wire bytes, %d still there afterwards",
			name, w.before, w.after, w.queued, w.pending)
		offsets := map[uint32]bool{}
		for off := range w.reads {
			offsets[off] = true
		}
		for off := range w.writes {
			offsets[off] = true
		}
		var sorted []uint32
		for off := range offsets {
			sorted = append(sorted, off)
		}
		sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
		for _, off := range sorted {
			t.Logf("    +%#02x  %7d reads  %7d writes   %s",
				off, w.reads[off], w.writes[off], csiRegisterName(off))
		}
	}

	immediate := press(t, 0)
	show("immediate", immediate)
	if immediate.after == 0 || immediate.after == immediate.before {
		t.Fatalf("harness: the press with no idle before it drew %08X and did not leave %08X, so "+
			"there is no working case here and the idle half would compare against nothing",
			immediate.after, immediate.before)
	}

	// HOW MUCH IDLING IT TAKES IS PART OF THE FINDING. "Idling loses a key" is a warning; "idling
	// past somewhere between N and M loses a key" is a number somebody can match against a timer.
	// Two intermediate points cost two acquisitions and turn one into the other.
	for _, idle := range []int{1_000_000, 4_000_000} {
		w := press(t, idle)
		verdict := "landed"
		if w.after == 0 || w.after == w.before {
			verdict = "SWALLOWED"
		}
		t.Logf("idle %2dM   %08X -> %08X   %s (%d wire bytes left)",
			idle/1_000_000, w.before, w.after, verdict, w.pending)
	}

	idled := press(t, 8_000_000)
	show("after idle", idled)

	if idled.after != 0 && idled.after != idled.before {
		t.Logf("MEASURED: the idle press LANDED this time (%08X -> %08X). The swallowing is "+
			"therefore not a plain function of idling, and whatever produced it on 2026-09-23 "+
			"needs re-deriving before anything here is believed",
			idled.before, idled.after)
		return
	}

	// THE VERDICT IS THE QUEUE, and nothing beyond it. A key still sitting on the wire was never
	// delivered, which is the model's fault; a key that left the wire was delivered, and what the
	// box then chose to do with it is the firmware's business. This probe can say which of those
	// happened and it cannot say why the firmware chose as it did -- so it does not.
	t.Logf("=== THE IDLE PRESS WAS SWALLOWED, and the queue says which half owns it ===")
	if idled.pending != 0 {
		t.Logf("    THE MODEL. %d bytes are STILL ON THE WIRE after eighty million instructions, "+
			"so the box was never told. Pump hands a queued byte over only once the guest has "+
			"written the transmit register since the last one (txSeen), and a box that has gone "+
			"quiet writes nothing -- while the IDLE path presents its zero byte with no such "+
			"condition. That asymmetry would be the whole of it: the link clocks itself when it "+
			"has nothing to say and refuses to when it does.", idled.pending)
		t.Logf("    guest writes to the transmit register during the press: %d (immediate: %d)",
			idled.writes[0x10], immediate.writes[0x10])
		return
	}
	t.Logf("    THE FIRMWARE, AND THE MODEL IS EXONERATED. The key left the wire -- %d bytes "+
		"queued when it was sent and none left afterwards -- and the guest read the data register "+
		"%d times during the press. It was told. It did not redraw.",
		idled.queued, idled.reads[0x10])
	t.Logf("    WHY it declined is NOT established here and must not be inferred from this run. " +
		"What is established is that the delivery happened, so internal/device/csi is not the " +
		"place to look.")

	// A LOST KEY AND A DEAF BOX ARE DIFFERENT FINDINGS. The routes in this package retry a
	// swallowed press, so which of the two this is decides whether that retry is a workaround or
	// a mask.
	switch {
	case idled.retry != 0 && idled.retry != idled.before:
		t.Logf("    IT LOSES ONE KEY, IT DOES NOT GO DEAF: a second LEFT drew %08X, so the box is "+
			"listening and the first press is what went missing. A route that retries recovers, "+
			"which is why most of this package never noticed.", idled.retry)
	default:
		t.Logf("    IT STAYS DEAF: a second LEFT drew %08X and left %d bytes on the wire, so this "+
			"is not one lost key. A route that retries will not recover from it.",
			idled.retry, idled.retryPending)
	}
}

func csiRegisterName(off uint32) string {
	switch off {
	case 0x00:
		return "control"
	case 0x10:
		return "data -- read by the guest, written to clock a byte out"
	case 0x20:
		return "ready"
	case 0x30:
		return "interrupt enable"
	default:
		return fmt.Sprintf("unmapped (%#02x)", off)
	}
}
