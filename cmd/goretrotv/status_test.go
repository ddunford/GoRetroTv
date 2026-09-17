package main

import "testing"

func TestColdStatusRequiresGuestEvidenceForEachPhase(t *testing.T) {
	checks := []struct {
		name     string
		evidence bootEvidence
		phase    string
	}{
		{"reset", bootEvidence{}, "booting"},
		{"handoff alone", bootEvidence{handoff: true}, "booting"},
		{"initial BGLOAD plateau", bootEvidence{handoff: true, bgFound: true, bgStatus: 7, bgRuns: 1, siPIDs: []uint16{0x14, 0x11, 0x10}}, "booting"},
		{"active flash check", bootEvidence{handoff: true, bgFound: true, bgStatus: 0, bgRuns: 547, siPIDs: []uint16{0x14, 0x11, 0x10}}, "flash-check"},
		{"flash done without SI request", bootEvidence{handoff: true, bgFound: true, bgStatus: 7, bgRuns: 2583}, "booting"},
		{"incomplete SI request", bootEvidence{handoff: true, bgFound: true, bgStatus: 7, bgRuns: 2583, siPIDs: []uint16{0x14, 0x11}}, "booting"},
		{"channel list requested", bootEvidence{handoff: true, bgFound: true, bgStatus: 7, bgRuns: 2583, siPIDs: []uint16{0x10, 0x14, 0x11}}, "channel-list"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			phase, reason := coldStatus(check.evidence)
			if phase != check.phase || reason == "" {
				t.Fatalf("coldStatus(%+v) = %q, %q; want %q and a reason", check.evidence, phase, reason, check.phase)
			}
			if phase == "ready" {
				t.Fatal("cold boot advertised Ready without verified acquisition")
			}
		})
	}
}
