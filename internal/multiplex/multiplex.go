package multiplex

import (
	"fmt"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
)

// The transmitter.
//
// One Multiplex belongs to one box, for that box's whole life. A reset builds
// a new box AND a new Multiplex, which is not an implementation convenience:
// the carousel's whole job is to hold the line-up back until the clock has
// landed, and a transmitter that carried its "the clock has gone out" state
// across a reset would release the line-up into a machine that had just
// forgotten the clock.

// Transport-stream constants. The frequency and symbol rate are cosmetic --
// nothing in the emulator tunes -- but they appear in the NIT the box parses,
// so they are named once here rather than invented at three call sites.
const (
	transportFrequencyMHz = 11778
	transportOrbitTenths  = 282 // 28.2 degrees east
	transportSymbolRate   = 27500
	transportFEC          = 2

	sectionPIDSI    = 0x11 // NIT, SDT and BAT
	sectionPIDClock = 0x14 // TDT and TOT
)

// Multiplex builds and transmits the modelled broadcast for one box.
type Multiplex struct {
	box      *board.Runtime
	carousel *broadcast.Carousel
	guide    *Guide
	dict     *broadcast.HuffmanDictionary
	clock    InWorldClock

	// version is the SI version_number, bumped whenever the transmitted
	// line-up changes. The box ignores a repeat of a version it has parsed, so
	// a carousel that never bumped it would be inert after its first wave --
	// which is correct, and is why the line-up repeats at all only to catch a
	// box that missed it.
	version byte

	// lastSubscriptionErr is remembered so the driver can report a box that
	// stopped asking, without logging the same line thousands of times.
	lastSubscriptionErr string

	// sent counts what has actually gone on air, for the status the server
	// reports. A transmitter that has emitted nothing and a transmitter whose
	// sections are being dropped look identical from outside otherwise.
	sent Counters

	onAir       func(Counters, []TitleRequest)
	onAirCalled bool

	onReload func(changed bool, err error)
	// lastReloadErr keeps a malformed schedule from being reported on every
	// pass. The file stays broken until somebody fixes it, and a line a second
	// saying so is a log nobody reads.
	lastReloadErr string
}

// Counters is what the transmitter has put on air.
type Counters struct {
	Clock  int
	Lineup int
	Titles int
	// TitlesUnaddressed counts title waves that could not be sent at all
	// because the box had armed no listings PID. It is the single most useful
	// number here: a box that never asks is a different fault from a box that
	// asks and is answered wrongly, and without this they read the same.
	TitlesUnaddressed int
	// TitlesDerived counts waves addressed from the in-world clock rather than
	// from a match unit the box programmed. It is a HOST INTERVENTION and is
	// counted so that it is reported rather than assumed away: this port
	// delivers by PID, and a section sent this way might not reach a Digibox.
	TitlesDerived int
}

// InWorldClock is the time the broadcast claims it is. It is an interface
// because the in-world clock is its own task: one authority, 1:1 with London
// wall-clock time, looping over a 28-day window. Until that exists, a fixed
// instant is a complete implementation of this interface and the transmitter
// does not need to know the difference.
type InWorldClock interface {
	// Now is the broadcast's current instant, in UTC.
	Now() time.Time
}

// FixedClock is an in-world clock stopped at one instant. It is what the tests
// use, and what an operator gets by pinning a day.
type FixedClock struct{ At time.Time }

// Now implements InWorldClock.
func (c FixedClock) Now() time.Time { return c.At.UTC() }

// LiveClock is the real wall clock: the box shows today's date and the actual
// time, and the schedule -- which is a day's television with no date in it --
// plays on whatever day that is.
//
// It is the one place in this program that reads the wall, and that is not a
// contradiction of the instruction-counter rule. The rule exists so that the
// SCHEDULE of emulated events is reproducible; this is the CONTENT of a
// broadcast, the same way a real transmitter's clock is not part of the
// receiver's determinism. Nothing about when a wave fires depends on it.
type LiveClock struct{}

// Now implements InWorldClock.
func (LiveClock) Now() time.Time { return time.Now().UTC() }

// New builds a transmitter for one box.
func New(box *board.Runtime, guide *Guide, dict *broadcast.HuffmanDictionary,
	clock InWorldClock, schedule broadcast.Schedule) (*Multiplex, error) {
	switch {
	case box == nil:
		return nil, fmt.Errorf("multiplex: no box to transmit to")
	case guide == nil:
		return nil, fmt.Errorf("multiplex: no schedule to transmit")
	case dict == nil:
		return nil, fmt.Errorf("multiplex: title sections need a huffman dictionary; see dictionaries/MANIFEST.md")
	case clock == nil:
		return nil, fmt.Errorf("multiplex: no in-world clock, so the broadcast has no date to claim")
	}
	m := &Multiplex{box: box, guide: guide, dict: dict, clock: clock, version: 1}
	carousel, err := broadcast.NewCarousel(schedule, broadcast.Source{
		Clock:  m.clockWave,
		Lineup: m.lineupWave,
		Titles: m.titleWave,
	})
	if err != nil {
		return nil, err
	}
	m.carousel = carousel
	return m, nil
}

