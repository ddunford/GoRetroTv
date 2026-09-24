package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// WHICH DMA CHANNELS THE BOX PROGRAMS, AND WHETHER ANY OF THEM IS ASKING FOR A STREAM.
//
// "No satellite signal is being received" appears while the box has selected a service but has not
// requested an elementary stream. Register +0x140 is no longer an uncertainty: ROM sets bit 0 and
// the application preserves it through read-modify-write operations. Returning zero from that read
// used to erase the bit and silently disable all transport packets in the model.
//
// PushTransport's own comment says what the real path is: packets "moved by the MEDIA DMA". The
// record puts that DMA at 0xB0009000 -- thirteen channels, descriptors at 0x80108A60 + 40*ch, the
// physical address written to +0x040 + 0x10*ch, then +0x220 = 1, then bit ch set in +0x010, with
// completion in +0x120 and the LISR clearing the enable itself.
//
// So a box asking for a stream would be a box PROGRAMMING A DMA CHANNEL for it. This watches every
// DMA register write from boot, through acquisition, through the guide, and through TUNING -- and
// reports which channels are armed and when, because a channel that only appears once the box is
// viewing is the media path asking.
//
// IT ASSERTS ITS OWN SUBJECT: the DMA must be written at all, or a tally of zero is the instrument.
//
// IT ONLY READS.
func TestWhichDMAChannelsTheBoxProgramsWhenViewing(t *testing.T) {
	const (
		dmaBase   = 0xB0009000
		dmaSize   = 0x1000
		enableReg = 0x010
		goReg     = 0x220
	)
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	type hit struct {
		reg   uint32
		value uint32
		phase string
	}
	type pathCall struct {
		pc, ra, a0, a1, a2, a3, t0 uint32
		ocode                      uint32
		config                     [36]byte
		object                     [40]byte
		second                     [48]byte
		manager                    [128]byte
		phase                      string
	}
	var enables []hit
	byReg := map[uint32]int{}
	peripheralByPhase := map[string]map[uint32]hit{}
	demuxReadsByPhase := map[string]map[uint32]hit{}
	demuxReadCount := map[string]map[uint32]int{}
	var pathCalls []pathCall
	var latestOCode uint32
	var serviceStateWrites []pathCall
	var fixedPSIStateWrites []pathCall
	var dispatchedTables [256]bool
	writes := 0
	phase := "boot and acquisition"
	hooks := board.StepHooks{AfterPump: func() error {
		switch box.Machine.Core.PC {
		case 0x80002E04, // generic demux-channel configuration; live control for service setup
			0x800ABB2C, // demux-parser callback which hands a completed table to the SI dispatcher
			0x800AFD74, // SI table-id dispatcher
			0x800AFE50, // table 0 (PAT) parser
			0x800A283C, // native wrapper which enters the service-state dispatcher
			0x800AD320, // native (7,0x04), which resolves the viewed service object
			0x800AD39C, // resolved-handle validation in native (7,0x04)
			0x800AD3A0, // service-object type check in native (7,0x04)
			0x800AD3AD, // type-13 service-object lock in native (7,0x04)
			0x800AD3B4, // result of the service-object lock
			0x800AD3E0, // return from native (7,0x04), with status in v0 and output at s1
			0x800A915C, // DVB/SI match-list entry which owns the channel-driver calls below
			0x800A9C5C, // first SI-completion callback which promotes service objects 4/5 -> 6
			0x800A9D9C, // starter which registers that callback
			0x800A9F70, // second SI-completion callback which promotes service objects 4/5 -> 6
			0x800AA0C8, // starter which registers the second callback
			0x800AE3A8, // service-state dispatcher which selects the transition routine below
			0x800AF32C, // service transition routine which calls the EIT builder below
			0x800AF49C, // type-13 viewed-service object constructor
			0x800B0EC0, // viewing-only present/following EIT subscription builder
			0x8001C908, // section-channel open/configure entry
			0x8001CB68, // companion channel-operation entry
			0x8001CC6C, // channel-state copy/transition entry
			0x8001C47C, // audio_encoder settings dispatcher
			0x8001C604, // 18-byte stream configuration entry
			0x8001C6C8, // stream start/stop/status dispatcher
			0x800D48D0, // lower-level routine reached by audio configuration
			0x800D4A0C, // shared lower-level routine reached from the audio driver
			0x800D4A6C: // shared lower-level routine reached from the audio driver
			g := box.Machine.Core.GPR
			call := pathCall{
				pc: box.Machine.Core.PC, ra: g[31], a0: g[4], a1: g[5],
				a2: g[6], a3: g[7], t0: g[8], ocode: latestOCode, phase: phase,
			}
			if call.pc == 0x800AD3E0 {
				call.a0 = g[2]
				call.a1 = g[17]
			}
			if call.pc >= 0x800AD39C && call.pc <= 0x800AD3B4 {
				call.a0 = g[16]
				call.a1 = g[16]
				call.a2 = g[2]
			}
			if call.pc == 0x80002E04 && call.a0 >= 0x80000000 && call.a0 < 0x80800000 {
				for i := range call.config {
					call.config[i] = byte(box.RAM.Read((call.a0&0x1FFFFFFF)+uint32(i), bus.Byte))
				}
			}
			if (call.pc == 0x800B0EC0 || call.pc == 0x800ABB2C || call.pc == 0x800AFD74 ||
				call.pc == 0x800A9D9C || call.pc == 0x800AA0C8) &&
				call.a0 >= 0x80000000 && call.a0 < 0x80800000 {
				for i := range call.object {
					call.object[i] = byte(box.RAM.Read((call.a0&0x1FFFFFFF)+uint32(i), bus.Byte))
				}
				if call.pc == 0x800AFD74 {
					dispatchedTables[call.object[0]] = true
				}
			}
			if call.pc == 0x800A915C && call.a1 >= 0x80000000 && call.a1 < 0x80800000 {
				for i := uint32(0); i < call.a2 && i < uint32(len(call.object))/4; i++ {
					value := box.RAM.Read((call.a1&0x1FFFFFFF)+i*4, bus.Word)
					call.object[i*4] = byte(value >> 24)
					call.object[i*4+1] = byte(value >> 16)
					call.object[i*4+2] = byte(value >> 8)
					call.object[i*4+3] = byte(value)
				}
			}
			if call.pc == 0x800AF49C && call.a0 >= 0x80000000 && call.a0 < 0x80800000 {
				for i := range call.manager {
					call.manager[i] = byte(box.RAM.Read((call.a0&0x1FFFFFFF)+uint32(i), bus.Byte))
				}
			}
			if (call.pc == 0x800AF32C || call.pc == 0x800AF49C || call.pc == 0x800AD320 || call.pc == 0x800AD3E0 ||
				(call.pc >= 0x800AD39C && call.pc <= 0x800AD3B4)) &&
				call.a1 >= 0x80000000 && call.a1 < 0x80800000 {
				for i := range call.second {
					call.second[i] = byte(box.RAM.Read((call.a1&0x1FFFFFFF)+uint32(i), bus.Byte))
				}
			}
			pathCalls = append(pathCalls, call)
		}
		return nil
	}, Access: func(a bus.ObservedAccess) {
		if !a.Write && !a.Fetch && a.Virtual >= 0x9FC00000 &&
			box.Machine.Core.State().PC&^1 == 0x80069298 {
			latestOCode = 0x9FC00000 | (a.Virtual & 0x00FFFFFF)
		}
		if !a.Write && !a.Fetch && a.Virtual >= 0xB000A000 && a.Virtual < 0xB000B000 {
			if demuxReadsByPhase[phase] == nil {
				demuxReadsByPhase[phase] = map[uint32]hit{}
				demuxReadCount[phase] = map[uint32]int{}
			}
			demuxReadCount[phase][a.Virtual]++
			if _, seen := demuxReadsByPhase[phase][a.Virtual]; !seen {
				demuxReadsByPhase[phase][a.Virtual] = hit{
					reg: box.Machine.Core.PC, value: a.Value, phase: phase,
				}
			}
		}
		if !a.Write || a.Fetch {
			return
		}
		if a.Virtual == 0x802B084C {
			g := box.Machine.Core.GPR
			serviceStateWrites = append(serviceStateWrites, pathCall{
				pc: box.Machine.Core.PC, ra: g[31], a0: a.Value, ocode: latestOCode, phase: phase,
			})
		}
		if a.Virtual&0x1fffffff == 0x00107064 {
			g := box.Machine.Core.GPR
			fixedPSIStateWrites = append(fixedPSIStateWrites, pathCall{
				pc: box.Machine.Core.PC, ra: g[31], a0: a.Value, ocode: latestOCode, phase: phase,
			})
		}
		if a.Virtual >= 0xB0000000 && a.Virtual < 0xC0000000 {
			if peripheralByPhase[phase] == nil {
				peripheralByPhase[phase] = map[uint32]hit{}
			}
			if _, seen := peripheralByPhase[phase][a.Virtual]; !seen {
				peripheralByPhase[phase][a.Virtual] = hit{
					reg: box.Machine.Core.PC, value: a.Value, phase: phase,
				}
			}
		}
		at := a.Virtual
		if at < dmaBase || at >= dmaBase+dmaSize {
			return
		}
		writes++
		reg := (at - dmaBase) &^ 3
		byReg[reg]++
		if reg == enableReg || reg == goReg {
			enables = append(enables, hit{reg: reg, value: a.Value, phase: phase})
		}
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntilHooked(t, box, transmitter, hooks, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	runUntilHooked(t, box, transmitter, hooks, 20_000_000, func(int) bool { return false })
	acquisitionWrites := writes

	pump := func() error { return transmitter.Pump(box.Machine.Retired) }
	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		drew := pressAndLetItFinishHooked(t, box, pump, hooks, raw, budget)
		if drew == 0 {
			t.Logf("    %-38s %08X -> swallowed", name, before)
			return 0
		}
		t.Logf("%-42s %08X -> %08X", name, before, drew)
		return drew
	}
	phase = "opening the guide"
	openAllChannels(t, press, ".artifacts/mediadma-grid.png", true)
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	guideWrites := writes

	phase = "VIEWING a channel"
	before := screenNow(t, box)
	viewing := uint32(0)
	for attempt := 1; attempt <= 6 && (viewing == 0 || viewing == before); attempt++ {
		viewing = press(keySelect, "select to view the channel", 80_000_000)
	}
	for i := 0; i < 40_000_000; i++ {
		if err := pump(); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
	}
	if err := dumpScreen(t, box, "mediadma-viewing.png"); err != nil {
		t.Fatal(err)
	}
	frame, err := box.Compose()
	if err != nil {
		t.Fatal(err)
	}
	for i, index := range frame.Pix {
		if index != 0 {
			t.Fatalf("firmware viewing handoff pixel %d = palette index %#02x, want transparent index 0", i, index)
		}
	}
	_, _, _, alpha := frame.Palette[0].RGBA()
	if alpha != 0 {
		t.Fatalf("firmware viewing handoff left palette index 0 opaque: alpha=%#04x", alpha)
	}
	root := box.Display.Read(0x200, bus.Word)
	descAt := 0xA0000000 | (root & 0x00FFFFFF)
	t.Logf("viewing display root=%08X descriptor=%08X", root, descAt)
	for at, seen := descAt, map[uint32]bool{}; at != 0 && !seen[at]; {
		seen[at] = true
		words := make([]uint32, 20)
		for i := range words {
			words[i] = box.RAM.Read((at&0x1FFFFFFF)+uint32(i*4), bus.Word)
		}
		t.Logf("display descriptor %08X: %08X", at, words)
		next := words[16]
		if next == 0 {
			break
		}
		at = next
	}
	if writes == 0 {
		t.Fatal("harness: the guest never wrote a DMA register in this whole run, so a tally of " +
			"zero is the instrument and not the box")
	}
	t.Logf("the box is viewing %08X", viewing)
	t.Logf("DMA writes: %d during boot and acquisition, %d more opening the guide, %d more while "+
		"viewing", acquisitionWrites, guideWrites-acquisitionWrites, writes-guideWrites)

	regs := make([]uint32, 0, len(byReg))
	for reg := range byReg {
		regs = append(regs, reg)
	}
	sort.Slice(regs, func(a, b int) bool { return byReg[regs[a]] > byReg[regs[b]] })
	t.Logf("=== every DMA register the box writes ===")
	for i, reg := range regs {
		if i >= 16 {
			t.Logf("    ... and %d more", len(regs)-16)
			break
		}
		note := ""
		switch {
		case reg == enableReg:
			note = "   <- the per-channel ENABLE bitmap"
		case reg == goReg:
			note = "   <- the go bit"
		case reg >= 0x040 && reg < 0x040+0x10*13:
			note = "   <- descriptor address for channel " +
				string(rune('0'+(reg-0x040)/0x10)) // #nosec G115 -- thirteen channels
		}
		t.Logf("    +%03X  %d writes%s", reg, byReg[reg], note)
	}
	t.Logf("=== the channels armed, in order ===")
	seen := map[uint32]bool{}
	for _, h := range enables {
		if h.reg != enableReg {
			continue
		}
		for ch := uint32(0); ch < 13; ch++ {
			if h.value&(1<<ch) == 0 || seen[ch] {
				continue
			}
			seen[ch] = true
			t.Logf("    channel %2d first armed during %s", ch, h.phase)
		}
	}
	if len(seen) == 0 {
		t.Logf("    NONE. The box arms no DMA channel at any point, so nothing is asking for a " +
			"stream through this path either.")
	}
	t.Log("=== peripheral write addresses exclusive to viewing ===")
	var exclusive []uint32
	for address := range peripheralByPhase["VIEWING a channel"] {
		if _, duringGuide := peripheralByPhase["opening the guide"][address]; duringGuide {
			continue
		}
		if _, duringAcquire := peripheralByPhase["boot and acquisition"][address]; duringAcquire {
			continue
		}
		exclusive = append(exclusive, address)
	}
	sort.Slice(exclusive, func(i, j int) bool { return exclusive[i] < exclusive[j] })
	for _, address := range exclusive {
		h := peripheralByPhase["VIEWING a channel"][address]
		t.Logf("    %08X first PC=%08X value=%08X", address, h.reg, h.value)
	}
	if len(exclusive) == 0 {
		t.Log("    NONE")
	}
	t.Log("=== demux reads while viewing ===")
	viewReads := make([]uint32, 0, len(demuxReadsByPhase["VIEWING a channel"]))
	for address := range demuxReadsByPhase["VIEWING a channel"] {
		viewReads = append(viewReads, address)
	}
	sort.Slice(viewReads, func(i, j int) bool { return viewReads[i] < viewReads[j] })
	for _, address := range viewReads {
		h := demuxReadsByPhase["VIEWING a channel"][address]
		_, duringGuide := demuxReadsByPhase["opening the guide"][address]
		_, duringAcquire := demuxReadsByPhase["boot and acquisition"][address]
		t.Logf("    %08X count=%d first PC=%08X value=%08X exclusive=%v", address,
			demuxReadCount["VIEWING a channel"][address], h.reg, h.value,
			!duringGuide && !duringAcquire)
	}
	if len(viewReads) == 0 {
		t.Fatal("harness: the guest made no demux reads while viewing")
	}
	t.Log("=== viewing-input and audio-path calls ===")
	for _, call := range pathCalls {
		t.Logf("    PC=%08X RA=%08X a0=%08X a1=%08X a2=%08X a3=%08X t0=%08X ocode=%08X during %s",
			call.pc, call.ra, call.a0, call.a1, call.a2, call.a3, call.t0, call.ocode, call.phase)
		if call.pc == 0x80002E04 {
			t.Logf("        demux config: % X", call.config)
		}
		if call.pc == 0x800B0EC0 || call.pc == 0x800ABB2C || call.pc == 0x800AFD74 ||
			call.pc == 0x800A9D9C || call.pc == 0x800AA0C8 {
			t.Logf("        input bytes: % X", call.object)
		}
		if call.pc == 0x800A915C {
			t.Logf("        requested indices: % X", call.object[:min(int(call.a2)*4, len(call.object))])
		}
		if call.pc == 0x800AF32C || call.pc == 0x800AF49C || call.pc == 0x800AD320 || call.pc == 0x800AD3E0 ||
			(call.pc >= 0x800AD39C && call.pc <= 0x800AD3B4) {
			t.Logf("        pointed data: % X", call.second)
		}
		if call.pc == 0x800AF49C {
			t.Logf("        manager before insertion: % X", call.manager)
		}
	}
	if len(pathCalls) == 0 {
		t.Log("    NONE")
	}
	t.Log("=== writes to viewed-service lifecycle state at object+0x0C ===")
	for _, call := range serviceStateWrites {
		t.Logf("    PC=%08X RA=%08X value=%08X ocode=%08X during %s",
			call.pc, call.ra, call.a0, call.ocode, call.phase)
	}
	t.Log("=== fixed-PSI manager state writes at 0x80107064 ===")
	for _, call := range fixedPSIStateWrites {
		t.Logf("    PC=%08X RA=%08X value=%08X ocode=%08X during %s",
			call.pc, call.ra, call.a0, call.ocode, call.phase)
	}
	if len(serviceStateWrites) == 0 {
		t.Fatal("harness: the viewed-service object was never constructed, so its lifecycle gate was not measured")
	}
	if !dispatchedTables[0x40] || !dispatchedTables[0x42] || !dispatchedTables[0x4A] ||
		!dispatchedTables[0x73] {
		t.Fatalf("harness: SI dispatcher control failed: NIT=%v SDT=%v BAT=%v TOT=%v",
			dispatchedTables[0x40], dispatchedTables[0x42], dispatchedTables[0x4A], dispatchedTables[0x73])
	}
	t.Logf("table-0 parser input observed: %v; table-1 parser input observed: %v",
		dispatchedTables[0], dispatchedTables[1])
}
