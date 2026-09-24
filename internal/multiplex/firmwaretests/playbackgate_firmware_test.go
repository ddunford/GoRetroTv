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

// This is a debugger experiment, not emulated behavior. Promoting the measured service lifecycle
// word lets us observe the firmware-owned requests immediately beyond the gate. The product never
// performs this write; the result names the hardware input that must eventually cause it for real.
func TestDiagnosticWhatTheFirmwareRequestsBeyondServiceStateFour(t *testing.T) {
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
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		return pressAndLetItFinish(t, box, pump, raw, budget)
	}
	openAllChannels(t, press, ".artifacts/playback-gate-grid.png", true)

	const objectAt = uint32(0x802B0840)
	promoted := false
	hits := map[uint32]int{}
	writes := map[uint32]uint32{}
	hooks := board.StepHooks{AfterPump: func() error {
		if !promoted && box.RAM.Read((objectAt&0x1fffffff)+12, bus.Word) == 4 {
			if kind := box.RAM.Read(objectAt&0x1fffffff, bus.Word); kind != 13 {
				t.Fatalf("diagnostic object moved: kind=%d", kind)
			}
			box.RAM.Write((objectAt&0x1fffffff)+12, bus.Word, 6)
			promoted = true
		}
		switch box.Machine.Core.PC &^ 1 {
		case 0x80002E04, 0x8001C604, 0x8001C6C8, 0x800D48D0, 0x800D4A0C, 0x800D4A6C:
			hits[box.Machine.Core.PC&^1]++
		}
		return nil
	}, Access: func(a bus.ObservedAccess) {
		if a.Write && !a.Fetch && a.Virtual >= 0xB0000000 && a.Virtual < 0xC0000000 {
			writes[a.Virtual] = a.Value
		}
	}}

	before := screenNow(t, box)
	viewing := pressAndLetItFinishHooked(t, box, pump, hooks, keySelect, 80_000_000)
	for range 40_000_000 {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if !promoted {
		t.Fatal("harness: viewed-service state 4 was never observed, so the diagnostic changed nothing")
	}
	t.Logf("screen %08X -> %08X", before, viewing)
	for _, pc := range []uint32{0x80002E04, 0x8001C604, 0x8001C6C8, 0x800D48D0, 0x800D4A0C, 0x800D4A6C} {
		t.Logf("PC %08X hits=%d", pc, hits[pc])
	}
	addresses := make([]uint32, 0, len(writes))
	for address := range writes {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
	for _, address := range addresses {
		if address >= 0xB0009000 && address < 0xB000B000 ||
			address >= 0xB4080100 && address < 0xB4080200 {
			t.Logf("peripheral %08X <- %08X", address, writes[address])
		}
	}
	t.Logf("armed PIDs after diagnostic: %v", box.Demux.ArmedPIDs())
	for _, filter := range box.Demux.ArmedFilters() {
		t.Logf("channel %d PID %04X word=%08X", filter.Filter, filter.PID, filter.Word)
	}
	for unit := uint8(0); unit < 16; unit++ {
		var rules []string
		for index := uint8(0); index < 16; index++ {
			match, ok := box.Demux.Match(unit, index)
			if ok && match.Mask != 0 {
				rules = append(rules, fmt.Sprintf("byte%d=%02X/%02X", index, match.Value, match.Mask))
			}
		}
		word9, _ := box.Demux.MatchWord(unit, 9)
		t.Logf("unit %d route=%08X rules=%v", unit, word9, rules)
	}
}
