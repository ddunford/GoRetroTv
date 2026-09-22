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

// THE SECOND CONDITION: A MASK TEST ON THE LINE-UP ENTRY'S OWN BYTES.
//
// With the private data specifier transmitted, the grid's filter callback now ACCEPTS every row --
// it returns 0, where it used to return -1 before it had asked anything. And the decompiled
// enumeration shows accepting is only half of what a channel needs:
//
//	LAB_800a4d4a:
//	    if ((local_60 == 0) &&                                  the filter accepted it
//	       ((*(uint *)(record + 0xc) & local_38) != 0))         AND this mask test passes
//	    {
//	      *local_68 = param_2;      report this channel
//	      iVar2 = in_zero;          status 0, success
//	      goto LAB_800a4dde;        and stop searching
//	    }
//
// `record + 0x0C` is bytes 12..15 of the twenty-four byte channel record, and this project already
// knows what puts them there, because it is our own broadcast. The BAT's private 0xB1 line-up entry
// maps:
//
//	Kind   (entry +2)      -> record[12]
//	Flags  (entry +7..8)   -> record[13..16], the low four bits, one byte each
//
// So the word being masked is **Kind in its top byte and three of the four Flags bytes below it**.
// This port transmits `Kind: 1` and has never set a flag, which makes that word 0x01000000 -- and
// whether it intersects the mask the grid passes is a measurement nobody has taken.
//
// **The earlier flag sweep does not answer this.** All four bits were swept across six channels in
// one run and the grid was byte-identical -- but that was before the specifier, when the filter
// callback returned -1 and `local_60 == 0` short-circuited the whole test. The mask was never
// reached. A negative measured behind a closed gate says nothing about the gate below it.
//
// So this reads both halves: the mask the grid actually passes (a3 at the enumeration's entry,
// before the prologue moves anything) and the word each channel record holds.
//
// IT ASSERTS ITS OWN SUBJECT: the enumeration must be entered, and the records must be found, or
// there is nothing to compare.
//
// IT ONLY READS.
func TestWhetherTheLineUpKindIntersectsTheGridsMask(t *testing.T) {
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
		enumEntry   = 0x800A4A90
		maskedAt    = 0x0c // the word the enumeration tests
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

	masks := map[uint32]int{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		if a.Virtual&^1 == enumEntry {
			// a3 is the enumeration's fourth argument, the mask it tests each record against.
			masks[box.Machine.Core.State().GPR[7]]++
		}
	}}

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		if raw == keySelect {
			masks, watching = map[uint32]int{}, true
		}
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, drew := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.StepWithHooks(hooks); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				drew = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		watching = false
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	grid := openAllChannels(t, press, ".artifacts/kind-mask-grid.png", false)
	if err := dumpScreen(t, box, "kind-mask-grid.png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the grid drew %08X", grid)
	if len(masks) == 0 {
		t.Fatalf("harness: the enumeration at %08X was never entered while the grid drew, so no "+
			"mask was captured", uint32(enumEntry))
	}

	// The records. The stride-24 array the grid walks is the one whose channel number sits at +16.
	listings := guide.On(day)
	announced := map[uint16]string{}
	for i := range listings.Services {
		announced[listings.Services[i].Channel] = listings.Services[i].Name
	}
	const distinctive = 250
	size := box.RAM.Size()
	half := func(at uint32) uint16 {
		return uint16(box.RAM.Read(at, bus.Half)) // #nosec G115 -- half read
	}
	ascending := func(start uint32) int {
		n, last := 0, uint16(0)
		for at := start; at+24 <= size; at += 24 {
			v := half(at)
			if _, mine := announced[v]; !mine || (n > 0 && v <= last) {
				break
			}
			n, last = n+1, v
		}
		return n
	}
	first := uint32(0)
	for at := uint32(0); at+24 <= size; at += 2 {
		if v := half(at); v < distinctive {
			continue
		} else if _, mine := announced[v]; !mine {
			continue
		}
		if ascending(at) >= 3 {
			first = at
			break
		}
	}
	if first == 0 {
		t.Fatal("harness: the twenty-four byte channel array was not found, so there are no " +
			"records to compare the mask against")
	}
	for first >= 24 {
		prev := first - 24
		v := half(prev)
		if _, mine := announced[v]; !mine || v >= half(first) {
			break
		}
		first = prev
	}

	keys := make([]uint32, 0, len(masks))
	for m := range masks {
		keys = append(keys, m)
	}
	sort.Slice(keys, func(a, b int) bool { return masks[keys[a]] > masks[keys[b]] })
	t.Logf("=== the mask the grid passes ===")
	for _, m := range keys {
		t.Logf("    %08X, %d times", m, masks[m])
	}

	t.Logf("=== each channel's record word at +%#02x, and whether it intersects ===", maskedAt)
	mask := keys[0]
	intersecting := 0
	for at := first; at+24 <= size; at += 24 {
		channel := half(at)
		name, mine := announced[channel]
		if !mine {
			break
		}
		base := at - 16
		wordAt := box.RAM.Read(base+maskedAt, bus.Word)
		hit := wordAt & mask
		note := "  <- NO INTERSECTION: this channel can never be reported"
		if hit != 0 {
			note = "  <- intersects"
			intersecting++
		}
		t.Logf("    %-14s channel %4d  record %08X  word %08X  & %08X = %08X%s",
			name, channel, 0x80000000|base, wordAt, mask, hit, note)
	}
	switch {
	case intersecting == 0:
		t.Logf("VERDICT: NOT ONE channel's record intersects the grid's mask, so the second "+
			"condition fails for every one of them and no channel is ever reported -- even though "+
			"the filter now accepts them all. The word is Kind in its top byte and three Flags "+
			"bytes below; this port sends Kind 1 and no flags. THE MASK IS %08X.", mask)
	default:
		t.Logf("VERDICT: %d channels intersect the mask, so the mask test is not what stops them "+
			"and the search should be reporting a channel. Look further down the enumeration.",
			intersecting)
	}
}

