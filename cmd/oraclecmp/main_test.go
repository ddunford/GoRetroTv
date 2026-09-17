package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const agreeing = "GRTV-CHECKPOINTS 1 interval=1000\n" +
	"0 0x00000001\n1000 0x00000002\n2000 0x00000003\n3000 0x00000004\nEND 4 3000\n"

// The same run with one checkpoint changed, at instruction 2,000.
const diverging = "GRTV-CHECKPOINTS 1 interval=1000\n" +
	"0 0x00000001\n1000 0x00000002\n2000 0xDEADBEEF\n3000 0x00000004\nEND 4 3000\n"

// The same run, cut short: no END line.
const truncated = "GRTV-CHECKPOINTS 1 interval=1000\n" +
	"0 0x00000001\n1000 0x00000002\n"

func write(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func exec(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestAgreeingStreamsExitZero(t *testing.T) {
	t.Parallel()
	code, out, errOut := exec(t, write(t, "go.txt", agreeing), write(t, "oracle.txt", agreeing))
	if code != exitAgree {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitAgree, out, errOut)
	}
	if !strings.HasPrefix(out, "AGREE") {
		t.Fatalf("stdout does not say it agreed: %q", out)
	}
	// An agreement must state its range, or it is an agreement about nothing in particular.
	if !strings.Contains(out, "4 checkpoints") {
		t.Fatalf("the agreement does not say how far it compared: %q", out)
	}
}

// TC-1.6: the divergence is reported, and the window is in the output a person reads.
func TestDivergingStreamsExitOneAndNameTheWindow(t *testing.T) {
	t.Parallel()
	code, out, errOut := exec(t, write(t, "go.txt", agreeing), write(t, "oracle.txt", diverging))
	if code != exitDiverge {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, exitDiverge, out, errOut)
	}
	for _, want := range []string{"DIVERGE", "2000..2999", "0xDEADBEEF", "tier 2",
		"re-run instructions 1001..2000"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the report does not contain %q:\n%s", want, out)
		}
	}
}

// The distinction the exit codes exist for. A comparison that could not be made must not exit 0,
// and must not exit 1 either: a gate reading $? has to be able to tell "they disagreed" from "I
// could not tell", and a person reading the log has to be told in words.
func TestAnUnusableStreamExitsTwoAndSaysSo(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"a truncated stream": {write(t, "go.txt", agreeing), write(t, "oracle.txt", truncated)},
		"an empty stream": {write(t, "go.txt", agreeing),
			write(t, "oracle.txt", "GRTV-CHECKPOINTS 1 interval=1000\n")},
		"mismatched intervals": {write(t, "go.txt", agreeing),
			write(t, "oracle.txt", strings.Replace(agreeing, "interval=1000", "interval=500", 1))},
	}
	for name, args := range cases {
		name, args := name, args
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, out, errOut := exec(t, args...)
			if code == exitAgree {
				t.Fatalf("%s exited 0 - a gate would read that as no divergence found\n%s", name, out)
			}
			if code != exitHarness {
				t.Fatalf("%s exited %d, want %d so it is distinguishable from a real divergence",
					name, code, exitHarness)
			}
			if !strings.Contains(errOut, "HARNESS FAILURE") {
				t.Fatalf("%s: the output must say in words that nothing was compared: %q",
					name, errOut)
			}
			t.Logf("caught: %s", strings.TrimSpace(errOut))
		})
	}
}

func TestBadInvocationExitsTwo(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"no arguments":     {},
		"one argument":     {write(t, "go.txt", agreeing)},
		"a missing file":   {write(t, "go.txt", agreeing), filepath.Join(t.TempDir(), "absent.txt")},
		"an unknown flag":  {"-nonsense", write(t, "a.txt", agreeing), write(t, "b.txt", agreeing)},
		"a malformed file": {write(t, "go.txt", agreeing), write(t, "oracle.txt", "not a stream\n")},
	}
	for name, args := range cases {
		name, args := name, args
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if code, out, _ := exec(t, args...); code != exitHarness {
				t.Fatalf("%s exited %d, want %d: %q", name, code, exitHarness, out)
			}
		})
	}
}

func TestLabelsDefaultToTheFileNames(t *testing.T) {
	t.Parallel()
	_, out, _ := exec(t, write(t, "port.txt", agreeing), write(t, "browser.txt", diverging))
	if !strings.Contains(out, "port.txt") || !strings.Contains(out, "browser.txt") {
		t.Fatalf("the report must name which stream is which: %q", out)
	}

	_, out, _ = exec(t, "-a", "go", "-b", "oracle",
		write(t, "port.txt", agreeing), write(t, "browser.txt", diverging))
	if !strings.Contains(out, "go") || !strings.Contains(out, "oracle") {
		t.Fatalf("explicit labels must be used: %q", out)
	}
}
