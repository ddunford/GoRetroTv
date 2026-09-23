package firmwaretests_test

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// A BYTECODE TRACE OF THE GRID, IN THE FORMAT THE DISASSEMBLER ALREADY READS.
//
// tools/ocode-disasm.py exists, it is validated, and it has never been pointed at this port. It
// needs a trace of the interpreter's flash reads -- `pc,address,size,icount` per read, where the
// PC distinguishes the main OPCODE fetch at 0x80069298 from every other site, which is an operand
// byte. That is how its operand table is MEASURED rather than guessed: the bytes consumed between
// two main-site reads are that opcode's operands.
//
// Until now the only producer was scripts/digibox-probes/bytecode-trace.js, which runs against the
// browser oracle. This port can produce the same trace from the same screen, and that matters for
// two reasons: the grid's decision is in o-code and nothing else can read it, and a trace taken
// HERE describes what THIS machine did rather than what the oracle did.
//
// WHAT IT IS FOR. The o-code runs sequentially to 0x9FC74248, SKIPS 0x74249..0x74256, and jumps to
// 0x9FC74374, which is the path that reaches for "..no listings available". That skip is the
// branch, and it is taken before the grid asks the listings module anything -- it never asks it
// anything at all. Rendering the bytes either side of it is the only way to see what it tested.
//
// IT ASSERTS ITS OWN SUBJECT: main-site fetches must actually appear, or the trace is operand
// bytes with no instruction boundaries in it and the disassembler would drift silently.
//
// IT ONLY READS, and writes one file under .artifacts.
func TestTraceTheGridsOCode(t *testing.T) {
	const mainFetchSite = 0x80069298
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
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntil(t, box, transmitter, 20_000_000, func(int) bool { return false })

	dir := filepath.Join("..", "..", "..", ".artifacts")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "grid-ocode-trace.csv")
	file, err := os.Create(path) // #nosec G304 -- a fixed artefact path under the repo
	if err != nil {
		t.Fatal(err)
	}
	out := bufio.NewWriterSize(file, 1<<20)
	rows, mainRows := 0, 0
	var writeErr error
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || a.Write || a.Fetch || writeErr != nil {
			return
		}
		if a.Virtual < 0x9FC00000 {
			return
		}
		pc := box.Machine.Core.State().PC &^ 1
		if pc == mainFetchSite {
			mainRows++
		}
		rows++
		// The disassembler compares the PC as TEXT against "0x80069298", so the formatting is part
		// of the contract rather than cosmetic.
		if _, err := fmt.Fprintf(out, "0x%08x,0x%08x,%d,%d\n",
			pc, 0x9FC00000|(a.Virtual&0x00ffffff), sizeBytes(a.Size), box.Machine.Retired); err != nil {
			writeErr = err
		}
	}}

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			watching = true
		}
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
	settled := openAllChannelsFinished(t, press, ".artifacts/ocode-trace-grid.png")
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	watching = false
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if err := out.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dumpScreen(t, box, "ocode-trace-grid.png"); err != nil {
		t.Fatal(err)
	}
	if mainRows == 0 {
		t.Fatalf("harness: %d flash reads were traced and NOT ONE came from the main opcode fetch "+
			"at %#08X, so the trace is all operand bytes with no instruction boundaries in it -- "+
			"the disassembler would decode from the wrong offset and drift silently",
			rows, uint32(mainFetchSite))
	}
	t.Logf("the grid settled on %08X", settled)
	t.Logf("wrote %s: %d flash reads, %d of them opcode fetches", path, rows, mainRows)
	t.Logf("render the branch with:")
	t.Logf("  python3 tools/ocode-disasm.py --trace .artifacts/grid-ocode-trace.csv " +
		"--check 0x9FC74200 0x9FC74400")
}

// sizeBytes is the width of an observed access, which the trace carries so the disassembler can
// count operand bytes correctly.
func sizeBytes(size bus.Size) int {
	switch size {
	case bus.Byte:
		return 1
	case bus.Half:
		return 2
	default:
		return 4
	}
}
