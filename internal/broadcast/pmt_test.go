package broadcast_test

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

func TestPMTAnnouncesProgrammeClockAndElementaryStreams(t *testing.T) {
	t.Parallel()
	section, err := broadcast.PMT(0x64, 3, 0x101, nil, []broadcast.ElementaryStream{
		{Type: 0x02, PID: 0x101},
		{Type: 0x03, PID: 0x102, Descriptors: []byte{0x0a, 0x04, 'e', 'n', 'g', 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if section[0] != 0x02 {
		t.Fatalf("table id = %#x, want PMT 0x02", section[0])
	}
	if programme := uint16(section[3])<<8 | uint16(section[4]); programme != 0x64 {
		t.Fatalf("programme = %#x, want 0x64", programme)
	}
	if pcr := uint16(section[8]&0x1f)<<8 | uint16(section[9]); pcr != 0x101 {
		t.Fatalf("PCR PID = %#x, want 0x101", pcr)
	}
	if section[12] != 0x02 || uint16(section[13]&0x1f)<<8|uint16(section[14]) != 0x101 {
		t.Fatalf("first stream = % x, want MPEG-2 video on PID 0x101", section[12:17])
	}
	if section[17] != 0x03 || uint16(section[18]&0x1f)<<8|uint16(section[19]) != 0x102 {
		t.Fatalf("second stream = % x, want MPEG audio on PID 0x102", section[17:])
	}
	if crc := dvb.MPEGCRC32(section); crc != 0 {
		t.Fatalf("PMT CRC remainder = %#x, want zero", crc)
	}
}

func TestPMTRejectsMalformedInputs(t *testing.T) {
	t.Parallel()
	stream := []broadcast.ElementaryStream{{Type: 3, PID: 0x102}}
	tests := map[string]struct {
		programme   uint16
		pcr         uint16
		descriptors []byte
		streams     []broadcast.ElementaryStream
	}{
		"reserved programme":    {0, 0x101, nil, stream},
		"PCR PID":               {1, 0x2000, nil, stream},
		"no streams":            {1, 0x101, nil, nil},
		"programme descriptor":  {1, 0x101, []byte{0x09, 4, 0}, stream},
		"elementary PID":        {1, 0x101, nil, []broadcast.ElementaryStream{{Type: 3, PID: 0x2000}}},
		"elementary descriptor": {1, 0x101, nil, []broadcast.ElementaryStream{{Type: 3, PID: 0x102, Descriptors: []byte{0x0a, 4, 0}}}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := broadcast.PMT(tc.programme, 0, tc.pcr, tc.descriptors, tc.streams); err == nil {
				t.Fatal("invalid PMT was accepted")
			}
		})
	}
}
