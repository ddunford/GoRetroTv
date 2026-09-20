// Package multiplex transmits the modelled broadcast into a running box.
//
// It owns one question, and the whole package exists because of the answer:
// **what is this box currently asking for?** Every id in a DVB section is a
// filter the hardware compares before anything in the firmware sees a byte, so
// a section addressed by guesswork is collected by the interrupt handler and
// dropped. From outside, that is indistinguishable from a section the box
// received and ignored -- which is why this project spent so long unable to
// tell "the listings are wrong" from "the listings never arrived".
//
// So nothing here chooses an id. The bouquet, the network, the table id, the
// table-id extension and the MJD are all read back off the box's own match
// units, and the transmitter builds sections to fit them.
package multiplex

import (
	"fmt"
	"time"

	"github.com/ddunford/goretrotv/internal/device/demux"
)

// Subscription is what a box has told its hardware it will accept.
type Subscription struct {
	// BouquetID and NetworkID are the ids the BAT and NIT must carry.
	BouquetID uint16
	NetworkID uint16
	// SIArmed is whether PID 0x11 is armed, i.e. whether a BAT pushed there
	// would reach the guest at all.
	SIArmed bool
	// ListingsPID is the title PID the box has armed, or zero.
	//
	// It is separate from Titles because THE BOX ARMS IT ON EVERY DAY while it
	// programs a match unit on only three days in eight (TASK-6.13). The PID is
	// therefore the reliable half of the subscription and the match unit is
	// not, which is what makes a derived request possible at all.
	ListingsPID uint16
	// Titles is every listings request the box has programmed, one per match
	// unit. It is empty until the box acquires, and on five days in eight it
	// stays empty even after it has.
	Titles []TitleRequest
}

// DerivedTitleRequest is the request to use when the box has armed a listings
// PID but programmed no match unit for it.
//
// THIS IS A HOST INTERVENTION AND IS REPORTED AS ONE. On real hardware the
// match unit is what admits a section, and this port's Push routes by PID
// alone -- so a section sent this way reaches the guest here and might not
// reach it on a Digibox. Every field is nevertheless measured rather than
// invented: the table id is 0xA3 on all three days the box does programme a
// unit, the extension is the channel's own listings id, and the MJD is the day
// the broadcast is claiming. The guest's parser then accepts them exactly as
// it accepts a filtered day -- 67 of 67 programmes registered on slots 2, 4
// and 7, which are three of the five that programme nothing.
func DerivedTitleRequest(pid uint16, mjd int, listingsID uint16) TitleRequest {
	return TitleRequest{
		TableID:       derivedTitleTable,
		TableMask:     0xfe,
		Extension:     listingsID,
		ExtensionMask: 0xffff,
		Filter:        [2]byte{byte(mjd >> 8), byte(mjd)}, // #nosec G115 -- an MJD is sixteen bits
		PID:           pid,
	}
}

// derivedTitleTable is the table id the box asks for whenever it asks at all.
// Measured constant across every subscribing day and every slot: 0xA3 with a
// 0xFE mask, so 0xA2 would also be admitted -- but the parser takes tableId & 3
// as part of its day-slot key, so the pair are not interchangeable and this is
// the one that was observed.
const derivedTitleTable = 0xa3

// TitleRequest is one programmed listings filter: which table, which service's
// listings id, which day, and the PID it will arrive on.
type TitleRequest struct {
	// TableID is the value the unit matches, and TableMask is how much of it
	// the hardware compares.
	//
	// THE MASK IS NOT CONSTANT. It has been measured as both 0xFE, where the
	// unit accepts a PAIR of table ids, and 0xFF, where it accepts exactly
	// one. Assuming 0xFE cost a day here: a reader that skipped anything else
	// reported "the box has programmed no listings filter" about a box whose
	// unit 7 was reading a3/ff with the right extension and the right day.
	// Sending back exactly the value that was read is correct under either,
	// which is why nothing downstream needs to branch on the mask -- the
	// parser takes tableId & 3 as part of its day-slot key, so the wrong one
	// of a pair is a silent mis-filing rather than a refusal.
	TableID   byte
	TableMask byte
	// Extension and ExtensionMask are data[3..4]: the listings id the box wants
	// programmes for, and how much of it the hardware compares.
	//
	// ONE REQUEST IS USUALLY ABOUT SEVERAL CHANNELS AT ONCE. The box builds a
	// set filter -- the value is the bitwise OR of the listings ids it wants
	// and the mask clears the bits that differ between them -- so six channels
	// on 0x0BB8..0x0BBD are asked for as a single bf/f8, matching 0x0BB8 to
	// 0x0BBF. Reading the value and ignoring the mask makes the box look like
	// it is asking about one channel that does not exist: with six channels
	// loaded it asks for 0x0BBF, which is nobody, and a transmitter that
	// answered literally would send nothing at all and report no error.
	Extension     uint16
	ExtensionMask uint16
	// Filter is data[8..9], the MJD of the day the box wants.
	Filter [2]byte
	// PID is where it must be transmitted. Measured: 0x30 | (MJD mod 8), Sky's
	// eight title PIDs being a day-of-eight rotation.
	PID uint16
}

// MJD is the day this request is for, as a Modified Julian Date.
func (r TitleRequest) MJD() int { return int(r.Filter[0])<<8 | int(r.Filter[1]) }