// WHAT BUILDS THE 24-BYTE CHANNEL RECORD, FIELD BY FIELD.
//
// The grid passes mask `0x00000010` and tests it against each record's word at `+0x0C`. Every
// record holds **zero** there, so no channel can ever be reported -- the filter accepts them all
// and then every one fails this.
//
// **The obvious next move is to guess which broadcast field feeds `+0x0C`, and this project has
// paid for guesses.** The 0xB1 line-up entry maps onto an EIGHTEEN-byte record; this is a
// TWENTY-FOUR byte one at a different address, so the layouts are not the same structure and
// carrying the mapping across would be exactly the kind of plausible reasoning that has been wrong
// here twice today.
//
// So this watches the box BUILD the records during acquisition and reports, per field, which
// instruction wrote it and what it wrote. A field nothing ever writes is a field no section we
// transmit populates -- and `+0x0C` being one of those would say the bit has to come from somewhere
// we are not sending, which is the whole question stated as an address.
//
// IT ASSERTS ITS OWN SUBJECT: the array must be found and something must write it, or the silence
// is the instrument.
//
// IT ONLY READS.
func TestWhatBuildsTheTwentyFourByteChannelRecords(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	// The array's address is stable across runs of this fixture and is re-verified below by the
	// channel numbers it must contain.
	const (
		arrayAt = 0x802A7BEC
		stride  = 24
		count   = 6
		lo      = arrayAt & 0x1fffffff
		hi      = lo + stride*count
	)

	type write struct {
		pc, value uint32
		size      int
	}
	fields := map[uint32][]write{} // offset within a record -> writes
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Write || a.Fetch {
			return
		}
		at := a.Virtual & 0x1fffffff
		if at < lo || at >= hi {
			return
		}
		off := (at - lo) % stride
		fields[off] = append(fields[off], write{
			pc: box.Machine.Core.State().PC &^ 1, value: a.Value, size: int(a.Size)})
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	for i := 0; i < 160_000_000 && registered < want; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			registered++
		}
	}

	// Prove it is the right array before reading anything into the writes.
	listings := guide.On(day)
	announced := map[uint16]string{}
	for i := range listings.Services {
		announced[listings.Services[i].Channel] = listings.Services[i].Name
	}
	found := 0
	for i := uint32(0); i < count; i++ {
		v := uint16(box.RAM.Read(lo+i*stride+16, bus.Half)) // #nosec G115 -- half read
		if _, mine := announced[v]; mine {
			found++
		}
	}
	if found < 3 {
		t.Fatalf("harness: only %d of the %d records at %08X hold an announced channel number at "+
			"+16, so this is not the array and its writes describe something else",
			found, count, uint32(arrayAt))
	}
	if len(fields) == 0 {
		t.Fatalf("harness: nothing wrote the array at %08X during acquisition, so it was built "+
			"before this run started and the field map below would be empty for the wrong reason",
			uint32(arrayAt))
	}

	offs := make([]uint32, 0, len(fields))
	for off := range fields {
		offs = append(offs, off)
	}
	sort.Slice(offs, func(a, b int) bool { return offs[a] < offs[b] })
	t.Logf("=== every field of the %d-byte record, and what wrote it ===", stride)
	for _, off := range offs {
		ws := fields[off]
		byPC := map[uint32]int{}
		values := map[uint32]int{}
		for _, w := range ws {
			byPC[w.pc]++
			values[w.value]++
		}
		pcs := make([]uint32, 0, len(byPC))
		for pc := range byPC {
			pcs = append(pcs, pc)
		}
		sort.Slice(pcs, func(a, b int) bool { return byPC[pcs[a]] > byPC[pcs[b]] })
		vals := make([]uint32, 0, len(values))
		for v := range values {
			vals = append(vals, v)
		}
		sort.Slice(vals, func(a, b int) bool { return values[vals[a]] > values[vals[b]] })
		shown := vals
		if len(shown) > 4 {
			shown = shown[:4]
		}
		note := ""
		if off >= 0x0c && off < 0x10 {
			note = "   <- THE WORD THE GRID MASKS WITH 0x10"
		}
		t.Logf("    +%#04x  %3d writes  by %08X  values %08X%s", off, len(ws), pcs[0], shown, note)
	}
	for off := uint32(0x0c); off < 0x10; off++ {
		if _, written := fields[off]; !written {
			t.Logf("    +%#04x IS NEVER WRITTEN during acquisition", off)
		}
	}
}