// Counts reports what has gone on air.
func (m *Multiplex) Counts() Counters { return m.sent }

// OnReload is called when a schedule edit is picked up, and when one is
// rejected. A rejected edit is the more important of the two: the broadcast
// carries on with the last good schedule, so without this the only symptom of
// a typo is that nothing changes.
func (m *Multiplex) OnReload(notify func(changed bool, err error)) { m.onReload = notify }

// reload picks up an edit to the schedule, keeping the last good one if the
// edit is malformed.
//
// The SI VERSION IS BUMPED when the schedule changes, and that is not
// cosmetic: a receiver ignores a repeat of a version it has already parsed, so
// an edited line-up broadcast under the old version number would be dropped by
// the box and the edit would appear to have done nothing.
func (m *Multiplex) reload() {
	changed, err := m.guide.Reload()
	switch {
	case err != nil:
		if problem := err.Error(); problem != m.lastReloadErr {
			m.lastReloadErr = problem
			if m.onReload != nil {
				m.onReload(false, err)
			}
		}
	case changed:
		m.lastReloadErr = ""
		m.version = (m.version + 1) & 0x1f
		if m.onReload != nil {
			m.onReload(true, nil)
		}
	default:
		m.lastReloadErr = ""
	}
}

// listings is the schedule for the day the broadcast is currently claiming.
// It is looked up per wave rather than held, so a clock that crosses midnight
// starts transmitting the next day's television without anything restarting.
func (m *Multiplex) listings() *Listings { return m.guide.On(m.clock.Now()) }

// OnAir is called once, the first time programmes are actually transmitted to
// a box that asked for them.
//
// It exists because the two failures this package can have are indistinguishable
// from outside and from each other: a server with no schedule and a server whose
// schedule never reached the box both present as a guide saying there is no
// information. One line in the log separates them, and it is the line an
// operator looks for after changing anything here.
func (m *Multiplex) OnAir(notify func(Counters, []TitleRequest)) { m.onAir = notify }

// Pump transmits whatever is due at this instruction.
//
// It is called from the instruction loop, which is the box's only owner, and
// it does no work of its own between waves: NextDue means a caller can ask
// rarely without missing anything.
func (m *Multiplex) Pump(now uint64) error {
	if now < m.carousel.NextDue() {
		return nil
	}
	wave, err := m.carousel.Wave(now)
	if err != nil {
		return err
	}
	for _, emission := range wave {
		if err := m.box.Demux.Push(emission.PID, emission.Section); err != nil {
			return fmt.Errorf("multiplex: transmit on PID %#02x: %w", emission.PID, err)
		}
	}
	return nil
}

// clockWave is the TOT and the TDT.
//
// THE TOT IS THE ONE THAT MATTERS. Measured: the box's match units carry 0x73
// and nothing matches 0x70, so the TDT is never delivered to anything. It is
// transmitted anyway because a real multiplex carries both and the box is
// entitled to see a well-formed stream -- but if the TOT were ever dropped
// from this function the box would stay on whatever day it woke with, move no
// PID, and report nothing at all.
func (m *Multiplex) clockWave(uint64) ([]broadcast.Emission, error) {
	at := m.clock.Now()
	tot, err := broadcast.TOT(at, londonOffset(at))
	if err != nil {
		return nil, err
	}
	tdt, err := broadcast.TDT(at)
	if err != nil {
		return nil, err
	}
	m.sent.Clock++
	return []broadcast.Emission{
		{PID: sectionPIDClock, Section: tot},
		{PID: sectionPIDClock, Section: tdt},
	}, nil
}

