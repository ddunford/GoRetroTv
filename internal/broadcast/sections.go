// Package broadcast builds DVB service information sections for the guest demux.
package broadcast

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ddunford/goretrotv/internal/dvb"
)

const maxSectionLength = 1021

// Service describes one service in a transport stream. Names use DVB's default
// character set; this builder accepts its unambiguous printable ASCII subset.
type Service struct {
	ID           uint16
	Type         byte   // zero defaults to digital television (1)
	Provider     string // empty defaults to BSkyB
	Name         string
	EITSchedule  bool
	NoEITPresent bool // the present/following flag defaults to enabled
}

// Transport describes a satellite transport and the services announced by its NIT.
type Transport struct {
	ID           uint16
	NetworkID    uint16
	FrequencyMHz int // in MHz; DVB encodes this in 10 kHz units
	OrbitTenths  int // e.g. 282 for 28.2 degrees east
	West         bool
	Polarisation byte // 0 horizontal, 1 vertical, 2 left, 3 right
	SymbolRate   int  // ksymbols per second
	FEC          byte // DVB inner FEC nibble
	Services     []Service
	// Lineup is Sky's channel list for this transport, carried in the BAT and
	// ignored by every other table. It rides on the transport because the guest
	// seeds each record it builds from that transport's own context.
	Lineup []LineupEntry
}

// LineupEntry is one nine-byte entry of the private 0xB1 channel-list
// descriptor. The field widths and their destinations were measured by
// watching which bytes the guest's parser reads and where it stores them
// (record sky-eluc.38); they are not read off a public table, and two of them
// still have no name this project is willing to assert.
//
// Each entry becomes an 18-byte record in the guest:
//
//	ServiceID +0..1  -> record[4..5]
//	Kind      +2     -> record[12]
//	Listings  +3..4  -> record[6..7]
//	Extra     +5..6  -> record[8..9]
//	Channel   +7..8  -> record[10..11], the top twelve bits
//	Flags     +7..8  -> record[13..16], the low four bits, one per byte
type LineupEntry struct {
	ServiceID uint16
	// Kind is the one-byte field at +2. Zero defaults to 1, the value every
	// measured feed used.
	Kind byte
	// Listings is the reference the box turns into a table-id extension when it
	// asks for this service's listings: feeding 0x0BB8 made it request table
	// 0xA1 extension 0x0BBB. That is observation, not a named field.
	Listings uint16
	// Extra is the halfword at +5..6. Its destination is measured, its meaning
	// is not established, and naming it would be a guess.
	Extra uint16
	// Channel is twelve bits, so the largest value is 4095.
	Channel uint16
	// Flags is four bits, unpacked by the guest into four separate bytes.
	Flags byte
}

// TimeOffset describes the local time offset descriptor carried in a TOT.
// OffsetMinutes and NextOffsetMinutes are east-of-UTC values. ChangeUTC is
// encoded even when the offset is unchanged, as in the measured 1998 GMT feed.
type TimeOffset struct {
	Country           string
	Region            byte
	OffsetMinutes     int
	ChangeUTC         time.Time
	NextOffsetMinutes int
}

