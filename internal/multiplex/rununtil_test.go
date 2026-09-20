package multiplex_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/multiplex"
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
