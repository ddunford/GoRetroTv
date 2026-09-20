package multiplex_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// The schedule is edited by hand, so every way of getting it wrong has to be
// named back at whoever got it wrong. These are the ones that would otherwise
// broadcast plausibly and silently: a duplicate listings id files two channels'
// programmes under one another, and a channel with no listings id can never be
// addressed at all.
func TestASchedulesFaultsAreRefusedAndNamed(t *testing.T) {
	const good = `{"bouquet":"Sky Digital","services":[
		{"name":"Sky One","channel":101,"serviceId":100,"listingsId":101,
		 "programmes":[{"start":"19:00","minutes":60,"title":"Dream Team"}]},
		{"name":"Sky News","channel":501,"serviceId":101,"listingsId":501,
		 "programmes":[{"start":"22:00","minutes":30,"title":"Sky News At Ten"}]}]}`

	for _, tc := range []struct {
		name    string
		body    string
		wantErr string
	}{
		{"two channels sharing a listings id", strings.Replace(good, `"listingsId":501`, `"listingsId":101`, 1),
			"share listingsId"},
		{"two channels sharing a service id", strings.Replace(good, `"serviceId":101`, `"serviceId":100`, 1),
			"share serviceId"},
		{"a channel with no listings id", strings.Replace(good, `"listingsId":101,`, `"listingsId":0,`, 1),
			"no listingsId"},
		{"a channel with no programmes", strings.Replace(good, `[{"start":"19:00","minutes":60,"title":"Dream Team"}]`, `[]`, 1),
			"lists no programmes"},
		{"a programme with no title", strings.Replace(good, `"title":"Dream Team"`, `"title":"  "`, 1),
			"no title"},
		{"a programme of no length", strings.Replace(good, `"minutes":60`, `"minutes":0`, 1),
			"runs for 0 minutes"},
		{"a start time that is not a time", strings.Replace(good, `"start":"19:00"`, `"start":"evening"`, 1),
			"not HH:MM"},
		{"a start time out of the day", strings.Replace(good, `"start":"19:00"`, `"start":"25:00"`, 1),
			"not a time of day"},
		{"a bouquet with no name", strings.Replace(good, `"bouquet":"Sky Digital"`, `"bouquet":""`, 1),
			"bouquet has no name"},
		{"no channels at all", `{"bouquet":"Sky Digital","services":[]}`,
			"lists no channels"},
		{"a field nobody reads", strings.Replace(good, `"bouquet":`, `"day":"1998-06-15","bouquet":`, 1),
			"unknown field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "listings.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := multiplex.LoadListings(path)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name the fault (%q), so an editor is sent back to read the whole file",
					err, tc.wantErr)
			}
		})
	}

	path := filepath.Join(t.TempDir(), "listings.json")
	if err := os.WriteFile(path, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := multiplex.LoadListings(path); err != nil {
		t.Fatalf("the control schedule was refused, so the cases above prove nothing: %v", err)
	}
}

// Programmes are sorted on load, because a hand-edited file is in whatever
// order the editor found convenient and the wire order is what the guide walks.
func TestProgrammesAreSortedByStart(t *testing.T) {
	body := `{"bouquet":"Sky","services":[{"name":"Sky One","channel":101,"serviceId":100,"listingsId":101,
		"programmes":[
			{"start":"22:00","minutes":60,"title":"Late"},
			{"start":"06:00","minutes":60,"title":"Early"},
			{"start":"19:00","minutes":60,"title":"Evening"}]}]}`
	path := filepath.Join(t.TempDir(), "listings.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	listings, err := multiplex.LoadListings(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(listings.Services[0].Programmes))
	for _, programme := range listings.Services[0].Programmes {
		got = append(got, programme.Title)
	}
	want := []string{"Early", "Evening", "Late"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("programmes are in order %v, want %v", got, want)
		}
	}
}

// The committed schedules are what the demo broadcasts, so they are checked
// like any other input rather than trusted for being ours. Every dated file is
// loaded too, because a directory's whole point is that most of it is only
// read on one day of the year.
func TestTheCommittedSchedulesAreValid(t *testing.T) {
	guide, err := multiplex.LoadGuide(filepath.Join("..", "..", "listings"))
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	if len(guide.On(day).Services) == 0 {
		t.Fatal("Christmas Eve has no channels")
	}
	if guide.Dated() == 0 {
		t.Error("no dated schedule was loaded, so the per-date path is untested against real files")
	}
}

