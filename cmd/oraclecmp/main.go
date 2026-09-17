// Command oraclecmp compares two tier-1 checkpoint streams and says where they first disagree.
//
// The Go port's only independent check is that it matches the browser oracle instruction for
// instruction (SPEC FR-6), and comparing whole machines that often costs 1.8-59 GB per run (spike
// 002). So both sides emit a 32-bit state hash every N instructions and this compares the two:
// tier 1 finds the window, and tier 2 re-runs that one window with per-instruction tracing.
//
// # The exit codes, and why there are three
//
//	0  the two agree over every window they share
//	1  they disagree, and the output says where
//	2  the comparison could not be made
//
// Two and one are separate deliberately. "The machines disagreed" and "I could not tell whether
// they disagreed" are different answers, and a gate that treats the second as a pass reports
// agreement over a run that never happened. An empty stream, a stream cut short with no END line,
// two streams sampled at different intervals, or two that share no instruction - each of those
// exits 2, names itself a harness failure, and must never be read as "no divergence found".
//
// # What the oracle proves, and what it does not
//
// It proves this port matches the browser emulator. Where both are wrong in the same way they will
// agree, so an inherited error passes this check silently; only the measured record in
// docs/reference/digibox-emulation.md catches those.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ddunford/goretrotv/internal/platform/instrument"
	"github.com/ddunford/goretrotv/internal/platform/statehash"
	"github.com/ddunford/goretrotv/internal/version"
)

const (
	exitAgree   = 0
	exitDiverge = 1
	exitHarness = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// Writes to out and errOut discard their errors deliberately: this is a command whose whole
// output is a few lines to a terminal or a CI log, and there is nowhere to report a failure to
// write the report except the thing that failed.
func run(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("oraclecmp", flag.ContinueOnError)
	fs.SetOutput(errOut)
	nameA := fs.String("a", "", "label for the first stream (default: its file name)")
	nameB := fs.String("b", "", "label for the second stream (default: its file name)")
	showVersion := fs.Bool("version", false, "print the build and exit")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(errOut, "usage: oraclecmp [flags] <stream-a> <stream-b>\n\n"+
			"Compares two tier-1 checkpoint streams and reports the first window in which they\n"+
			"disagree. Exit 0 they agree, 1 they diverge, 2 the comparison could not be made.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitHarness
	}
	if *showVersion {
		_, _ = fmt.Fprintf(out, "oraclecmp %s (%s, %s)\n", version.Version, version.Commit, version.Date)
		return exitAgree
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return exitHarness
	}

	a, err := readStream(fs.Arg(0), *nameA)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "oraclecmp: %v\n", err)
		return exitHarness
	}
	b, err := readStream(fs.Arg(1), *nameB)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "oraclecmp: %v\n", err)
		return exitHarness
	}

	cmp, err := statehash.Compare(a, b)
	if err != nil {
		// A harness failure is labelled as one in the output as well as in the exit code, so a
		// person reading a CI log is told the difference and not only a machine reading $?.
		if instrument.IsHarnessFailure(err) {
			_, _ = fmt.Fprintf(errOut, "oraclecmp: HARNESS FAILURE - nothing was compared.\n  %v\n", err)
		} else {
			_, _ = fmt.Fprintf(errOut, "oraclecmp: %v\n", err)
		}
		return exitHarness
	}

	if cmp.Agreed() {
		_, _ = fmt.Fprintf(out, "AGREE  %s and %s %s\n", cmp.A, cmp.B, cmp)
		return exitAgree
	}

	_, _ = fmt.Fprintf(out, "DIVERGE  %s\n", cmp)
	_, _ = fmt.Fprintf(out, "  %d checkpoints agreed\n", cmp.Compared)
	if cmp.Cadence > 0 {
		_, _ = fmt.Fprintf(out, "  %d windows could not be compared: the two sampled them at "+
			"different instruction counts, so their hashes describe different instants\n", cmp.Cadence)
	}
	_, _ = fmt.Fprintf(out, "  %s reached instruction %d, %s reached %d\n",
		cmp.A, cmp.ReachedA, cmp.B, cmp.ReachedB)

	// A differing checkpoint describes the state AFTER the preceding instructions. Tracing the
	// numerically named window of that checkpoint starts too late and can miss the fault.
	// A cadence difference is an emitter issue; trace only when states differ at equal counts.
	lo, hi, hasStateDifference := cmp.Tier2Range()
	if !hasStateDifference {
		_, _ = fmt.Fprintf(out, "  the states agree everywhere both sampled alike, so this is a "+
			"difference between the two EMITTERS rather than between the two machines: they must "+
			"sample at the same instant, which means never inside a branch pair\n")
		return exitDiverge
	}
	_, _ = fmt.Fprintf(out, "  next: re-run instructions %d..%d on both sides with per-instruction "+
		"tracing (tier 2)\n", lo, hi)
	return exitDiverge
}

func readStream(path, label string) (*statehash.Stream, error) {
	if label == "" {
		label = filepath.Base(path)
	}
	f, err := os.Open(path) //#nosec G304 -- the operator names the streams to compare
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return statehash.ReadStream(label, f)
}
