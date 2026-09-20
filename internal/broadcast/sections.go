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
