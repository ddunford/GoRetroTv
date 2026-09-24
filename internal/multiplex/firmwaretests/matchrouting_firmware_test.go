package firmwaretests_test

import (
	"fmt"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
)

// TestDumpEveryMatchUnitWord records all sixteen words of all sixteen hardware match units.
// Match unit numbers and PID-channel numbers are separate namespaces; decoding only the first
// eight words concealed additional guest programming and cannot establish their binding.
func TestDumpEveryMatchUnitWord(t *testing.T) {
	box := restoredBox(t)

	filters := box.Demux.ArmedFilters()
	if len(filters) == 0 {
		t.Fatal("harness: no armed PID channels in the post-acquisition fixture")
	}
	for _, filter := range filters {
		t.Logf("channel %2d PID %04X register %08X", filter.Filter, filter.PID, filter.Word)
	}

	programmed := 0
	for unit := uint8(0); unit < 16; unit++ {
		words := make([]string, 16)
		for index := uint8(0); index < 16; index++ {
			word, ok := box.Demux.MatchWord(unit, index)
			if !ok {
				t.Fatalf("match word %d/%d unavailable", unit, index)
			}
			words[index] = fmt.Sprintf("%08X", word)
		}
		table, _ := box.Demux.Match(unit, 0)
		if table.Mask != 0 {
			programmed++
		}
		t.Logf("unit %2d words %v", unit, words)
	}
	if programmed == 0 {
		t.Fatal("harness: all sixteen match units were empty")
	}
}

// The ROM programs PID channels 0 and 1 before the application clears every channel. Capture the
// complete hardware filter state in that short window; treating those two writes as application
// PAT/CAT subscriptions previously sent the investigation down a false section-injection route.
func TestDumpEveryROMMatchUnitBeforeApplicationInit(t *testing.T) {
	_ = restoredBox(t)
	box, err := board.New(cachedImages, true)
	if err != nil {
		t.Fatal(err)
	}
	pidWrites := map[uint32]uint32{}
	hooks := board.StepHooks{Access: func(access bus.ObservedAccess) {
		if !access.Write || access.Virtual < 0xb000a014 || access.Virtual >= 0xb000a094 {
			return
		}
		pidWrites[(access.Virtual-0xb000a014)/4] = access.Value
	}}
	for range 4_000_000 {
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if len(pidWrites) == 0 {
		t.Fatal("harness: ROM made no PID-channel writes by four million instructions")
	}
	for channel := uint32(0); channel < 32; channel++ {
		if word, ok := pidWrites[channel]; ok {
			t.Logf("ROM channel %2d final write %08X", channel, word)
		}
	}
	for unit := uint8(0); unit < 16; unit++ {
		words := make([]string, 16)
		for index := uint8(0); index < 16; index++ {
			word, ok := box.Demux.MatchWord(unit, index)
			if !ok {
				t.Fatalf("ROM match word %d/%d unavailable", unit, index)
			}
			words[index] = fmt.Sprintf("%08X", word)
		}
		t.Logf("ROM unit %2d words %v", unit, words)
	}
}

func TestTraceMatchAndPIDProgrammingOrder(t *testing.T) {
	// Load and verify the private firmware through the package's one cached path, then start a
	// clean board so the observer sees the writes which the post-acquisition snapshot preserves
	// only as final state.
	_ = restoredBox(t)
	box, err := board.New(cachedImages, true)
	if err != nil {
		t.Fatal(err)
	}

	var lastMatchValue uint32
	demuxWrites := map[uint32]int{}
	demuxFirst := map[uint32][2]uint32{}
	matchWrites, pidWrites := 0, 0
	hooks := board.StepHooks{AfterPump: func() error {
		if box.Machine.Core.PC&^1 != 0x800035A4 {
			return nil
		}
		state := box.Machine.Core.State()
		arg := state.GPR[4]
		var words [13]uint32
		for i := range words {
			words[i] = box.RAM.Read((arg&0x1fffffff)+uint32(i)*4, bus.Word)
		}
		t.Logf("icount=%d configure-entry caller=%08X arg=%08X words=%08X", box.Machine.Retired,
			state.GPR[31]&^1, arg, words)
		return nil
	}, Access: func(access bus.ObservedAccess) {
		if !access.Write || access.Virtual < 0xb000a000 || access.Virtual >= 0xb000b000 {
			return
		}
		off := access.Virtual - 0xb000a000
		demuxWrites[off]++
		if _, seen := demuxFirst[off]; !seen {
			demuxFirst[off] = [2]uint32{box.Machine.Core.PC, access.Value}
		}
		switch {
		case off == 0x148:
			lastMatchValue = access.Value
		case off == 0x140:
			t.Logf("icount=%d pc=%08X match-control=%08X", box.Machine.Retired,
				box.Machine.Core.PC, access.Value)
		case off == 0x144 && access.Value&0x4000 != 0:
			matchWrites++
			t.Logf("icount=%d pc=%08X match-command=%08X value=%08X unit=%d index=%d",
				box.Machine.Retired, box.Machine.Core.PC, access.Value, lastMatchValue,
				access.Value&0xf, access.Value>>4&0xf)
		case off >= 0x14 && off < 0x14+4*32:
			pidWrites++
			t.Logf("icount=%d pc=%08X pid-channel=%d value=%08X",
				box.Machine.Retired, box.Machine.Core.PC, (off-0x14)/4, access.Value)
		case off == 0xd8:
			t.Logf("icount=%d pc=%08X enable=%08X", box.Machine.Retired, box.Machine.Core.PC, access.Value)
		}
	}}
	for range 90_000_000 {
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if matchWrites == 0 || pidWrites == 0 {
		t.Fatalf("harness: observed %d match commands and %d PID writes", matchWrites, pidWrites)
	}
	for off := uint32(0); off < 0x1000; off += 4 {
		if demuxWrites[off] == 0 {
			continue
		}
		first := demuxFirst[off]
		t.Logf("demux-write +%03X count=%d first-pc=%08X first-value=%08X", off,
			demuxWrites[off], first[0], first[1])
	}
}
