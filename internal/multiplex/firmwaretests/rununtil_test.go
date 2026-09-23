package firmwaretests_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/multiplex"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

// runUntil steps the box, transmitting as it goes, until observe reports that
// what the test was waiting for has happened -- or until the budget runs out.
// It returns the instruction at which observe first held, or -1.
//
// EVERY firmware loop in this package goes through here, and that is the point
// rather than tidiness. A loop that always runs its whole budget put
// internal/broadcast past Go's ten-minute timeout under the race detector, was
// fixed, and then put internal/multiplex past it a second time in the same
// session by the same author. The rule ("stop at the event you are waiting
// for") was already written down both times. A rule competes for attention; a
// helper that is less work than a bare loop does not.
//
// observe runs BEFORE each step, so it can read the PC the box is about to
// execute. A generous budget costs nothing when the machine behaves, and is
// the only thing that turns a hang into a named failure when it does not.
func runUntil(t *testing.T, box *board.Runtime, transmitter *multiplex.Multiplex,
	budget int, observe func(i int) bool) int {
	t.Helper()
	for i := 0; i < budget; i++ {
		if transmitter != nil {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
		}
		if observe(i) {
			return i
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	return -1
}

// drawnScreen returns the hash of a screen the box PUT UP and finished
// drawing, ignoring the one it was showing before.
//
// Two things make this harder than hashing a frame at a chosen instruction,
// and the first attempt at it got both wrong. The box can be caught MID-REDRAW
// -- "NOW Dream Team" painting over the line it replaces -- so the frame has
// to be stable before it means anything. And the banner is TRANSIENT: wait for
// the screen to reach its final state and the answer is the blank picture it
// returns to, which is the same whatever the title said.
//
// So: sample, ignore the frame that was already up, and take the first one
// that holds still. It fails by name when nothing new is ever drawn, because a
// box that draws nothing is a finding rather than a hash.
func drawnScreen(t *testing.T, box *board.Runtime, transmitter *multiplex.Multiplex,
	budget int, before uint32) uint32 {
	t.Helper()
	const sample = 250_000
	const stableSamples = 4 // a million instructions with nothing redrawn
	var candidate uint32
	steady := 0
	found := runUntil(t, box, transmitter, budget, func(i int) bool {
		if i%sample != 0 {
			return false
		}
		picture, err := box.Compose()
		if err != nil {
			t.Fatal(err)
		}
		hash := statehash.HashBytes(picture.Pix)
		if hash == before {
			candidate, steady = 0, 0
			return false
		}
		if hash == candidate {
			steady++
		} else {
			candidate, steady = hash, 0
		}
		return steady >= stableSamples
	})
	if found < 0 {
		t.Fatalf("nothing new was drawn and held still within %d instructions", budget)
	}
	return candidate
}

// screenNow is what the box is showing at this instant.
func screenNow(t *testing.T, box *board.Runtime) uint32 {
	t.Helper()
	picture, err := box.Compose()
	if err != nil {
		t.Fatal(err)
	}
	return statehash.HashBytes(picture.Pix)
}

// registeringProgrammes counts the box's own per-event register and reports
// when it has taken the whole schedule, so a test stops the moment the thing
// it is measuring has finished happening.
func registeringProgrammes(box *board.Runtime, want int, got *int) func(int) bool {
	return func(int) bool {
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			*got++
		}
		return *got >= want
	}
}

// screenBodyNow is what the box is showing BELOW the tab strip, and it exists because the tab strip
// moves on its own.
//
// The TV GUIDE menu carries an animated icon band across the top -- BOX OFFICE, SERVICES,
// INTERACTIVE and a strip that shears while it paints. screenNow hashes the whole framebuffer, so
// on that menu the hash never repeats, a press's "four identical frames" never arrive, and the
// press reports a settled screen of 00000000 while the box is in fact responding perfectly. Two
// probes read that as "the box has stopped taking input" and one of them chased it as a smartcard
// fault; a third concluded the tv guide tab was unreachable and refused to continue.
//
// Hashing the body alone fixes it for every menu in the guide, because the animation is confined to
// the band above them. The cut is at the top of the light panel, which is where the box's own
// screens start their content.
//
// It is NOT a replacement for screenNow. A screen whose CONTENT is what changed -- a grid filling,
// a list arriving -- must still be hashed whole, because the point there is to notice any
// difference at all.
const screenBodyTop = 120

func screenBodyNow(t *testing.T, box *board.Runtime) uint32 {
	t.Helper()
	picture, err := box.Compose()
	if err != nil {
		t.Fatal(err)
	}
	bounds := picture.Bounds()
	if bounds.Dy() <= screenBodyTop {
		t.Fatalf("harness: the composed picture is %d rows, at or below the %d-row tab strip this "+
			"crops, so the body hash would cover nothing", bounds.Dy(), screenBodyTop)
	}
	start := screenBodyTop * picture.Stride
	if start >= len(picture.Pix) {
		t.Fatalf("harness: cropping %d rows leaves nothing of a %d-byte framebuffer",
			screenBodyTop, len(picture.Pix))
	}
	return statehash.HashBytes(picture.Pix[start:])
}

// pressAndLetItFinish sends a key, waits for the screen to settle, and THEN LETS THE PAINT FINISH.
//
// THIS IS THE SETTLE TRAP IN ITS THIRD COSTUME, and it has now bitten this project at every level.
// The settle detector calls a screen finished after four identical samples 65,536 instructions
// apart -- about a quarter of a million instructions of stillness. A menu painting under a busy
// carousel holds a HALF-DRAWN frame still for longer than that, so the detector returns a real
// framebuffer of a screen that has not finished drawing.
//
// That is where this package's screen pins came from, and two of them are the same screen:
//
//	0xDDBC18E9  the TV GUIDE menu MID-PAINT, its tab icon still sheared
//	0x43779DC8  the same menu FINISHED, which route_test.go names "tvGuideMenuRedrawn" and treats
//	            as evidence that a SELECT failed to land
//
// Both are the ten-entry menu with ALL CHANNELS highlighted; the pictures say so. So a route that
// waits for 0xDDBC18E9 presses SELECT into a menu that is still painting -- which is exactly when a
// press is swallowed -- then sees 0x43779DC8 and concludes the select did not land. It did not, and
// the reason was the route.
//
// Running a tail after the settle costs ten million instructions a press and removes the whole
// class. A screen that was already finished reports the same hash twice and nothing changes.
func pressAndLetItFinish(t *testing.T, box *board.Runtime, pump func() error,
	raw uint8, budget int) uint32 {
	t.Helper()
	before := screenNow(t, box)
	if err := box.CSI.Key(raw, 0); err != nil {
		t.Fatal(err)
	}
	step := func() {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	stable, last, settled := 0, before, uint32(0)
	for i := 0; i < budget; i++ {
		step()
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
	if settled == 0 {
		return 0
	}
	for i := 0; i < paintTail; i++ {
		step()
	}
	return screenNow(t, box)
}

// paintTail is how long a settled screen is given to finish painting. Ten million instructions is
// forty times the stillness the settle detector asks for, and was measured to be enough for the
// slowest menu in the guide under a carousel carrying clock, line-up, titles and index.
const paintTail = 10_000_000

// runUntilHooked is runUntil with an observer attached, for probes that must watch from BOOT
// rather than from the first key press.
//
// It exists because the interesting write is often the one that happens before anything is on
// screen: the grid's row state and the type its rows inherit are both set long before the grid is
// opened, and a probe that starts observing at the first press sees neither and reports a
// confident nothing.
func runUntilHooked(t *testing.T, box *board.Runtime, transmitter *multiplex.Multiplex,
	hooks board.StepHooks, budget int, observe func(i int) bool) int {
	t.Helper()
	for i := 0; i < budget; i++ {
		if transmitter != nil {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
		}
		if observe(i) {
			return i
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	return -1
}