// NIT builds a network information table (actual network, table 0x40).
// networkID and transport IDs must come from the guest's current subscription.
func NIT(networkID uint16, version byte, networkName string, transports []Transport) ([]byte, error) {
	name, err := ascii(networkName)
	if err != nil {
		return nil, fmt.Errorf("broadcast: network name: %w", err)
	}
	if len(name) > 255 {
		return nil, fmt.Errorf("broadcast: network name too long")
	}
	netDesc := append([]byte{0x40, byte(len(name))}, name...) // #nosec G115 -- length checked above
	var loop []byte
	for _, tr := range transports {
		sat, err := satelliteDescriptor(tr)
		if err != nil {
			return nil, err
		}
		services, err := serviceListDescriptor(tr.Services)
		if err != nil {
			return nil, err
		}
		desc := append(append([]byte(nil), sat...), services...)
		if len(desc) > 0x0fff {
			return nil, fmt.Errorf("broadcast: transport descriptor loop too long")
		}
		loop = appendU16(loop, tr.ID)
		loop = appendU16(loop, tr.NetworkID)
		loop = append(loop, 0xf0|byte(len(desc)>>8), byte(len(desc))) // #nosec G115 -- 12-bit length checked above
		loop = append(loop, desc...)
	}
	if len(loop) > 0x0fff {
		return nil, fmt.Errorf("broadcast: transport loop too long")
	}
	payload := make([]byte, 0, 4+len(netDesc)+len(loop))
	payload = append(payload, 0xf0|byte(len(netDesc)>>8), byte(len(netDesc))) // #nosec G115 -- descriptor length is at most 257
	payload = append(payload, netDesc...)
	payload = append(payload, 0xf0|byte(len(loop)>>8), byte(len(loop))) // #nosec G115 -- 12-bit loop length checked above
	payload = append(payload, loop...)
	return longSection(0x40, networkID, version, payload)
}

// SDT builds a service description table (actual transport, table 0x42).
func SDT(transportID, networkID uint16, version byte, services []Service) ([]byte, error) {
	payload := appendU16(nil, networkID)
	payload = append(payload, 0xff)
	for _, svc := range services {
		providerName := svc.Provider
		if providerName == "" {
			providerName = "BSkyB"
		}
		provider, err := ascii(providerName)
		if err != nil {
			return nil, fmt.Errorf("broadcast: service %d provider: %w", svc.ID, err)
		}
		name, err := ascii(svc.Name)
		if err != nil {
			return nil, fmt.Errorf("broadcast: service %d name: %w", svc.ID, err)
		}
		if len(provider) > 255 || len(name) > 255 || len(provider)+len(name)+9 > 255 {
			return nil, fmt.Errorf("broadcast: service %d descriptor too long", svc.ID)
		}
		// THE PRIVATE DATA SPECIFIER GOES FIRST, AND IT IS WHY THE GUIDE'S LIST SCREENS ARE EMPTY.
		//
		// DVB scopes a private_data_specifier to the descriptors that FOLLOW it in the same loop,
		// and the BAT already carries one ahead of its 0xB1 line-up for exactly that reason. What
		// no loop carried until now is one per SERVICE -- so a service object in the box had no
		// specifier at all.
		//
		// Measured, not reasoned: the ALL CHANNELS grid's row callback (0x800CB7B8) asks its
		// object for descriptor tag 0x5F, gets ZERO, and returns -1 without drawing, once per
		// channel. A census of every tag lookup both screens make found 0x4A and 0x4D answering
		// non-zero and 0x5F answering ZERO on seven attempts out of seven, across both screens.
		// Nothing in this box has ever had one.
		desc := make([]byte, 0, 11+len(provider)+len(name))
		desc = append(desc, 0x5f, 4, 0x00, 0x00, 0x00, skyPrivateDataSpecifier)
		desc = append(desc, 0x48, byte(3+len(provider)+len(name)), serviceType(svc), byte(len(provider))) // #nosec G115 -- both lengths checked above
		desc = append(desc, provider...)
		desc = append(desc, byte(len(name))) // #nosec G115 -- name length checked above
		desc = append(desc, name...)
		flags := byte(0xfc)
		if svc.EITSchedule {
			flags |= 2
		}
		if !svc.NoEITPresent {
			flags |= 1
		}
		payload = appendU16(payload, svc.ID)
		payload = append(payload, flags, 0x80|byte(len(desc)>>8), byte(len(desc))) // #nosec G115 -- descriptor length checked above
		payload = append(payload, desc...)
	}
	return longSection(0x42, transportID, version, payload)
}

