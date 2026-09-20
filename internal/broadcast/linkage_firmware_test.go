package broadcast_test

import (
	"encoding/binary"
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// The two arms of the guide's single database question, read off the firmware
// in the record's linkage measurement. The caller presets its out-parameter,
// searches the SI for a 0x4A descriptor, and then requires both that the
// search succeeded and that the linkage_type is 0x91.
const (
	pcGuideKey        = 0x80 // tv guide
	pcLinkageAnswered = 0x800a4040
	pcLinkageNotFound = 0x800ac774
)

// TC-6.3. Both directions, because "the guide drew something" is not evidence:
// the answered and not-answered arms both end in a screen, and the difference
// between them is the whole task. The linkage_type is patched rather than the
// descriptor removed, so the section keeps its lengths and only the one byte
// the firmware tests actually changes.
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

			section, err := broadcast.BAT(bouquetID, 5, "Sky", []broadcast.Transport{{
				ID: networkID, NetworkID: networkID, Lineup: lineup,
				Services: []broadcast.Service{{ID: 0x0064}, {ID: 0x0065}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.linkageType != 0x91 {
				section = repointLinkage(t, section, tc.linkageType)
			}
			if err := box.Demux.Push(0x11, section); err != nil {
				t.Fatal(err)
			}
			// Let the BAT be parsed before asking the guide anything.
			for i := 0; i < 20_000_000; i++ {
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if err := box.CSI.Key(pcGuideKey, 0); err != nil {
				t.Fatal(err)
			}
			answered, notFound := 0, 0
			for i := 0; i < 60_000_000; i++ {
				switch box.Machine.Core.State().PC {
				case pcLinkageAnswered:
					answered++
				case pcLinkageNotFound:
					notFound++
				}
				if err := box.Step(); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("answered arm %d, not-answered arm %d", answered, notFound)
			if tc.wantAnswered && answered == 0 {
				t.Errorf("the guide never took the answered arm with a 0x91 linkage")
			}
			if !tc.wantAnswered && answered != 0 {
				t.Errorf("the guide took the answered arm %d times for linkage type %#x", answered, tc.linkageType)
			}
			if !tc.wantAnswered && notFound == 0 {
				t.Errorf("neither arm ran, so this proves nothing about the linkage")
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
