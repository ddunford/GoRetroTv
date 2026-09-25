package multiplex

import (
	"fmt"
	"sort"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// EncodedPayload is one timestamped piece of an already encoded elementary stream. PTS90k uses
// the MPEG 90 kHz clock; callers split long audio streams so each audio PES remains below 64 KiB.
type EncodedPayload struct {
	PTS90k uint64
	Data   []byte
}

// MediaTransport builds a complete single-programme MPEG transport stream using the component
// allocations announced by programmeWave. It is deliberately independent of guest delivery: the
// demux/device path decides which of these packets the firmware-requested PIDs admit.
func MediaTransport(transportStreamID, serviceID uint16, version byte,
	video, audio []EncodedPayload) ([]byte, error) {
	if len(video) == 0 || len(audio) == 0 {
		return nil, fmt.Errorf("multiplex: media transport requires video and audio payloads")
	}
	pat, err := broadcast.PAT(transportStreamID, version,
		[]broadcast.Programme{{Number: serviceID, MapPID: programmeMapPID}})
	if err != nil {
		return nil, err
	}
	pmt, err := broadcast.PMT(serviceID, version, videoPID, nil, []broadcast.ElementaryStream{
		{Type: 0x02, PID: videoPID},
		{Type: 0x03, PID: audioPID},
	})
	if err != nil {
		return nil, err
	}
	out := append(dvb.PacketizeSection(0x00, pat, 0),
		dvb.PacketizeSection(programmeMapPID, pmt, 0)...)

	type emission struct {
		pid      uint16
		streamID byte
		payload  EncodedPayload
	}
	emissions := make([]emission, 0, len(video)+len(audio))
	for _, payload := range video {
		emissions = append(emissions, emission{pid: videoPID, streamID: 0xe0, payload: payload})
	}
	for _, payload := range audio {
		emissions = append(emissions, emission{pid: audioPID, streamID: 0xc0, payload: payload})
	}
	sort.SliceStable(emissions, func(i, j int) bool {
		return emissions[i].payload.PTS90k < emissions[j].payload.PTS90k
	})
	continuity := map[uint16]byte{}
	for _, emission := range emissions {
		pes, err := broadcast.PES(emission.streamID, emission.payload.PTS90k, emission.payload.Data)
		if err != nil {
			return nil, err
		}
		packets := dvb.PacketizePES(emission.pid, pes, continuity[emission.pid])
		continuity[emission.pid] = (continuity[emission.pid] + byte((len(packets)/188)&0x0f)) & 0x0f
		out = append(out, packets...)
	}
	return out, nil
}