// BAT builds a bouquet association table (table 0x4A) carrying Sky's channel
// list. bouquetID must be the one the guest's own section filter is asking for:
// Sky's real bouquets are 0x1001..0x1004, but a box whose filter matches 0x1000
// exactly will never be delivered a section announcing anything else, and an
// undelivered section is indistinguishable from an ignored one.
//
// The channel list is a private descriptor, so it is only looked at inside a
// declared namespace: a 0x5F private_data_specifier carrying value 2 goes ahead
// of it in the same loop, and with any other specifier the guest walks straight
// past the 0xB1 without reading a byte of it.
//
// Every loop also carries a 0x4A linkage of type 0x91, because the guide asks
// its database exactly ONE question and that is the question. Without an
// answer it takes its not-answered arm and draws nothing, which is why a box
// fed a perfectly good line-up still reports no schedule information.
func BAT(bouquetID uint16, version byte, name string, transports []Transport) ([]byte, error) {
	bouquetName, err := ascii(name)
	if err != nil {
		return nil, fmt.Errorf("broadcast: bouquet name: %w", err)
	}
	if len(bouquetName) > 255 {
		return nil, fmt.Errorf("broadcast: bouquet name too long")
	}
	if len(transports) == 0 {
		return nil, fmt.Errorf("broadcast: a BAT must announce at least one transport")
	}
	bouquetLoop := append([]byte{0x47, byte(len(bouquetName))}, bouquetName...) // #nosec G115 -- length checked above
	// The bouquet-level linkage names a service the same BAT declares, on the
	// first transport it announces.
	bouquetLinkage, err := linkageDescriptor(transports[0])
	if err != nil {
		return nil, err
	}
	bouquetLoop = append(bouquetLoop, bouquetLinkage...)

	var tsLoop []byte
	for _, tr := range transports {
		descriptors, err := lineupDescriptors(tr.Lineup)
		if err != nil {
			return nil, err
		}
		services, err := serviceListDescriptor(tr.Services)
		if err != nil {
			return nil, err
		}
		descriptors = append(descriptors, services...)
		linkage, err := linkageDescriptor(tr)
		if err != nil {
			return nil, err
		}
		descriptors = append(descriptors, linkage...)
		if len(descriptors) > 0xfff {
			return nil, fmt.Errorf("broadcast: transport descriptors exceed twelve bits")
		}
		tsLoop = appendU16(tsLoop, tr.ID)
		tsLoop = appendU16(tsLoop, tr.NetworkID)
		tsLoop = append(tsLoop, 0xf0|byte(len(descriptors)>>8), byte(len(descriptors))) // #nosec G115 -- length checked above
		tsLoop = append(tsLoop, descriptors...)
	}
	if len(bouquetLoop) > 0xfff || len(tsLoop) > 0xfff {
		return nil, fmt.Errorf("broadcast: BAT descriptor loop exceeds twelve bits")
	}

	payload := append([]byte{0xf0 | byte(len(bouquetLoop)>>8), byte(len(bouquetLoop))}, bouquetLoop...) // #nosec G115 -- length checked above
	payload = append(payload, 0xf0|byte(len(tsLoop)>>8), byte(len(tsLoop)))                             // #nosec G115 -- length checked above
	payload = append(payload, tsLoop...)
	return longSection(0x4a, bouquetID, version, payload)
}

// linkageDescriptor builds the one thing the TV guide asks for. A guide press
// makes a single database call, and the callback behind it reads exactly these
// fields off any descriptor tagged 0x4A:
//
//	desc[2..3] transport_stream_id   desc[4..5] original_network_id
//	desc[6..7] service_id            desc[8]    linkage_type
//
// The caller then requires the search to have succeeded AND the type to be
// 0x91. Give it that and the answered arm runs -- measured as 14,944 more
// o-code instructions and three more events on the same press -- and the box
// goes on to ask for PID 0x30, the first of Sky's title PIDs, which it had
// never asked for before. Give it anything else and the guide reaches its
// not-answered arm and draws nothing, which looks exactly like a box with no
// listings rather than like a section it rejected.
//
// IT MUST BE IN BOTH OF THE BAT'S DESCRIPTOR LOOPS. Which structure the search
// walks is not established, and the measurement that proved the mechanism put
// it in both; in the transport loop alone it changed nothing. A descriptor in
// the wrong loop is a section the box accepts and silently ignores.
func linkageDescriptor(tr Transport) ([]byte, error) {
	if len(tr.Services) == 0 {
		// The linkage has to name a service this BAT declares. Emitting one
		// that names nothing, or a plausible default, would be a BAT the guide
		// answers with a service that does not exist.
		return nil, fmt.Errorf("broadcast: transport %#x declares no service for its linkage to name", tr.ID)
	}
	desc := []byte{0x4a, 7}
	desc = appendU16(desc, tr.ID)
	desc = appendU16(desc, tr.NetworkID)
	desc = appendU16(desc, tr.Services[0].ID)
	return append(desc, linkageGuideSchedule), nil
}

