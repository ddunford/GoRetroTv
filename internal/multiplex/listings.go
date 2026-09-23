package multiplex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The editable schedule.
//
// It carries the things that are OURS -- which channels exist, what is on them
// and when -- and deliberately none of the things that are the BOX's. There is
// no bouquet id here, no network id, no transport id, no table id and no date,
// because every one of those is read back off the running machine in
// request.go. A schedule file that named them would be a second opinion about
// facts the hardware already settles, and the first time the two disagreed the
// broadcast would go out addressed to nobody.
//
// The day is absent for a second reason as well: the same schedule plays on
// whichever day the box asks for, which is what lets an in-world clock move
// without the content having to follow it.

// Listings is a day's television, by channel.
type Listings struct {
	// Bouquet is the name the BAT announces, e.g. "Sky".
	Bouquet string `json:"bouquet"`
	// Services are the channels, in the order they should be numbered.
	Services []ListedService `json:"services"`
}

// ListedService is one channel and its day.
type ListedService struct {
	// Name is what the channel is called in the SDT and the guide.
	Name string `json:"name"`
	// Channel is the number a viewer types on the handset.
	Channel uint16 `json:"channel"`
	// ServiceID is the DVB service_id. It ties the SDT entry, the line-up
	// entry and the guide's linkage together, so it must be unique.
	ServiceID uint16 `json:"serviceId"`
	// ListingsID is the value the line-up entry carries at +3..4, which the
	// box turns into the table-id extension it asks listings for. It is the
	// join between a channel and its programmes, and matching it is how a
	// title section reaches the right row of the guide.
	ListingsID uint16 `json:"listingsId"`
	// Type is the SDT's service_type for this channel. Left unset it defaults to 1, digital
	// television, which is what every feed this port has ever sent.
	//
	// It is transmittable because the grid's row type is copied out of an object at MIPS
	// 0x800CB138 and reads 1 on every row, and 1 is also what we send here -- a correlation worth
	// being able to test rather than argue about. What the o-code's three branches (1, 2 and 0x10)
	// mean is not established.
	Type byte `json:"serviceType,omitempty"`
	// RowAt8 overrides the 0xB2 descriptor's first scalar, the byte that lands at the grid row
	// record's +8 and that the row's o-code branches on at 0x9FC73961. Left unset it is 1, which
	// is the smallest non-zero value and is what makes the grid draw at all.
	//
	// It is transmittable because WHAT THE VALUE MEANS IS NOT ESTABLISHED -- only that zero leaves
	// the row drawing "..no listings available". The path it unlocks does a bounds check and then
	// runs a layout loop, so a count is as plausible as a flag, and that is a sweep rather than an
	// argument.
	RowAt8 byte `json:"rowAt8,omitempty"`
	// Genre is the channel's genre, and it is the 0xB2 descriptor's three-bit scalar -- the one
	// the parser stores at the row record's +9, which this file used to call RowAt9 because
	// nothing was known about it.
	//
	// IT IS WHAT THE TV GUIDE'S GENRE SCREENS FILTER ON. Each of them carries a four-byte filter
	// whose +2 is the genre it wants, and FUN_800cb7b8 accepts a channel when that byte equals the
	// value it finds by searching the SI for this descriptor. Measured by reading each screen's
	// filter and confirmed by putting values in this field and watching the right channels appear:
	//
	//	1 SPECIALIST   2 KIDS   3 ENTERTAINMENT   4 MUSIC & RADIO
	//	5 NEWS & DOCUMENTARIES   6 MOVIES   7 SPORTS
	//
	// Zero is no genre, which is what every channel answered before this field existed and why
	// every genre screen was empty. ALL CHANNELS has no filter at all -- its mask half reads 0000
	// -- so a channel appears there whatever its genre.
	Genre byte `json:"genre,omitempty"`
	// RowAt10 and RowAt11 are the 0xB2's remaining scalars, at the row record's +10 and +11 (one
	// bit). NOTHING IS KNOWN ABOUT EITHER -- the grid draws correctly with both at zero -- and
	// they stay transmittable for the same reason RowAt8 is: an unknown that can be varied is an
	// unknown that can be measured.
	RowAt10 byte `json:"rowAt10,omitempty"`
	RowAt11 byte `json:"rowAt11,omitempty"`
	// Kind is the byte the line-up entry carries at +2, between the service id
	// and the listings id.
	//
	// EVERY FEED THIS PORT HAS EVER SENT SET IT TO 1, and 1 was a guess made
	// when the entry was first laid out. It sits in the one descriptor that
	// ties a channel to its programmes, so a value that means "this channel
	// has no schedule" would empty the guide's grid while leaving the tuned
	// service -- which nothing filters -- showing programmes perfectly. That
	// is the exact shape of the failure, which is why the field is now
	// transmittable instead of hard-coded. What the values MEAN is not
	// established and is not guessed at; left unset it keeps the long-standing
	// 1 so no existing schedule changes behaviour.
	Kind byte `json:"kind,omitempty"`
	// Flags is the four-bit field the line-up entry carries in the low nibble
	// at +7..8. The guest unpacks it into a word the TV GUIDE's list screens
	// mask before they will show a channel at all, so a channel with no flags
	// is a channel the guide will not draw. Left unset it defaults to the
	// measured guide-visible combination; set it to say otherwise. What the
	// individual bits mean is not established and is not guessed at.
	Flags byte `json:"flags,omitempty"`
	// Programmes are the day's events, in any order; they are sorted on load.
	Programmes []ListedProgramme `json:"programmes"`
}

