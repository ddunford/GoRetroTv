package broadcast_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/bus"
)

// The private snapshot must parse a NIT addressed to its live match unit. A
// neighboring ID reaches the guest parser but is discarded after its header.
func TestGuestAcceptsRequestedNITAndRejectsDifferentNetwork(t *testing.T) {
	for _, tc := range []struct {
		name         string
		networkDelta uint16
		wantFetches  int
		wantAccepted bool
	}{
		{"guest-requested network", 0, 37, true},
		{"different network", 1, 9, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := restoredBox(t)
			table, ok := runtime.Demux.Match(1, 0)
			if !ok || table.Value != 0x40 || table.Mask != 0xfe {
				t.Fatalf("fixture lacks NIT subscription: %+v", table)
			}
			high, okHigh := runtime.Demux.Match(1, 1)
			low, okLow := runtime.Demux.Match(1, 2)
			if !okHigh || !okLow || high.Mask != 0xff || low.Mask != 0xff {
				t.Fatal("fixture lacks exact NIT extension match")
			}
			networkID := uint16(high.Value)<<8 | uint16(low.Value)
			armed := false
			for _, pid := range runtime.Demux.ArmedPIDs() {
				if pid == 0x10 {
					armed = true
				}
			}
			if !armed {
				t.Fatal("fixture lacks armed NIT PID")
			}
			transport := broadcast.Transport{ID: networkID, NetworkID: networkID,
				FrequencyMHz: 11778, OrbitTenths: 282, SymbolRate: 27500, FEC: 2,
				Services: []broadcast.Service{{ID: 100, Type: 1}}}
			section, err := broadcast.NIT(networkID+tc.networkDelta, 7, "Sky Digital", []broadcast.Transport{transport})
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Demux.Push(0x10, section); err != nil {
				t.Fatal(err)
			}
			fetches, accepted := 0, false
			for i := 0; i < 2_000_000; i++ {
				switch runtime.Machine.Core.State().PC {
				case 0x800a92d8, 0x800a9306, 0x800a937e:
					fetches++
				case 0x800ab0c2, 0x800ab116:
					accepted = true
				}
				if err := runtime.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if got := runtime.Demux.Read(0xb8, bus.Word); got != 0 {
				t.Fatalf("firmware did not acknowledge section: status %#x", got)
			}
			if fetches != tc.wantFetches || accepted != tc.wantAccepted {
				t.Fatalf("guest NIT parse: fetches=%d accepted=%t, want %d/%t", fetches, accepted, tc.wantFetches, tc.wantAccepted)
			}
		})
	}
}
