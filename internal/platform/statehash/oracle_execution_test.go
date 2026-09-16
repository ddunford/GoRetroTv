package statehash

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
)

// TestTheOracleComputesTheSameHash RUNS the oracle's hash rather than reading what it claims.
//
// # Why this exists, and why the test beside it was not enough
//
// TestTheOracleAgreesWithThisHash compares the LITERALS the page's self-test expects against what
// this package computes. That catches someone editing the expected values. It does not catch
// someone editing the algorithm: swapping HI and LO in the page's hash, folding COP0 before the
// general registers, encoding the ISA bit as a word instead of an octet, or leaving DRAM out
// entirely all leave those literals untouched. Each of those was tried against that test and each
// one PASSED it - the page's own self-test would have caught them at runtime, but nothing
// committed runs the page.
//
// A divergence of that kind does not announce itself as a broken instrument. It arrives at the far
// end as a mismatch at checkpoint zero, which reads exactly like a CPU fault, and the reader goes
// looking in the decoder.
//
// So this lifts the page's hash out of the HTML and executes it, against a machine whose every
// field is different from every other, and compares the answer with this package's. It is the
// composition that gets pinned, not just the arithmetic.
func TestTheOracleComputesTheSameHash(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("NOT RUN: node is not on this machine (%v), so the oracle's own JavaScript could "+
			"not be executed. The literal comparison in TestTheOracleAgreesWithThisHash still "+
			"runs, but it cannot see a change to the page's field ORDER - which is the failure "+
			"this test exists for.", err)
	}

	page, err := os.ReadFile(oraclePage)
	if err != nil {
		t.Fatalf("harness failure: could not read the oracle at %s: %v", oraclePage, err)
	}

	// The hash machinery is one contiguous block in the page, between the constant that opens it
	// and the reset function that follows it. Taking it whole rather than function by function is
	// deliberate: an extractor that picked out named functions would silently stop covering one
	// the day it was renamed.
	const (
		from = "var CP_PAGE = 4096;"
		to   = "  function cpReset(){"
	)
	src := string(page)
	i, j := strings.Index(src, from), strings.Index(src, to)
	if i < 0 || j < 0 || j <= i {
		t.Fatalf("harness failure: could not find the oracle's hash between %q and %q, so this "+
			"test is not examining it", from, to)
	}
	block := src[i:j]
	for _, must := range []string{"cpMixOctet", "cpMixWord", "cpPageDigest", "cpHashOf", "Math.imul"} {
		if !strings.Contains(block, must) {
			t.Fatalf("harness failure: the extracted block has no %s in it", must)
		}
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "oracle-hash.mjs")
	if err := os.WriteFile(script, []byte(harness(block)), 0o600); err != nil {
		t.Fatalf("writing the harness: %v", err)
	}

	out, err := exec.Command(node, script).CombinedOutput() //#nosec G204 -- node and a file this test just wrote
	if err != nil {
		t.Fatalf("running the oracle's hash under node: %v\n%s", err, out)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("harness failure: the oracle harness printed %q, which is not the result: %v",
			out, err)
	}

	// The same machine, on this side.
	h, err := New(&fixedRAM{pages: 4, len: 4096})
	if err != nil {
		t.Fatalf("harness failure: %v", err)
	}
	machine := State{PC: 0x80081C58, ISA: 1, HI: 0x0000DEAD, LO: 0x0000BEEF}
	for k := range machine.GPR {
		machine.GPR[k] = 0x10000000 + uint32(k)*0x11
		machine.COP0[k] = 0x20000000 + uint32(k)*0x13
	}
	counting := make([]byte, 4096)
	for k := range counting {
		counting[k] = byte(k)
	}

	want := map[string]uint32{
		"mixWord":      mixWord(fnvOffset, 0x80081C58),
		"pageDigest0":  pageDigest(0, make([]byte, 4096)),
		"pageDigest3":  pageDigest(3, counting),
		"ramDigest":    h.RAMDigest(),
		"wholeMachine": h.Hash(machine),
	}
	if len(got) != len(want) {
		t.Fatalf("harness failure: the oracle harness reported %d values and this test compares "+
			"%d: %v", len(got), len(want), got)
	}
	for name, expected := range want {
		raw, ok := got[name]
		if !ok {
			t.Errorf("harness failure: the oracle harness reported nothing for %s", name)
			continue
		}
		normalised, err := hexfmt.NormalizeAddr(raw)
		if err != nil {
			t.Errorf("harness failure: the oracle reported %q for %s: %v", raw, name, err)
			continue
		}
		if normalised != hexfmt.Word(expected) {
			t.Errorf("%s: the oracle computes %s and this package computes %s - the two "+
				"implementations of the state hash have diverged, and a checkpoint comparison "+
				"between them would disagree at instruction zero",
				name, normalised, hexfmt.Word(expected))
		}
	}
}

// harness wraps the page's hash block in the globals it expects and prints the vectors.
//
// Everything the block reaches for that lives elsewhere on the page is declared here and nowhere
// else, so a block that grew a new dependency fails loudly under node rather than quietly reading
// an undefined.
func harness(block string) string {
	return fmt.Sprintf(`"use strict";
var ram = null, pc = 0, isa = 0, hi = 0, lo = 0;
var reg = new Int32Array(32), cp0 = new Int32Array(32);
var RAM_SIZE = 4 * 4096, icount = 0;
var hex32 = function(v){ return "0x" + (v >>> 0).toString(16).toUpperCase().padStart(8, "0"); };

%s

// Four zeroed pages, digested through the page's own incremental fold.
ram = new Uint8Array(RAM_SIZE);
cpPages = new Uint32Array(4);
cpDirty = new Uint32Array(1);
cpTotal = 0;
for (var w = 0; w < cpDirty.length; w++) cpDirty[w] = 0xFFFFFFFF;
var ramDigest = cpFoldDirty();

var page0 = cpPageDigest(0, 0);
for (var b = 0; b < 4096; b++) ram[3 * 4096 + b] = b & 0xFF;
var page3 = cpPageDigest(3, 3 * 4096);

var vReg = new Int32Array(32), vCp0 = new Int32Array(32);
for (var r = 0; r < 32; r++) {
  vReg[r] = (0x10000000 + r * 0x11) | 0;
  vCp0[r] = (0x20000000 + r * 0x13) | 0;
}

process.stdout.write(JSON.stringify({
  mixWord:      hex32(cpMixWord(2166136261 >>> 0, 0x80081C58)),
  pageDigest0:  hex32(page0),
  pageDigest3:  hex32(page3),
  ramDigest:    hex32(ramDigest),
  wholeMachine: hex32(cpHashOf(0x80081C58, 1, 0x0000DEAD, 0x0000BEEF, vReg, vCp0, ramDigest))
}));
`, block)
}
