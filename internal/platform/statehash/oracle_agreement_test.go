package statehash

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// oraclePage is the browser implementation this hash exists to be compared against.
const oraclePage = "../../../reference/digibox-boot.html"

// The oracle carries its own copy of this hash, because it has to: it is a browser page and
// cannot call Go. Two implementations of one function is a standing invitation to drift, and the
// drift would not announce itself - it would arrive at the far end as a divergence at checkpoint
// zero, which reads like a CPU fault rather than like a broken instrument.
//
// So this reads the oracle's self-test expectations out of the page and checks them against what
// this package actually computes. It needs no browser, so it runs wherever the tests run; driving
// the page and calling __cpVectors() is a stronger check and belongs to the boot gate, but a check
// that only runs when someone remembers to open a browser is not the one that catches this.
//
// It is structural on purpose - it reads literals, it does not evaluate JavaScript - and it
// asserts its own subject at every step: a page it cannot find, or a self-test it cannot locate
// inside the page, is a harness failure rather than a pass.
func TestTheOracleAgreesWithThisHash(t *testing.T) {
	t.Parallel()

	page, err := os.ReadFile(oraclePage)
	if err != nil {
		t.Fatalf("harness failure: the oracle is the only independent check this port has and "+
			"it could not be read at %s: %v", oraclePage, err)
	}
	src := string(page)

	t.Run("it uses the same constants", func(t *testing.T) {
		t.Parallel()
		for _, want := range []struct {
			what    string
			literal string
		}{
			{"the FNV offset basis", strconv.FormatUint(uint64(fnvOffset), 10)},
			{"the FNV prime", strconv.FormatUint(uint64(fnvPrime), 10)},
			{"Math.imul, because JavaScript's multiply goes through a double and loses the low " +
				"bits silently", "Math.imul"},
		} {
			if !strings.Contains(src, want.literal) {
				t.Errorf("the oracle does not mention %s (%q)", want.what, want.literal)
			}
		}
	})

	// The three values the page checks itself against, read out of __cpVectors's own verdict.
	t.Run("its self-test expects what this package computes", func(t *testing.T) {
		t.Parallel()

		zero := make([]byte, 4096)
		counting := make([]byte, 4096)
		for i := range counting {
			counting[i] = byte(i)
		}
		// The whole-machine value is computed by running this package's real Hash over a known
		// machine, and the oracle's by running ITS real hash over the same one. Pinning only the
		// primitives left the COMPOSITION unpinned: a change to the field order, or to how the
		// ISA bit is encoded, passed both sides' checks and would have surfaced as a divergence
		// at checkpoint zero the first time anyone ran a real comparison.
		h, err := New(&fixedRAM{pages: 4, len: 4096})
		if err != nil {
			t.Fatalf("harness failure: %v", err)
		}
		machine := State{PC: 0x80081C58, ISA: 1, HI: 0x0000DEAD, LO: 0x0000BEEF}
		for i := range machine.GPR {
			machine.GPR[i] = 0x10000000 + uint32(i)*0x11
			machine.COP0[i] = 0x20000000 + uint32(i)*0x13
		}
		wholeMachine := h.Hash(machine)

		want := map[string]uint32{
			"mixWord_offset_80081C58":   mixWord(fnvOffset, 0x80081C58),
			"pageDigest_0_zeros":        pageDigest(0, zero),
			"pageDigest_3_counting":     pageDigest(3, counting),
			"ramDigest_four_zero_pages": h.RAMDigest(),
			"wholeMachine":              wholeMachine,
		}

		found := 0
		for name, value := range want {
			// e.g.  out.pageDigest_0_zeros      === "0x9B5D3515"
			re := regexp.MustCompile(`out\.` + regexp.QuoteMeta(name) + `\s*===\s*"(0x[0-9A-Fa-f]{8})"`)
			m := re.FindStringSubmatch(src)
			if m == nil {
				t.Errorf("harness failure: the oracle's self-test has no expectation for %s, so "+
					"this check is not examining it", name)
				continue
			}
			found++
			// Through hexfmt, not strings.ToUpper. The first version of this line upper-cased
			// the whole literal including its "0x" and reported drift between two values that
			// were identical - which is the casing mismatch hexfmt exists to stop, committed
			// by the test written to catch a different one.
			got, err := hexfmt.NormalizeAddr(m[1])
			if err != nil {
				t.Errorf("harness failure: the oracle's expectation for %s is %q, which is not "+
					"an address: %v", name, m[1], err)
				continue
			}
			if want := hexfmt.Word(value); got != want {
				t.Errorf("%s: the oracle expects %s and this package computes %s - the two "+
					"implementations have drifted, and a checkpoint comparison between them "+
					"would report a divergence at instruction zero", name, got, want)
			}
		}
		if found != len(want) {
			t.Fatalf("harness failure: found %d of %d expectations in the oracle's self-test",
				found, len(want))
		}
	})

	// The emitter must stay behind its flag. Every probe ever written against this page measured
	// some particular state, and a default that moved underneath them would invalidate those
	// measurements with nothing looking wrong.
	t.Run("its emitter stays behind a URL flag", func(t *testing.T) {
		t.Parallel()
		for _, want := range []string{`q.get("cp")`, `if(cpOn && icount >= cpNext) cpEmit();`, `if(cpOn){`} {
			if !strings.Contains(src, want) {
				t.Errorf("harness failure: %q is not in the oracle, so this check cannot tell "+
					"whether the emitter is still switched off by default", want)
			}
		}
	})

	t.Run("it writes the same stream format", func(t *testing.T) {
		t.Parallel()
		header := fmt.Sprintf(`"%s %d interval="`, StreamMagic, StreamVersion)
		if !strings.Contains(src, header) {
			t.Errorf("the oracle's stream header is not %s - two streams that disagree about "+
				"their own format cannot be compared", header)
		}
		if !strings.Contains(src, `"`+StreamEnd+` "`) {
			t.Errorf("the oracle does not write an %s trailer, which is what makes a truncated "+
				"stream distinguishable from a short one", StreamEnd)
		}
	})
}