// A dated file plays on its date and the default plays on every other, which is
// the whole point of a directory: a real listings page keeps the date it was
// printed for.
func TestADatedScheduleReplacesTheDefaultOnItsOwnDay(t *testing.T) {
	dir := t.TempDir()
	write := func(name, bouquet string) {
		body := `{"bouquet":"` + bouquet + `","services":[{"name":"Sky One","channel":101,` +
			`"serviceId":100,"listingsId":101,"programmes":[{"start":"19:00","minutes":60,"title":"Dream Team"}]}]}`
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("default.json", "Every Other Day")
	write("1998-12-24.json", "Christmas Eve")

	guide, err := multiplex.LoadGuide(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		day  time.Time
		want string
	}{
		{time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC), "Christmas Eve"},
		{time.Date(1998, 12, 23, 23, 59, 0, 0, time.UTC), "Every Other Day"},
		{time.Date(1998, 12, 25, 0, 1, 0, 0, time.UTC), "Every Other Day"},
		{time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC), "Every Other Day"},
	} {
		if got := guide.On(tc.day).Bouquet; got != tc.want {
			t.Errorf("%s served %q, want %q", tc.day.Format("2006-01-02"), got, tc.want)
		}
	}
}

// A directory with no default would broadcast nothing on most days, and would
// do it silently.
func TestAScheduleDirectoryWithoutADefaultIsRefused(t *testing.T) {
	dir := t.TempDir()
	body := `{"bouquet":"Sky","services":[{"name":"Sky One","channel":101,"serviceId":100,` +
		`"listingsId":101,"programmes":[{"start":"19:00","minutes":60,"title":"Dream Team"}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "1998-12-24.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := multiplex.LoadGuide(dir); err == nil {
		t.Fatal("accepted a schedule directory with no default.json")
	}
	// And a file that is neither a date nor the default is a typo, not a
	// schedule to be skipped quietly.
	if err := os.WriteFile(filepath.Join(dir, "default.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chistmas.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := multiplex.LoadGuide(dir); err == nil {
		t.Fatal("accepted a schedule file whose name is neither a date nor default.json")
	}
}

// One request covers a SET of channels. Reading the value and ignoring the mask
// is the bug this pins: with six channels loaded the box asks for 0x0BBF, which
// is nobody's listings id, and a transmitter that answered literally would send
// nothing at all and report no error.
func TestOneRequestCoversTheWholeSetItsMaskAdmits(t *testing.T) {
	request := multiplex.TitleRequest{Extension: 0x0bbf, ExtensionMask: 0xfff8}
	for _, id := range []uint16{0x0bb8, 0x0bb9, 0x0bba, 0x0bbb, 0x0bbc, 0x0bbd, 0x0bbe, 0x0bbf} {
		if !request.Wants(id) {
			t.Errorf("the filter %#04x/%#04x does not want %#04x, which it admits",
				request.Extension, request.ExtensionMask, id)
		}
	}
	for _, id := range []uint16{0x0bb7, 0x0bc0, 0x0000, 0xffff} {
		if request.Wants(id) {
			t.Errorf("the filter %#04x/%#04x wants %#04x, which it does not admit",
				request.Extension, request.ExtensionMask, id)
		}
	}
	// A zero mask matches everything if taken literally, which would put every
	// channel's programmes on a filter the box never asked about.
	open := multiplex.TitleRequest{Extension: 0, ExtensionMask: 0}
	if open.Wants(101) {
		t.Error("a request with no mask claims to want a channel; it must want nothing")
	}
}

// MJD is anchored on a fixed point rather than on a number someone produced:
// MJD 50000 is 10 October 1995.
func TestMJDIsAnchoredOnAFixedPoint(t *testing.T) {
	anchor := multiplex.DayOfMJD(50000)
	if got := anchor.Format("2006-01-02"); got != "1995-10-10" {
		t.Errorf("MJD 50000 is %s, want 1995-10-10", got)
	}
	if got := multiplex.MJDOf(anchor); got != 50000 {
		t.Errorf("round trip gave %d, want 50000", got)
	}
	if got := multiplex.MJDOf(multiplex.DayOfMJD(50979)); got != 50979 {
		t.Errorf("round trip of the demo day gave %d", got)
	}
}
