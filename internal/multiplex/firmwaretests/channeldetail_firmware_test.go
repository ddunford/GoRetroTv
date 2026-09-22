package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// THE FIELD THAT DECIDES WHETHER A CHANNEL HAS ANY DETAIL TO GIVE.
//
// Following the one instruction that reads the sorted channel index in the working screen led,
// through memmove and the application's C-library thunk table, to a function at 0x800A45C4 that is
// plainly "fill in this channel's details, by index". It multiplies the index by twenty-four, adds
// it to a base held at +88 of the channel database, and then does this BEFORE it will fill a single
// field:
//
//	800a45d4  lw   v1,20(v1)          entry+20
//	800a45d6  lw   v0,0x800a48b4      the constant 0xFFFFFFFF
//	800a45d8  cmp  v1,v0
//	800a45da  btnez 0x800a45e1        different -> go and fill the record
//	800a45dc  lw   v1,0x800a48b8      the same -> load 0xFFFFFFFC, which is -4
//	800a45de  b    0x800a4649         and RETURN, having written nothing at all
//
// Everything the caller wanted -- the channel number, the name, the two halfwords it copies out,
// the thirty bytes it memmoves -- is on the far side of that test. And where the field is NOT the
// sentinel the function immediately uses it as an INDEX: `entry+20 << 3` added to a table at
// 0x80164A80, whose entry it then passes to another routine. So +20 is a reference into an
// eight-byte-stride table, and 0xFFFFFFFF is that reference saying "not set".
//
// **A SCREEN THAT ASKS FOR EVERY CHANNEL AND IS TOLD -4 EVERY TIME DRAWS ITS FURNITURE AND NO
// ROWS**, which is exactly and precisely what the ALL CHANNELS grid draws. So this reads the field
// off the running box, for every channel the broadcast announced, and asks the only question that
// matters: is the reference set, and for how many of them?
//
// The two outcomes are both worth the run and they point opposite ways. If every channel carries
// the sentinel, nothing the box has received has populated that reference and the work is to find
// what does -- in the signal, which is where it must come from. If some channels carry a real index
// and others do not, then the table it indexes is the thing that is short, and the count says how
// short.
//
// IT FINDS THE ARRAY RATHER THAN HARD-CODING IT. The base moves between runs; what does not move is
// that the channel number sits at entry+16 and the stride is twenty-four, both read off the
// instructions above rather than inferred from a memory dump.
//
// IT ONLY READS.
func TestWhetherEveryChannelHasADetailReference(t *testing.T) {
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

	// The field widths are the function's own, not a guess at a layout.
	const (
		entryStride  = 24
		channelAt    = 16         // lhu v1,16(v1) -> the caller's channel number
		referenceAt  = 20         // lw  v1,20(v1) -> the field tested against the sentinel
		notSet       = 0xFFFFFFFF // the constant at 0x800A48B4
		refusalValue = 0xFFFFFFFC // the -4 the function returns when it matches
	)

	listings := guide.On(day)
	announced := map[uint16]string{}
	for i := range listings.Services {
		announced[listings.Services[i].Channel] = listings.Services[i].Name
	}
	const distinctive = 250
	size := box.RAM.Size()
	word := func(off uint32) uint32 { return box.RAM.Read(off, bus.Word) }
	half := func(off uint32) uint16 {
		return uint16(box.RAM.Read(off, bus.Half)) // #nosec G115 -- half read
	}

	// Locate the array by the properties that identify it and by nothing weaker. Requiring only
	// "an announced channel number every twenty-four bytes" is not enough, and the first version of
	// this probe proved it by locking onto 0x800FE77A -- a run of forty halfwords all holding 501,
	// eight bytes apart, over which any stride at all finds an announced number. It was the count
	// assertion below that caught it, which is the whole reason that assertion exists.
	//
	// The array is in STRICTLY ASCENDING channel order and every entry is a DIFFERENT channel, both
	// read off the working screen's own walk of it. A region of one repeated number satisfies
	// neither.
	ascending := func(start uint32) int {
		n, last := 0, uint16(0)
		for at := start; at+entryStride <= size; at += entryStride {
			v := half(at)
			if _, mine := announced[v]; !mine || (n > 0 && v <= last) {
				break
			}
			n, last = n+1, v
		}
		return n
	}
	var runs []uint32
	for off := uint32(0); off+entryStride <= size; off += 2 {
		if v := half(off); v < distinctive {
			continue
		} else if _, mine := announced[v]; !mine {
			continue
		}
		if ascending(off) >= 3 {
			runs = append(runs, off)
		}
	}
	if len(runs) == 0 {
		t.Fatal("harness: nowhere in DRAM is there a run of three or more DIFFERENT announced " +
			"channel numbers, twenty-four bytes apart and ascending, so the array this probe is " +
			"built on was not found and its readings would be of nothing")
	}
	sort.Slice(runs, func(a, b int) bool { return runs[a] < runs[b] })
	anchor := runs[0]

	// Walk backwards to the first entry: the array is in ascending channel order and two of our six
	// channels (101 and 121) sit below the distinctive threshold, so the anchor is not the start.
	first := anchor
	for first >= entryStride {
		prev := first - entryStride
		v := half(prev)
		if _, mine := announced[v]; !mine || v >= half(first) {
			break
		}
		first = prev
	}
	t.Logf("the twenty-four byte channel array starts at %08X (channel number at +%d)",
		0x80000000|(first-channelAt), channelAt)

	set, unset := 0, 0
	for at := first; at+entryStride <= size; at += entryStride {
		channel := half(at)
		name, mine := announced[channel]
		if !mine {
			break
		}
		base := at - channelAt
		ref := word(base + referenceAt)
		state := "SET"
		if ref == notSet {
			state = "NOT SET -- this channel answers -4"
			unset++
		} else {
			set++
		}
		t.Logf("  %-14s channel %4d  entry %08X  reference %08X  %s",
			name, channel, 0x80000000|base, ref, state)
	}
	if set+unset != len(announced) {
		t.Fatalf("harness: walked %d entries but the broadcast announced %d channels, so this is "+
			"not the array it claims to be and every reading above is of something else",
			set+unset, len(announced))
	}

	switch {
	case unset == 0:
		t.Logf("VERDICT: EVERY one of the %d channels carries a real detail reference, so the "+
			"sentinel is not why the grid draws nothing and this field is closed.", set)
	case set == 0:
		t.Logf("VERDICT: NOT ONE of the %d channels carries a detail reference -- every entry holds "+
			"%08X, so 0x800A45C4 returns %08X for every channel anything asks about. A screen that "+
			"asks for each channel in turn and is refused each time draws its furniture and no "+
			"rows, which is the grid exactly. The work is to find what populates this reference, "+
			"and it has to arrive in the signal.", unset, uint32(notSet), uint32(refusalValue))
	default:
		t.Logf("VERDICT: %d of %d channels carry a detail reference and %d do not. The reference is "+
			"real and something populates it for some channels only; what distinguishes the two "+
			"sets is the next question.", set, set+unset, unset)
	}
}