// Wants reports whether a channel's listings id falls inside this request.
//
// A zero mask would match everything, which is the one answer that must not be
// given by default: it would put every channel's programmes on a filter the
// box never asked about. It is treated as matching nothing instead.
func (r TitleRequest) Wants(listingsID uint16) bool {
	if r.ExtensionMask == 0 {
		return false
	}
	return listingsID&r.ExtensionMask == r.Extension&r.ExtensionMask
}

// mjdEpoch is day zero of the Modified Julian Date. MJD 50000 is 10 October
// 1995; check any other value against that fixed point rather than against a
// number someone produced.
var mjdEpoch = time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC)

// MJDOf is a day as a Modified Julian Date.
func MJDOf(day time.Time) int {
	return int(day.UTC().Truncate(24*time.Hour).Sub(mjdEpoch) / (24 * time.Hour))
}

// DayOfMJD is the midnight UTC a Modified Julian Date names.
func DayOfMJD(mjd int) time.Time { return mjdEpoch.AddDate(0, 0, mjd) }

// matchUnits is the number of independent byte-matching units the demux has.
const matchUnits = 16

// Standard PIDs, excluded when working out which PID the box armed for its
// listings: the three SI tables and the one it boots with.
var standardPIDs = map[uint16]bool{0x10: true, 0x11: true, 0x14: true, 0x52: true}

// isTitleUnit reports whether a match unit is a listings filter.
//
// It tests the two things that are actually invariant -- that the unit
// compares the table id at all, and that the id is one the box's title parsers
// take (0xA0..0xA4 and 0xB0) -- and nothing else. Every narrowing beyond that
// has been wrong at least once: assuming the 0xAn family alone, and assuming a
// 0xFE mask. A census that narrows its subject by a guess reports a plausible
// zero, and a plausible zero here reads as "the box is not interested".
func isTitleUnit(m demux.MatchByte) bool {
	if m.Mask == 0 {
		return false // the unit does not compare the table id at all
	}
	return (m.Value >= 0xa0 && m.Value <= 0xa4) || m.Value == 0xb0
}

// Read reports what the box is asking for.
//
// It fails rather than returning a plausible empty answer when the box is not
// asking for the SI at all, because that is a broken fixture or a machine that
// has not booted, not a box with nothing to say. The listings half is allowed
// to be empty: that is the ordinary state before acquisition.
func Read(d *demux.Demux) (Subscription, error) {
	if d == nil {
		return Subscription{}, fmt.Errorf("multiplex: no demux to read a subscription from")
	}
	var sub Subscription

	bouquet, ok := d.Match(3, 0)
	if !ok || bouquet.Value != 0x4a || bouquet.Mask != 0xff {
		return Subscription{}, fmt.Errorf("multiplex: the box is not asking for a BAT (unit 3 is %#02x/%#02x), "+
			"so a bouquet id cannot be read off it", bouquet.Value, bouquet.Mask)
	}
	high, okHigh := d.Match(3, 1)
	low, okLow := d.Match(3, 2)
	if !okHigh || !okLow || high.Mask != 0xff || low.Mask != 0xff {
		return Subscription{}, fmt.Errorf("multiplex: the box has no exact bouquet match, so the id cannot be read off it")
	}
	sub.BouquetID = uint16(high.Value)<<8 | uint16(low.Value)

	network, okNetwork := d.Match(1, 0)
	if !okNetwork || network.Value != 0x40 {
		return Subscription{}, fmt.Errorf("multiplex: the box is not asking for a NIT (unit 1 is %#02x), "+
			"so a network id cannot be read off it", network.Value)
	}
	networkHigh, okNH := d.Match(1, 1)
	networkLow, okNL := d.Match(1, 2)
	if !okNH || !okNL {
		return Subscription{}, fmt.Errorf("multiplex: the box has no network id to seed a transport with")
	}
	sub.NetworkID = uint16(networkHigh.Value)<<8 | uint16(networkLow.Value)

	listingsPID := uint16(0)
	for _, pid := range d.ArmedPIDs() {
		if pid == 0x11 {
			sub.SIArmed = true
		}
		if !standardPIDs[pid] {
			listingsPID = pid
		}
	}

	for unit := uint8(0); unit < matchUnits; unit++ {
		table, ok := d.Match(unit, 0)
		if !ok || !isTitleUnit(table) {
			continue
		}
		extHigh, _ := d.Match(unit, 1)
		extLow, _ := d.Match(unit, 2)
		mjdHigh, _ := d.Match(unit, 6)
		mjdLow, _ := d.Match(unit, 7)
		sub.Titles = append(sub.Titles, TitleRequest{
			TableID:       table.Value,
			TableMask:     table.Mask,
			Extension:     uint16(extHigh.Value)<<8 | uint16(extLow.Value),
			ExtensionMask: uint16(extHigh.Mask)<<8 | uint16(extLow.Mask),
			Filter:        [2]byte{mjdHigh.Value, mjdLow.Value},
			PID:           listingsPID,
		})
	}
	// A filter with no armed PID is half a subscription: there is nowhere to
	// deliver it, so it is not something the box is asking for.
	sub.ListingsPID = listingsPID
	if listingsPID == 0 {
		sub.Titles = nil
	}
	return sub, nil
}