// linkageGuideSchedule is the linkage_type the guide demands. It is outside
// DVB's assigned range, which is why it is here and not in a public table.
const linkageGuideSchedule = 0x91

// skyPrivateDataSpecifier is the namespace Sky's private descriptors live in. DVB scopes a
// private_data_specifier to the descriptors that follow it in the same loop, and with any other
// value the guest walks straight past them.
const skyPrivateDataSpecifier = 2

// lineupGate is the halfword the guest tests before it will read a single
// entry. Anything else and it re-reads those two bytes and returns, decoding
// nothing at all and reporting nothing -- a descriptor the box accepts and
// silently ignores, which is the failure mode this whole line of work kept
// meeting. It is a measured sentinel, not a length or a count.
const lineupGate = 0xffff

// maxLineupEntries is what fits one descriptor: the length field is a byte, the
// gate costs two of it, and each entry is nine. A longer line-up is split
// across several 0xB1 descriptors, which the guest appends into one array
// because its running index lives in the transport context rather than being
// reset per descriptor.
const maxLineupEntries = (255 - 2) / 9

func lineupDescriptors(lineup []LineupEntry) ([]byte, error) {
	if len(lineup) == 0 {
		return nil, nil
	}
	// The specifier goes FIRST and once: DVB scopes it to the descriptors that
	// follow it in the same loop, so a 0xB1 ahead of its own namespace is one
	// the guest walks past without looking.
	out := []byte{0x5f, 4, 0x00, 0x00, 0x00, skyPrivateDataSpecifier}
	for start := 0; start < len(lineup); start += maxLineupEntries {
		end := min(start+maxLineupEntries, len(lineup))
		body := []byte{lineupGate >> 8, lineupGate & 0xff}
		for _, entry := range lineup[start:end] {
			if entry.Channel > 0x0fff {
				return nil, fmt.Errorf("broadcast: channel %d exceeds twelve bits", entry.Channel)
			}
			if entry.Flags > 0x0f {
				return nil, fmt.Errorf("broadcast: flags %#x exceed four bits", entry.Flags)
			}
			kind := entry.Kind
			if kind == 0 {
				kind = 1
			}
			body = appendU16(body, entry.ServiceID)
			body = append(body, kind)
			body = appendU16(body, entry.Listings)
			body = appendU16(body, entry.Extra)
			body = appendU16(body, entry.Channel<<4|uint16(entry.Flags))
		}
		out = append(out, 0xb1, byte(len(body))) // #nosec G115 -- maxLineupEntries keeps this under 255
		out = append(out, body...)
	}
	return out, nil
}

// TDT builds the UTC time and date table. It has no CRC.
func TDT(utc time.Time) ([]byte, error) {
	clock, err := dvbTime(utc)
	if err != nil {
		return nil, err
	}
	return append([]byte{0x70, 0x70, 0x05}, clock...), nil
}

