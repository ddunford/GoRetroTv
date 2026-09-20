package multiplex_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/multiplex"
)

// schedule writes a one-channel schedule whose programme count says which
// version of the file it is, so a reload is visible in the content rather than
// only in a return value.
func schedule(t *testing.T, dir, name string, programmes int) {
	t.Helper()
	var body strings.Builder
	body.WriteString(`{"bouquet":"Sky Digital","services":[{"name":"Sky One","channel":101,` +
		`"serviceId":100,"listingsId":101,"programmes":[`)
	for i := 0; i < programmes; i++ {
		if i > 0 {
			body.WriteString(",")
		}
		body.WriteString(`{"start":"` + [...]string{"06", "07", "08", "09", "10", "11"}[i%6] +
			`:00","minutes":60,"title":"Programme"}`)
	}
	body.WriteString(`]}]}`)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TC-6.8. An edit reaches the air; a malformed edit does not, and the last
// good schedule stays on.
func TestAnEditIsPickedUpAndAMalformedOneKeepsTheLastGood(t *testing.T) {
	dir := t.TempDir()
	schedule(t, dir, "default.json", 3)
	guide, err := multiplex.LoadGuide(dir)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	if got := len(guide.On(day).Services[0].Programmes); got != 3 {
		t.Fatalf("loaded %d programmes, want 3", got)
	}

	// Unchanged bytes are not a reload. This runs on the carousel's cadence,
	// so it happens many times for every real edit, and re-parsing each time
	// would bump the SI version and make the box re-acquire for nothing.
	if changed, err := guide.Reload(); err != nil || changed {
		t.Fatalf("an untouched schedule reported changed=%v err=%v", changed, err)
	}

	// A real edit.
	schedule(t, dir, "default.json", 5)
	changed, err := guide.Reload()
	if err != nil || !changed {
		t.Fatalf("an edited schedule reported changed=%v err=%v", changed, err)
	}
	if got := len(guide.On(day).Services[0].Programmes); got != 5 {
		t.Errorf("after the edit the guide has %d programmes, want 5", got)
	}

	// A malformed edit: the last good schedule stays on air and the error
	// names the file.
	if err := os.WriteFile(filepath.Join(dir, "default.json"), []byte(`{"bouquet":`), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err = guide.Reload()
	if err == nil {
		t.Fatal("a truncated schedule was accepted")
	}
	if changed {
		t.Error("a truncated schedule reported that it changed the line-up")
	}
	if got := len(guide.On(day).Services[0].Programmes); got != 5 {
		t.Errorf("the broken edit left %d programmes on air, want the 5 from the last good schedule", got)
	}

	// And a broken file stays retryable: saving a fix takes effect without
	// anything being restarted, which is only true because the failed reload
	// did not record the broken bytes as the ones it had loaded.
	schedule(t, dir, "default.json", 2)
	changed, err = guide.Reload()
	if err != nil || !changed {
		t.Fatalf("the repaired schedule reported changed=%v err=%v", changed, err)
	}
	if got := len(guide.On(day).Services[0].Programmes); got != 2 {
		t.Errorf("after the repair the guide has %d programmes, want 2", got)
	}
}

// A schedule that is semantically wrong rather than unparseable is refused the
// same way. Two channels sharing a listings id would file one's programmes
// under the other, which broadcasts plausibly and wrongly.
func TestAScheduleThatParsesButIsWrongAlsoKeepsTheLastGood(t *testing.T) {
	dir := t.TempDir()
	schedule(t, dir, "default.json", 3)
	guide, err := multiplex.LoadGuide(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"bouquet":"Sky","services":[` +
		`{"name":"Sky One","channel":101,"serviceId":100,"listingsId":101,` +
		`"programmes":[{"start":"06:00","minutes":60,"title":"A"}]},` +
		`{"name":"Sky Two","channel":102,"serviceId":101,"listingsId":101,` +
		`"programmes":[{"start":"07:00","minutes":60,"title":"B"}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "default.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := guide.Reload(); err == nil {
		t.Fatal("two channels sharing a listings id were accepted")
	}
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	if got := len(guide.On(day).Services); got != 1 {
		t.Errorf("the refused edit changed the line-up to %d channels", got)
	}
}

// A new dated file appearing is an edit too -- it is how a real listings page
// gets added for the day it was printed for, and it must not need a restart.
func TestADatedFileAddedWhileRunningIsPickedUp(t *testing.T) {
	dir := t.TempDir()
	schedule(t, dir, "default.json", 3)
	guide, err := multiplex.LoadGuide(dir)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(1998, 12, 24, 19, 0, 0, 0, time.UTC)
	if got := len(guide.On(day).Services[0].Programmes); got != 3 {
		t.Fatalf("started with %d programmes, want the default's 3", got)
	}
	schedule(t, dir, "1998-12-24.json", 6)
	if changed, err := guide.Reload(); err != nil || !changed {
		t.Fatalf("a new dated file reported changed=%v err=%v", changed, err)
	}
	if got := len(guide.On(day).Services[0].Programmes); got != 6 {
		t.Errorf("Christmas Eve has %d programmes, want the new file's 6", got)
	}
	other := time.Date(1998, 12, 25, 19, 0, 0, 0, time.UTC)
	if got := len(guide.On(other).Services[0].Programmes); got != 3 {
		t.Errorf("Boxing Day has %d programmes, want the default's 3", got)
	}
}