// ListedProgramme is one event.
type ListedProgramme struct {
	// Start is "HH:MM" in the box's own day.
	Start string `json:"start"`
	// Minutes is how long it runs.
	Minutes int `json:"minutes"`
	// Title is what the guide shows.
	Title string `json:"title"`
	// Genre and Rating are the OpenTV bytes, both optional.
	Genre  byte `json:"genre,omitempty"`
	Rating byte `json:"rating,omitempty"`
}

// StartSeconds is the programme's start as seconds into the day.
//
// The wire carries start and duration HALVED, so both quantise to two seconds;
// "HH:MM" can never offend that, which is why the file is written in minutes
// rather than in seconds that would have to be validated.
func (p ListedProgramme) StartSeconds() (int, error) {
	hours, minutes, ok := strings.Cut(p.Start, ":")
	if !ok {
		return 0, fmt.Errorf("multiplex: start %q is not HH:MM", p.Start)
	}
	h, err := strconv.Atoi(hours)
	if err != nil {
		return 0, fmt.Errorf("multiplex: start %q: %w", p.Start, err)
	}
	m, err := strconv.Atoi(minutes)
	if err != nil {
		return 0, fmt.Errorf("multiplex: start %q: %w", p.Start, err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("multiplex: start %q is not a time of day", p.Start)
	}
	return h*3600 + m*60, nil
}

// Guide is every schedule available to broadcast, keyed by the date it is for.
//
// A day with its own file gets that file; every other day gets the default.
// That is what lets a real 1998 listings page be dropped in for the date it
// was printed for, while any other date still has television on it.
type Guide struct {
	byDate   map[string]*Listings
	fallback *Listings
	// Source is where it was loaded from, for the startup log and for Reload.
	Source string
	// stamp fingerprints the bytes on disk, so an unchanged schedule costs a
	// read and a hash rather than a parse and a validation.
	stamp string
}

// scheduleFor names the file a date is served by.
const defaultScheduleName = "default.json"

// LoadGuide reads either a single schedule file, which then plays on every
// date, or a DIRECTORY of them named YYYY-MM-DD.json with a default.json
// beside them.
//
// A directory with no default.json is refused. The alternative is a demo that
// works on the dates somebody happened to write up and silently shows nothing
// on the rest, which is the failure mode this whole area keeps producing.
func LoadGuide(path string) (*Guide, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("multiplex: schedule: %w", err)
	}
	if !info.IsDir() {
		listings, err := LoadListings(path)
		if err != nil {
			return nil, err
		}
		stamp, err := stampOf(path)
		if err != nil {
			return nil, err
		}
		return &Guide{fallback: listings, Source: path, stamp: stamp}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("multiplex: schedule directory: %w", err)
	}
	guide := &Guide{byDate: map[string]*Listings{}, Source: path}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		listings, err := LoadListings(filepath.Join(path, name))
		if err != nil {
			return nil, err
		}
		if name == defaultScheduleName {
			guide.fallback = listings
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if _, err := time.Parse(dateLayout, date); err != nil {
			return nil, fmt.Errorf("multiplex: %s is neither %s nor a YYYY-MM-DD.json date", name, defaultScheduleName)
		}
		guide.byDate[date] = listings
	}
	if guide.fallback == nil {
		return nil, fmt.Errorf("multiplex: %s has no %s, so any date without its own file would broadcast nothing",
			path, defaultScheduleName)
	}
	if guide.stamp, err = stampOf(path); err != nil {
		return nil, err
	}
	return guide, nil
}