// lineupWave is the NIT, the SDT and the BAT -- the acquisition tables.
//
// Every id in them is read off the box rather than chosen. A BAT carrying a
// bouquet id the box's match unit does not want is not rejected anywhere
// visible: the hardware never delivers it, and the box simply goes on not
// having a channel list.
func (m *Multiplex) lineupWave(uint64) ([]broadcast.Emission, error) {
	// On the line-up cadence, because this is the wave whose content an edit
	// changes and because the loop is the only thread allowed to touch any of
	// this.
	m.reload()
	sub, asking := m.subscription()
	if !asking || !sub.SIArmed {
		// Not an error: a box that is not asking, or has not armed PID 0x11
		// yet, is mid-boot. A transmitter that failed here would take the
		// server down over a machine that is merely not ready.
		return nil, nil
	}
	listings := m.listings()
	transport := m.transport(sub, listings)
	nit, err := broadcast.NIT(sub.NetworkID, m.version, listings.Bouquet, []broadcast.Transport{transport})
	if err != nil {
		return nil, err
	}
	sdt, err := broadcast.SDT(transport.ID, sub.NetworkID, m.version, transport.Services)
	if err != nil {
		return nil, err
	}
	bat, err := broadcast.BAT(sub.BouquetID, m.version, listings.Bouquet, []broadcast.Transport{transport})
	if err != nil {
		return nil, err
	}
	m.sent.Lineup++
	return []broadcast.Emission{
		{PID: sectionPIDSI, Section: nit},
		{PID: sectionPIDSI, Section: sdt},
		{PID: sectionPIDSI, Section: bat},
	}, nil
}

// transport turns the schedule's channels into the one transport stream this
// multiplex models.
func (m *Multiplex) transport(sub Subscription, listings *Listings) broadcast.Transport {
	transport := broadcast.Transport{
		ID: sub.NetworkID, NetworkID: sub.NetworkID,
		FrequencyMHz: transportFrequencyMHz, OrbitTenths: transportOrbitTenths,
		SymbolRate: transportSymbolRate, FEC: transportFEC,
	}
	for _, service := range listings.Services {
		transport.Services = append(transport.Services, broadcast.Service{
			ID: service.ServiceID, Name: service.Name, EITSchedule: true,
		})
		transport.Lineup = append(transport.Lineup, broadcast.LineupEntry{
			ServiceID: service.ServiceID,
			Kind:      1,
			Listings:  service.ListingsID,
			Extra:     service.ListingsID,
			Channel:   service.Channel,
		})
	}
	return transport
}

// titleWave answers every listings filter the box has programmed, and only
// those.
//
// This is the part that had to be measured rather than designed. The table id,
// the extension, the extension MASK and the MJD all come back off the match
// unit; the day is whatever the box decided, which is why the schedule holds
// no date. A channel the filter does not want is skipped rather than sent
// anyway -- an empty row in the guide is honest, and a row filed against a
// filter nobody asked for is not.
func (m *Multiplex) titleWave(uint64) ([]broadcast.Emission, error) {
	sub, asking := m.subscription()
	if !asking {
		return nil, nil
	}
	listings := m.listings()
	mjd := MJDOf(m.clock.Now())
	// Only a filter for the day the broadcast is CLAIMING is worth honouring.
	// The box programs its request once and never re-subscribes, so after
	// midnight its filter still names yesterday -- and answering that would
	// fill the guide with a day that has gone.
	var requests []TitleRequest
	for _, request := range sub.Titles {
		if request.MJD() == mjd {
			requests = append(requests, request)
		}
	}
	if len(requests) == 0 {
		// The box has acquired and armed its listings PID but programmed no
		// match unit -- which it does on five days in eight (TASK-6.13). The
		// PID is armed on every day, so the request is derived and the
		// intervention is counted rather than hidden.
		if sub.ListingsPID == 0 {
			m.sent.TitlesUnaddressed++
			return nil, nil
		}
		for _, service := range listings.Services {
			requests = append(requests, DerivedTitleRequest(sub.ListingsPID, mjd, service.ListingsID))
		}
		m.sent.TitlesDerived++
	}
	defer func() {
		if !m.onAirCalled && m.sent.Titles > 0 && m.onAir != nil {
			m.onAirCalled = true
			m.onAir(m.sent, sub.Titles)
		}
	}()
	var wave []broadcast.Emission
	for _, request := range requests {
		// One request covers a SET of channels, so this asks each channel
		// whether the filter wants it rather than looking one up by the
		// request's value -- that value is the OR of the ids in the set and is
		// frequently nobody's id at all.
		for i := range listings.Services {
			service := &listings.Services[i]
			if !request.Wants(service.ListingsID) {
				continue
			}
			records, err := m.records(service)
			if err != nil {
				return nil, err
			}
			// The section is addressed with the CHANNEL's own listings id, not
			// the request's: the filter admits the whole set, and the guide
			// files what arrives by the id in the section.
			section, err := broadcast.TitleSection(request.TableID, service.ListingsID, request.Filter,
				m.version, 0, 0, m.dict, records)
			if err != nil {
				return nil, err
			}
			wave = append(wave, broadcast.Emission{PID: request.PID, Section: section})
		}
	}
	if len(wave) > 0 {
		m.sent.Titles++
	}
	return wave, nil
}

