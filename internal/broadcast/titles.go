package broadcast

import "fmt"

// Sky's OpenTV title sections: the programmes themselves.
//
//	data[0]      table id -- titles 0xA0..0xA4 and 0xB0
//	data[3..4]   channel id, the value fed in the BAT's 0xB1 entry +3..4
//	data[8..9]   the two bytes the hardware match unit filters on (the MJD)
//	data[10..]   records
//
// and one record:
//
//	r+0..1  event id
//	r+2..3  packet_length, twelve bits
//	r+4     0xB5, the programme tag -- the reader BREAKS if it is not this
//	r+5     descriptor length
//	r+6..7  start time, seconds into the day, carried halved
//	r+8..9  duration, carried halved
//	r+10    genre
//	r+12    rating, low nibble
//	r+13..  Huffman title
//
// Field offsets are the firmware's, confirmed against tvheadend's reader.

// TitleRecord is one programme in a title section. Times are seconds into the
// day; the wire carries them halved, so both quantise to two seconds.
type TitleRecord struct {
	EventID  uint16
	Start    int // seconds into the day
	Duration int // seconds
	Title    string
	Genre    byte
	Rating   byte // low nibble
}

// titleRecordHeader is the four bytes the length field does NOT count: the
// event id and the length itself.
const titleRecordHeader = 4

// TitleSection builds one Sky/OpenTV title section.
//
// channelID is the value fed in the BAT's 0xB1 entry +3..4 for this service,
// which is what the box turns into the table-id extension it asks for. filter
// is the two bytes at data[8..9] that the hardware match unit demands — the
// MJD the box is currently requesting, which on an unclocked box is 0x9E 0x8B,
// MJD 40587, the Unix epoch. Read both off the machine rather than choosing
// them: a section the filter does not match is never delivered, and from
// outside that is indistinguishable from one the box received and ignored.
func TitleSection(tableID byte, channelID uint16, filter [2]byte, version byte,
	sectionNumber, lastSectionNumber byte, dict *HuffmanDictionary, records []TitleRecord) ([]byte, error) {
	if dict == nil {
		return nil, fmt.Errorf("broadcast: title sections need a huffman dictionary; see dictionaries/MANIFEST.md")
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("broadcast: a title section with no records announces nothing")
	}
	payload := []byte{byte(channelID >> 8), byte(channelID & 0xff), // #nosec G115 -- masked
		0xc1 | (version&0x1f)<<1, sectionNumber, lastSectionNumber, filter[0], filter[1]}
	for _, record := range records {
		encoded, err := titleRecord(dict, record)
		if err != nil {
			return nil, err
		}
		payload = append(payload, encoded...)
	}
	length := len(payload) + 4 // the CRC, counted from data[3]
	if length > maxSectionLength {
		return nil, fmt.Errorf("broadcast: title section length %d exceeds %d; split it across section_number", length, maxSectionLength)
	}
	section := append([]byte{tableID, 0xb0 | byte(length>>8), byte(length)}, payload...) // #nosec G115 -- length checked above
	return withCRC(section), nil
}

// titleRecord encodes one programme.
//
// THE LENGTH COUNTS THE BYTES AFTER THE FOUR-BYTE HEADER, AND THE READER ADDS
// THOSE FOUR BACK. The firmware's loop is `local_84 += local_8a + 4` with
// `memcpy(dst, section + local_84 + 4, local_8a)`, and tvheadend reads it the
// same way: `slen = buf[2..3] & 0xfff; i = 4; while (i < slen+4)`.
//
// openTVtoXML — the reference nearly everyone copies — advances by the field
// ALONE. A writer that follows it declares four bytes too many per record, so
// the walk lands inside record two, reads the tag 0xB5 as a length of 0x511,
// and runs off the end of the section. Twelve records then arrive as TWO,
// with no error anywhere: the box accepts the section and quietly registers a
// sixth of it. That is why this is spelled out rather than left to the reader.
func titleRecord(dict *HuffmanDictionary, record TitleRecord) ([]byte, error) {
	if record.Start%2 != 0 || record.Duration%2 != 0 {
		return nil, fmt.Errorf("broadcast: start and duration are carried halved, so both must be even seconds")
	}
	if record.Start < 0 || record.Duration < 0 || record.Start/2 > 0xffff || record.Duration/2 > 0xffff {
		return nil, fmt.Errorf("broadcast: start %d and duration %d must halve into sixteen bits", record.Start, record.Duration)
	}
	if record.Rating > 0x0f {
		return nil, fmt.Errorf("broadcast: rating %#x exceeds the low nibble", record.Rating)
	}
	text, err := dict.Encode(record.Title)
	if err != nil {
		return nil, err
	}
	start, duration := record.Start/2, record.Duration/2
	body := make([]byte, 0, 7+len(text))
	body = append(body,
		byte(start>>8&0xff), byte(start&0xff), // #nosec G115 -- range checked above
		byte(duration>>8&0xff), byte(duration&0xff), // #nosec G115 -- range checked above
		record.Genre,
		0x00, // r+11: read by nothing we have measured, and left zero rather than named
		record.Rating&0x0f,
	)
	body = append(body, text...)
	if len(body) > 0xff {
		return nil, fmt.Errorf("broadcast: programme %q does not fit one descriptor", record.Title)
	}
	descriptor := append([]byte{0xb5, byte(len(body))}, body...) // #nosec G115 -- length checked above
	if len(descriptor) > 0x0fff {
		return nil, fmt.Errorf("broadcast: record too long for a twelve-bit length")
	}
	out := make([]byte, 0, titleRecordHeader+len(descriptor))
	out = append(out, byte(record.EventID>>8), byte(record.EventID&0xff), // #nosec G115 -- masked
		0xf0|byte(len(descriptor)>>8), byte(len(descriptor))) // #nosec G115 -- length checked above
	return append(out, descriptor...), nil
}
