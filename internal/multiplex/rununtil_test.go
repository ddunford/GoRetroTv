package multiplex_test

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