// TOT builds the time offset table. Unlike TDT, it has a CRC despite its
// section_syntax_indicator being zero.
func TOT(utc time.Time, offset TimeOffset) ([]byte, error) {
	clock, err := dvbTime(utc)
	if err != nil {
		return nil, err
	}
	change, err := dvbTime(offset.ChangeUTC)
	if err != nil {
		return nil, fmt.Errorf("broadcast: offset change: %w", err)
	}
	if len(offset.Country) != 3 {
		return nil, fmt.Errorf("broadcast: country must have three ASCII letters")
	}
	for _, c := range offset.Country {
		if c < 'A' || c > 'Z' {
			return nil, fmt.Errorf("broadcast: country must have three ASCII letters")
		}
	}
	if offset.Region > 63 {
		return nil, fmt.Errorf("broadcast: region exceeds six bits")
	}
	current, err := offsetBCD(offset.OffsetMinutes)
	if err != nil {
		return nil, err
	}
	next, err := offsetBCD(offset.NextOffsetMinutes)
	if err != nil {
		return nil, err
	}
	polarity := byte(0)
	if offset.OffsetMinutes < 0 {
		polarity = 1
	}
	desc := make([]byte, 0, 15)
	desc = append(desc, 0x58, 13, offset.Country[0], offset.Country[1], offset.Country[2], offset.Region<<2|2|polarity)
	desc = append(desc, current...)
	desc = append(desc, change...)
	desc = append(desc, next...)
	length := 5 + 2 + len(desc) + 4
	section := make([]byte, 0, 3+len(clock)+2+len(desc)+4)
	section = append(section, 0x73, 0x70|byte(length>>8), byte(length)) // #nosec G115 -- fixed-length TOT is below 4096
	section = append(section, clock...)
	section = append(section, 0xf0|byte(len(desc)>>8), byte(len(desc))) // #nosec G115 -- fixed-length descriptor is 15 bytes
	section = append(section, desc...)
	return withCRC(section), nil
}

func longSection(table byte, extension uint16, version byte, payload []byte) ([]byte, error) {
	if version > 31 {
		return nil, fmt.Errorf("broadcast: version exceeds five bits")
	}
	length := 5 + len(payload) + 4
	if length > maxSectionLength {
		return nil, fmt.Errorf("broadcast: section length %d exceeds %d", length, maxSectionLength)
	}
	section := []byte{table, 0xb0 | byte(length>>8), byte(length)} // #nosec G115 -- section length checked above
	section = appendU16(section, extension)
	section = append(section, 0xc1|(version<<1), 0, 0)
	section = append(section, payload...)
	return withCRC(section), nil
}

func withCRC(section []byte) []byte {
	crc := dvb.MPEGCRC32(section)
	return binary.BigEndian.AppendUint32(section, crc)
}

func appendU16(dst []byte, value uint16) []byte { return binary.BigEndian.AppendUint16(dst, value) }

func ascii(s string) ([]byte, error) {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return nil, fmt.Errorf("non-ASCII printable byte at %d", i)
		}
	}
	return []byte(s), nil
}

func bcd(n int) (byte, error) {
	if n < 0 || n > 99 {
		return 0, fmt.Errorf("broadcast: BCD value %d out of range", n)
	}
	return byte(n/10<<4 | n%10), nil // #nosec G115 -- n is in 0..99
}

func bcdDigits(value, digits int) ([]byte, error) {
	limit := 1
	for i := 0; i < digits; i++ {
		limit *= 10
	}
	if value < 0 || value >= limit || digits%2 != 0 {
		return nil, fmt.Errorf("broadcast: %d does not fit %d BCD digits", value, digits)
	}
	out := make([]byte, digits/2)
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = byte(value % 10)
		value /= 10
		out[i] |= byte(value%10) << 4
		value /= 10
	}
	return out, nil
}

