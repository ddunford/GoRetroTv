package broadcast

import (
	"fmt"
)

// The transmitter. This file owns WHEN a section goes on air and in what
// order; it owns neither what is in it nor where it is addressed.
//
// The order is load-bearing and the rate is not, and both halves of that are
// measured rather than assumed.
//
// **Clock first.** The box's entire listings request is day-addressed and it
// programs the request ONCE, during acquisition, so a line-up that arrives
// before the clock leaves the box asking for the wrong day for ever, with no
// error anywhere. Set the clock first and the whole request moves together:
// the requested MJD becomes the clock's own day and the listings PID becomes
// 0x30 | (MJD mod 8), Sky's eight title PIDs being a day-of-eight rotation.
// Both were measured across eight consecutive days on 2026-09-20; the record
// carries the sweep.
//
// **The rate is not load-bearing.** Sixteen sections pushed by hand acquire a
// line-up with no rate involved at all, measured twice. So the periods here
// are chosen for how quickly a person watching the demo sees the guide fill,
// and nothing downstream may come to depend on their values.
//
// Everything is counted in instructions, never in wall time, because the
// instruction counter is this machine's only clock (CLAUDE.md). A carousel
// scheduled off the wall is what made the predecessor produce different event
// counts on two runs of the same firmware, and sent a session hunting a device
// bug that did not exist.

// Emission is one section and the PID it is transmitted on. A section without
// a PID cannot be delivered -- the hardware filters on the PID before anything
// else looks at the table id -- so the two never travel separately.
type Emission struct {
	PID     uint16
	Section []byte
}

// Source supplies the sections for a wave. The carousel owns the schedule; the
// source owns the content and the addressing, because the table id, the PID
// and the MJD must all be read off the running box rather than chosen (the
// box's own match unit is the only thing that says what it will accept) and
// that reading is TASK-6.6's job, not this one's.
//
// A source function returning no emissions is not an error: a title source
// with no armed PID yet has nothing to say, and saying nothing is the correct
// behaviour rather than a fault.
type Source struct {
	// Clock supplies the clock tables.
	//
	// THE TOT IS THE CLOCK TABLE AND THE TDT IS NOT. Measured 2026-09-20:
	// the box's match units carry 0x73 and nothing matches 0x70 at all, so a
	// TDT is never delivered to anything. A carousel that sends only a TDT
	// leaves the box on whatever day it woke with, moves no PID, and reports
	// no error -- which is what every earlier measurement in this project that
	// "set the clock" was actually doing.
	Clock func(now uint64) ([]Emission, error)
	// Lineup supplies NIT, BAT and SDT -- the acquisition tables.
	Lineup func(now uint64) ([]Emission, error)
	// Titles supplies the OpenTV title sections.
	Titles func(now uint64) ([]Emission, error)
}

// Schedule is the carousel's timing, in instructions.
type Schedule struct {
	// ClockPeriod is the gap between clock waves. TDT and TOT repeat from the
	// first wave and never stop, because the box treats a stale clock as no
	// clock.
	ClockPeriod uint64
	// LineupPeriod is the gap between acquisition waves.
	LineupPeriod uint64
	// TitlePeriod is the gap between title waves.
	TitlePeriod uint64
	// ClockSettle is how long after the FIRST clock wave the line-up is held
	// back. This is the "clock first" rule with a number on it: the box needs
	// the TDT delivered, parsed and applied before the BAT starts acquisition,
	// or it programs a day-addressed request against the epoch.
	ClockSettle uint64
	// LineupSettle is how long after the FIRST line-up wave the titles are
	// held back, for the same reason one rung up: until acquisition has run,
	// the box has armed no title PID and a title section is addressed to a
	// filter that does not exist. The hardware collects it and drops it
	// silently, which from outside is indistinguishable from a box that
	// received it and ignored it.
	LineupSettle uint64
}

// Carousel emits waves of sections on an instruction-counted schedule, holding
// each rung back until the one below it has had time to take effect.
//
// It is not safe for concurrent use, deliberately: it is driven from the
// instruction loop, which is single-threaded by construction.
type Carousel struct {
	schedule Schedule
	source   Source

	started    bool
	nextClock  uint64
	nextLineup uint64
	nextTitles uint64

	clockedAt  uint64 // instruction of the first clock wave
	lineupAt   uint64 // instruction of the first line-up wave
	sentClock  bool
	sentLineup bool
}