// dateLayout is how a schedule file names its date, and how an operator writes
// one in the environment.
const dateLayout = "2006-01-02"

// Reload re-reads the schedule from disk, so an edit reaches the air without
// the box being restarted.
//
// A MALFORMED EDIT KEEPS THE LAST GOOD SCHEDULE AND SAYS SO. It returns the
// error and leaves this Guide exactly as it was -- which is the only safe
// answer while somebody is halfway through typing into a file a running
// broadcast reads. The alternative, an empty or partial schedule going on air
// because a closing brace was missing for half a second, would look to a
// viewer like the box breaking.
//
// It reports changed=false when the bytes on disk are the same as the ones
// already loaded, which is the usual case: this is called on the carousel's
// own cadence, so it runs many times for every real edit.
//
// It is called FROM the instruction loop rather than from a watcher goroutine.
// The loop is the board's only owner (CLAUDE.md -> no goroutine in the
// instruction loop), so a background reloader would need a lock around
// something that has never needed one, to save a file stat every few seconds.
func (g *Guide) Reload() (changed bool, err error) {
	stamp, err := stampOf(g.Source)
	if err != nil {
		return false, err
	}
	if stamp == g.stamp {
		return false, nil
	}
	fresh, err := LoadGuide(g.Source)
	if err != nil {
		// The stamp is deliberately NOT updated. A file that is broken now
		// will be read again on the next pass, so saving a fix takes effect
		// without anything else happening.
		return false, err
	}
	g.byDate, g.fallback, g.stamp = fresh.byDate, fresh.fallback, fresh.stamp
	return true, nil
}

// stampOf fingerprints what a schedule path currently holds.
//
// It hashes CONTENTS rather than modification times. An editor that writes a
// file twice within one filesystem timestamp tick is ordinary, and a schedule
// that silently failed to reload because two edits shared a second would be a
// bug nobody could reproduce.
func stampOf(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("multiplex: schedule: %w", err)
	}
	sum := sha256.New()
	add := func(name string) error {
		blob, err := os.ReadFile(name) // #nosec G304 -- an operator-supplied schedule path
		if err != nil {
			return err
		}
		// The name and length go in as well as the bytes, so two files
		// swapping contents changes the stamp. hash.Hash documents that Write
		// never returns an error, so both results are discarded deliberately
		// rather than left unchecked.
		_, _ = fmt.Fprintf(sum, "%s:%d:", filepath.Base(name), len(blob))
		_, _ = sum.Write(blob)
		return nil
	}
	if !info.IsDir() {
		if err := add(path); err != nil {
			return "", err
		}
		return hex.EncodeToString(sum.Sum(nil)), nil
	}
	entries, err := os.ReadDir(path) // sorted by name, so the hash is stable
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := add(filepath.Join(path, entry.Name())); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// On returns the schedule to broadcast for a given day.
func (g *Guide) On(day time.Time) *Listings {
	if listings, ok := g.byDate[day.UTC().Format(dateLayout)]; ok {
		return listings
	}
	return g.fallback
}

// Dated is how many days have a schedule of their own.
func (g *Guide) Dated() int { return len(g.byDate) }