func satelliteDescriptor(tr Transport) ([]byte, error) {
	if tr.FrequencyMHz < 0 || tr.FrequencyMHz >= 1_000_000 ||
		tr.OrbitTenths < 0 || tr.OrbitTenths >= 10_000 ||
		tr.SymbolRate < 0 || tr.SymbolRate >= 10_000_000 {
		return nil, fmt.Errorf("broadcast: satellite BCD field out of range")
	}
	freq, err := bcdDigits(tr.FrequencyMHz*100, 8)
	if err != nil {
		return nil, fmt.Errorf("broadcast: satellite frequency: %w", err)
	}
	orbit, err := bcdDigits(tr.OrbitTenths, 4)
	if err != nil {
		return nil, fmt.Errorf("broadcast: orbital position: %w", err)
	}
	if tr.Polarisation > 3 || tr.FEC > 15 {
		return nil, fmt.Errorf("broadcast: satellite flags out of range")
	}
	// Seven BCD digits of 100-symbol/s rate, followed by the FEC nibble.
	sym, err := bcdDigits(tr.SymbolRate*10, 8)
	if err != nil {
		return nil, fmt.Errorf("broadcast: symbol rate: %w", err)
	}
	sym[3] = sym[3]&0xf0 | tr.FEC
	flags := byte(0x81 | tr.Polarisation<<5)
	if tr.West {
		flags &^= 0x80
	}
	return append(append(append([]byte{0x43, 11}, freq...), orbit...), append([]byte{flags}, sym...)...), nil
}

func serviceListDescriptor(services []Service) ([]byte, error) {
	if len(services)*3 > 255 {
		return nil, fmt.Errorf("broadcast: service list descriptor too long")
	}
	desc := []byte{0x41, byte(len(services) * 3)} // #nosec G115 -- descriptor length checked above
	for _, svc := range services {
		desc = appendU16(desc, svc.ID)
		desc = append(desc, serviceType(svc))
	}
	return desc, nil
}

func serviceType(svc Service) byte {
	if svc.Type == 0 {
		return 1
	}
	return svc.Type
}

func dvbTime(value time.Time) ([]byte, error) {
	if value.IsZero() {
		return nil, fmt.Errorf("broadcast: zero UTC time")
	}
	value = value.UTC()
	if value.Year() < 1858 || value.Year() > 9999 {
		return nil, fmt.Errorf("broadcast: time outside MJD range")
	}
	// 1970-01-01 is MJD 40587. Floor division also handles pre-1970 dates.
	seconds := value.Unix()
	days := seconds / 86400
	if seconds < 0 && seconds%86400 != 0 {
		days--
	}
	days += 40587
	if days < 0 || days > 65535 {
		return nil, fmt.Errorf("broadcast: time outside MJD range")
	}
	h, _ := bcd(value.Hour())
	m, _ := bcd(value.Minute())
	s, _ := bcd(value.Second())
	return []byte{byte(days >> 8), byte(days), h, m, s}, nil // #nosec G115 -- MJD range checked above
}

func offsetBCD(minutes int) ([]byte, error) {
	if minutes < -(23*60+59) || minutes > 23*60+59 {
		return nil, fmt.Errorf("broadcast: offset outside BCD range")
	}
	if minutes < 0 {
		minutes = -minutes
	}
	h, _ := bcd(minutes / 60)
	m, _ := bcd(minutes % 60)
	return []byte{h, m}, nil
}

// Programme is one entry of a programme association table: a programme number and
// the PID its programme map table is carried on.
type Programme struct {
	// Number is the programme_number. Zero is reserved for the network PID and is
	// refused here, because a caller that means "the NIT" should say so elsewhere.
	Number uint16
	// MapPID is the programme_map_PID, the PID its PMT is broadcast on.
	MapPID uint16
}