// THE STRUCT THE RECORD BUILDER IS HANDED, AND WHERE ITS FIRST WORD COMES FROM.
//
// The record builder is FUN_800a332c and its writes are plain:
//
//	*(undefined4 *)(records + param_3 + 0xc)  = *param_2;                  <- THE MASKED WORD
//	*(undefined4 *)(records + param_3 + 4)    = param_2[1];
//	                        (records + 8)     <- copied from param_2[2]
//	*(undefined2 *)(records + param_3 + 0x10) = *(undefined2 *)(param_2 + 3);   the channel number
//
// So the word the grid masks with `0x10` is simply **the first word of a descriptor struct** the
// caller hands in, and for our channels it is zero. What fills that struct is the question, and
// reading the caller to find out would be a guess of exactly the kind that has been wrong twice
// today.
//
// So this dumps the struct at the call. `a1` is param_2; thirty-two bytes of it, per call, with the
// channel number at +0x0C read back so each dump can be attributed to a channel by name rather than
// by the order the box happened to build them in.
//
// IT ASSERTS ITS OWN SUBJECT: the builder must be called during acquisition, and the struct's
// channel number must match one we announced, or the dump is of something else.
//
// IT ONLY READS.
func TestTheStructTheRecordBuilderIsHanded(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const builder = 0x800A332C

	type call struct {
		at    uint32
		words [8]uint32
	}
	var calls []call
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Fetch || a.Virtual&^1 != builder {
			return
		}
		st := box.Machine.Core.State()
		at := st.GPR[5] // a1 = param_2
		if at < 0x80000000 || (at&0x1fffffff)+32 > box.RAM.Size() {
			return
		}
		c := call{at: at}
		for i := uint32(0); i < 8; i++ {
			c.words[i] = box.RAM.Read((at&0x1fffffff)+i*4, bus.Word)
		}
		calls = append(calls, c)
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	for i := 0; i < 160_000_000 && registered < want; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			registered++
		}
	}
	if len(calls) == 0 {
		t.Fatalf("harness: the record builder at %08X was never called during acquisition, so no "+
			"struct was captured", uint32(builder))
	}

	listings := guide.On(day)
	byChannel := map[uint16]string{}
	for i := range listings.Services {
		byChannel[listings.Services[i].Channel] = listings.Services[i].Name
	}
	t.Logf("the record builder was called %d times; the struct it was handed each time:", len(calls))
	matched := 0
	for i, c := range calls {
		if i >= 12 {
			t.Logf("    ... and %d more", len(calls)-12)
			break
		}
		channel := uint16(c.words[3] >> 16) // #nosec G115 -- the halfword at +0x0C
		name, mine := byChannel[channel]
		if !mine {
			channel = uint16(c.words[3]) // #nosec G115 -- or the low half, depending on packing
			name, mine = byChannel[channel]
		}
		who := "(not one of ours)"
		if mine {
			who = name
			matched++
		}
		t.Logf("    %08X: %08X %08X %08X %08X %08X %08X %08X %08X   %s",
			c.at, c.words[0], c.words[1], c.words[2], c.words[3],
			c.words[4], c.words[5], c.words[6], c.words[7], who)
	}
	if matched == 0 {
		t.Logf("NOTE: no struct carried a channel number we announced at the offset this probe " +
			"reads, so the attribution above is unproven -- the first word is still the value that " +
			"reaches record+0x0C, which is what matters.")
	}
	zero := 0
	for _, c := range calls {
		if c.words[0] == 0 {
			zero++
		}
	}
	t.Logf("VERDICT: the first word -- the one that becomes record+0x0C and is masked with 0x10 -- "+
		"is ZERO in %d of %d calls.", zero, len(calls))
}

