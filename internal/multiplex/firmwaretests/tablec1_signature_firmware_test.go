package firmwaretests_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// indexPID is where the box arms its table 0xC1 filter, and it is measured rather than chosen: the
// subscription walk reads a 0xC1 branch with ninety-two children off PID 0x52, and it is the one
// armed PID this transmitter never fed until the A-Z index shipped.
const indexPID = 0x52

// THE ACCEPTANCE SIGNATURE: what DISPATCHING a table 0xC1 costs, measured against what merely
// RECEIVING one costs.
//
// The two sweeps that ask which PID and which extension the 0xC1 consumer wants used to answer by
// differencing a box that was delivered a section against a box that was delivered NOTHING. That
// counts the whole cost of a section ARRIVING -- the ring drain, the dispatcher, the table check,
// the allocation and the free -- as though it were evidence of acceptance, and it worked only
// while that cost was small. On 2026-09-23 broadcasting the 0xB2 guide-row descriptor put more
// work in the SI path that every delivered section shares, and the floor went from about twenty
// exclusive PCs to about eight hundred:
//
//	PID 0x52 ext 0x0100 -> 1362 exclusive PCs   the target
//	PID 0x11 ext 0x0100 ->  808 exclusive PCs   a PID nothing dispatches from
//	PID 0x52 ext 0x0200 ->  826 exclusive PCs   an extension no arm claims
//
// Both guards fired and both were right: at that margin the addressing is not established. Nothing
// about the 0xC1 consumer had changed -- the INSTRUMENT had stopped being a measurement.
//
// THE FIX IS TO COMPARE LIKE WITH LIKE, which is what the EIT probe does when it runs the same
// schedule twice and gets a noise floor of exactly zero. A section that arrives and is thrown away
// is the right control for a section that arrives and is dispatched; silence is not. So:
//
//	signature = (accepted A ∩ accepted B) \ (control ∪ rejected A ∪ rejected B)
//
// Two ACCEPTED extensions from DIFFERENT arms of the dispatcher, intersected, leave the work that
// dispatching itself does rather than what either arm does; subtracting two REJECTED deliveries
// removes everything that merely arriving costs. What is left is the acceptance path, and a case
// is then read by how much of it the case reproduces.
//
// THE FLOOR IS MEASURED IN THE SAME RUN AND FROM A DELIVERY THAT DEFINED NOTHING. A third rejected
// extension, subtracted from nothing, reports how much of the signature a section the box refuses
// hits by accident. That number is the instrument's own noise, and every verdict below is stated
// against it rather than against a constant somebody chose.
//
// IT ASSERTS ITS OWN SUBJECT. If the two "accepted" extensions were in fact both refused, the
// signature would collapse to the floor and every case would read as rejected -- a clean, quiet,
// entirely wrong answer. So the build fails outright unless the signature stands well clear of its
// own floor.
type indexSignature struct {
	pcs   map[uint32]bool
	floor int
}

// overlap is how many of the acceptance path's addresses a delivery reproduced.
func (s indexSignature) overlap(run map[uint32]bool) int {
	n := 0
	for pc := range s.pcs {
		if run[pc] {
			n++
		}
	}
	return n
}

// The signature is a property of the fixture rather than of either sweep, and building it costs six
// acquisitions. Both probes share one.
var (
	signatureOnce sync.Once
	signatureVal  indexSignature
)

func acceptanceSignature(t *testing.T) indexSignature {
	t.Helper()
	// restoredBox owns the -short and missing-fixture skips, and it must be consulted BEFORE the
	// once-guard: a skip recorded inside sync.Once would be remembered as a built signature by
	// every later caller.
	restoredBox(t)
	signatureOnce.Do(func() {
		control := indexRun(t, indexPID, 0, false)
		// 0x0000 is the dispatcher's first arm and 0x0041 ('A') is its second, so what the two
		// share is the dispatch and not one arm's list-head work.
		acceptedA := indexRun(t, indexPID, 0x0000, true)
		acceptedB := indexRun(t, indexPID, 0x0041, true)
		// Three extensions no arm claims, well clear of every boundary the decompiler names, so
		// that none of them is a case any probe here is trying to decide.
		rejectedA := indexRun(t, indexPID, 0x0300, true)
		rejectedB := indexRun(t, indexPID, 0x0400, true)
		rejectedC := indexRun(t, indexPID, 0x0500, true)

		pcs := map[uint32]bool{}
		for pc := range acceptedA {
			if acceptedB[pc] && !control[pc] && !rejectedA[pc] && !rejectedB[pc] {
				pcs[pc] = true
			}
		}
		sig := indexSignature{pcs: pcs}
		sig.floor = sig.overlap(rejectedC)
		signatureVal = sig
	})
	// THE GUARD IS OUTSIDE THE ONCE, and that is not tidiness. t.Fatal inside a sync.Once calls
	// runtime.Goexit, and Do marks itself done on the way out regardless -- so a build that died
	// half way would leave every later caller holding an EMPTY signature, against which every
	// delivery reproduces nothing and every case reads as refused. A clean, plausible, entirely
	// wrong answer, which is the failure this package exists to stop. Checking here means a
	// poisoned build is loud for whoever asks next.
	sig := signatureVal
	if len(sig.pcs) < 100 || sig.floor*4 >= len(sig.pcs) {
		t.Fatalf("harness: the acceptance path is not distinguishable from an arrival: 0x0000 "+
			"and 0x0041 share %d addresses that three refused extensions do not, and a fourth "+
			"refused extension hits %d of them by accident. Either the 0xC1 consumer is gone, or "+
			"this instrument is, or the signature build died part way through.",
			len(sig.pcs), sig.floor)
	}
	return sig
}

// indexRun acquires a box, optionally delivers one table 0xC1 section, and returns every guest PC
// it executed afterwards.
//
// The budget is censusBudget for the reason written where that constant lives: two runs need to
// converge before the residue between them means anything, and a shorter one reports noise.
func indexRun(t *testing.T, pid, extension uint16, deliver bool) map[uint32]bool {
	t.Helper()
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 15_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if deliver {
		marker := []byte{0xDE, 0xAD, 0xC1, 0x05, 0x5E, 0xC7, 0x10, 0x4E}
		if err := box.Demux.Push(pid, sectionTableVersioned(0xC1, extension, 0, marker)); err != nil {
			t.Fatalf("harness: PID %#04x extension %04X was not delivered at all (%v), so its "+
				"count would measure the push and not the consumer", pid, extension, err)
		}
	}
	seen := make(map[uint32]bool, 8192)
	runUntil(t, box, transmitter, censusBudget, func(int) bool {
		seen[box.Machine.Core.State().PC&^1] = true
		return false
	})
	return seen
}