// PAT builds a programme association table (table 0x00), the root of a transport
// stream: it says which programmes exist and where each one's map table is found.
//
// THIS BOX ASKS FOR IT AND HAS NEVER BEEN ANSWERED. PID 0x0000 is armed during
// acquisition -- transiently, which is why a single sample of the demux missed it
// and a continuous watch across a session found it -- and a section on a PID with
// no armed filter is dropped by the hardware before any code sees it. So the PAT
// is not a guess about what the firmware might like; it is the one table the box
// demonstrably opens a filter for and receives nothing on.
//
// The layout is ISO/IEC 13818-1, which is why this can be built rather than
// measured: phase 7's rule is not to guess a format, and this format is specified.
// What is NOT assumed is what the box does with it -- that is measured by watching
// which PID it arms next.
func PAT(transportStreamID uint16, version byte, programmes []Programme) ([]byte, error) {
	if len(programmes) == 0 {
		return nil, fmt.Errorf("broadcast: a PAT with no programmes announces nothing")
	}
	var loop []byte
	for _, p := range programmes {
		if p.Number == 0 {
			return nil, fmt.Errorf("broadcast: programme number 0 is reserved for the network PID")
		}
		if p.MapPID > 0x1fff {
			return nil, fmt.Errorf("broadcast: programme map PID %#x exceeds thirteen bits", p.MapPID)
		}
		loop = appendU16(loop, p.Number)
		// Three reserved bits set, then the thirteen-bit PID.
		loop = appendU16(loop, 0xe000|p.MapPID)
	}
	return longSection(0x00, transportStreamID, version, loop)
}

// IndexRecord is one nine-byte entry of a table 0xC1 A-Z index section.
//
// The box subscribes to table 0xC1 under an extension PER LETTER -- ninety-two such subscriptions
// were walked out of its own dispatcher tree -- and this port has never answered one. The consumer
// at 0x800C4C34 states the format in its own arithmetic: it takes the section length, subtracts
// nine, divides by NINE for the record count, and allocates ten bytes per record plus a twelve-byte
// header. So the wire record is nine bytes and the stored one is ten.
//
// WHAT IS MEASURED AND WHAT IS NOT. The FIELD WIDTHS are measured, by sweeping every bit of every
// byte through a real box and watching where each one lands:
//
//	wire (9 bytes)                     memory (10 bytes)
//	rec[0..1]  a 16-bit id         ->  out[0..1]
//	rec[2] bits 7..4               ->  out[2] bits 7..4
//	rec[3] bits 7..6               ->  out[2] bits 1..0
//	rec[2] bits 3..0               ->  out[3], as four TWO-BIT fields, 1 set / 2 clear
//	rec[3] bits 5..0               ->  nothing; six wire bits this parser does not read
//	rec[4] -> out[4]   rec[5] -> out[6]   rec[6] -> out[7]
//	rec[7] -> out[8]   rec[8] -> out[9]        (out[5] is the box's own, never from the wire)
//
// **What those bytes MEAN is not established**, so this type carries them and names none of them.
// A field named on a guess is worse than a field named Data: the name gets believed.
type IndexRecord struct {
	// ID is the sixteen-bit identifier at rec[0..1], stored first in the in-memory record.
	ID uint16
	// Packed is rec[2]: its high nibble is copied straight through and its low nibble is unpacked
	// into four two-bit fields.
	Packed byte
	// Selector is rec[3]. Only its top two bits are read; the low six are ignored by the parser.
	Selector byte
	// Data is rec[4..8], copied verbatim into the stored record.
	Data [5]byte
}

// indexRecordSize is the wire size the consumer's own divisor declares.
const indexRecordSize = 9

// The extensions the 0xC1 consumer dispatches, and the ONLY ones worth transmitting.
//
// 0x800C4C34 ends in a four-way branch on the table-id extension, and an extension that falls
// through it is not merely ignored: the parser has already allocated and filled the whole record
// array by that point, so the section is decoded, dropped and LEAKED. From outside, a screen fed a
// mis-addressed index looks exactly like a screen fed nothing.
//
//	0x0000                one list head                       the alphabetical family
//	'A'..'Z'  0x41..0x5A  twenty-six list heads                the A-Z LISTINGS screens
//	0x00FF                one list head                        the alphabetical family
//	0x0100..0x01CF        indexed [category*4 + block]         the genre screens
//
// The genre arm computes its slot as `(ext & 0x0F) * 4 + ((ext & 0xC0) >> 6)` -- SIXTEEN categories
// by FOUR six-hour blocks, the same four blocks the title tables' low two bits select. Bits 4 and 5
// take no part in that sum, so 0x0110 aliases onto 0x0100's slot; the box subscribes only to the
// forms with those bits clear, and IndexCategory builds only those.
//
// AN EARLIER READING OF THIS PARSER SAID "THE EXTENSION MUST BE A LETTER". That was taken from the
// free path at 0x800C4F94 alone and it is wrong in the expensive direction: it refuses precisely
// the sixty-four genre extensions the box subscribes to, which is most of what table 0xC1 is for.
const (
	indexCategories = 16
	indexBlocks     = 4
)

