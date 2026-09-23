package firmwaretests_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE ROW CALLBACK: WHICH FUNCTION THE GRID HANDS EACH CHANNEL TO.
//
// Ghidra's decompilation of the grid's enumeration (FUN_800a4a90, which contains the loop head at
// 0x800A4B60) shows the shape hand-reading MIPS16 had not reached:
//
//	local_44 = (*DAT_800a4c84)(iVar1);                        the transport health code
//	if ((local_44 == 4) || (local_44 == 2 && ...)) {          the OUTER gate
//	  ...
//	  if (local_5c < 6 && *(int *)(DAT_800a4c90 + local_5c*0x2c + 0x28) != DAT_800a4c58) {
//	    while (param_2 < local_6c && -1 < param_2) {          the row loop
//	      if (*(int *)(iVar2 + 0x28) == 1) {
//	        iVar3 = (*DAT_800a4c94)(handle, local_30);        the handle resolver, 0x800ADD08
//	        if (iVar3 == 2) { ...bail... }
//	        else if (1 < iVar3) {
//	          local_60 = (**(code **)(DAT_800a4c90 + local_5c*0x2c + 0x24))(...)   THE ROW CALLBACK
//
// `DAT_800a4c90` is a literal pool word and it holds **0x80164978**: a table of six 44-byte
// descriptors, each carrying a MODE at +0x28 and a FUNCTION POINTER at +0x24. `local_5c` is which
// of the six the caller asked for, and the grid takes the `mode == 1` arm -- which is the arm that
// resolves a handle and then calls the row callback.
//
// **With the transport ready the resolver answers 4, so `1 < iVar3` is TRUE and that callback IS
// being called, once per channel.** Six calls, four thousand instructions each, and not one extra
// pixel on the drawing surface. So the callback is the thing to read, and its address is a runtime
// value: 0x80164978 is past the end of the loaded application image, so the table is built in DRAM
// at boot and can only be read off a running box.
//
// This reads it, and captures which of the six the grid selects, so the next decompilation is of
// the right function rather than of all six.
//
// IT ASSERTS ITS OWN SUBJECT: at least one descriptor must carry a plausible code pointer, or the
// table is not where the literal pool says and every address below is noise.
//
// IT ONLY READS.
func TestWhichRowCallbackTheGridUses(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6

		tableAt     = 0x80164978 // the word at 0x800A4C90
		entries     = 6
		entryStride = 0x2c
		callbackAt  = 0x24
		modeAt      = 0x28

		enumEntry = 0x800A4A90 // FUN_800a4a90, where t0 still holds the caller's selector
		loopHead  = 0x800A4B60
	)
	off := uint32(transportAt) & 0x1fffffff
	state := func() uint32 { return box.RAM.Read(off+stateOff, bus.Word) }

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	if at := runUntil(t, box, transmitter, 120_000_000,
		registeringProgrammes(box, want, &registered)); at < 0 {
		t.Fatalf("harness: only %d of %d programmes registered", registered, want)
	}
	if reached := runUntil(t, box, transmitter, 400_000_000,
		func(int) bool { return state() >= readyFrom }); reached < 0 {
		t.Fatalf("harness: the transport never reached state %d; it is still %d", readyFrom, state())
	}
	t.Logf("the transport is ready at state %d", state())

	word := func(at uint32) uint32 { return box.RAM.Read(at&0x1fffffff, bus.Word) }
	t.Logf("=== the six descriptors at %08X ===", uint32(tableAt))
	plausible := 0
	for i := uint32(0); i < entries; i++ {
		base := uint32(tableAt) + i*entryStride
		cb, mode := word(base+callbackAt), word(base+modeAt)
		note := ""
		if cb >= 0x80000000 && cb < 0x80200000 {
			note = "  <- a code pointer"
			plausible++
		}
		t.Logf("    [%d] at %08X  mode %08X  callback %08X%s", i, base, mode, cb, note)
	}
	if plausible == 0 {
		t.Fatalf("harness: not one of the six descriptors at %08X carries a plausible code "+
			"pointer, so this is not the table the literal pool names and every address above is "+
			"noise", uint32(tableAt))
	}

	// WHICH ONE THE GRID PICKS. t0 carries the selector into the enumeration; capturing it at the
	// function's first instruction is before the prologue has moved anything.
	selectors := map[uint32]int{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		if a.Virtual&^1 == enumEntry {
			selectors[box.Machine.Core.State().GPR[8]]++
		}
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			selectors, watching = map[uint32]int{}, true
		}
		drew := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		watching = false
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	grid := openAllChannels(t, press, ".artifacts/row-callback-grid.png", false)
	if err := dumpScreen(t, box, "row-callback-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid drew %08X", grid)
	if len(selectors) == 0 {
		t.Fatalf("harness: the enumeration at %08X was never entered while the grid drew, so no "+
			"selector was captured", uint32(enumEntry))
	}
	for sel, n := range selectors {
		if sel >= entries {
			t.Logf("selector %d, %d times -- OUT OF RANGE for a six-entry table, which the "+
				"decompilation's own `local_5c < 6` guard rejects", sel, n)
			continue
		}
		base := uint32(tableAt) + sel*entryStride
		t.Logf("SELECTOR %d, %d times -> descriptor %08X, mode %08X, ROW CALLBACK %08X",
			sel, n, base, word(base+modeAt), word(base+callbackAt))
		t.Logf("    decompile it:  ./ctl.sh ghidra:decompile %#08x", word(base+callbackAt))
	}
}
