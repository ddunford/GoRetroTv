package oracle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/oracle"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// oraclePath is the page itself. CLAUDE.md keeps it deliberately: it is the only independent check
// this port has, and it must not be deleted.
const oraclePath = "../../reference/digibox-boot.html"

func readOracle(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(oraclePath))
	if err != nil {
		t.Fatalf("read %s: %v", oraclePath, err)
	}
	return string(body)
}

// TestTheOraclePageDefinesEverythingItCalls is the gate, run against the real page on every push.
//
// The failure it exists to catch shipped to the demo host for an hour: window.__siGuideSlot was
// deleted while both of its call sites remained, and nothing noticed because a deleted assignment
// is not a syntax error.
func TestTheOraclePageDefinesEverythingItCalls(t *testing.T) {
	t.Parallel()

	if err := oracle.CheckSymbols(readOracle(t)); err != nil {
		t.Errorf("%v", err)
	}
}

// TestTheScanActuallyFindsTheOraclesDebugAPI asserts the subject before anything asserts a verdict
// about it. Without this, a regex that had stopped matching would make the test above pass.
func TestTheScanActuallyFindsTheOraclesDebugAPI(t *testing.T) {
	t.Parallel()

	syms := oracle.ScanSymbols(readOracle(t))
	t.Logf("oracle debug API: %d defined, %d called", len(syms.Defined), len(syms.Called))

	if err := instrument.MustFind("oracle scan", "window.__x definitions", len(syms.Defined), 20); err != nil {
		t.Fatalf("%v", err)
	}
	if err := instrument.MustFind("oracle scan", "window.__x calls", len(syms.Called), 5); err != nil {
		t.Fatalf("%v", err)
	}

	// Named symbols the record depends on. If the page stops defining these, the measured record
	// in docs/reference/digibox-emulation.md is describing something that no longer exists.
	for _, want := range []string{"__tasks", "__eeprom", "__key"} {
		if !contains(syms.Defined, want) {
			t.Errorf("the oracle no longer defines window.%s, which the measured record relies on", want)
		}
	}
}

// TestAPageThatCallsAnUndefinedSymbolIsRefused is the negative control, and it is the exact shape
// of the incident: a definition removed, its call sites left behind.
func TestAPageThatCallsAnUndefinedSymbolIsRefused(t *testing.T) {
	t.Parallel()

	page := syntheticPage(40, 10)
	// The deletion. The call site below survives it, and so does the parser.
	page = strings.Replace(page, "window.__sym7 = function () {};", "", 1)

	err := oracle.CheckSymbols(page)
	if err == nil {
		t.Fatal("a page calling a symbol it does not define was accepted; this is the __siGuideSlot failure")
	}
	if !strings.Contains(err.Error(), "__sym7") {
		t.Errorf("error %q does not name the undefined symbol", err)
	}
	if instrument.IsHarnessFailure(err) {
		t.Errorf("error %v is a harness failure; this page is well-formed and the finding is real", err)
	}
}

// TestAScanThatFindsNothingIsAHarnessFailureNotACleanPage is the rule this project keeps paying
// for. A pattern that matches nothing reports a clean page exactly as it reports one with no
// faults, and only the second is worth hearing.
func TestAScanThatFindsNothingIsAHarnessFailureNotACleanPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		page string
	}{
		{name: "an empty page", page: ""},
		{name: "a page with no debug API at all", page: "<html><body>nothing to see</body></html>"},
		{name: "a page whose API has nearly all gone", page: syntheticPage(3, 2)},
		// The case that distinguishes a broken scan from a catastrophic finding, and the one
		// that keeps the definitions assertion honest rather than shadowed by the calls one.
		// If the scan for DEFINITIONS breaks while the scan for CALLS still works, every call
		// looks undefined - which reads as ninety-seven real faults rather than as one broken
		// regex, and sends the reader to the page instead of to the instrument.
		{name: "definitions vanish while the calls remain", page: syntheticPage(3, 12)},
		// And the mirror image, which is the more dangerous of the two: if the scan for CALLS
		// breaks while definitions still scan, there is nothing left to be undefined and the
		// check reports a clean page having examined no calls at all. That is the vacuous green
		// this project keeps paying for, so it has to be a harness failure and not a pass.
		{name: "calls vanish while the definitions remain", page: syntheticPage(40, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := oracle.CheckSymbols(tt.page)
			if err == nil {
				t.Fatal("reported a clean page having found nothing to examine")
			}
			if !instrument.IsHarnessFailure(err) {
				t.Errorf("error %v is not a harness failure; a broken scan must not read as a finding", err)
			}
		})
	}
}

func TestScanSymbolsDeduplicatesAndSorts(t *testing.T) {
	t.Parallel()

	syms := oracle.ScanSymbols(`
		window.__b = function () {};
		window.__a = function () {};
		window.__a = function () {};
		window.__a(); window.__a(); window.__b();
	`)
	if got, want := strings.Join(syms.Defined, ","), "__a,__b"; got != want {
		t.Errorf("Defined = %q, want %q", got, want)
	}
	if got, want := strings.Join(syms.Called, ","), "__a,__b"; got != want {
		t.Errorf("Called = %q, want %q", got, want)
	}
	if got := syms.Undefined(); len(got) != 0 {
		t.Errorf("Undefined = %v, want none", got)
	}
}

// syntheticPage builds a page defining `defined` symbols and calling the first `called` of them.
func syntheticPage(defined, called int) string {
	var b strings.Builder
	for i := 0; i < defined; i++ {
		b.WriteString("window.__sym")
		b.WriteString(itoa(i))
		b.WriteString(" = function () {};\n")
	}
	for i := 0; i < called; i++ {
		b.WriteString("window.__sym")
		b.WriteString(itoa(i))
		b.WriteString("();\n")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
