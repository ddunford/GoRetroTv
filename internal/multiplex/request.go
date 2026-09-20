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
	"sort"
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
	// ListingsPIDs are the title PIDs the box has armed, in ascending order.
	//
	// THERE IS USUALLY MORE THAN ONE, AND TAKING WHICHEVER CAME LAST IS A COIN
	// TOSS THAT LOSES ONE DAY IN EIGHT. A box in the evening arms today's PID
	// and tomorrow's, which are adjacent on seven days and 0x37 and 0x30 on
	// the eighth -- so "the last armed PID" is today's on some days, tomorrow's
	// on others, and on MJD mod 8 == 7 it silently transmitted the whole
	// schedule on tomorrow's PID and the box registered none of it. Which PID
	// a day's listings belong on is not a matter of choice: it is TitlePID.
	//
	// It is separate from Titles because THE BOX ARMS A PID ON EVERY DAY while
	// it programs a match unit only for days whose slot is one of three
	// (TASK-6.13). The PID is therefore the reliable half of the subscription
	// and the match unit is not, which is what makes a derived request
	// possible at all.
	ListingsPIDs []uint16
	// Titles is every listings request the box has programmed, one per match
	// unit. It is empty until the box acquires, and on the days it programs
	// nothing it stays empty even after it has.
	Titles []TitleRequest
}

// Arms reports whether the box has armed a PID, i.e. whether a section pushed
// there would reach the guest at all.
func (s Subscription) Arms(pid uint16) bool {
	for _, armed := range s.ListingsPIDs {
		if armed == pid {
			return true
		}
	}
	return false
}

// TitlePID is the PID a day's listings are carried on: Sky's eight title PIDs
// are a day-of-eight rotation, 0x30 | (MJD mod 8).
//
// Measured on sixteen days across two months and two times of day: every box
// armed exactly this PID for the day its clock was in, and an evening box also
// armed the one this function gives for the following day. It is a function of
// the day and nothing else, which is why the transmitter computes it rather
// than picking one off the box.
func TitlePID(mjd int) uint16 {
	return uint16(0x30 | ((mjd%8)+8)%8) // #nosec G115 -- three bits
}

// DerivedTitleRequest is the request to use when the box has armed a listings
// PID but programmed no match unit for it.
//
// THIS IS A HOST INTERVENTION AND IS REPORTED AS ONE. On real hardware the
// match unit is what admits a section, and this port's Push routes by PID
// alone -- so a section sent this way reaches the guest here and might not
// reach it on a Digibox. Every field is nevertheless measured rather than
// invented: the extension is the channel's own listings id, the PID is the
// day's own place in the eight-PID rotation, and the MJD is the day the
// broadcast is claiming. The guest's parser then accepts them exactly as it
// accepts a filtered day -- the box registers the blocks it is listening to,
// and the title reaches the screen, on every one of the eight slots and at
// every hour the schedule has television in.
func DerivedTitleRequest(mjd int, listingsID uint16) TitleRequest {
	return TitleRequest{
		// No unit matched, so there is no matched table id to report. The
		// blocks the sections are stamped with are TitleTableID's business
		// and are the same on a derived day as on a filtered one.
		Extension:     listingsID,
		ExtensionMask: 0xffff,
		Filter:        [2]byte{byte(mjd >> 8), byte(mjd)}, // #nosec G115 -- an MJD is sixteen bits
		PID:           TitlePID(mjd),
	}
}