// records turns one channel's day into title records.
//
// THE SCHEDULE IS WRITTEN IN THE BOX'S LOCAL TIME AND THE WIRE CARRIES UTC.
// Measured: the box adds its declared offset to the programme times as well as
// to the clock, so a record sent as 19:00 in June displays as 8.00pm. Without
// the conversion below a schedule reads correctly in winter and is an hour out
// all summer -- the kind of wrong that looks like a plausible listing rather
// than like a bug.
//
// The conversion is modular, and that is correct rather than a shortcut: the
// same schedule plays every day, so a programme that converts to the previous
// day's 23:30 UTC is exactly what is on air at 23:30 UTC on the day being
// broadcast.
//
// A section has a hard length limit, and a day of television will exceed it on
// a busy channel. The overflow is DROPPED rather than split across
// section_number here, and that is a stated limitation rather than an
// oversight: multi-section title tables are their own measurement, and a
// half-understood split would file programmes under the wrong section number
// silently. The builder refuses an over-long section, so this cannot ship a
// truncated one by accident -- it ships fewer programmes, visibly.
func (m *Multiplex) records(service *ListedService) ([]broadcast.TitleRecord, error) {
	const secondsPerDay = 24 * 60 * 60
	offset := londonOffset(m.clock.Now()).OffsetMinutes * 60
	records := make([]broadcast.TitleRecord, 0, len(service.Programmes))
	for i, programme := range service.Programmes {
		local, err := programme.StartSeconds()
		if err != nil {
			return nil, err
		}
		start := ((local-offset)%secondsPerDay + secondsPerDay) % secondsPerDay
		records = append(records, broadcast.TitleRecord{
			EventID:  uint16(i + 1), // #nosec G115 -- a day's programmes, bounded by the section length below
			Start:    start,
			Duration: programme.Minutes * 60,
			Title:    programme.Title,
			Genre:    programme.Genre,
			Rating:   programme.Rating,
		})
		// Ask the builder whether what we now hold still fits, and put the
		// last one back if it does not. Trying it on a copy would be the same
		// question asked of a slice that may share this one's backing array.
		if _, err := broadcast.TitleSection(0xa0, service.ListingsID, [2]byte{}, 0, 0, 0, m.dict, records); err != nil {
			records = records[:len(records)-1]
			break
		}
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("multiplex: %q: not even one programme fits in a title section", service.Name)
	}
	return records, nil
}

// subscription reads the box.
//
// It returns a BOOL rather than an error on purpose. A box that is not asking
// for the SI -- mid-boot, or halted -- is a state this transmitter waits out,
// not a reason to bring the server down over a television schedule, and an
// error value that every caller is required to discard is a worse way of
// saying so than a flag that says it.
func (m *Multiplex) subscription() (Subscription, bool) {
	sub, err := Read(m.box.Demux)
	if err != nil {
		m.lastSubscriptionErr = err.Error()
		return Subscription{}, false
	}
	m.lastSubscriptionErr = ""
	return sub, true
}

// LastProblem is the most recent reason the box could not be read, or empty.
func (m *Multiplex) LastProblem() string { return m.lastSubscriptionErr }

// londonOffset is the UK's local-time rule for the instant given.
//
// It is computed rather than configured because the TOT declares both the
// current offset and the next change, and a hand-set pair goes stale the first
// time the clock crosses one. British Summer Time runs from the last Sunday in
// March to the last Sunday in October, 01:00 UTC at both ends.
func londonOffset(at time.Time) broadcast.TimeOffset {
	at = at.UTC()
	start := lastSunday(at.Year(), time.March)
	end := lastSunday(at.Year(), time.October)
	summer := !at.Before(start) && at.Before(end)
	offset := broadcast.TimeOffset{Country: "GBR", Region: 0}
	if summer {
		offset.OffsetMinutes = 60
		offset.ChangeUTC = end
		offset.NextOffsetMinutes = 0
		return offset
	}
	offset.OffsetMinutes = 0
	if at.Before(start) {
		offset.ChangeUTC = start
	} else {
		offset.ChangeUTC = lastSunday(at.Year()+1, time.March)
	}
	offset.NextOffsetMinutes = 60
	return offset
}

// lastSunday is 01:00 UTC on the last Sunday of a month, which is when the UK
// changes its clocks.
func lastSunday(year int, month time.Month) time.Time {
	day := time.Date(year, month+1, 1, 1, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	for day.Weekday() != time.Sunday {
		day = day.AddDate(0, 0, -1)
	}
	return day
}
