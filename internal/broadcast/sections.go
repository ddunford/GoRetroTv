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
		if len(provider) > 255 || len(name) > 255 || len(provider)+len(name)+3 > 255 {
			return nil, fmt.Errorf("broadcast: service %d descriptor too long", svc.ID)
		}
		desc := make([]byte, 0, 5+len(provider)+len(name))
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
	out := []byte{0x5f, 4, 0x00, 0x00, 0x00, 0x02}
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
