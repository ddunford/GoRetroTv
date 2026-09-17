package main

import (
	"github.com/ddunford/goretrotv/internal/board"
)

type bootEvidence struct {
	handoff  bool
	bgFound  bool
	bgStatus uint8
	bgRuns   uint32
	siPIDs   []uint16
}

func readBootEvidence(box *board.Runtime) (bootEvidence, error) {
	status, runs, found, err := box.TaskState("TASK20")
	if err != nil {
		return bootEvidence{}, err
	}
	return bootEvidence{
		handoff:  box.Machine.Handoff.Done(),
		bgFound:  found,
		bgStatus: status,
		bgRuns:   runs,
		siPIDs:   box.Demux.ArmedPIDs(),
	}, nil
}

// coldStatus names only phases for which the guest has supplied evidence.
// TASK20 is BGLOAD: it stays at one schedule before the bank-1 CRC begins,
// runs repeatedly during that check, then waits for events. The three SI PIDs
// are the firmware's own request for channel-list sections.
func coldStatus(e bootEvidence) (phase, reason string) {
	if !e.handoff || !e.bgFound || e.bgRuns <= 1 {
		return "booting", "The box is starting its firmware."
	}
	if e.bgStatus != 7 {
		return "flash-check", "The firmware is checking its flash."
	}
	if hasSIPIDs(e.siPIDs) {
		return "channel-list", "The firmware is waiting for its channel list."
	}
	return "booting", "The firmware is starting reception."
}

func hasSIPIDs(pids []uint16) bool {
	var found uint8
	for _, pid := range pids {
		switch pid {
		case 0x14: // time
			found |= 1
		case 0x11: // service list
			found |= 2
		case 0x10: // network list
			found |= 4
		}
	}
	return found == 7
}