// THE WRITE ITSELF: WHAT VALUE REACHES record+0x0C, AND WHERE FROM.
//
// The reading and the measurement disagreed, so neither is trusted and the instruction is watched
// instead. The MIPS is unambiguous:
//
//	800a3586  lw v0,16(sp)     the saved struct pointer
//	800a3588  lw v0,0(v0)      v0 = param_2[0]
//	800a358c  sw v0,12(v1)     record + 0x0C = param_2[0]
//
// So the decompiler's `record+0x0C = *param_2` is right, and the earlier field map was wrong for a
// reason worth keeping: it bucketed writes by `(address - base) % 24` against a base read at the
// END of acquisition, and the array MOVES while it is being built. Every offset it reported was
// shifted. `0x800A34A2` is `sw v0,20(v1)` -- record+0x14, the reference -- not +0x0C at all.
//
// This watches `0x800A358C` execute: `v0` is the value and `v1` the record base, so each write is
// recorded as an exact (address, value) pair with no modulo arithmetic and no assumed base. If the
// value is non-zero here while the record reads zero afterwards, something clears it later, and
// that is a different bug from "we never send it".
//
// IT ASSERTS ITS OWN SUBJECT: the write must execute, or there is nothing to report.
//
// IT ONLY READS.
func TestWhatValueReachesTheMaskedWord(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
	const theWrite = 0x800A358C

	type put struct{ at, value uint32 }
	var puts []put
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !a.Fetch || a.Virtual&^1 != theWrite {
			return
		}
		st := box.Machine.Core.State()
		puts = append(puts, put{at: st.GPR[3] + 12, value: st.GPR[2]})
	}}

	want := programmesInTheBlock(t, guide, day)
	registered := 0
	for i := 0; i < 160_000_000 && registered < want; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			registered++
		}
	}
	if len(puts) == 0 {
		t.Fatalf("harness: the write at %08X never executed during acquisition, so nothing filled "+
			"record+0x0C and its zero is simply never having been written", uint32(theWrite))
	}

	t.Logf("=== every write to record+0x0C during acquisition ===")
	nonZero := 0
	for i, p := range puts {
		if i >= 12 {
			t.Logf("    ... and %d more", len(puts)-12)
			break
		}
		now := uint32(0)
		if p.at >= 0x80000000 && (p.at&0x1fffffff)+4 <= box.RAM.Size() {
			now = box.RAM.Read(p.at&0x1fffffff, bus.Word)
		}
		note := ""
		if p.value != 0 {
			nonZero++
			note = "  <- NON-ZERO"
			if now != p.value {
				note = "  <- NON-ZERO WHEN WRITTEN, AND SOMETHING CHANGED IT SINCE"
			}
		}
		t.Logf("    wrote %08X to %08X   (it now holds %08X)%s", p.value, p.at, now, note)
	}
	switch {
	case nonZero == 0:
		t.Logf("VERDICT: all %d writes put ZERO into record+0x0C. The value comes straight from the "+
			"first word of the struct the builder is handed, so nothing upstream ever supplies a "+
			"bit for the grid's 0x10 mask to find. THE SOURCE OF THAT WORD IS THE NEXT THING TO "+
			"WALK BACK -- it is param_2[0] at 0x800A3588.", len(puts))
	default:
		t.Logf("VERDICT: %d of %d writes carried a NON-ZERO value, so the word is populated at "+
			"least sometimes and the question is what clears it or which records miss out.",
			nonZero, len(puts))
	}
}