// IndexLetter is the extension for one A-Z LISTINGS screen.
func IndexLetter(letter byte) (uint16, error) {
	if letter < 'A' || letter > 'Z' {
		return 0, fmt.Errorf("broadcast: index letter %q is outside 'A'..'Z'; the consumer's "+
			"letter arm indexes twenty-six list heads and nothing else reaches them", letter)
	}
	return uint16(letter), nil
}

// IndexCategory is the extension for one genre screen's six-hour block.
//
// The category is the guide's own genre number and the block is the quarter of the day, the same
// one TitleTableID stamps into a title section's low two bits. Neither is named here: which number
// is MOVIES and which is KIDS is not established, and a constant called indexCategoryMovies would
// assert what nobody has measured.
func IndexCategory(category, block byte) (uint16, error) {
	if category >= indexCategories {
		return 0, fmt.Errorf("broadcast: index category %d is outside 0..%d; the consumer masks "+
			"the extension's low nibble", category, indexCategories-1)
	}
	if block >= indexBlocks {
		return 0, fmt.Errorf("broadcast: index block %d is outside 0..%d; a day is four six-hour "+
			"blocks", block, indexBlocks-1)
	}
	return 0x0100 | uint16(block)<<6 | uint16(category), nil
}

// IndexDispatched reports whether the consumer would route a section carrying this extension, or
// decode it and throw it away.
func IndexDispatched(extension uint16) bool {
	switch {
	case extension == 0x0000, extension == 0x00ff:
		return true
	case extension >= 'A' && extension <= 'Z':
		return true
	case extension >= 0x0100 && extension <= 0x01cf:
		return true
	default:
		return false
	}
}

// IndexSection builds one table 0xC1 index section.
//
// Build the extension with IndexLetter or IndexCategory rather than writing a number: a section the
// consumer does not dispatch is decoded, dropped and leaked, and says nothing on the way past.
func IndexSection(extension uint16, version, sectionNumber, lastSectionNumber byte,
	records []IndexRecord) ([]byte, error) {
	if !IndexDispatched(extension) {
		return nil, fmt.Errorf("broadcast: extension %#04x is not one the 0xC1 consumer "+
			"dispatches, so the box would build the record array and free it", extension)
	}
	// A SECTION WITH NO RECORDS IS LEGAL HERE, and unlike every other builder in this package it
	// is worth sending. The consumer computes its count as (section_length - 9) / 9, so an empty
	// payload is a count of zero -- a well-formed EMPTY LIST -- and the parser still allocates the
	// twelve-byte header and puts it in the letter's list head. That is the difference between a
	// letter with nothing on it and a letter the box has never heard of, and the screen behaves
	// differently for the two.
	payload := make([]byte, 0, len(records)*indexRecordSize)
	for _, record := range records {
		payload = appendU16(payload, record.ID)
		payload = append(payload, record.Packed, record.Selector)
		payload = append(payload, record.Data[:]...)
	}
	section, err := longSection(0xc1, extension, version, payload)
	if err != nil {
		return nil, err
	}
	// The consumer reads the section and last-section numbers out of the long-section header, and
	// longSection writes zeros there because every other table this port sends is a single section.
	// The section number is not decoration here: the parser stores it as the list's first byte and
	// hands it to the screen alongside the records.
	section[6], section[7] = sectionNumber, lastSectionNumber
	return withCRC(section[:len(section)-4]), nil
}
