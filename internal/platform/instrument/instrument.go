// Package instrument makes the subject-assertion rule mechanical.
//
// The rule: an instrument that cannot find what it counts reports a harness failure, never zero.
// It has been a project convention in prose before, and prose lost. Three agents broke it in one
// afternoon upstream, and an earlier phase shipped eight instruments that reported clean while
// examining nothing at all — a route walk that found no routes and so satisfied "no unauthenticated
// route" vacuously, a census whose lookup key was mistyped and answered zero for ever. Every one of
// them was green, loudly. A green that means nothing is worse than a red, because a red gets
// investigated.
//
// Two failure modes get their own guard, because they are genuinely different:
//
// An empty population. A census that examined nothing knows nothing, and its tallies are all zero
// for a reason that has nothing to do with the thing being measured. Result refuses to report.
//
// An undeclared finding. Asking for a tally by a name nobody registered is a typo, a rename or a
// lookup against the wrong key — the failure that reported zero for 0x80081C58. Count returns a
// harness failure rather than the zero value a map would hand back.
//
// A declared finding with a tally of zero is left alone: that one is a real measurement, and the
// whole point of declaring findings up front is to make "we looked and found none" distinguishable
// from "we never looked".
package instrument

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Sentinel causes, matched with errors.Is. Each is wrapped in a HarnessError carrying the detail
// that says which instrument and which subject.
var (
	// ErrNothingExamined means the census never saw a member of its population.
	ErrNothingExamined = errors.New("instrument examined nothing")
	// ErrUnknownFinding means a finding was asked for, or recorded, without being declared.
	ErrUnknownFinding = errors.New("finding was never declared")
	// ErrNoFindings means the census declared nothing to count.
	ErrNoFindings = errors.New("instrument declared no findings")
)

// HarnessError says the instrument is broken, not that the measurement came out low.
//
// It is a distinct type so a caller can tell the two apart without string matching: a gate that
// treats a harness failure as a finding of zero is the failure this package exists to prevent, and
// IsHarnessFailure is how a gate avoids it.
type HarnessError struct {
	// Instrument is what was being measured.
	Instrument string
	// Subject is the population being examined.
	Subject string
	// Detail says what specifically went wrong.
	Detail string
	// Err is the sentinel cause.
	Err error
}

func (e *HarnessError) Error() string {
	return fmt.Sprintf("instrument %q over %q: %s: %s", e.Instrument, e.Subject, e.Err, e.Detail)
}

// Unwrap exposes the sentinel cause to errors.Is.
func (e *HarnessError) Unwrap() error { return e.Err }

// IsHarnessFailure reports whether err says the instrument was broken.
func IsHarnessFailure(err error) bool {
	var h *HarnessError
	return errors.As(err, &h)
}

// Census counts declared findings across a population it must first prove it examined.
//
// It is not safe for concurrent use; nothing in this project's instruction loop is (CLAUDE.md ->
// No goroutine in the instruction loop). An instrument that needs to gather across goroutines
// should collect first and record afterwards.
type Census struct {
	name     string
	subject  string
	examined int
	declared []string
	counts   map[string]int
}

// NewCensus starts a census called name over a population of subject.
//
// name is what is being measured ("unauthenticated routes"); subject is what is being walked
// ("registered http routes"). Both appear in every failure, because a harness failure whose
// message does not say what it was looking at is nearly as unhelpful as the silent zero.
func NewCensus(name, subject string) *Census {
	return &Census{name: name, subject: subject, counts: make(map[string]int)}
}

// Declare registers findings this census can report. Declaring up front is what makes a tally of
// zero meaningful: it says somebody decided this was worth counting before the answer was known.
func (c *Census) Declare(findings ...string) {
	for _, f := range findings {
		if _, ok := c.counts[f]; ok {
			continue
		}
		c.counts[f] = 0
		c.declared = append(c.declared, f)
	}
}