// WHICH LINE-UP FLAG BIT SETS THE ONE THE GRID MASKS FOR.
//
// The word the grid masks with 0x10 is `record+0x0C`, written by `0x800A358C` and holding **0xAA**
// for a channel with no flags. 0xAA is a signature this project has already decoded on another
// structure: four two-bit fields packed MSB-first, 1 where a bit is set and 2 where it is clear, so
// all-clear reads 0xAA rather than zero. Bit 4 is the low bit of the SECOND field, so exactly one
// of the line-up's four flag bits can set it.
//
// **That is a derivation, and derivations get measured here.** This gives each channel a different
// flag value, reads back the word each record ends up with, and reports which value produced a word
// the grid's mask can find. The record is located by watching the write itself rather than by
// assuming a base -- an earlier probe bucketed by `(address - base) % 24` against a base read after
// acquisition, and the array MOVES while it is built, so every offset it reported was shifted.
//
// IT ONLY READS. The flags ride in the BAT, as they always have.
func TestWhichFlagBitSetsTheGridsMaskBit(t *testing.T) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	const (
		theWrite = 0x800A358C
		gridMask = 0x10
	)

	listings := guide.On(day)
	assign := []byte{0x1, 0x2, 0x4, 0x8, 0x0f, 0x0}
	if len(listings.Services) != len(assign) {
		t.Fatalf("harness: %d flag values for %d channels", len(assign), len(listings.Services))
	}
	sent := map[uint16]byte{}
	for i := range listings.Services {
		listings.Services[i].Flags = assign[i]
		sent[listings.Services[i].Channel] = assign[i]
	}
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}

	var records []uint32
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if a.Fetch && a.Virtual&^1 == theWrite {
			records = append(records, box.Machine.Core.State().GPR[3]+12)
		}
	}}
	want := programmesInTheBlock(t, guide, day)
	registered := 0
	for i := 0; i < 160_000_000 && registered < want; i++ {
		if err := transmitter.Pump(box.Machine.Retired); err != nil {
			t.Fatal(err)
		}
		if err := box.StepWithHooks(hooks); err != nil {
			t.Fatal(err)
		}
		if box.Machine.Core.State().PC&^1 == pcPerEventRegister {
			registered++
		}
	}
	if len(records) == 0 {
		t.Fatalf("harness: the write at %08X never executed, so no record was located", uint32(theWrite))
	}

	byChannel := map[uint16]string{}
	for i := range listings.Services {
		byChannel[listings.Services[i].Channel] = listings.Services[i].Name
	}
	t.Logf("=== each record's masked word, against the flags its channel was sent ===")
	passing := map[byte]bool{}
	for _, at := range records {
		if at < 0x80000000 || (at&0x1fffffff)+8 > box.RAM.Size() {
			continue
		}
		word := box.RAM.Read(at&0x1fffffff, bus.Word)
		// The channel number sits at record+0x10, which is four bytes past the word just read.
		channel := uint16(box.RAM.Read((at&0x1fffffff)+4, bus.Word)) // #nosec G115 -- low half
		name, mine := byChannel[channel]
		if !mine {
			name = "(unidentified)"
		}
		flags := sent[channel]
		hit := word & gridMask
		note := ""
		if hit != 0 {
			note = "   <- PASSES THE GRID'S MASK"
			passing[flags] = true
		}
		t.Logf("    %08X  word %08X  channel %4d %-14s flags %#03x  & %#02x = %02X%s",
			at, word, channel, name, flags, gridMask, hit, note)
	}
	switch len(passing) {
	case 0:
		t.Logf("VERDICT: no flag value in this sweep produced a word the mask can find.")
	default:
		for f := range passing {
			t.Logf("VERDICT: FLAGS %#03x SETS THE BIT THE GRID MASKS FOR. Every channel needs it.", f)
		}
	}
}

