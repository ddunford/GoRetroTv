package multiplex

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
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
