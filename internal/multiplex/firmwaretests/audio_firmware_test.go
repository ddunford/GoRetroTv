package firmwaretests_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

// TestWhetherTheFirmwareDrivesAudioHardware answers TC-7.5 from guest behaviour.
//
// The flash contains audio device names and a SOUND SETTINGS screen, but strings are not hardware
// activity. This walks to that screen with the handset, changes its selected value, and observes
// every peripheral write plus every media-DMA enable. Observation is inert: no register answer,
// guest byte, or firmware instruction is changed.
func TestWhetherTheFirmwareDrivesAudioHardware(t *testing.T) {
	const keyRight = 0x5B
	box := restoredBox(t)
	type write struct {
		pc, address, value uint32
		size               bus.Size
		isWrite            bool
		phase              string
	}
	var writes []write
	var audioAccesses []write
	type call struct {
		pc, a0, a1, a2, a3 uint32
		phase              string
	}
	var driverCalls []call
	phase := "idle"
	hooks := board.StepHooks{AfterPump: func() error {
		pc := box.Machine.Core.PC
		switch pc {
		case 0x8001C47C, 0x8001C528, 0x8001C604, 0x8001C6C8:
			g := box.Machine.Core.GPR
			driverCalls = append(driverCalls, call{pc: pc, a0: g[4], a1: g[5], a2: g[6], a3: g[7], phase: phase})
		}
		return nil
	}, Access: func(a bus.ObservedAccess) {
		if a.Fetch {
			return
		}
		access := write{pc: box.Machine.Core.PC, address: a.Virtual, value: a.Value,
			size: a.Size, isWrite: a.Write, phase: phase}
		if a.Virtual >= 0xB4080100 && a.Virtual < 0xB4080200 {
			audioAccesses = append(audioAccesses, access)
		}
		if a.Write && a.Virtual >= 0xB0000000 && a.Virtual < 0xC0000000 {
			writes = append(writes, access)
		}
	}}
	step := func(raw uint8, name string) uint32 {
		t.Helper()
		phase = name
		return pressAndLetItFinishHooked(t, box, func() error { return nil }, hooks, raw, 80_000_000)
	}

	// Measured menu structure printed by this firmware: SERVICES opens on USING YOUR SKY DIGIBOX;
	// SYSTEM SETUP is the fourth entry, and SOUND SETTINGS is the second entry within it.
	if step(servicesKey, "open SERVICES") == 0 {
		t.Fatal("SERVICES drew nothing")
	}
	for i := 0; i < 3; i++ {
		if step(keyDown, fmt.Sprintf("down to SYSTEM SETUP %d/3", i+1)) == 0 {
			t.Fatalf("SERVICES selection did not move on down %d", i+1)
		}
	}
	if step(keySelect, "open SYSTEM SETUP") == 0 {
		t.Fatal("SYSTEM SETUP drew nothing")
	}
	if step(keyDown, "down to SOUND SETTINGS") == 0 {
		t.Fatal("SYSTEM SETUP selection did not move to SOUND SETTINGS")
	}
	if step(keySelect, "open SOUND SETTINGS") == 0 {
		t.Fatal("SOUND SETTINGS drew nothing")
	}
	if err := dumpScreen(t, box, "audio-sound-settings.png"); err != nil {
		t.Fatal(err)
	}
	beforeChange := len(writes)
	before := screenNow(t, box)
	step(keyRight, "change Audio Output")
	step(keyDown, "down to Volume")
	step(keyRight, "change Volume")
	step(keyDown, "down to Background Music")
	step(keyRight, "change Background Music")
	step(keyDown, "down to Beep")
	step(keyRight, "change Beep")
	step(keyDown, "down to Save New Settings")
	step(keySelect, "save sound settings")
	changed := writes[beforeChange:]
	frame, err := box.Compose()
	if err != nil {
		t.Fatal(err)
	}
	after := statehash.HashBytes(frame.Pix)
	if after == before {
		t.Fatal("sound-setting controls did not change the screen, so the probe did not exercise its subject")
	}

	byBlock := map[uint32]int{}
	byAddress := map[uint32]int{}
	for _, w := range changed {
		byBlock[w.address&0xFFFFF000]++
		byAddress[w.address]++
	}
	blocks := make([]uint32, 0, len(byBlock))
	for block := range byBlock {
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
	for _, block := range blocks {
		t.Logf("change wrote peripheral block %08X %d times", block, byBlock[block])
	}
	addresses := make([]uint32, 0, len(byAddress))
	for address := range byAddress {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
	for _, address := range addresses {
		t.Logf("    %08X %d writes", address, byAddress[address])
	}
	audioWrites := 0
	for _, w := range changed {
		if w.address >= 0xB4080100 && w.address < 0xB4080200 {
			audioWrites++
			t.Logf("audio control during %s: PC=%08X address=%08X value=%08X",
				w.phase, w.pc, w.address, w.value)
		}
		if w.address >= 0xB0009000 && w.address < 0xB000A000 && w.address&0xFFF == 0x010 {
			if w.value != 0 && w.value != 1<<12 {
				t.Errorf("sound-setting change armed media DMA value %08X; channel 12 is the menu redraw", w.value)
			}
		}
	}
	if audioWrites == 0 {
		t.Fatal("sound settings produced no writes to the audio control block")
	}
	for off, want := range map[uint32]byte{0: 0xE7, 1: 0x0F, 2: 0x82, 3: 0x35, 6: 0x10} {
		if got := box.Audio.Register(off); got != want {
			t.Errorf("modelled audio register +%d = %02X, want firmware write %02X", off, got, want)
		}
	}
	for _, a := range audioAccesses {
		t.Logf("audio block %s during %s: PC=%08X address=%08X size=%d value=%08X",
			map[bool]string{false: "read", true: "write"}[a.isWrite],
			a.phase, a.pc, a.address, a.size, a.value)
	}
	for _, c := range driverCalls {
		t.Logf("audio_encoder entry %08X during %s: a0=%08X a1=%08X a2=%08X a3=%08X",
			c.pc, c.phase, c.a0, c.a1, c.a2, c.a3)
	}
	t.Log("VERDICT: yes, the firmware drives audio control hardware when SOUND SETTINGS are saved; " +
		"media DMA remains graphics-only channel 12, so this proves control activity rather than " +
		"decoded audio samples")
}
