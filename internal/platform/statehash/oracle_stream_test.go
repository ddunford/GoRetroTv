package statehash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/instrument"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
)

// oracleColdBoot is a checkpoint stream from a REAL cold boot of the browser oracle running the
// real firmware, captured through a browser with ?cp=100000 and the broadcast silenced.
//
// It is committed because everything this package knows about the oracle otherwise came from a
// measurement somebody made by hand, and a measurement nothing re-runs cannot catch a regression.
// With it, the reader and the comparison are exercised against the bytes the other implementation
// actually produces rather than against fixtures shaped by the same mind that wrote the parser.
//
// It also has a second life: when the Go emulator exists (phase 2), this is the stream its own
// output gets compared against, and the comparison is `oraclecmp go.stream <this file>`.
const oracleColdBoot = "testdata/oracle-cold-boot.stream"

func realOracleStream(t *testing.T) (string, *statehash.Stream) {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(oracleColdBoot))
	if err != nil {
		t.Fatalf("harness failure: the recorded oracle stream is the subject of this test and it "+
			"could not be read: %v", err)
	}
	s, err := statehash.ReadStream("oracle", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("the reader cannot read what the oracle actually writes: %v", err)
	}
	return string(body), s
}

// The fixture must be a real boot, not a fragment that happens to parse. Every assertion below is
// about it being big enough and far enough along to mean something: agreement over a few thousand
// instructions is the easiest vacuous pass in this project, because two implementations agree
// trivially before anything has happened.
func TestTheRecordedOracleStreamIsARealBoot(t *testing.T) {
	t.Parallel()
	_, s := realOracleStream(t)

	if !s.Complete {
		t.Fatal("the recorded stream has no END line, so it was cut short and nothing may be " +
			"concluded from it")
	}
	if err := s.Usable(); err != nil {
		t.Fatalf("the recorded stream is not usable: %v", err)
	}
	if s.Interval != 100000 {
		t.Fatalf("the recorded stream's interval is %d, want 100000", s.Interval)
	}
	if n := len(s.Checkpoints); n < 4000 {
		t.Fatalf("the recorded stream has %d checkpoints; a boot of this firmware is roughly "+
			"4,500 at this interval, so this is not a whole one", n)
	}
	// The record puts a cold boot at about 447 million instructions.
	if reached := s.Checkpoints[len(s.Checkpoints)-1].ICount; reached < 440_000_000 {
		t.Fatalf("the recorded stream reaches instruction %d; a cold boot is about 447,000,000, "+
			"so this stopped somewhere in the middle of one", reached)
	}
	// The machine at reset: DRAM zeroed, registers zeroed, PC at the reset vector. If this
	// changes, either the oracle's hash changed or its reset did, and both are worth knowing.
	first := s.Checkpoints[0]
	if first.ICount != 0 {
		t.Fatalf("the first checkpoint is at instruction %d, want 0", first.ICount)
	}
	if first.Hash != 0xF1F29240 {
		t.Fatalf("the oracle's state at reset hashes to 0x%08X and this stream recorded "+
			"0xF1F29240; the oracle's hash or its reset has changed", first.Hash)
	}
}

// TC-1.6 against the bytes the other implementation really emits, rather than against fixtures
// this package wrote for itself.
func TestTheComparisonWorksOnARealOracleStream(t *testing.T) {
	t.Parallel()
	body, s := realOracleStream(t)

	t.Run("it agrees with itself, over the whole boot", func(t *testing.T) {
		t.Parallel()
		_, other := realOracleStream(t)
		got, err := statehash.Compare(s, other)
		if err != nil {
			t.Fatalf("Compare: %v", err)
		}
		if !got.Agreed() {
			t.Fatalf("a stream disagrees with itself: %s", got)
		}
		if got.Compared != len(s.Checkpoints) {
			t.Fatalf("it compared %d of %d checkpoints; an agreement over part of a boot is an "+
				"agreement about less than the run", got.Compared, len(s.Checkpoints))
		}
	})

	t.Run("a single corrupted checkpoint is localised", func(t *testing.T) {
		t.Parallel()
		// Instruction 4,500,000 is TC-1.6's, and at this stream's interval it is window 45.
		const at = "4500000 "
		if !strings.Contains(body, "\n"+at) {
			t.Fatalf("harness failure: the recorded stream has no checkpoint at instruction %s, "+
				"so this test would be corrupting nothing", strings.TrimSpace(at))
		}
		i := strings.Index(body, "\n"+at) + 1
		j := i + strings.Index(body[i:], "\n")
		corrupted := body[:i] + at + "0xDEADBEEF" + body[j:]

		other, err := statehash.ReadStream("port", strings.NewReader(corrupted))
		if err != nil {
			t.Fatalf("ReadStream: %v", err)
		}
		got, err := statehash.Compare(s, other)
		if err != nil {
			t.Fatalf("Compare: %v", err)
		}
		if got.Kind != statehash.StateDiverged {
			t.Fatalf("got %q, want a state divergence: %s", got.Kind, got)
		}
		if got.Lo != 4_500_000 || got.Hi != 4_599_999 {
			t.Fatalf("localised to instructions %d..%d, want 4500000..4599999", got.Lo, got.Hi)
		}
		// Every window of the boot agreed except the one corrupted, so the comparison walked
		// the whole run rather than stopping at the first thing it found. A tool that reported
		// the right window having examined forty-five checkpoints of a 4,629-checkpoint boot
		// would satisfy a looser assertion.
		if want := len(s.Checkpoints) - 1; got.Compared != want {
			t.Fatalf("%d windows agreed, want %d - one short of the whole boot", got.Compared, want)
		}
		if got.Cadence != 0 {
			t.Fatalf("%d windows were not comparable; a stream against a copy of itself samples "+
				"every window at the same instruction count", got.Cadence)
		}
		t.Logf("caught: %s", got)
	})

	t.Run("the same stream cut short is a harness failure", func(t *testing.T) {
		t.Parallel()
		cut := body[:strings.LastIndex(strings.TrimRight(body, "\n"), "\n")+1]
		if strings.Contains(cut, statehash.StreamEnd) {
			t.Fatal("harness failure: the trimmed stream still has its END line")
		}
		other, err := statehash.ReadStream("port", strings.NewReader(cut))
		if err != nil {
			t.Fatalf("ReadStream: %v", err)
		}
		_, err = statehash.Compare(s, other)
		if err == nil {
			t.Fatal("a real stream that stopped must be refused, not reported as agreement over " +
				"the part that arrived")
		}
		if !instrument.IsHarnessFailure(err) {
			t.Fatalf("want a harness failure, got: %v", err)
		}
		t.Logf("caught: %v", err)
	})
}
