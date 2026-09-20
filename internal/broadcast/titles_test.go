package broadcast

import (
	"testing"

	"github.com/ddunford/goretrotv/internal/dvb"
)

func twelveProgrammes(t *testing.T, dict *HuffmanDictionary) []byte {
	t.Helper()
	records := make([]TitleRecord, 12)
	for i := range records {
		records[i] = TitleRecord{
			// The match unit constrains section byte 11, which is the event
			// id's low byte, to have 0x70 clear.
			EventID:  uint16(i + 1),
			Start:    (6 + i) * 3600,
			Duration: 30 * 60,
			Title:    "The Simpsons",
		}
	}
	section, err := TitleSection(0xa1, 0x0bb8, [2]byte{0x9e, 0x8b}, 0, 0, 0, dict, records)
	if err != nil {
		t.Fatal(err)
	}
	return section
}

// walkTitles counts records the way a reader with the given stride does.
// firmwareStride adds the four header bytes the length field does not count;
// false is openTVtoXML's arithmetic, which advances by the field alone.
func walkTitles(section []byte, firmwareStride bool) int {
	end := len(section) - 4 // the CRC
	count := 0
	for off := 10; off+4 <= end; {
		length := int(section[off+2]&0x0f)<<8 | int(section[off+3])
		if length == 0 || section[off+4] != 0xb5 {
			break // not a programme record: the reader breaks here too
		}
		count++
		if firmwareStride {
			off += length + titleRecordHeader
		} else {
			off += length
		}
	}
	return count
}

// TC-6.4. Twelve records must arrive as twelve, and the reference arithmetic
// has to be SHOWN failing on the same bytes — otherwise "12" is just a number
// the test and the builder agreed on between themselves.
//
// The firmware's loop is `local_84 += local_8a + 4` with
// `memcpy(dst, section + local_84 + 4, local_8a)`, so the twelve-bit field at
// r+2..3 counts the bytes AFTER the four-byte header. tvheadend reads it the
// same way. openTVtoXML advances by the field alone, and a writer that
// followed it declared four bytes too many per record: the walk landed inside
// record two, read the tag 0xB5 as a length of 0x511, and ran off the end.
func TestTwelveRecordsArriveAsTwelveAndAsTwoTheReferenceWay(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	section := twelveProgrammes(t, dict)

	if got := walkTitles(section, true); got != 12 {
		t.Errorf("the firmware's arithmetic finds %d records, want 12", got)
	}
	// The wrong version, shown failing on the very same bytes.
	//
	// The COUNT this walk lands on is deliberately not pinned. THE BOX READS
	// TWO from the same bytes (titles_firmware_test.go), and this simulated
	// walk reads one, because the two stop for different reasons: ours breaks
	// when the byte at +4 is not 0xB5, while the firmware carries on into a
	// length it reads out of the middle of a record. That divergence is worth
	// knowing and is exactly why the box-level test exists — a reader that
	// merely agrees with our own arithmetic proves nothing about the firmware's.
	// What is invariant in both is that the reference reads a fraction of what
	// was broadcast and reports no error.
	wrong := walkTitles(section, false)
	if wrong >= 12 {
		t.Errorf("openTVtoXML's arithmetic found %d of 12 records, so this section does not "+
			"distinguish the two readings and proves nothing", wrong)
	}
	t.Logf("firmware arithmetic 12 of 12; openTVtoXML's %d of 12, silently", wrong)
}

// The length field is the whole task, so it is asserted directly rather than
// only through a walk: every record must declare exactly its descriptor, and
// the reader supplies the four header bytes itself.
func TestRecordLengthCountsOnlyTheBytesAfterTheHeader(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	section := twelveProgrammes(t, dict)
	end := len(section) - 4
	records := 0
	for off := 10; off+4 <= end; {
		length := int(section[off+2]&0x0f)<<8 | int(section[off+3])
		if section[off+4] != 0xb5 {
			t.Fatalf("record %d at %d is tagged %#x, want 0xB5", records, off, section[off+4])
		}
		// The descriptor is [0xB5][len][body], so the record's declared length
		// is exactly that, and the whole record is four bytes longer.
		if declared := int(section[off+5]) + 2; declared != length {
			t.Fatalf("record %d declares %d but its descriptor is %d bytes", records, length, declared)
		}
		records++
		off += length + titleRecordHeader
	}
	if records != 12 {
		t.Fatalf("walked %d records, want 12", records)
	}
	if got := 10 + sumRecordLengths(section); got != end {
		t.Fatalf("records end at %d, section body ends at %d", got, end)
	}
}

func sumRecordLengths(section []byte) int {
	end := len(section) - 4
	total := 0
	for off := 10; off+4 <= end; {
		length := int(section[off+2]&0x0f)<<8 | int(section[off+3])
		total += length + titleRecordHeader
		off += length + titleRecordHeader
	}
	return total
}

func TestTitleSectionFramesItsHeaderAndCRC(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	section := twelveProgrammes(t, dict)
	if section[0] != 0xa1 {
		t.Errorf("table id %#x, want 0xa1", section[0])
	}
	if declared := int(section[1]&0x0f)<<8 | int(section[2]); declared+3 != len(section) {
		t.Errorf("section_length %d does not describe %d bytes", declared, len(section))
	}
	if got := uint16(section[3])<<8 | uint16(section[4]); got != 0x0bb8 {
		t.Errorf("channel id %#x, want the BAT's listings reference 0x0bb8", got)
	}
	// The two bytes the hardware match unit tests. Get these wrong and the
	// section is never delivered, which looks exactly like being ignored.
	if section[8] != 0x9e || section[9] != 0x8b {
		t.Errorf("filter bytes %#x %#x, want the requested MJD", section[8], section[9])
	}
	if dvb.MPEGCRC32(section) != 0 {
		t.Error("title section CRC does not verify")
	}
}

func TestTitleRecordRefusesWhatTheWireCannotCarry(t *testing.T) {
	t.Parallel()
	dict := testDictionary(t)
	for _, tc := range []struct {
		name   string
		record TitleRecord
	}{
		{"odd start", TitleRecord{Start: 1, Duration: 60, Title: "A"}},
		{"odd duration", TitleRecord{Start: 0, Duration: 61, Title: "A"}},
		{"start beyond sixteen bits halved", TitleRecord{Start: 0x20000, Duration: 60, Title: "A"}},
		{"rating beyond a nibble", TitleRecord{Start: 0, Duration: 60, Title: "A", Rating: 0x10}},
	} {
		if _, err := titleRecord(dict, tc.record); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}
	if _, err := TitleSection(0xa1, 1, [2]byte{}, 0, 0, 0, dict, nil); err == nil {
		t.Error("built a title section with no records")
	}
	if _, err := TitleSection(0xa1, 1, [2]byte{}, 0, 0, 0, nil, []TitleRecord{{Title: "A"}}); err == nil {
		t.Error("built a title section with no dictionary")
	}
}
