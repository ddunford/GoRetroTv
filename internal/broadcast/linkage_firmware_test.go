package broadcast_test

import (
	"encoding/binary"
	"testing"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// The two arms of the guide's single database question, read off the firmware
// in the record's linkage measurement. The caller presets its out-parameter,
// searches the SI for a 0x4A descriptor, and then requires both that the
// search succeeded and that the linkage_type is 0x91.
const (
	pcGuideKey        = 0xCC // tv guide
	pcLinkageAnswered = 0x800a4040
	pcLinkageNotFound = 0x800ac774
)

// Budgets, not durations. Every run below stops at the event it is waiting
// for, so a generous cap costs nothing when the machine behaves and is the
// only thing that turns a hang into a named failure when it does not.
//
// They are caps BECAUSE the first version of this test ran fixed windows --
// twenty million instructions to settle, then sixty million to watch -- and a
// fixed window is wrong in both directions at once. Too long and the package
// walked past Go's ten-minute timeout under the race detector; too short and
// the result did not merely weaken, it INVERTED: at an eight-million settle
// the 0x91 section reported the not-answered arm and the 0x90 section reported
// the answered one. A window that observes a machine mid-parse is not
// measuring the thing it names.
const (
	batParseBudget     = 40_000_000
	guideAnswerBudget  = 80_000_000
	batParseEntryCount = 2
)

// runUntilTheLineupIsParsed steps until the guest's channel-list parser has
// decoded every entry of the BAT just pushed, and fails if it never does.
//
// This replaces "run twenty million instructions and hope": the settle is over
// when an observable thing has happened, and if it has not happened the test
// says so instead of quietly measuring a half-parsed machine.
func runUntilTheLineupIsParsed(t *testing.T, box *board.Runtime, entries int) {
	t.Helper()
	decoded := 0
	for i := 0; i < batParseBudget; i++ {
		if box.Machine.Core.State().PC == pcEntryServiceID {
			decoded++
			if decoded == entries {
				t.Logf("the BAT's %d line-up entries were parsed by instruction %d", entries, i)
				return
			}
		}
		if err := box.Step(); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("the guest decoded %d of %d line-up entries in %d instructions, so the BAT was never parsed "+
		"and anything measured after this point is about a mid-parse machine", decoded, entries, batParseBudget)
}

// TC-6.3. Both directions, because "the guide drew something" is not evidence:
// the answered and not-answered arms both end in a screen, and the difference
// between them is the whole task. The linkage_type is patched rather than the
// descriptor removed, so the section keeps its lengths and only the one byte
// the firmware tests actually changes.
//
// The record measured that the guide asks its database EXACTLY ONCE per press,
// so the first arm to run is the answer and there is nothing to gain by
// watching for more of them.
func TestGuideTakesTheAnsweredArmOnlyWithLinkageType91(t *testing.T) {
	lineup := []broadcast.LineupEntry{
		{ServiceID: 0x0064, Kind: 1, Listings: 0x0bb8, Extra: 0x1770, Channel: 101, Flags: 0},
		{ServiceID: 0x0065, Kind: 1, Listings: 0x0bb9, Extra: 0x1771, Channel: 102, Flags: 0},
	}
	for _, tc := range []struct {
		name         string
		linkageType  byte
		wantAnswered bool
	}{
		{"linkage type 0x91 answers the guide", 0x91, true},
		{"linkage type 0x90 leaves it unanswered", 0x90, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := restoredBox(t)
			bouquetID, networkID := guestSubscription(t, box)

			sections, err := broadcast.BAT(bouquetID, 5, "Sky", []broadcast.Transport{{
				ID: networkID, NetworkID: networkID, Lineup: lineup,
				Services: []broadcast.Service{{ID: 0x0064}, {ID: 0x0065}},
			}})
			section := oneSection(t, sections, err)
			if tc.linkageType != 0x91 {
				section = repointLinkage(t, section, tc.linkageType)
			}
			if err := box.Demux.Push(0x11, section); err != nil {
				t.Fatal(err)
			}
			runUntilTheLineupIsParsed(t, box, batParseEntryCount)

			if err := box.CSI.Key(pcGuideKey, 0); err != nil {
				t.Fatal(err)
			}
			answered, asked := false, false
			for i := 0; i < guideAnswerBudget && !asked; i++ {
				switch box.Machine.Core.State().PC {
				case pcLinkageAnswered:
					answered, asked = true, true
					t.Logf("the guide took the ANSWERED arm %d instructions after the key", i)
				case pcLinkageNotFound:
					asked = true
					t.Logf("the guide took the not-answered arm %d instructions after the key", i)
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if !asked {
				t.Fatalf("neither arm ran in %d instructions, so the guide never asked its database "+
					"and this says nothing about the linkage", guideAnswerBudget)
			}
			if answered != tc.wantAnswered {
				t.Errorf("linkage type %#x: answered arm = %v, want %v", tc.linkageType, answered, tc.wantAnswered)
			}
		})
	}
}

// repointLinkage rewrites every linkage_type in the section and repairs the
// CRC. Every one, because the descriptor is in both loops and changing one
// would leave the other answering.
func repointLinkage(t *testing.T, section []byte, linkageType byte) []byte {
	t.Helper()
	patched := append([]byte(nil), section...)
	found := 0
	for i := 0; i+8 < len(patched)-4; i++ {
		if patched[i] == 0x4a && patched[i+1] == 7 && patched[i+8] == 0x91 {
			patched[i+8] = linkageType
			found++
		}
	}
	if found < 2 {
		t.Fatalf("found %d linkage descriptors to repoint, want both loops", found)
	}
	body := patched[:len(patched)-4]
	return binary.BigEndian.AppendUint32(body, dvb.MPEGCRC32(body))
}