// TitleRequest is one programmed listings filter: which table, which service's
// listings id, which day, and the PID it will arrive on.
type TitleRequest struct {
	// TableID is the value the unit matches, and TableMask is how much of it
	// the hardware compares. Both are zero on a derived request, where no unit
	// matched anything.
	//
	// THEY ARE A MEASUREMENT, NOT AN ADDRESS. The low two bits of a title
	// table id are which six-hour block of the day the section carries, and
	// the unit names the one block the box's hardware would admit; the guide
	// listens for the block it is in. The transmitter sends every block and
	// stamps each with TitleTableID, so nothing downstream reads these.
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
	// PID is where it must be transmitted: TitlePID of the day this request
	// names, which is the whole of the rule. It is NOT read off the box's
	// armed-PID list, because a box arms more than one and the list says
	// nothing about which day each belongs to.
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

	for _, pid := range d.ArmedPIDs() {
		if pid == 0x11 {
			sub.SIArmed = true
		}
		if !standardPIDs[pid] {
			sub.ListingsPIDs = append(sub.ListingsPIDs, pid)
		}
	}
	sort.Slice(sub.ListingsPIDs, func(i, j int) bool { return sub.ListingsPIDs[i] < sub.ListingsPIDs[j] })

	for unit := uint8(0); unit < matchUnits; unit++ {
		table, ok := d.Match(unit, 0)
		if !ok || !isTitleUnit(table) {
			continue
		}
		extHigh, _ := d.Match(unit, 1)
		extLow, _ := d.Match(unit, 2)
		mjdHigh, _ := d.Match(unit, 6)
		mjdLow, _ := d.Match(unit, 7)
		request := TitleRequest{
			TableID:       table.Value,
			TableMask:     table.Mask,
			Extension:     uint16(extHigh.Value)<<8 | uint16(extLow.Value),
			ExtensionMask: uint16(extHigh.Mask)<<8 | uint16(extLow.Mask),
			Filter:        [2]byte{mjdHigh.Value, mjdLow.Value},
		}
		// THE UNIT'S DAY IS NOT ALWAYS TODAY. A box in the evening programmes
		// its filter for TOMORROW as often as for today -- 0xA0 with the next
		// MJD rather than 0xA3 with this one -- so the PID follows the day the
		// unit names rather than the day the broadcast is claiming.
		request.PID = TitlePID(request.MJD())
		sub.Titles = append(sub.Titles, request)
	}
	// Reported as measured, including a request whose PID the box has not
	// armed. Deliverability is the transmitter's decision and it counts what
	// it could not send; a reader that quietly dropped half the subscription
	// would make a box that asked wrongly look like a box that never asked.
	return sub, nil
}

// A DAY OF LISTINGS IS BROADCAST IN FOUR SIX-HOUR BLOCKS, AND THE TABLE ID
// SAYS WHICH. This is the whole of TASK-6.13 and it had been mistaken for a
// day-of-eight problem for a week.
//
// The guide registers its notification slot for the block its own clock is in
// -- measured across eleven times of day on one date, with the boundaries
// pinned at 06:00, 12:00 and 18:00 local:
//
//	00:00-05:59 -> 0    06:00-11:59 -> 1    12:00-17:59 -> 2    18:00-23:59 -> 3
//
// and 0x800C579C fires a slot only when the arriving section's tableID & 3
// equals it. A transmitter that stamps every section 0xA3 therefore files a
// whole day of television in the evening block: at 19:00 the guide draws it,
// and at every other hour the box says FURTHER SCHEDULE INFORMATION IS NOT
// AVAILABLE with 67 of 67 programmes sitting in its store. That is what made
// this look like a rotation: the demo pins 19:00, and the days that "worked"
// were the days somebody happened to look at in the evening.
//
// The box's match unit names ONE block -- 0xA3 by day, 0xA1 in the small hours
// -- and on a Digibox that is the only one the hardware would admit. This port
// delivers by PID and sends all four, which is the same declared host
// intervention as DerivedTitleRequest and is declared here for the same
// reason.
//
// Sending all four covers the block the guide is in and the rest of its day.
// It does NOT cover the second slot the guide registers, which is the block
// AFTER the one it is in: in the evening that belongs to tomorrow, and this
// transmitter broadcasts one day. See gort-k1z.
const (
	// TitleQuarters is how many blocks a broadcast day is cut into.
	TitleQuarters = 4
	// titleTableBase is the table id of the first block; the low two bits are
	// the block.
	titleTableBase    = 0xa0
	secondsPerQuarter = 24 * 60 * 60 / TitleQuarters
)

// QuarterOf is the block a local time of day falls in, given as seconds since
// local midnight. It wraps rather than refusing, so a schedule that has been
// converted round a day boundary still lands somewhere real.
func QuarterOf(secondsOfDay int) int {
	const secondsPerDay = TitleQuarters * secondsPerQuarter
	return ((secondsOfDay%secondsPerDay + secondsPerDay) % secondsPerDay) / secondsPerQuarter
}

// TitleTableID is the table id a block's listings are carried under.
func TitleTableID(quarter int) byte {
	return byte(titleTableBase | quarter%TitleQuarters) // #nosec G115 -- two bits
}
