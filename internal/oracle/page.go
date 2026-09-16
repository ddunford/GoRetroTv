// Package oracle holds checks on the browser emulator this port measures itself against.
//
// The oracle is reference/digibox-boot.html. It is a measuring instrument rather than a sibling
// implementation, and the checks here are about the page's own integrity - not about whether it
// agrees with the Go port, which is oraclecmp's job.
package oracle

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// The page exposes its debug API by assigning to window.__name, and uses it by calling
// window.__name(). Both patterns are structural - does a name that is called also exist - and
// deliberately not semantic. A pattern match is allowed to ask whether a symbol is present; it is
// not allowed to judge whether the symbol is the right one (CLAUDE.md -> Code Accuracy Rules).
var (
	definedPattern = regexp.MustCompile(`window\.(__\w+)\s*=`)
	calledPattern  = regexp.MustCompile(`window\.(__\w+)\s*\(`)
)

// Plausibility floors for the page's debug API.
//
// They are not a count of anything in particular; they exist so that a regex which has stopped
// matching cannot report a clean page. A page with three debug symbols is not a tidy page, it is a
// scan that has broken.
const (
	minDefined = 20
	minCalled  = 5
)

// Symbols is the debug API a page declares and the part of it the page uses.
type Symbols struct {
	// Defined are the window.__x names the page assigns.
	Defined []string
	// Called are the window.__x names the page invokes.
	Called []string
}

// ScanSymbols reads a page's debug API out of its source.
func ScanSymbols(src string) Symbols {
	return Symbols{
		Defined: uniqueMatches(definedPattern, src),
		Called:  uniqueMatches(calledPattern, src),
	}
}

// Undefined lists names the page calls without defining, sorted.
func (s Symbols) Undefined() []string {
	defined := make(map[string]bool, len(s.Defined))
	for _, name := range s.Defined {
		defined[name] = true
	}
	var missing []string
	for _, name := range s.Called {
		if !defined[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

// CheckSymbols reports whether every window.__x() the page calls is also defined by it.
//
// This is the cheap half of the predecessor's boot gate, and the only half that needs no browser.
// It exists because a patch deleted window.__siGuideSlot while leaving both of its call sites and
// nothing caught it: the file still parsed, because a deleted assignment is not a syntax error;
// the gate still passed, because the caller sits on a broadcast path that gate never drove; and
// the demo host served it for an hour. The box halted the moment it first opened a title PID.
//
// It asserts its own subject before asserting anything about it. A regex that has stopped matching
// reports a clean page exactly as it reports a page with no faults, and the second of those is the
// only one worth hearing. The full record is docs/reference/oracle-boot-gate.md.
func CheckSymbols(src string) error {
	syms := ScanSymbols(src)

	if err := instrument.MustFind("oracle debug API", "window.__x definitions", len(syms.Defined), minDefined); err != nil {
		return fmt.Errorf("scanning the oracle page: %w", err)
	}
	if err := instrument.MustFind("oracle debug API", "window.__x calls", len(syms.Called), minCalled); err != nil {
		return fmt.Errorf("scanning the oracle page: %w", err)
	}

	if missing := syms.Undefined(); len(missing) > 0 {
		return fmt.Errorf("the oracle page calls %v without defining %s; the page still parses and "+
			"fails only when that path is first taken",
			missing, plural(len(missing), "it", "them"))
	}
	return nil
}

func uniqueMatches(re *regexp.Regexp, src string) []string {
	matches := re.FindAllStringSubmatch(src, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
