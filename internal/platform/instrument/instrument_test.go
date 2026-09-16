package instrument_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// TestAnAbsentFindingIsAHarnessFailureNotAZero is TC-1.8's third clause, and it is the assertion
// this package exists for. A plain map answers 0, and 0 reads as a measurement.
func TestAnAbsentFindingIsAHarnessFailureNotAZero(t *testing.T) {
	t.Parallel()

	c := instrument.NewCensus("register writes", "device registers")
	c.Declare("control", "status")
	c.Examine()
	if err := c.Record("control"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	n, err := c.Count("interrupt-mask") // never declared: a rename, or a typo
	if err == nil {
		t.Fatalf("Count of an undeclared finding returned %d and no error; that zero is indistinguishable from a measurement", n)
	}
	if n != 0 {
		t.Errorf("Count returned %d alongside its error; callers that ignore the error must not get a plausible number", n)
	}
	if !instrument.IsHarnessFailure(err) {
		t.Errorf("error %v is not a HarnessError; a gate cannot tell it apart from a finding", err)
	}
	if !errors.Is(err, instrument.ErrUnknownFinding) {
		t.Errorf("error %v does not wrap ErrUnknownFinding", err)
	}
	// The message has to say what was asked for and what exists, or the next reader assumes the
	// instrument is right and the code is wrong.
	for _, want := range []string{"interrupt-mask", "control", "status", "register writes", "device registers"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestADeclaredFindingWithNoObservationsIsARealZero is the other side of the rule. Declaring up
// front is what earns the right to report zero, and a package that refused every zero would just
// be a different kind of useless.
func TestADeclaredFindingWithNoObservationsIsARealZero(t *testing.T) {
	t.Parallel()

	c := instrument.NewCensus("unauthenticated routes", "registered http routes")
	c.Declare("unauthenticated")
	for i := 0; i < 12; i++ {
		c.Examine()
	}

	n, err := c.Count("unauthenticated")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Errorf("Count = %d, want 0", n)
	}

	res, err := c.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if res.Examined != 12 {
		t.Errorf("Examined = %d, want 12", res.Examined)
	}
	if res.Total() != 0 {
		t.Errorf("Total = %d, want 0", res.Total())
	}
}

// TestACensusThatExaminedNothingRefusesToReport is the vacuous-green failure: the route walk that
// found no routes and therefore reported no unauthenticated ones.
func TestACensusThatExaminedNothingRefusesToReport(t *testing.T) {
	t.Parallel()

	c := instrument.NewCensus("unauthenticated routes", "registered http routes")
	c.Declare("unauthenticated")
	// No Examine calls: the walk matched nothing at all.

	res, err := c.Result()
	if err == nil {
		t.Fatalf("Result succeeded over an empty population: %s", res)
	}
	if !instrument.IsHarnessFailure(err) {
		t.Errorf("error %v is not a HarnessError", err)
	}
	if !errors.Is(err, instrument.ErrNothingExamined) {
		t.Errorf("error %v does not wrap ErrNothingExamined", err)
	}
	if res.Examined != 0 || res.Findings != nil {
		t.Errorf("a refused Result handed back usable-looking data: %+v", res)
	}
}

func TestACensusThatDeclaredNothingRefusesToReport(t *testing.T) {
	t.Parallel()

	c := instrument.NewCensus("nothing in particular", "bytes")
	c.Examine()

	if _, err := c.Result(); !errors.Is(err, instrument.ErrNoFindings) {
		t.Errorf("error is %v, want it to wrap ErrNoFindings", err)
	}
}

func TestRecordingAnUndeclaredFindingIsRefused(t *testing.T) {
	t.Parallel()

	c := instrument.NewCensus("register writes", "device registers")
	c.Declare("control")
	c.Examine()

	err := c.Record("cotnrol") // the typo that becomes a tally nobody reads
	if err == nil {
		t.Fatal("Record accepted an undeclared finding; a typo must not create a second tally")
	}
	if !errors.Is(err, instrument.ErrUnknownFinding) {
		t.Errorf("error %v does not wrap ErrUnknownFinding", err)
	}

	// And the real finding is untouched, so the typo cannot also corrupt the number that matters.
	if n, err := c.Count("control"); err != nil || n != 0 {
		t.Errorf("control = %d, %v; want 0, nil", n, err)
	}
}

func TestCensusCounts(t *testing.T) {
	t.Parallel()

	c := instrument.NewCensus("section types", "transport stream sections")
	c.Declare("pat", "pmt", "eit")

	// Declaring twice must not reset a tally; instruments get assembled in pieces.
	c.Declare("pat")

	observed := []string{"pat", "pmt", "pmt", "eit", "eit", "eit"}
	for _, s := range observed {
		c.Examine()
		if err := c.Record(s); err != nil {
			t.Fatalf("Record(%q): %v", s, err)
		}
	}

	res, err := c.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	want := map[string]int{"pat": 1, "pmt": 2, "eit": 3}
	for f, n := range want {
		if res.Findings[f] != n {
			t.Errorf("%s = %d, want %d", f, res.Findings[f], n)
		}
	}
	if res.Examined != len(observed) {
		t.Errorf("Examined = %d, want %d", res.Examined, len(observed))
	}
	if res.Total() != len(observed) {
		t.Errorf("Total = %d, want %d", res.Total(), len(observed))
	}

	// The rendered form is what lands in a gate's output, so it has to carry the population size
	// as well as the tallies: tallies alone can be read as clean when nothing was walked.
	s := res.String()
	for _, want := range []string{"section types", "6", "transport stream sections", "eit=3"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, missing %q", s, want)
		}
	}
}

// TestResultIsASnapshotNotAView stops a caller's later Record calls from rewriting a result that
// has already been reported.
func TestResultIsASnapshotNotAView(t *testing.T) {
	t.Parallel()

	c := instrument.NewCensus("section types", "sections")
	c.Declare("pat")
	c.Examine()
	if err := c.Record("pat"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	res, err := c.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	c.Examine()
	if err := c.Record("pat"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if res.Findings["pat"] != 1 {
		t.Errorf("a reported result changed under the caller: pat = %d, want 1", res.Findings["pat"])
	}
}

func TestMustFind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		found      int
		want       int
		wantFailed bool
	}{
		{name: "found what it expected", found: 42, want: 1},
		{name: "found more than it expected", found: 42, want: 10},
		{name: "found nothing", found: 0, want: 1, wantFailed: true},
		{name: "found fewer than expected", found: 3, want: 10, wantFailed: true},
		{name: "expected none and found none is still a broken harness", found: 0, want: 0, wantFailed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := instrument.MustFind("symbol table", "firmware symbols", tt.found, tt.want)
			if tt.wantFailed {
				if err == nil {
					t.Fatalf("MustFind(%d, %d) = nil, want a harness failure", tt.found, tt.want)
				}
				if !instrument.IsHarnessFailure(err) {
					t.Errorf("error %v is not a HarnessError", err)
				}
				return
			}
			if err != nil {
				t.Errorf("MustFind(%d, %d) = %v, want nil", tt.found, tt.want, err)
			}
		})
	}
}

// TestAPlainErrorIsNotAHarnessFailure keeps IsHarnessFailure from degrading into "err != nil",
// which would make it useless for telling a broken instrument from a real finding.
func TestAPlainErrorIsNotAHarnessFailure(t *testing.T) {
	t.Parallel()

	if instrument.IsHarnessFailure(errors.New("the device returned an error")) {
		t.Error("IsHarnessFailure said yes to an ordinary error")
	}
	if instrument.IsHarnessFailure(nil) {
		t.Error("IsHarnessFailure said yes to nil")
	}
}
