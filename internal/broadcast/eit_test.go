package broadcast_test

import (
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// AN EIT IS CHECKED AGAINST THE SPECIFICATION IT CLAIMS, field by field, because like the PAT it
// is built from a document rather than measured off the box. What the box NAMED is the table id,
// the PID and the extension -- 4e/fe 00/ff 64/ff on filter 18, measured at the moment it tunes --
// and nothing at all about the bytes inside. So the layout is ETSI EN 300 468 §4.4.2 and this test
// is where that claim is made good; a section that is merely well-formed to its own builder proves
// only that the builder is self-consistent.
func TestThePresentFollowingEITSaysWhatIsOnAndWhatIsNext(t *testing.T) {
	t.Parallel()
	start := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	section, err := broadcast.EITPresentFollowing(100, 0x0020, 0x0020, 7,
		broadcast.EventPresent, &broadcast.Event{
			ID:       11,
			Start:    start,
			Duration: 90 * time.Minute,
			Name:     "Dream Team",
			Text:     "Football drama",
			Running:  broadcast.RunningRunning,
		})
	if err != nil {
		t.Fatal(err)
	}
	if section[0] != 0x4e {
		t.Errorf("table id is %#02x, want 0x4e -- the box's match unit carries 4e/fe", section[0])
	}
	if section[1]&0xb0 != 0xb0 {
		t.Errorf("syntax indicator and reserved bits are %#02x, want the top nibble 0xb", section[1])
	}
	if length := int(section[1]&0x0f)<<8 | int(section[2]); length != len(section)-3 {
		t.Errorf("section_length says %d, but %d bytes follow it", length, len(section)-3)
	}
	// table_id_extension IS the service id for an EIT, which is the whole reason the box can ask
	// for one channel's events: its match unit pins the extension to 0x0064.
	if id := uint16(section[3])<<8 | uint16(section[4]); id != 100 {
		t.Errorf("service_id is %d, want 100 (the extension the box arms as 0x0064)", id)
	}
	if v := (section[5] >> 1) & 0x1f; v != 7 {
		t.Errorf("version is %d, want 7", v)
	}
	if section[5]&1 != 1 {
		t.Error("current_next_indicator is not set, so the box would hold this as the NEXT table")
	}
	if section[6] != broadcast.EventPresent || section[7] != broadcast.EventFollowing {
		t.Errorf("section %d of %d, want %d of %d -- present/following is exactly two sections",
			section[6], section[7], broadcast.EventPresent, broadcast.EventFollowing)
	}
	if id := uint16(section[8])<<8 | uint16(section[9]); id != 0x0020 {
		t.Errorf("transport_stream_id is %#04x, want 0x0020", id)
	}
	if id := uint16(section[10])<<8 | uint16(section[11]); id != 0x0020 {
		t.Errorf("original_network_id is %#04x, want 0x0020", id)
	}
	if section[12] != broadcast.EventFollowing || section[13] != 0x4e {
		t.Errorf("segment_last_section_number/last_table_id are %d/%#02x, want %d/0x4e",
			section[12], section[13], broadcast.EventFollowing)
	}

	event := section[14 : len(section)-4]
	if id := uint16(event[0])<<8 | uint16(event[1]); id != 11 {
		t.Errorf("event_id is %d, want 11", id)
	}
	// 1998-12-24 is MJD 51171, and the time is BCD rather than binary.
	if mjd := int(event[2])<<8 | int(event[3]); mjd != 51171 {
		t.Errorf("start MJD is %d, want 51171 for 1998-12-24", mjd)
	}
	if event[4] != 0x19 || event[5] != 0x00 || event[6] != 0x00 {
		t.Errorf("start time is %02x:%02x:%02x, want BCD 19:00:00", event[4], event[5], event[6])
	}
	if event[7] != 0x01 || event[8] != 0x30 || event[9] != 0x00 {
		t.Errorf("duration is %02x:%02x:%02x, want BCD 01:30:00", event[7], event[8], event[9])
	}
	if status := event[10] >> 5; status != broadcast.RunningRunning {
		t.Errorf("running_status is %d, want %d for the programme on air",
			status, broadcast.RunningRunning)
	}
	if event[10]&0x10 != 0 {
		t.Error("free_CA_mode is set, and nothing this port broadcasts is scrambled")
	}
	loop := int(event[10]&0x0f)<<8 | int(event[11])
	if loop != len(event)-12 {
		t.Errorf("descriptor loop says %d bytes, but %d follow", loop, len(event)-12)
	}
	desc := event[12:]
	if desc[0] != 0x4d {
		t.Errorf("descriptor tag is %#02x, want 0x4d (short event)", desc[0])
	}
	if int(desc[1]) != len(desc)-2 {
		t.Errorf("descriptor length says %d, but %d bytes follow", desc[1], len(desc)-2)
	}
	if string(desc[2:5]) != "eng" {
		t.Errorf("language is %q, want \"eng\"", desc[2:5])
	}
	name := int(desc[5])
	if got := string(desc[6 : 6+name]); got != "Dream Team" {
		t.Errorf("event name is %q, want \"Dream Team\"", got)
	}
	rest := desc[6+name:]
	if got := string(rest[1 : 1+int(rest[0])]); got != "Football drama" {
		t.Errorf("event text is %q, want \"Football drama\"", got)
	}

	// THE CRC IS THE WHOLE POINT OF A SECTION: the hardware checks it, and a table that fails it
	// is discarded before any firmware sees it -- which would look exactly like "the box ignored
	// our EIT".
	want := uint32(section[len(section)-4])<<24 | uint32(section[len(section)-3])<<16 |
		uint32(section[len(section)-2])<<8 | uint32(section[len(section)-1])
	if crc := dvb.MPEGCRC32(section[:len(section)-4]); crc != want {
		t.Error("the CRC does not cover the section, so the demux would drop it")
	}
}

// A SERVICE WITH NOTHING NEXT STILL SENDS ITS SECOND SECTION. "I have no following programme" and
// "you have never heard from me" are different statements, and a transmitter that skipped the
// section would make them the same -- which is the mistake the A-Z index taught this project, where
// an empty letter and an unknown letter draw different screens.
func TestAnEventlessSectionIsAWellFormedStatement(t *testing.T) {
	t.Parallel()
	section, err := broadcast.EITPresentFollowing(100, 0x0020, 0x0020, 0,
		broadcast.EventFollowing, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Eight bytes of long-section header, six of EIT header, four of CRC, and no event loop.
	if len(section) != 8+6+4 {
		t.Fatalf("an empty following section is %d bytes, want %d", len(section), 8+6+4)
	}
	if section[6] != broadcast.EventFollowing {
		t.Errorf("section number is %d, want %d", section[6], broadcast.EventFollowing)
	}
}

func TestTheEITRefusesWhatWouldNotSurviveTheWire(t *testing.T) {
	t.Parallel()
	start := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	long := make([]byte, 250)
	for i := range long {
		long[i] = 'x'
	}
	for _, c := range []struct {
		name    string
		section byte
		event   *broadcast.Event
	}{
		{"a third section of a two-section table", 2, nil},
		{"a running status that does not fit three bits", broadcast.EventPresent,
			&broadcast.Event{Start: start, Running: 8}},
		{"a duration past six BCD digits", broadcast.EventPresent,
			&broadcast.Event{Start: start, Duration: 100 * time.Hour}},
		{"a negative duration", broadcast.EventPresent,
			&broadcast.Event{Start: start, Duration: -time.Second}},
		{"a zero start time", broadcast.EventPresent, &broadcast.Event{}},
		{"a name that overflows the descriptor", broadcast.EventPresent,
			&broadcast.Event{Start: start, Name: string(long), Text: string(long)}},
		{"a name that is not printable ASCII", broadcast.EventPresent,
			&broadcast.Event{Start: start, Name: "Dream\tTeam"}},
	} {
		if _, err := broadcast.EITPresentFollowing(100, 0x20, 0x20, 0, c.section, c.event); err == nil {
			t.Errorf("%s was accepted", c.name)
		}
	}
}
