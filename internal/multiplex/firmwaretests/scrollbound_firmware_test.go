package firmwaretests_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// TRACE THE TWO SIDES OF THE GRID'S FORWARD-SCROLL BOUND.
//
// At 21:21 the first yellow press advances the grid to 22:30, while the second runs the same
// user action and raises "Further schedule information is not available" because its proposed
// window would cross midnight. The screen is interpreted OpenTV o-code, so each flash data read
// made by the interpreter is its bytecode PC. Keeping the two presses in one machine removes boot,
// acquisition and layout differences from the comparison.
//
// This test writes the two traces consumed by tools/ocode-disasm.py. It changes no guest state
// beyond the handset presses whose branch it measures.
func TestTraceTheGridsForwardScrollBound(t *testing.T) {
	const mainFetchSite = 0x80069298
	day := time.Date(1998, 12, 24, 21, 21, 0, 0, time.UTC)
	guide := demoGuide(t)
	box := restoredBox(t)
	transmitter, err := multiplex.New(box, guide, demoDictionary(t), multiplex.FixedClock{At: day},
		demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	want, registered := programmesInTheBlock(t, guide, day), 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d evening programmes registered", registered, want)
	}

	type traceRow struct {
		pc, at uint32
		size   int
		icount uint64
	}
	var (
		active bool
		rows   []traceRow
		lastOp uint32
		times  []uint32
		paths  = map[uint32]bool{}
	)
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !active || a.Write || a.Fetch {
			return
		}
		if a.Virtual < 0x9FC00000 {
			if (lastOp == 0x9FC7A358 || lastOp == 0x9FC7A364) &&
				a.Value >= uint32(day.Truncate(24*time.Hour).Unix()) && a.Value <= uint32(day.AddDate(0, 0, 1).Unix()) { // #nosec G115 -- 1998 fixture
				times = append(times, a.Value)
			}
			return
		}
		if box.Machine.Core.State().PC&^1 == mainFetchSite {
			lastOp = 0x9FC00000 | (a.Virtual & 0x00ffffff)
			paths[lastOp] = true
		}
		rows = append(rows, traceRow{
			pc: box.Machine.Core.State().PC &^ 1, at: 0x9FC00000 | (a.Virtual & 0x00ffffff),
			size: sizeBytes(a.Size), icount: box.Machine.Retired,
		})
	}}
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		return pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
	}

	grid := openAllChannels(t, press, ".artifacts/scrollbound-open.png", true)
	if grid == 0 {
		t.Fatal("harness: ALL CHANNELS did not open")
	}
	// The o-code routine traced below reads the current window start from its data segment. Locate
	// DS structurally by its RSRC signature, as the measured record requires; its heap address moves.
	ds := uint32(0)
	for off := uint32(0x0fc66c); off+4 <= box.RAM.Size(); off += 4 {
		if box.RAM.Read(off, bus.Word) == 0x52535243 {
			ds = 0x80000000 | off
		}
	}
	if ds == 0 {
		t.Fatal("harness: the live o-code data segment's RSRC signature was not found above the image")
	}
	readWord := func(off uint32) uint32 { return box.RAM.Read((ds+off)&0x1fffffff, bus.Word) }
	logBound := func(label string) {
		t.Helper()
		t.Logf("%s: DS=%08X current-window-start=%08X", label, ds, readWord(0x02e1f0))
	}

	writeTrace := func(name string, captured []traceRow) {
		t.Helper()
		var out bytes.Buffer
		mainRows := 0
		for _, row := range captured {
			if row.pc == mainFetchSite {
				mainRows++
			}
			fmt.Fprintf(&out, "0x%08x,0x%08x,%d,%d\n", row.pc, row.at, row.size, row.icount)
		}
		if mainRows == 0 {
			t.Fatalf("harness: %s captured %d flash reads but no opcode fetches", name, len(captured))
		}
		path := filepath.Join("..", "..", "..", ".artifacts", name)
		if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s: %d flash reads, %d opcode fetches", path, len(captured), mainRows)
	}

	logBound("before accepted scroll")
	active, rows, times, paths = true, rows[:0], times[:0], map[uint32]bool{}
	advanced := press(keyYellow, "yellow within today", pressBudget)
	active = false
	first := append([]traceRow(nil), rows...)
	acceptedTimes, acceptedPaths := append([]uint32(nil), times...), paths
	if advanced == 0 || advanced == grid {
		t.Fatalf("harness: the within-day yellow press did not advance from %08X", grid)
	}
	writeTrace("scrollbound-accepted.csv", first)

	logBound("before refused scroll")
	active, rows, times, paths = true, rows[:0], times[:0], map[uint32]bool{}
	refused := press(keyYellow, "yellow across midnight", pressBudget)
	active = false
	second := append([]traceRow(nil), rows...)
	refusedTimes, refusedPaths := append([]uint32(nil), times...), paths
	if refused == 0 || refused == advanced {
		t.Fatalf("harness: the across-midnight press did not draw the refusal (%08X -> %08X)",
			advanced, refused)
	}
	writeTrace("scrollbound-refused.csv", second)
	contains := func(values []uint32, want uint32) bool {
		for _, value := range values {
			if value == want {
				return true
			}
		}
		return false
	}
	midnight := uint32(day.AddDate(0, 0, 1).Truncate(24 * time.Hour).Unix()) // #nosec G115 -- 1998 fixture
	for label, evidence := range map[string]struct {
		times []uint32
		paths map[uint32]bool
		start uint32
		path  uint32
	}{
		"accepted": {acceptedTimes, acceptedPaths, uint32(time.Date(1998, 12, 24, 21, 0, 0, 0, time.UTC).Unix()), 0x9FC7A36B}, // #nosec G115 -- 1998 fixture
		"refused":  {refusedTimes, refusedPaths, uint32(time.Date(1998, 12, 24, 22, 30, 0, 0, time.UTC).Unix()), 0x9FC7A367},  // #nosec G115 -- 1998 fixture
	} {
		if !contains(evidence.times, midnight) || !contains(evidence.times, evidence.start) {
			t.Errorf("%s trace did not read midnight %#08x and window start %#08x: %08x",
				label, midnight, evidence.start, evidence.times)
		}
		if !evidence.paths[evidence.path] {
			t.Errorf("%s trace did not take o-code path %#08x", label, evidence.path)
		}
	}
	t.Logf("the guest compared latest row end %s with 90-minute windows starting at 21:00 and 22:30; equality took the refusal path",
		time.Unix(int64(midnight), 0).UTC().Format(time.RFC3339))
	if err := dumpScreen(t, box, "scrollbound-refused.png"); err != nil {
		t.Fatal(err)
	}
}