// Examine records that one member of the population was inspected.
//
// Call it for every member walked, including the ones that yield no finding. This is the count
// that turns "we looked and found none" into a statement anyone can check.
func (c *Census) Examine() { c.examined++ }

// Examined reports how many members of the population were inspected.
func (c *Census) Examined() int { return c.examined }

// Record increments a declared finding.
//
// An undeclared finding is a harness failure rather than a new map entry: silently accepting it is
// how a typo becomes a second tally that nobody ever reads.
func (c *Census) Record(finding string) error {
	if _, ok := c.counts[finding]; !ok {
		return c.harnessErr(ErrUnknownFinding,
			fmt.Sprintf("Record(%q); declared: %s", finding, c.declaredList()))
	}
	c.counts[finding]++
	return nil
}

// Count returns a declared finding's tally.
//
// An undeclared finding returns a harness failure, never zero. That refusal is the whole package
// in one method: a map would have answered 0, and 0 is indistinguishable from an answer.
func (c *Census) Count(finding string) (int, error) {
	n, ok := c.counts[finding]
	if !ok {
		return 0, c.harnessErr(ErrUnknownFinding,
			fmt.Sprintf("Count(%q); declared: %s", finding, c.declaredList()))
	}
	return n, nil
}

// Result is a completed census: what was examined, and what was found.
type Result struct {
	// Name is what was measured.
	Name string
	// Subject is the population that was walked.
	Subject string
	// Examined is how many members of that population were inspected.
	Examined int
	// Findings maps each declared finding to its tally. A zero here is a real zero.
	Findings map[string]int
}

// Total sums every finding.
func (r Result) Total() int {
	total := 0
	for _, n := range r.Findings {
		total += n
	}
	return total
}

// String renders the result for a log line or a gate's output.
func (r Result) String() string {
	names := make([]string, 0, len(r.Findings))
	for f := range r.Findings {
		names = append(names, f)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "%s: examined %d %s", r.Name, r.Examined, r.Subject)
	for _, f := range names {
		fmt.Fprintf(&b, "; %s=%d", f, r.Findings[f])
	}
	return b.String()
}

// Result closes the census and returns its tallies, or a harness failure.
//
// It refuses on an empty population and on a census with nothing declared. Both are instruments
// that would otherwise report a clean, confident, meaningless zero.
func (c *Census) Result() (Result, error) {
	if len(c.declared) == 0 {
		return Result{}, c.harnessErr(ErrNoFindings, "nothing was declared, so every answer would be an absence")
	}
	if c.examined == 0 {
		return Result{}, c.harnessErr(ErrNothingExamined,
			fmt.Sprintf("no %s were examined, so the tallies below are absences rather than zeroes: %s",
				c.subject, c.declaredList()))
	}

	findings := make(map[string]int, len(c.counts))
	for f, n := range c.counts {
		findings[f] = n
	}
	return Result{Name: c.name, Subject: c.subject, Examined: c.examined, Findings: findings}, nil
}

func (c *Census) declaredList() string {
	if len(c.declared) == 0 {
		return "(none)"
	}
	sorted := append([]string(nil), c.declared...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

func (c *Census) harnessErr(cause error, detail string) error {
	return &HarnessError{Instrument: c.name, Subject: c.subject, Detail: detail, Err: cause}
}

// MustFind reports a harness failure when want members of a population were expected and n were
// found, for instruments that know in advance roughly what they are walking.
//
// Use it for the corpus check an instrument should do before it measures anything: a firmware
// image with no symbols, a device table with no devices, a trace with no instructions. Asserting
// the subject exists before asserting anything about it is the cheap half of this discipline.
func MustFind(name, subject string, n, want int) error {
	if n >= want && n > 0 {
		return nil
	}
	return &HarnessError{
		Instrument: name,
		Subject:    subject,
		Detail:     fmt.Sprintf("found %d, expected at least %d", n, want),
		Err:        ErrNothingExamined,
	}
}