// NewCarousel validates a schedule and a source and returns a carousel that
// has not yet transmitted anything.
//
// Every period is rejected at zero rather than treated as "every instruction".
// A wave due every zero instructions is due again the moment it fires, which
// presents as a hang inside one Advance rather than as the mistake it is --
// the same reason platform/clock rejects a zero period.
func NewCarousel(schedule Schedule, source Source) (*Carousel, error) {
	for _, p := range []struct {
		name   string
		period uint64
	}{
		{"ClockPeriod", schedule.ClockPeriod},
		{"LineupPeriod", schedule.LineupPeriod},
		{"TitlePeriod", schedule.TitlePeriod},
	} {
		if p.period == 0 {
			return nil, fmt.Errorf("broadcast: carousel %s is zero; a wave due every zero instructions never stops firing", p.name)
		}
	}
	if source.Clock == nil {
		return nil, fmt.Errorf("broadcast: a carousel with no clock source can never release the line-up, so the box would ask for the epoch for ever")
	}
	if source.Lineup == nil {
		return nil, fmt.Errorf("broadcast: a carousel with no line-up source never acquires")
	}
	if source.Titles == nil {
		return nil, fmt.Errorf("broadcast: a carousel with no title source carries no programmes")
	}
	return &Carousel{schedule: schedule, source: source}, nil
}

// Wave returns everything due at instruction now, in transmission order, and
// advances the schedule past it.
//
// now must not go backwards: the carousel is driven off a monotonic
// instruction counter, and rewinding it would replay waves that already went
// out. A snapshot restore rewinds the machine and the carousel together, which
// is why the check is against the carousel's own last call rather than against
// absolute zero.
func (c *Carousel) Wave(now uint64) ([]Emission, error) {
	if !c.started {
		c.started = true
		c.nextClock, c.nextLineup, c.nextTitles = now, now, now
	}
	var wave []Emission

	if now >= c.nextClock {
		sections, err := c.source.Clock(now)
		if err != nil {
			return nil, fmt.Errorf("broadcast: carousel clock wave at %d: %w", now, err)
		}
		wave = append(wave, sections...)
		c.nextClock = now + c.schedule.ClockPeriod
		if !c.sentClock && len(sections) > 0 {
			c.sentClock, c.clockedAt = true, now
		}
	}

	// Each rung is gated on the one below it having gone out AND having had
	// its settle. Both halves matter: without the first the order is wrong,
	// and without the second the order is right but the box has not acted on
	// it yet, which produces exactly the same permanently-wrong request.
	if c.sentClock && now >= c.clockedAt+c.schedule.ClockSettle && now >= c.nextLineup {
		sections, err := c.source.Lineup(now)
		if err != nil {
			return nil, fmt.Errorf("broadcast: carousel line-up wave at %d: %w", now, err)
		}
		wave = append(wave, sections...)
		c.nextLineup = now + c.schedule.LineupPeriod
		if !c.sentLineup && len(sections) > 0 {
			c.sentLineup, c.lineupAt = true, now
		}
	}

	if c.sentLineup && now >= c.lineupAt+c.schedule.LineupSettle && now >= c.nextTitles {
		sections, err := c.source.Titles(now)
		if err != nil {
			return nil, fmt.Errorf("broadcast: carousel title wave at %d: %w", now, err)
		}
		wave = append(wave, sections...)
		c.nextTitles = now + c.schedule.TitlePeriod
	}

	return wave, nil
}

// NextDue is the earliest instruction at which Wave could produce anything, so
// a driver can schedule one clock event instead of polling every instruction.
//
// It is a lower bound rather than a promise: a rung still held back reports the
// instruction its hold expires, and a source may legitimately have nothing to
// say when asked. Waking early and emitting nothing is correct; sleeping past
// a due wave is not.
func (c *Carousel) NextDue() uint64 {
	if !c.started {
		return 0
	}
	due := c.nextClock
	lineup := c.nextLineup
	if c.sentClock {
		if hold := c.clockedAt + c.schedule.ClockSettle; hold > lineup {
			lineup = hold
		}
		if lineup < due {
			due = lineup
		}
	}
	if c.sentLineup {
		titles := c.nextTitles
		if hold := c.lineupAt + c.schedule.LineupSettle; hold > titles {
			titles = hold
		}
		if titles < due {
			due = titles
		}
	}
	return due
}