// LoadListings reads and validates a schedule file.
//
// Everything it rejects, it rejects by NAMING the channel and the programme,
// because the file is meant to be edited by hand and an error that says only
// "invalid" sends the editor back to read the whole thing.
func LoadListings(path string) (*Listings, error) {
	blob, err := os.ReadFile(path) // #nosec G304 -- an operator-supplied schedule path
	if err != nil {
		return nil, fmt.Errorf("multiplex: read schedule: %w", err)
	}
	var listings Listings
	decoder := json.NewDecoder(strings.NewReader(string(blob)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&listings); err != nil {
		return nil, fmt.Errorf("multiplex: parse %s: %w", path, err)
	}
	if err := listings.validate(); err != nil {
		return nil, fmt.Errorf("multiplex: %s: %w", path, err)
	}
	return &listings, nil
}

// validate refuses a schedule that would broadcast plausibly and wrongly.
func (l *Listings) validate() error {
	if strings.TrimSpace(l.Bouquet) == "" {
		return fmt.Errorf("the bouquet has no name")
	}
	if len(l.Services) == 0 {
		return fmt.Errorf("the schedule lists no channels, so there is nothing to broadcast")
	}
	seenService := map[uint16]string{}
	seenListings := map[uint16]string{}
	for i := range l.Services {
		service := &l.Services[i]
		switch {
		case strings.TrimSpace(service.Name) == "":
			return fmt.Errorf("channel %d has no name", service.Channel)
		case service.ServiceID == 0:
			return fmt.Errorf("%q has no serviceId", service.Name)
		case service.ListingsID == 0:
			return fmt.Errorf("%q has no listingsId, so its programmes could never be addressed", service.Name)
		case service.Flags > 0x0f:
			return fmt.Errorf("%q has flags %#x, and the line-up entry carries four bits", service.Name, service.Flags)
		case service.Genre > 7:
			// REFUSED AT LOAD RATHER THAN AT TRANSMIT. guideRowDescriptor would reject it too,
			// but by then the schedule is on air and the failure arrives as a carousel error in
			// the middle of a wave -- which is how a validation narrower than its builders let a
			// valid-looking schedule halt the box before (gort-sgc).
			return fmt.Errorf("%q has genre %d, and the 0xB2 descriptor carries three bits: the "+
				"guide's screens are 1 SPECIALIST, 2 KIDS, 3 ENTERTAINMENT, 4 MUSIC & RADIO, "+
				"5 NEWS & DOCUMENTARIES, 6 MOVIES, 7 SPORTS, and 0 is no genre",
				service.Name, service.Genre)
		}
		// Duplicates are the failure this catches: two channels sharing a
		// listingsId would have their programmes filed under one another, and
		// the guide would show a plausible wrong day's television.
		if other, dup := seenService[service.ServiceID]; dup {
			return fmt.Errorf("%q and %q share serviceId %d", other, service.Name, service.ServiceID)
		}
		if other, dup := seenListings[service.ListingsID]; dup {
			return fmt.Errorf("%q and %q share listingsId %d", other, service.Name, service.ListingsID)
		}
		seenService[service.ServiceID] = service.Name
		seenListings[service.ListingsID] = service.Name

		if len(service.Programmes) == 0 {
			return fmt.Errorf("%q lists no programmes", service.Name)
		}
		for _, programme := range service.Programmes {
			if strings.TrimSpace(programme.Title) == "" {
				return fmt.Errorf("%q has a programme at %s with no title", service.Name, programme.Start)
			}
			if programme.Minutes <= 0 {
				return fmt.Errorf("%q: %q runs for %d minutes", service.Name, programme.Title, programme.Minutes)
			}
			if _, err := programme.StartSeconds(); err != nil {
				return fmt.Errorf("%q: %q: %w", service.Name, programme.Title, err)
			}
		}
		sort.SliceStable(service.Programmes, func(a, b int) bool {
			left, _ := service.Programmes[a].StartSeconds()
			right, _ := service.Programmes[b].StartSeconds()
			return left < right
		})
	}
	return nil
}

// Service returns the channel whose listings id the box asked about, which is
// the only way a title request names a channel.
func (l *Listings) Service(listingsID uint16) (*ListedService, bool) {
	for i := range l.Services {
		if l.Services[i].ListingsID == listingsID {
			return &l.Services[i], true
		}
	}
	return nil, false
}