// EVERY CHANNEL WITH FLAG 0x04: DOES THE GRID DRAW ITS ROWS?
//
// The word the grid masks with 0x10 is four two-bit fields, and exactly one of the line-up's flag
// bits sets the field that carries bit 4. Measured across a sweep that gave every channel a
// different value:
//
//	flags 0x0 -> 0xAA (all clear)   flags 0x8 -> 0x6A (field0)
//	flags 0x1 -> 0xA9 (field3)      flags 0x2 -> 0xA6 (field2)
//	flags 0x4 -> 0x9A (field1)  <- 0x9A & 0x10 = 0x10, PASSES
//	flags 0xF -> 0x55 (all set) <- also passes, being all of them
//
// So `0x04` is the bit. With one channel carrying it the grid drew content in its row area for the
// first time in this project's history. This gives it to ALL SIX and asks the only question left:
// does the whole grid fill?
//
// IT ONLY READS the box; the flag rides in the BAT's 0xB1 line-up entry exactly as the field was
// always meant to.
func TestTheGridWithEveryChannelFlagged(t *testing.T) {
	// EVERY UNIFORM VALUE, BECAUSE 0x04 ALONE DID NOT DO IT. Giving all six channels 0x04 -- the bit
	// measured to set the field the grid masks -- left the grid on its empty screen, while a MIXED
	// sweep (0x1, 0x2, 0x4, 0x8, 0xF, 0x0) drew content in the row area. So passing the mask is
	// necessary and not sufficient, and the four bits evidently do not all mean the same kind of
	// thing. Sweeping the whole nibble costs one acquisition each and settles it without guessing
	// which combination matters.
	for _, flags := range []byte{0x04, 0x0f, 0x06, 0x0c, 0x05, 0x07} {
		t.Run(fmt.Sprintf("flags_%#02x", flags), func(t *testing.T) { gridWithFlags(t, flags) })
	}
}

func gridWithFlags(t *testing.T, uniform byte) {
	guide := demoGuide(t)
	dict := demoDictionary(t)
	box := restoredBox(t)
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	const (
		transportAt = 0x802B2A54
		stateOff    = 12
		readyFrom   = 6
	)
	inTheGuide := uniform
	listings := guide.On(day)
	for i := range listings.Services {
		listings.Services[i].Flags = inTheGuide
	}
	transmitter, err := multiplex.New(box, guide, dict, multiplex.FixedClock{At: day}, demoSchedule())
	if err != nil {
		t.Fatal(err)
	}
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
	t.Logf("all %d channels flagged %#02x; transport ready at state %d",
		len(listings.Services), inTheGuide, state())

	press := func(raw uint8, label string, budget int) uint32 {
		t.Helper()
		before := screenNow(t, box)
		if err := box.CSI.Key(raw, 0); err != nil {
			t.Fatal(err)
		}
		stable, last, drew := 0, before, uint32(0)
		for i := 0; i < budget; i++ {
			if err := transmitter.Pump(box.Machine.Retired); err != nil {
				t.Fatal(err)
			}
			if err := box.Step(); err != nil {
				t.Fatal(err)
			}
			if i%65536 != 0 {
				continue
			}
			now := screenNow(t, box)
			if now == last && now != before {
				stable++
				drew = now
				if stable >= 4 {
					break
				}
				continue
			}
			stable, last = 0, now
		}
		t.Logf("%-32s drew %08X", label, drew)
		return drew
	}

	artefact := fmt.Sprintf("grid-flags-%02x.png", uniform)
	grid := openAllChannels(t, press, ".artifacts/"+artefact, true)
	if err := dumpScreen(t, box, artefact); err != nil {
		t.Fatal(err)
	}
	const emptyGrid = 0x42DBD889
	if grid == emptyGrid {
		t.Logf("flags %#02x: the grid is STILL the empty screen -- this value changes nothing", uniform)
		return
	}
	t.Logf("*** flags %#02x: THE GRID DREW %08X, not the empty screen. Artefact %s ***",
		uniform, grid, artefact)
}
