package firmwaretests_test

import (
	"sort"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/multiplex"
)

// HOW MANY CHANNELS DOES EACH SCREEN THINK THERE ARE?
//
// Two addresses now name the channel database's front door, both read off the instructions rather
// than guessed at:
//
//	0x800A45C4  "fill in this channel's details, by index" -- multiplies the index by twenty-four,
//	            refuses with -4 if the entry's reference field is 0xFFFFFFFF, and otherwise fills
//	            the caller's record and memmoves the channel's blob into it.
//	0x800A4B60  a BOUNDS-CHECKED LOOP HEAD over the same array:
//
//	              800a4b60  slt   s1,v1          index < limit ?
//	              800a4b62  bteqz 0x800a4d73      no  -> leave
//	              800a4b66  slti  s1,0           index < 0 ?
//	              800a4b68  btnez 0x800a4d73      yes -> leave
//	              800a4b6c  li    v1,24
//	              800a4b6e  mult  s1,v1          index * 24
//
// The read watch already says something sharp about these: while the working banner drew, the reads
// came from all over 0x800A45C4's body, and while the ALL CHANNELS grid drew, NOT ONE read came
// from anywhere inside it. The grid never asks the database for a channel's details at all. What it
// does instead is touch the array once at each of six instructions and stop.
//
// So this counts, on both screens: how often each of those two entry points is reached, and -- at
// the loop head -- what the INDEX and the LIMIT actually are. A limit of zero is a screen told
// there are no channels, and a limit of six that never turns into a detail call is a screen that
// gave up for some other reason. Those two want completely different work, and no amount of reading
// disassembly distinguishes them.
//
// **THE BANNER IS THE CONTROL.** A count taken only on the screen that fails cannot tell a number
// that is wrong from a number that is normal, and this project has twice filed the second as the
// first.
//
// IT ASSERTS ITS OWN SUBJECT: if neither address is reached on EITHER screen the addresses are
// wrong, and a pair of zeros would otherwise read as a finding about the firmware.
//
// IT ONLY READS.
func TestHowManyChannelsEachScreenThinksThereAre(t *testing.T) {
	var (
		bannerCounts map[uint32]int
		bannerBounds map[bound]int
		gridCounts   map[uint32]int
		gridBounds   map[bound]int
		// DID THE SUBTESTS RUN AT ALL? Under -short every box in this package skips, and a skip
		// unwinds the subtest without unwinding the parent -- so the cross-screen assertion below
		// ran against two empty maps and FAILED the per-turn gate, reporting "the two addresses
		// this probe is built on are wrong" about a probe that had not executed a single
		// instruction. A subject assertion has to be able to tell "measured nothing" from "did not
		// measure", and this is the flag that lets it.
		bannerRan bool
		gridRan   bool
	)
	t.Run("the now-and-next banner", func(t *testing.T) {
		bannerCounts, bannerBounds = countChannelEntryPoints(t, "banner", func(press pressFunc, watch func(bool)) uint32 {
			watch(true)
			drew := press(keySky, "sky (now-and-next banner)", 60_000_000)
			if drew == 0 {
				t.Fatal("harness: the tv guide key drew nothing new, so the control does not exist")
			}
			return drew
		})
		bannerRan = true
	})
	t.Run("the ALL CHANNELS grid", func(t *testing.T) {
		gridCounts, gridBounds = countChannelEntryPoints(t, "all-channels",
			func(press pressFunc, watch func(bool)) uint32 {
				// The route is not the subject: only the press that opens the grid is watched, or the
				// box office menu's own walk of the channel database lands in the grid's count.
				watch(false)
				return openAllChannels(t, func(raw uint8, name string, budget int) uint32 {
					if raw == keySelect {
						watch(true)
					}
					return press(raw, name, budget)
				}, ".artifacts/channel-count-all-channels.png", false)
			})
		gridRan = true
	})

	if !bannerRan && !gridRan {
		t.Skip("neither screen was measured -- the firmware, the dictionary or the snapshot is " +
			"absent, or this is a -short run, in which every box in this package skips")
	}
	if len(bannerCounts) == 0 && len(gridCounts) == 0 {
		t.Fatal("harness: neither entry point was reached on either screen, so the two addresses " +
			"this probe is built on are wrong and its zeros describe the instrument")
	}
	t.Logf("=== the two screens side by side ===")
	for _, at := range []uint32{channelDetailEntry, channelLoopHead} {
		t.Logf("  %08X  banner %d  grid %d", at, bannerCounts[at], gridCounts[at])
	}
	reportBounds := func(label string, seen map[bound]int) {
		keys := make([]bound, 0, len(seen))
		for b := range seen {
			keys = append(keys, b)
		}
		sort.Slice(keys, func(a, b int) bool { return seen[keys[a]] > seen[keys[b]] })
		if len(keys) == 0 {
			t.Logf("  %s never reached the loop head", label)
			return
		}
		for i, b := range keys {
			if i >= 10 {
				break
			}
			t.Logf("  %s loop head: index %d, limit %d, %d times", label, b.index, b.limit, seen[b])
		}
	}
	reportBounds("banner", bannerBounds)
	reportBounds("grid", gridBounds)
}

const (
	// channelDetailEntry is 0x800A45C4, the instruction that loads the caller's index before
	// multiplying it by the twenty-four byte stride.
	channelDetailEntry = 0x800A45C4
	// channelLoopHead is 0x800A4B60, the `slt s1,v1` that decides whether the loop body runs.
	channelLoopHead = 0x800A4B60
)

// bound is one (index, limit) pair observed at the loop head.
type bound struct{ index, limit int32 }

// pressFunc sends one handset key and waits for the screen to settle.
type pressFunc func(raw uint8, name string, budget int) uint32

func countChannelEntryPoints(t *testing.T, artefact string,
	route func(pressFunc, func(bool)) uint32) (map[uint32]int, map[bound]int) {
	t.Helper()
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

	counts := map[uint32]int{}
	bounds := map[bound]int{}
	watching := false
	hooks := board.StepHooks{Access: func(a bus.ObservedAccess) {
		if !watching || !a.Fetch {
			return
		}
		switch a.Virtual &^ 1 {
		case channelDetailEntry:
			counts[channelDetailEntry]++
		case channelLoopHead:
			counts[channelLoopHead]++
			// s1 is the running index and v1 the limit it is tested against; both are ordinary
			// general registers whatever the ISA encoding calls them.
			st := box.Machine.Core.State()
			bounds[bound{index: int32(st.GPR[17]), limit: int32(st.GPR[3])}]++ // #nosec G115 -- register width
		}
	}}

	press := func(raw uint8, name string, budget int) uint32 {
		t.Helper()
		settled := pressAndLetItFinishHooked(t, box,
			func() error { return transmitter.Pump(box.Machine.Retired) }, hooks, raw, budget)
		t.Logf("%-32s drew %08X", name, settled)
		return settled
	}
	watch := func(on bool) {
		if on {
			counts, bounds = map[uint32]int{}, map[bound]int{}
		}
		watching = on
	}

	// The watch is armed for the LAST press of the route, which is the one that draws the screen
	// under test; the presses that get there are route, not subject.
	watch(true)
	drew := route(press, watch)
	watching = false
	if err := dumpScreen(t, box, "channel-count-"+artefact+".png"); err != nil {
		t.Fatal(err)
	}
	t.Logf("the screen settled on %08X; detail entry reached %d times, loop head %d times",
		drew, counts[channelDetailEntry], counts[channelLoopHead])
	return counts, bounds
}
