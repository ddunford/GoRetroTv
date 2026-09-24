package multiplex

import (
	"fmt"
	"time"

	"github.com/ddunford/goretrotv/internal/board"
	"github.com/ddunford/goretrotv/internal/broadcast"
	"github.com/ddunford/goretrotv/internal/dvb"
)

// The transmitter.
//
// One Multiplex belongs to one box, for that box's whole life. A reset builds
// a new box AND a new Multiplex, which is not an implementation convenience:
// the carousel's whole job is to hold the line-up back until the clock has
// landed, and a transmitter that carried its "the clock has gone out" state
// across a reset would release the line-up into a machine that had just
// forgotten the clock.

// Transport-stream constants. The firmware validates the frequency and symbol
// rate before it reports front-end success, so these are part of the emulated
// signal rather than display metadata.
const (
	transportFrequencyMHz = 11778
	transportOrbitTenths  = 282 // 28.2 degrees east
	transportSymbolRate   = 27500
	transportFEC          = 2
	// programmeMapPID is the transmitter's allocation for the service PMT. It
	// is announced through the PAT rather than written into guest state; the
	// firmware must open this PID itself before anything is sent on it.
	programmeMapPID = 0x100
	videoPID        = 0x101
	audioPID        = 0x102

	// THE NIT HAS ITS OWN PID AND IT IS NOT THE SDT'S. EN 300 468 puts the
	// network information table on 0x10 and the service description and bouquet
	// association tables on 0x11, and this transmitter sent all three on 0x11.
	// The box had ARMED 0x10 the whole time and nothing ever answered it --
	// measured by dumping all sixteen demux match units after acquisition, where
	// 0x10 sits beside 0x11, 0x14 and the day's title PID as a filter the guest
	// programmed and the broadcast never filled.
	sectionPIDNIT   = 0x10 // NIT
	sectionPIDSI    = 0x11 // SDT and BAT
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
	// indexCursor is which letter the next index wave carries. The A-Z index goes out one letter
	// at a time, so the transmitter has to remember where it had got to.
	indexCursor int
	// titleCursor is which title section the next title wave carries. The demux can admit a large
	// set of channel extensions with one match unit, but the guest still has to drain each admitted
	// section before another overwrites its delivery slot.
	titleCursor int
	// indexVersion is bumped once per full pass of the alphabet so each pass is a fresh delivery
	// rather than a repeat the guest deduplicates away.
	indexVersion byte
	// eventVersion is bumped every present/following wave. Unlike the line-up, what is on now
	// changes with the clock, so a frozen version would freeze the box's idea of what is on.
	eventVersion byte
	continuity   map[uint16]byte

	// lastSubscriptionErr is remembered so the driver can report a box that
	// stopped asking, without logging the same line thousands of times.
	lastSubscriptionErr string

	// sent counts what has actually gone on air, for the status the server
	// reports. A transmitter that has emitted nothing and a transmitter whose
	// sections are being dropped look identical from outside otherwise.
	sent Counters

	// onAir reports a MILESTONE rather than a tally, and fires once per milestone. The titles
	// starting is one; the event information starting is the other, and it is separate because
	// the box arms PID 0x0012 only when a viewer tunes -- which may be hours after the guide
	// filled, or never. Reporting both on one line at the first title wave would print
	// event_sections: 0 for ever and read as "the EIT never goes out".
	onAir        func(Counters, []TitleRequest)
	onAirCalled  bool
	onEventsSent bool

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
	// TitlesUnaddressed counts DAYS whose title sections could not be sent at
	// all, because the box had not armed that day's listings PID. It is the
	// single most useful number here: a box that never asks is a different
	// fault from a box that asks and is answered wrongly, and without this they
	// read the same.
	//
	// IT COUNTS DAYS RATHER THAN WAVES, and that distinction is load-bearing
	// now that a wave carries today AND tomorrow. A pass that broadcasts today
	// and is refused tomorrow increments Titles once and this once, and those
	// two numbers together are the only thing that separates "both days went
	// out" from "the box is not asking for the second one yet" -- which is
	// exactly what an empty midnight column in the grid looks like from the
	// outside. cmd/goretrotv prints it for that reason.
	TitlesUnaddressed int
	// TitlesDerived counts waves addressed from the in-world clock rather than
	// from a match unit the box programmed. It is a HOST INTERVENTION and is
	// counted so that it is reported rather than assumed away: this port
	// delivers by PID, and a section sent this way might not reach a Digibox.
	TitlesDerived int
	// Index counts table 0xC1 index waves -- the A-Z LISTINGS screens. It is separate from Titles
	// because the two answer different questions: the titles are the programmes themselves and the
	// index is only a set of references into them, so an index sent without titles is a screen
	// full of unresolvable rows rather than a screen with fewer of them.
	Index int
	// Events counts present/following EIT sections put on air. It counts SECTIONS rather than
	// waves because a wave is two per service and a service with nothing on now still sends both,
	// so waves would say less than the number they are made of.
	//
	// IT IS NOT A MEASURE OF THE BOX READING THEM. The demux accepting a section and a guest task
	// draining the ring are different events, and a number that conflated them would read as
	// success in exactly the case this port has been wrong about four times.
	Events int
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
	m := &Multiplex{box: box, guide: guide, dict: dict, clock: clock, version: 1,
		continuity: make(map[uint16]byte)}
	source := broadcast.Source{
		Clock:  m.clockWave,
		Lineup: m.lineupWave,
		Titles: m.titleWave,
	}
	// THE INDEX RUNG IS OPT-IN BY PERIOD, and the carousel's refusal of a zero period stays strict
	// because of it. A schedule that does not name an IndexPeriod is one that does not want the
	// A-Z index -- which is a supported way to run, and is how the probes pinned to screen hashes
	// taken under a lighter load keep the broadcast they were calibrated against. Wiring the
	// source unconditionally instead turned "you forgot the period" into an error every caller
	// without one had to work around.
	if schedule.IndexPeriod != 0 {
		source.Index = m.indexWave
	}
	// The event rung is opt-in by period for the same reason.
	if schedule.EventPeriod != 0 {
		source.Events = m.eventWave
	}
	if schedule.ProgrammePeriod != 0 {
		source.Programmes = m.programmeWave
	}
	carousel, err := broadcast.NewCarousel(schedule, source)
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

// MediaSelection resolves the receiver's tuned service to the programme currently on air.
// Decoder PID programming is checked separately by the instruction loop; this method answers only
// which configured source, if any, that guest-selected service owns at the in-world time.
func (m *Multiplex) MediaSelection() (serviceName, programmeName, kind string, ok bool) {
	sub, asking := m.subscription()
	if !asking || !sub.EITArmed || sub.TunedServiceID == 0 {
		return "", "", "", false
	}
	listings := m.listings()
	if listings == nil {
		return "", "", "", false
	}
	return listings.MediaFor(sub.TunedServiceID, m.clock.Now())
}

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
		packets := dvb.PacketizeSection(emission.PID, emission.Section, m.continuity[emission.PID])
		m.continuity[emission.PID] = (m.continuity[emission.PID] + byte((len(packets)/188)&0xff)) & 0x0f
		if err := m.box.Demux.PushTransport(packets); err != nil {
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
	transport, err := m.transport(sub, listings)
	if err != nil {
		return nil, err
	}
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
	// SDT AND BAT ARE TABLES, AND EVERY SECTION OF THEM GOES OUT. Each is one section while the
	// demo carries a handful of channels and several once it carries a real line-up; a wave that
	// emitted only the first would announce the first twenty-five services and silently drop the
	// rest.
	wave := make([]broadcast.Emission, 0, 2+len(sdt)+len(bat))
	// The NIT goes where the standard puts it and where the box is listening --
	// but only once the box IS listening. A section on an unarmed PID is dropped
	// by the hardware before any code sees it, so sending it early would not be
	// harmless, it would be invisible; the next wave carries it instead.
	if sub.NITArmed {
		wave = append(wave, broadcast.Emission{PID: sectionPIDNIT, Section: nit})
	}
	for _, section := range sdt {
		wave = append(wave, broadcast.Emission{PID: sectionPIDSI, Section: section})
	}
	for _, section := range bat {
		wave = append(wave, broadcast.Emission{PID: sectionPIDSI, Section: section})
	}
	return wave, nil
}

// programmeWave answers the fixed-PSI filters the firmware opens after a successful tune.
func (m *Multiplex) programmeWave(uint64) ([]broadcast.Emission, error) {
	sub, asking := m.subscription()
	if !asking {
		return nil, nil
	}
	listings := m.listings()
	if listings == nil || len(listings.Services) == 0 {
		return nil, nil
	}
	var wave []broadcast.Emission
	// PAT AND CAT ARE VIEWING-TIME REQUESTS ON THIS FIRMWARE. They are not sent merely because a
	// DVB transport normally carries them: a successful tune makes the guest arm PID 0 and PID 1,
	// and this answers only those measured requests.
	if sub.PATArmed {
		programmes := make([]broadcast.Programme, 0, len(listings.Services))
		for _, service := range listings.Services {
			programmes = append(programmes, broadcast.Programme{
				Number: service.ServiceID,
				MapPID: programmeMapPID,
			})
		}
		pat, err := broadcast.PAT(sub.NetworkID, m.version, programmes)
		if err != nil {
			return nil, err
		}
		wave = append(wave, broadcast.Emission{PID: 0x00, Section: pat})
	}
	if sub.CATArmed {
		cat, err := broadcast.CAT(m.version, nil)
		if err != nil {
			return nil, err
		}
		wave = append(wave, broadcast.Emission{PID: 0x01, Section: cat})
	}
	if sub.PMTArmed {
		// H.222.0 assigns 0x02 to MPEG-2 video and 0x03 to MPEG-1 audio.
		// These are transmitter-owned component allocations, announced on air;
		// the guest still has to accept the PMT and request their PIDs itself.
		streams := []broadcast.ElementaryStream{
			{Type: 0x02, PID: videoPID},
			{Type: 0x03, PID: audioPID},
		}
		for _, service := range listings.Services {
			pmt, err := broadcast.PMT(service.ServiceID, m.version, videoPID, nil, streams)
			if err != nil {
				return nil, err
			}
			wave = append(wave, broadcast.Emission{PID: programmeMapPID, Section: pmt})
		}
	}
	return wave, nil
}

// transport turns the schedule's channels into the one transport stream this
// multiplex models.
func (m *Multiplex) transport(sub Subscription, listings *Listings) (broadcast.Transport, error) {
	transport := broadcast.Transport{
		ID: sub.NetworkID, NetworkID: sub.NetworkID,
		FrequencyMHz: transportFrequencyMHz, OrbitTenths: transportOrbitTenths,
		SymbolRate: transportSymbolRate, FEC: transportFEC,
	}
	for i := range listings.Services {
		service := listings.Services[i]
		row, err := m.guideRow(&listings.Services[i])
		if err != nil {
			return broadcast.Transport{}, err
		}
		transport.Services = append(transport.Services, broadcast.Service{
			ID: service.ServiceID, Name: service.Name, Type: service.Type,
			EITSchedule: true, Row: row,
		})
		transport.Lineup = append(transport.Lineup, broadcast.LineupEntry{
			ServiceID: service.ServiceID,
			Kind:      lineupKind(service.Kind),
			Listings:  service.ListingsID,
			Extra:     service.ListingsID,
			Channel:   service.Channel,
			Flags:     lineupFlags(service.Flags),
		})
	}
	return transport, nil
}

// guideRow is the private 0xB2 descriptor a service carries: what the ALL CHANNELS grid reads into
// the row record it draws a programme from.
//
// WHY IT IS THE PROGRAMME ON AIR. The descriptor rides in the SDT, which announces SERVICES, so it
// carries one thing per channel rather than a schedule -- and the row it fills is the row the grid
// draws for that channel now. The programme on air at the in-world clock is the only candidate
// that makes the grid's first column mean anything.
//
// THE SCALAR FIELDS ARE LEFT ZERO ON PURPOSE. The parser at 0x800CB000 stores four of them into the
// row record and nothing here knows what any of them is for; the project's rule is that a field
// named on a guess gets believed, so they are carried as zero until something measures them. The
// TEXT is the part that is understood: the parser hands it to 0x800BECF0, the Huffman decompressor,
// exactly as a title record's text is handed to it.
func (m *Multiplex) guideRow(service *ListedService) (*broadcast.GuideRow, error) {
	if m.dict == nil {
		return nil, nil
	}
	// A CHANNEL WITH NOTHING ON AIR STILL CARRIES ITS GENRE, and that is not a nicety. At9 is the
	// only place the guide's genre screens look, so a service whose descriptor is omitted has no
	// genre and vanishes from its own screen -- measured: Sky Soap carries ENTERTAINMENT and did
	// not appear on it, because it has nothing on at seven o'clock. It stays in ALL CHANNELS,
	// which has no filter, so the fault is invisible on the one screen anyone checks.
	//
	// At8 stays ZERO for that row on purpose: zero is what draws "..no listings available", which
	// is exactly what a channel between programmes should say.
	offAir := &broadcast.GuideRow{At8: 0, At9: service.Genre, At10: service.RowAt10,
		At11: service.RowAt11}
	if len(service.Programmes) == 0 {
		return offAir, nil
	}
	now := secondsOfDay(m.clock.Now())
	var on *ListedProgramme
	for n := range service.Programmes {
		start, err := service.Programmes[n].StartSeconds()
		if err != nil {
			return nil, err
		}
		if start <= now && now < start+service.Programmes[n].Minutes*60 {
			on = &service.Programmes[n]
			break
		}
	}
	if on == nil {
		return offAir, nil
	}
	text, err := m.dict.Encode(on.Title)
	if err != nil {
		return nil, err
	}
	// At8 IS NOT DECORATION: THE GRID BRANCHES ON IT. A type-1 row -- which is what a normal TV
	// channel gets, because its line-up kind is 1 -- reads the BYTE at record+8 and takes a
	// different path when it is non-zero:
	//
	//	9fc73957  add_nnnnnnnn 0x0002e258   ; ds + 0x2E258 + 336*row, i.e. record+8
	//	9fc7395d  getc
	//	9fc73961  jnz_nn 0x9fc73985         ; non-zero -> away from the "no listings" draw
	//
	// and record+8 is exactly where the 0xB2 parser puts the descriptor's first scalar
	// (rec[8] = d[2]). Left at zero it falls through to "..no listings available" every time.
	// WHAT THE VALUE MEANS is not established; 1 is the smallest thing that is not zero.
	at8 := service.RowAt8
	if at8 == 0 {
		at8 = 1
	}
	// At9 IS THE GENRE, and it is the whole of what the TV GUIDE's genre screens filter on. Every
	// channel sent 0 here from the day this descriptor shipped, which is why ENTERTAINMENT,
	// MOVIES, SPORTS, NEWS & DOCUMENTARIES, KIDS, MUSIC & RADIO and SPECIALIST all drew the grid's
	// chrome with no rows in it: they were asking for 3, 6, 7, 5, 2, 4 and 1 and every channel was
	// answering 0.
	return &broadcast.GuideRow{At8: at8, At10: service.RowAt10, At9: service.Genre,
		At11: service.RowAt11, Text: text}, nil
}

// secondsOfDay is the in-world clock's time as seconds since midnight.
func secondsOfDay(now time.Time) int {
	return now.Hour()*3600 + now.Minute()*60 + now.Second()
}

// inTheGuide is the line-up flag combination the TV GUIDE's list screens require, measured rather
// than chosen.
//
// The guest unpacks the 0xB1 entry's four flag bits into a word the ALL CHANNELS enumeration masks
// with 0x10 before it will report a channel, and that word is the nibble encoded as four two-bit
// fields -- 01 where a bit is set, 10 where it is clear, so an all-clear channel reads 0xAA and can
// never satisfy the mask. Bit 2 alone sets the field the mask looks at and is NOT enough: swept
// past the settle, 0x04, 0x05, 0x0C and 0x0D all leave the grid empty, while 0x06, 0x07, 0x0E and
// 0x0F draw it. **0x06 is the minimal set that works** -- adding bit 0 or bit 3 changes nothing,
// and adding both (0x0F) puts a stray block over the header.
//
// What the two bits MEAN individually is not established, so this is the smallest combination
// measured to work rather than a claim about their names.
const inTheGuide = 0x06

// lineupFlags defaults a channel with no flags of its own to the guide-visible set.
//
// Zero is a real value on the wire, so a schedule CAN ask for it -- but a channel nobody can see in
// the guide is not what any schedule in this project means by listing a channel, and every one of
// them predates the field existing. A schedule that wants something else says so and is obeyed.
func lineupFlags(declared byte) byte {
	if declared == 0 {
		return inTheGuide
	}
	return declared
}

// lineupKind defaults a channel with no declared kind to the value every feed
// before the field existed transmitted.
//
// It is deliberately NOT named for a meaning. 1 is what this port has always
// sent and what the box has always accepted into its line-up; whether it says
// "television" or "has a schedule" or something else is unmeasured, and a
// constant called kindTelevision would assert what nobody here has shown.
const lineupKindDefault = 1

func lineupKind(declared byte) byte {
	if declared == 0 {
		return lineupKindDefault
	}
	return declared
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
	now := m.clock.Now()
	wave, err := m.titlesFor(sub, now)
	if err != nil {
		return nil, err
	}
	// AND TOMORROW, WHEN THE BOX HAS ASKED FOR IT.
	//
	// The guide registers TWO notification slots -- the block its clock is in and the one after --
	// and in the evening the second is the next DAY's first block, because what follows 18:00-24:00
	// belongs to tomorrow. Answering only today leaves that subscription unanswered, and the box
	// says so in its own words: at 23:15, with the whole evening registered, the banner reads
	// "Further schedule information is not available" under a programme that runs to midnight, and
	// the grid's 12.00am column is empty on every channel.
	//
	// WHAT GATES IT IS THE BOX, not a clock rule here. It arms the next day's listings PID itself
	// when it wants that day, so sub.Arms is the signal -- the same "let the box name what it
	// wants" that every other rung on this carousel follows. A transmitter that decided for itself
	// when tomorrow was due would be broadcasting at a subscription rather than answering one.
	tomorrow, err := m.titlesFor(sub, now.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	wave = append(wave, tomorrow...)
	if len(wave) > 0 {
		// ONE TITLE SECTION PER WAVE. Pump delivers every emission in a wave without running a guest
		// instruction between them. A six-channel fixture hid that distinction; the launch line-up
		// exposed it when a burst of 83 sections left only 23 programmes in the guest store. A real
		// carousel serialises sections, and the A-Z rung already follows the same measured rule.
		section := wave[m.titleCursor%len(wave)]
		m.titleCursor = (m.titleCursor + 1) % len(wave)
		wave = []broadcast.Emission{section}
		m.sent.Titles++
	}
	return wave, nil
}

// titlesFor is one day's title sections, addressed to that day's PID, or nothing when the box has
// not armed it.
func (m *Multiplex) titlesFor(sub Subscription, day time.Time) ([]broadcast.Emission, error) {
	listings := m.guide.On(day)
	if listings == nil || len(listings.Services) == 0 {
		return nil, nil
	}
	mjd := MJDOf(day)
	// THE DAY DECIDES THE PID, AND THE BOX MUST HAVE ARMED IT. A box arms the
	// PID for the day it is in and, in the evening, for the day after; a
	// section pushed at a PID it has not armed reaches nothing and reports no
	// error, which is the one failure in this package that looks exactly like
	// success.
	pid := TitlePID(mjd)
	if !sub.Arms(pid) {
		m.sent.TitlesUnaddressed++
		return nil, nil
	}
	// Only a filter for the day the broadcast is CLAIMING is worth honouring.
	// The box programs its request once and never re-subscribes, so after
	// midnight its filter still names yesterday -- and answering that would
	// fill the guide with a day that has gone. An evening box also programmes
	// one for TOMORROW, which its caller answers by calling this a second time
	// with the next day rather than by widening what one call accepts.
	var requests []TitleRequest
	for _, request := range sub.Titles {
		if request.MJD() == mjd {
			requests = append(requests, request)
		}
	}
	if len(requests) == 0 {
		// The box has acquired and armed the day's PID but programmed no match
		// unit for it -- which it does on most days (TASK-6.13), because it
		// only ever filters for a day whose slot is one of three. The request
		// is derived and the intervention is counted rather than hidden.
		for _, service := range listings.Services {
			requests = append(requests, DerivedTitleRequest(mjd, service.ListingsID))
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
			quarters, err := m.records(service, day)
			if err != nil {
				return nil, err
			}
			for quarter, records := range quarters {
				if len(records) == 0 {
					continue
				}
				// The section is addressed with the CHANNEL's own listings id,
				// not the request's: the filter admits the whole set, and the
				// guide files what arrives by the id in the section. The TABLE
				// ID is the quarter's, not the match unit's -- see
				// TitleTableID.
				section, err := broadcast.TitleSection(TitleTableID(quarter), service.ListingsID,
					request.Filter, m.version, 0, 0, m.dict, records)
				if err != nil {
					return nil, err
				}
				wave = append(wave, broadcast.Emission{PID: request.PID, Section: section})
			}
		}
	}
	return wave, nil
}

// records turns one channel's day into title records, split into the four
// quarters the box files them under.
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
// A section has a hard length limit, and the overflow is DROPPED rather than
// split across section_number here -- a stated limitation rather than an
// oversight, because multi-section title tables are their own measurement and
// a half-understood split would file programmes under the wrong section number
// silently. The builder refuses an over-long section, so this cannot ship a
// truncated one by accident: it ships fewer programmes, visibly. Cutting the
// day into blocks made that far less likely than it was -- each section now
// carries a sixth of a day rather than all of it -- and a programme that will
// not fit in its own block no longer stops the rest of the day being built,
// which it did when the whole day was one section.
func (m *Multiplex) records(service *ListedService, day time.Time) ([TitleQuarters][]broadcast.TitleRecord, error) {
	const secondsPerDay = 24 * 60 * 60
	var quarters [TitleQuarters][]broadcast.TitleRecord
	// THE OFFSET IS THE BROADCAST DAY'S, not the clock's, and those differ twice a year. Using
	// the clock's would put tomorrow's listings an hour out on the two changeover days -- the kind
	// of wrong that reads as a plausible schedule rather than as a bug, which is exactly what the
	// UTC conversion below was written to avoid in the first place.
	offset := londonOffset(day).OffsetMinutes * 60
	total := 0
	for i, programme := range service.Programmes {
		local, err := programme.StartSeconds()
		if err != nil {
			return quarters, err
		}
		// THE QUARTER IS THE LOCAL ONE. The wire carries UTC, so in summer a
		// programme's transmitted start is in the quarter before its local
		// one -- and the guide asks for the quarter ITS OWN CLOCK is in, which
		// is local. Filing by the UTC start would put an hour of every summer
		// evening in the block the box is not listening to.
		quarter := QuarterOf(local)
		start := ((local-offset)%secondsPerDay + secondsPerDay) % secondsPerDay
		candidate := quarters[quarter]
		candidate = append(candidate, broadcast.TitleRecord{
			EventID:  uint16(i + 1), // #nosec G115 -- a day's programmes, bounded by the section length below
			Start:    start,
			Duration: programme.Minutes * 60,
			Title:    programme.Title,
			Genre:    programme.Genre,
			Rating:   programme.Rating,
		})
		// Ask the builder whether what this quarter now holds still fits, and
		// put the last one back if it does not. Trying it on a copy would be
		// the same question asked of a slice that may share this one's backing
		// array.
		if _, err := broadcast.TitleSection(0xa0, service.ListingsID, [2]byte{}, 0, 0, 0, m.dict, candidate); err != nil {
			continue
		}
		quarters[quarter] = candidate
		total++
	}
	if total == 0 {
		return quarters, fmt.Errorf("multiplex: %q: not even one programme fits in a title section", service.Name)
	}
	return quarters, nil
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

// indexPID is where table 0xC1 arrives, measured rather than chosen.
//
// The subscription tree cannot answer this -- its PID node holds a field in 0x10..0x1F that reads
// as a plausible PID on a box whose PIDs are 0x10, 0x11 and 0x14, and is not one. Pushing a section
// at each armed PID in turn and watching the consumer execute does: 0x10, 0x11, 0x14, 0x33 and 0x34
// never reach it, 0x52 runs 409 instructions inside it.
const indexPID = 0x52

// indexFlags and indexSelector are the two bytes an index record carries beside its references.
//
// They are the values MEASURED to work, and they are deliberately not named for a meaning. rec[2]'s
// low nibble unpacks into four two-bit fields exactly as the line-up entry's flags nibble does --
// 01 where a bit is set, 10 where it is clear -- and only the top two bits of rec[3] are read at
// all. What any individual bit MEANS is unestablished, so a constant called indexVisible would
// assert what nobody here has shown.
const (
	indexFlags    = 0x0f
	indexSelector = 0xc0
)

// indexWave builds the table 0xC1 A-Z index: for each letter, the programmes whose titles begin
// with it.
//
// WHAT AN INDEX RECORD IS, measured by sending six candidate layouts pointing at six different
// programmes and reading which titles the screen drew: rec[0..1] is the CHANNEL'S LISTINGS ID and
// rec[5..6] is the PROGRAMME'S EVENT ID. Layouts that put the event id at rec[0..1] drew nothing,
// and so did one that put it at rec[7..8]; a layout that additionally wrote the listings id into
// rec[7..8] still drew, which is what shows those two bytes are ignored rather than merely spare.
//
// THE INDEX IS A REFERENCE, NOT A COPY, so it names programmes the box may or may not hold: it
// registers only the six-hour blocks it has subscribed to. It carries the WHOLE DAY anyway, and
// that is measured rather than assumed -- sending one index entry for a stored programme and one
// for an unstored one drew exactly the same screen as the stored one alone, so the box skips what
// it cannot resolve. Carrying the day means the index is already right when the clock moves into
// the next block, instead of being correct only for the six hours it was built in.
//
// A letter with no programmes is SKIPPED rather than sent empty: the builder refuses a section with
// no records, because a section that announces nothing is indistinguishable at the screen from one
// that never arrived.
func (m *Multiplex) indexWave(uint64) ([]broadcast.Emission, error) {
	sub, asking := m.subscription()
	if !asking || !sub.IndexArmed {
		return nil, nil
	}
	listings := m.listings()
	if listings == nil || len(listings.Services) == 0 {
		return nil, nil
	}
	byLetter := map[byte][]broadcast.IndexRecord{}
	for i := range listings.Services {
		service := &listings.Services[i]
		for n := range service.Programmes {
			programme := &service.Programmes[n]
			// This literal is the schedule's explicit statement that no programme has been
			// reconstructed. It keeps the grid populated honestly, but it is not a programme title
			// and must not be advertised four times per channel in ALL PROGRAMMES A-Z. Besides making
			// that search useless, 68 channels would turn the L index into an invalid oversized DVB
			// section. Real reconstructed titles continue through the ordinary signal path below.
			if programme.Title == "Listings not yet reconstructed" {
				continue
			}
			letter, ok := indexInitial(programme.Title)
			if !ok {
				continue
			}
			event := uint16(n + 1) // #nosec G115 -- a day's programmes, bounded by the section length
			byLetter[letter] = append(byLetter[letter], broadcast.IndexRecord{
				ID:       service.ListingsID,
				Packed:   indexFlags,
				Selector: indexSelector,
				Data:     [5]byte{0, byte(event >> 8), byte(event & 0xff), 0, 0}, // #nosec G115 -- masked
			})
		}
	}
	// ONE LETTER PER WAVE, ROUND-ROBIN -- NOT NINETEEN SECTIONS IN ONE PUSH.
	//
	// This is what a carousel IS, and sending the alphabet in one burst is not a shortcut, it is a
	// different thing that loses most of what it sends. Every index section goes to the same PID
	// and therefore the same section filter, and a wave is pushed inside a single Pump with no
	// guest instructions between the pushes -- so the box is handed nineteen sections without ever
	// running the task that drains them. Measured: one burst of nineteen letters filled EIGHT of
	// the twenty-six list heads, and the screen that opens on 'A' found nothing because 'A' was
	// among the eleven lost. Nothing errored; the ring is twelve kilobytes and never wrapped.
	//
	// Serially, each section gets a whole IndexPeriod of guest time to itself, which is both the
	// fix and what a real multiplex does.
	// EVERY LETTER, INCLUDING THE EMPTY ONES. A six-channel schedule leaves seven letters with no
	// programmes at all, and skipping them leaves those list heads null -- which is not the same
	// thing as a letter with nothing on it. An empty section is a well-formed list of length zero
	// and costs one wave.
	letters := make([]byte, 0, 26)
	for letter := byte('A'); letter <= 'Z'; letter++ {
		letters = append(letters, letter)
	}
	letter := letters[m.indexCursor%len(letters)]
	m.indexCursor = (m.indexCursor + 1) % len(letters)
	// A NEW VERSION EACH TIME ROUND THE ALPHABET, because the box DEDUPLICATES a repeat.
	//
	// A carousel that sends the same bytes for ever is only useful if the receiver keeps what it
	// was given, and this one does not: the guest drops a section whose table, extension and
	// version it has already parsed, so every cycle after the first was discarded -- measured, all
	// nineteen list heads filled and the screen still drawing "Searching for listings" a hundred
	// million instructions later, while the same sections hand-delivered just before the screen
	// opened filled it. Bumping the version once per full cycle makes each pass a fresh delivery,
	// which is what a screen opened at a moment of the viewer's choosing needs.
	if m.indexCursor == 0 {
		m.indexVersion = (m.indexVersion + 1) & 0x1f
	}
	extension, err := broadcast.IndexLetter(letter)
	if err != nil {
		return nil, err
	}
	section, err := broadcast.IndexSection(extension, m.indexVersion, 0, 0, byLetter[letter])
	if err != nil {
		return nil, err
	}
	m.sent.Index++
	return []broadcast.Emission{{PID: indexPID, Section: section}}, nil
}

// indexInitial is the letter a programme files under: the first character in 'A'..'Z', case
// folded.
//
// "The X Files" files under T. Sky may well have dropped a leading article -- every A-Z index in
// television does something about it -- but which words it stripped is not established here, and a
// stop-word list invented now would put programmes under letters nobody measured.
func indexInitial(title string) (byte, bool) {
	for i := 0; i < len(title); i++ {
		c := title[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c >= 'A' && c <= 'Z' {
			return c, true
		}
	}
	return 0, false
}

// eventWave is the present/following EIT: for every service on the transport, what is on now and
// what is on next.
//
// THE BOX ASKED FOR THIS BY NAME, which is why it exists at all and why nothing about its
// addressing is a choice. The moment it tunes it arms filter 18 on PID 0x0012 with match unit 4
// carrying 4e/fe 00/ff 64/ff -- table 0x4E or 0x4F, table_id_extension 0x0064, service_id 100,
// which is the channel it has just selected. Measured 2026-09-23 by dumping all sixteen match
// units either side of a tune; before that, this port sent nothing on that PID at all.
//
// EVERY SERVICE, NOT THE TUNED ONE. Which service the viewer is on is the box's business and the
// filter's: it accepts the extension it armed and the hardware drops the rest, exactly as a real
// multiplex works. Transmitting only the service we believe is tuned would mean tracking the
// tuning in the transmitter, which is a second source of truth for something the box already
// knows.
//
// A SERVICE WITH NO PROGRAMME ON AIR STILL SENDS ITS SECTIONS, with an empty event loop. "I have
// nothing on now" and "you have never heard from me" are different statements and the box may
// treat them differently; sending nothing would make them the same.
func (m *Multiplex) eventWave(uint64) ([]broadcast.Emission, error) {
	listings := m.listings()
	if listings == nil || len(listings.Services) == 0 {
		return nil, nil
	}
	sub, asking := m.subscription()
	if !asking {
		return nil, nil
	}
	// THE BOX MUST HAVE ARMED THE PID, and this is not politeness. The demux refuses a section
	// nobody asked for, and Pump turns that refusal into a transmitter error, so a wave sent
	// before the box tunes would bring the server down over a television schedule. It arms PID
	// 0x0012 when it TUNES and not before -- measured: six filters after acquisition and with the
	// guide open, seven while viewing a channel.
	if !sub.EITArmed {
		return nil, nil
	}
	now := m.clock.Now()
	var wave []broadcast.Emission
	for i := range listings.Services {
		service := &listings.Services[i]
		present, following, err := m.onAirAndNext(service, now)
		if err != nil {
			return nil, err
		}
		for _, pair := range []struct {
			number byte
			event  *broadcast.Event
		}{{broadcast.EventPresent, present}, {broadcast.EventFollowing, following}} {
			section, err := broadcast.EITPresentFollowing(service.ServiceID, sub.NetworkID,
				sub.NetworkID, m.eventVersion, pair.number, pair.event)
			if err != nil {
				return nil, err
			}
			wave = append(wave, broadcast.Emission{PID: broadcast.EventPID, Section: section})
			m.sent.Events++
		}
	}
	// A NEW VERSION EACH WAVE, because the guest drops a section whose table, extension and
	// version it has already parsed -- the finding that made the A-Z index work. Present and
	// following move with the clock, so a version that never changed would freeze the banner on
	// whatever was on when the box first tuned.
	m.eventVersion = (m.eventVersion + 1) & 0x1f
	if !m.onEventsSent && m.sent.Events > 0 && m.onAir != nil {
		m.onEventsSent = true
		m.onAir(m.sent, sub.Titles)
	}
	return wave, nil
}

// onAirAndNext is the programme running at now and the one after it, as EIT events.
//
// IT WORKS IN THE BOX'S OWN DAY and returns nil for either half rather than inventing one. A
// schedule that ends at midnight has no following event on its last programme, and saying so is
// the truth; borrowing tomorrow's first programme would be a guess about a day the transmitter has
// not been asked for.
func (m *Multiplex) onAirAndNext(service *ListedService, now time.Time) (*broadcast.Event, *broadcast.Event, error) {
	seconds := secondsOfDay(now)
	// Midnight of the box's own day, written out rather than truncated: Truncate rounds against
	// the zero time in UTC, so it is only midnight by coincidence of the demo's clock being UTC.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var present, following *broadcast.Event
	for n := range service.Programmes {
		programme := &service.Programmes[n]
		start, err := programme.StartSeconds()
		if err != nil {
			return nil, nil, err
		}
		event := &broadcast.Event{
			ID:       uint16(n + 1), //#nosec G115 -- a day's programmes, bounded by the section length
			Start:    midnight.Add(time.Duration(start) * time.Second),
			Duration: time.Duration(programme.Minutes) * time.Minute,
			Name:     programme.Title,
			Running:  broadcast.RunningNotRunning,
		}
		switch {
		case start <= seconds && seconds < start+programme.Minutes*60:
			event.Running = broadcast.RunningRunning
			present = event
		case start > seconds && following == nil:
			// THE NEXT ONE TO START, WHETHER OR NOT ANYTHING IS ON NOW. A channel with a gap in
			// its schedule still has a following programme, and tying "following" to the presence
			// of a "present" would make the gap look like the end of the day. The list is in
			// start order, so the first one past the clock is the one.
			event.Running = broadcast.RunningStartsShortly
			following = event
		}
		if present != nil && following != nil {
			break
		}
	}
	return present, following, nil
}
