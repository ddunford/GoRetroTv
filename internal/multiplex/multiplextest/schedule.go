// Package multiplextest holds fixtures shared by the multiplex package's own
// tests and by the firmware tests that were split out of it.
//
// It exists for one helper and that is reason enough: Schedule's programmes are
// deliberately in the EVENING because the firmware test that shares it runs a
// 19:00 clock and the box only registers the six-hour block its clock is in. A
// copy of this in each package would drift, and the drift would present as an
// edit that never arrived rather than as a fixture that disagreed.
package multiplextest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Schedule writes a one-channel schedule whose programme count says which
// version of the file it is, so a reload is visible in the content rather than
// only in a return value.
func Schedule(t *testing.T, dir, name string, programmes int) {
	t.Helper()
	var body strings.Builder
	body.WriteString(`{"bouquet":"Sky Digital","services":[{"name":"Sky One","channel":101,` +
		`"serviceId":100,"listingsId":101,"programmes":[`)
	for i := 0; i < programmes; i++ {
		if i > 0 {
			body.WriteString(",")
		}
		// EVENING programmes, because the firmware test that shares this
		// fixture runs a 19:00 clock and the box registers the six-hour block
		// its clock is in. A morning schedule is broadcast, parsed and
		// discarded, which looks exactly like an edit that never arrived.
		body.WriteString(`{"start":"` + [...]string{"18", "19", "20", "21", "22", "23"}[i%6] +
			`:00","minutes":60,"title":"Programme"}`)
	}
	body.WriteString(`]}]}`)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}
