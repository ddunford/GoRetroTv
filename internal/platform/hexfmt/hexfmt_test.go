package hexfmt_test

import (
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

func TestCanonicalForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"address with hex letters", hexfmt.Addr(0x80081C58), "0x80081C58"},
		{"address that is all digits", hexfmt.Addr(0x80000000), "0x80000000"},
		{"small value is padded to the full width", hexfmt.Addr(4), "0x00000004"},
		{"zero", hexfmt.Addr(0), "0x00000000"},
		{"top of the address space", hexfmt.Addr(0xFFFFFFFF), "0xFFFFFFFF"},
		{"word", hexfmt.Word(0xBFC00380), "0xBFC00380"},
		{"half", hexfmt.Half(0x1C58), "0x1C58"},
		{"half is padded", hexfmt.Half(0xB), "0x000B"},
		{"byte", hexfmt.Byte(0xAF), "0xAF"},
		{"byte is padded", hexfmt.Byte(5), "0x05"},
		{"range", hexfmt.Range(0x80000000, 0x81FFFFFF), "0x80000000..0x81FFFFFF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("= %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestEveryFormatterAgreesOnCasing is the assertion that the predecessor's two formatters would
// have failed. It does not check a particular casing so much as check that there is only one.
func TestEveryFormatterAgreesOnCasing(t *testing.T) {
	t.Parallel()

	produced := []string{
		hexfmt.Addr(0xABCDEF01),
		hexfmt.Word(0xABCDEF01),
		hexfmt.Half(0xABCD),
		hexfmt.Byte(0xAB),
	}
	for _, s := range produced {
		body := strings.TrimPrefix(s, "0x")
		if body != strings.ToUpper(body) {
			t.Errorf("%q has lower-case digits; every formatter in this package must agree", s)
		}
		if !strings.HasPrefix(s, "0x") {
			t.Errorf("%q does not carry the canonical 0x prefix", s)
		}
	}
}

// TestLookupByCanonicalKeyHitsAndByLowerCasedKeyMisses is TC-1.8's negative control, and it is the
// point of the whole package.
//
// The hit proves the key round-trips. The miss is the one that matters: it is the exact shape of
// the failure that reported zero for 0x80081C58 twice, and it is left in the suite permanently so
// that anyone who "helpfully" makes ParseAddr's leniency leak into Addr's output sees it go red.
func TestLookupByCanonicalKeyHitsAndByLowerCasedKeyMisses(t *testing.T) {
	t.Parallel()

	const subject = 0x80081C58

	// An instrument's lookup table, keyed the only way this project keys one.
	table := map[string]int{
		hexfmt.Addr(subject):    42,
		hexfmt.Addr(0x80000000): 7,
	}

	if got := table[hexfmt.Addr(subject)]; got != 42 {
		t.Fatalf("canonical key missed its own table: got %d, want 42", got)
	}

	// The negative control. A hand-written lower-cased key is a different string, so it misses,
	// and a miss on a map of counts reads as a genuine zero. Watch this assertion, not the one
	// above: if it ever stops failing to find the entry, the guard has gone.
	lowered := strings.ToLower(hexfmt.Addr(subject))
	if lowered == hexfmt.Addr(subject) {
		t.Fatalf("test is vacuous: %q has no letters to lower-case, so it cannot demonstrate the mismatch", lowered)
	}
	if _, found := table[lowered]; found {
		t.Errorf("lower-cased key %q found an entry; the casing mismatch this package prevents is no longer detectable", lowered)
	}

	// And the fix: text of unknown casing goes through NormalizeAddr, and then it hits.
	key, err := hexfmt.NormalizeAddr(lowered)
	if err != nil {
		t.Fatalf("NormalizeAddr(%q): %v", lowered, err)
	}
	if got, found := table[key]; !found || got != 42 {
		t.Errorf("normalized key %q: got %d, found %v; want 42, true", key, got, found)
	}
}

func TestParseAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    uint32
		wantErr bool
	}{
		{name: "canonical", in: "0x80081C58", want: 0x80081C58},
		{name: "lower case digits", in: "0x80081c58", want: 0x80081C58},
		{name: "upper case prefix", in: "0X80081C58", want: 0x80081C58},
		{name: "no prefix", in: "80081C58", want: 0x80081C58},
		{name: "surrounding space", in: "  0x80081C58\n", want: 0x80081C58},
		{name: "short form", in: "0x4", want: 4},
		{name: "empty", in: "", wantErr: true},
		{name: "prefix only", in: "0x", wantErr: true},
		{name: "not hex", in: "0xZZ", wantErr: true},
		{name: "wider than 32 bits", in: "0x1FFFFFFFF", wantErr: true},
		{name: "negative", in: "-0x4", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := hexfmt.ParseAddr(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseAddr(%q) = %s, want an error", tt.in, hexfmt.Addr(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddr(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseAddr(%q) = %s, want %s", tt.in, hexfmt.Addr(got), hexfmt.Addr(tt.want))
			}
		})
	}
}

func TestNormalizeAddrIsIdempotentAndCaseBlind(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"0x80081C58", "0x80081c58", "80081C58", "0X80081c58"} {
		got, err := hexfmt.NormalizeAddr(in)
		if err != nil {
			t.Fatalf("NormalizeAddr(%q): %v", in, err)
		}
		if got != "0x80081C58" {
			t.Errorf("NormalizeAddr(%q) = %q, want 0x80081C58", in, got)
		}
		again, err := hexfmt.NormalizeAddr(got)
		if err != nil {
			t.Fatalf("NormalizeAddr(%q): %v", got, err)
		}
		if again != got {
			t.Errorf("NormalizeAddr is not idempotent: %q -> %q", got, again)
		}
	}
}

func TestNormalizeAddrRefusesRubbish(t *testing.T) {
	t.Parallel()

	if got, err := hexfmt.NormalizeAddr("not an address"); err == nil {
		t.Errorf("NormalizeAddr(%q) = %q, want an error", "not an address", got)
	}
}
