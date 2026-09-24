package firmwaretests_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// TestWhetherFreshSIAfterServiceSelectionStartsFixedPSI asks whether a real
// version change, delivered through the normal transport, is the event which
// makes the firmware open PAT/PMT/CAT. The probe observes only guest calls.
func TestWhetherFreshSIAfterServiceSelectionStartsFixedPSI(t *testing.T) {
	guide, guidePath := demoGuideWithPath(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	schedule := demoSchedule()
	transmitter, err := multiplex.New(box, guide, demoDictionary(t), multiplex.FixedClock{At: day}, schedule)
	if err != nil {
		t.Fatal(err)
	}

	type call struct {
		pc, ra uint32
		group  uint32
		index  []uint32
	}
	var calls []call
	hits := map[uint32]int{}
	hooks := board.StepHooks{AfterPump: func() error {
		state := box.Machine.Core.State()
		pc := state.PC &^ 1
		switch pc {
		case 0x800A915C:
			c := call{pc: pc, ra: state.GPR[31] &^ 1, group: state.GPR[4]}
			for i := uint32(0); i < state.GPR[6] && i < 32; i++ {
				c.index = append(c.index, box.RAM.Read((state.GPR[5]&0x1fffffff)+i*4, bus.Word))
			}
			calls = append(calls, c)
		case 0x800B0F2C, 0x800B0F8C, 0x800B1064, 0x800AFE50, 0x800AFF1C,
			0x800A9C5C, 0x800A9F70, 0x8001C604, 0x8001C6C8, 0x800D48D0, 0x800D4A0C:
			hits[pc]++
		}
		return nil
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntilHooked(t, box, transmitter, hooks, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntilHooked(t, box, transmitter, hooks, 20_000_000, func(int) bool { return false })
	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		return pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
	}
	openAllChannels(t, press, ".artifacts/fresh-psi-grid.png", true)
	press(keySelect, "select service", 80_000_000)

	data, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	var listings multiplex.Listings
	if err := json.Unmarshal(data, &listings); err != nil {
		t.Fatal(err)
	}
	listings.Services[0].Name += " "
	data, err = json.Marshal(&listings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guidePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	before := len(calls)
	for range 100_000_000 {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range calls[before:] {
		t.Logf("after version change pc=%08X ra=%08X group=%d indices=%v", c.pc, c.ra, c.group, c.index)
	}
	for _, pc := range []uint32{0x800B0F2C, 0x800B0F8C, 0x800B1064, 0x800AFE50, 0x800AFF1C,
		0x800A9C5C, 0x800A9F70, 0x8001C604, 0x8001C6C8, 0x800D48D0, 0x800D4A0C} {
		t.Logf("PC %08X hits=%d", pc, hits[pc])
	}
}
