package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/ddunford/goretrotv/internal/logging"
	"github.com/ddunford/goretrotv/internal/platform/clock"
)

// line decodes one JSON log record.
func line(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("log line is not JSON (%v): %s", err, buf.String())
	}
	return got
}

// TestALogLineCarriesBothEmulatorTimeAndWallTime is TC-1.1. The counter is a real clock advanced a
// real number of instructions, not a stub, so this also proves the two packages agree about what
// "now" means.
func TestALogLineCarriesBothEmulatorTimeAndWallTime(t *testing.T) {
	t.Parallel()

	const instructions = 4_500_000

	machine := clock.New()
	if err := machine.Advance(instructions); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var buf bytes.Buffer
	base, err := logging.New(&buf, "info", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger := logging.WithCounter(base, machine)

	logger.Info("task table scanned", "tasks", 42)

	got := line(t, &buf)

	icount, ok := got[logging.ICountKey]
	if !ok {
		t.Fatalf("no %q field in %s", logging.ICountKey, buf.String())
	}
	// JSON numbers decode as float64; 4.5 million is exact in a float64, so this comparison is
	// safe and keeps the assertion about the value rather than about the encoding.
	if n, isNum := icount.(float64); !isNum || uint64(n) != instructions {
		t.Errorf("%s = %v (%T), want %d", logging.ICountKey, icount, icount, instructions)
	}
	if uint64(icount.(float64)) != machine.Now() {
		t.Errorf("logged icount %v does not match the machine's %d", icount, machine.Now())
	}

	wall, ok := got["time"].(string)
	if !ok {
		t.Fatalf("no wall timestamp in %s", buf.String())
	}
	if _, err := time.Parse(time.RFC3339Nano, wall); err != nil {
		t.Errorf("wall timestamp %q is not RFC3339: %v", wall, err)
	}

	if got["msg"] != "task table scanned" {
		t.Errorf("msg = %v, want %q", got["msg"], "task table scanned")
	}
	if got["tasks"] != float64(42) {
		t.Errorf("tasks = %v, want 42", got["tasks"])
	}
}

// TestICountTracksTheMachineRatherThanTheLoggersAge is the bug this design has to avoid: a logger
// that captured the count when it was built would report the same instruction for the whole run.
func TestICountTracksTheMachineRatherThanTheLoggersAge(t *testing.T) {
	t.Parallel()

	machine := clock.New()
	var buf bytes.Buffer
	base, err := logging.New(&buf, "info", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Built once, at instruction zero, and held for the whole run the way a device would hold it.
	logger := logging.WithCounter(base, machine).With("device", "demux")

	steps := []uint64{1000, 2000, 3000}
	seen := make([]uint64, 0, len(steps))
	for _, step := range steps {
		if err := machine.Advance(step); err != nil {
			t.Fatalf("Advance: %v", err)
		}
		buf.Reset()
		logger.Info("section delivered")
		got := line(t, &buf)
		n, ok := got[logging.ICountKey].(float64)
		if !ok {
			t.Fatalf("no %s in %s", logging.ICountKey, buf.String())
		}
		seen = append(seen, uint64(n))
		if got["device"] != "demux" {
			t.Errorf("device = %v, want demux", got["device"])
		}
	}

	want := []uint64{1000, 3000, 6000}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("line %d logged icount %d, want %d", i, seen[i], want[i])
		}
	}
}

// TestWithoutACounterThereIsNoICountField is the absent-versus-zero rule. A line claiming
// icount=0 when nothing is counting is indistinguishable from one emitted at the start of a boot.
func TestWithoutACounterThereIsNoICountField(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger, err := logging.New(&buf, "info", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("listening", "addr", "127.0.0.1:8099")

	got := line(t, &buf)
	if v, present := got[logging.ICountKey]; present {
		t.Errorf("a logger with no machine behind it logged %s=%v; absent must mean absent", logging.ICountKey, v)
	}
}

func TestWithCounterIgnoresANilCounter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base, err := logging.New(&buf, "info", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := logging.WithCounter(base, nil); got != base {
		t.Error("WithCounter with a nil counter did not return the logger unchanged")
	}
}

// TestICountStaysAtTheTopLevelUnderAGroup guards the field's queryability. Buried inside a group
// it is still visible to a human reading one line and invisible to the query that asks what the
// machine was doing at instruction N, which is the only reason the field exists.
func TestICountStaysAtTheTopLevelUnderAGroup(t *testing.T) {
	t.Parallel()

	machine := clock.New()
	if err := machine.Advance(777); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var buf bytes.Buffer
	base, err := logging.New(&buf, "info", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger := logging.WithCounter(base, machine).
		With("service", "goretrotv").
		WithGroup("demux").
		With("pid", 1010)

	logger.Info("filter armed", "table", "eit")

	got := line(t, &buf)
	if n, ok := got[logging.ICountKey].(float64); !ok || uint64(n) != 777 {
		t.Fatalf("%s = %v at the top level, want 777: %s", logging.ICountKey, got[logging.ICountKey], buf.String())
	}
	if got["service"] != "goretrotv" {
		t.Errorf("service = %v; an attr added before the group must stay at the top level", got["service"])
	}

	group, ok := got["demux"].(map[string]any)
	if !ok {
		t.Fatalf("no demux group in %s", buf.String())
	}
	if group["pid"] != float64(1010) {
		t.Errorf("demux.pid = %v, want 1010", group["pid"])
	}
	if group["table"] != "eit" {
		t.Errorf("demux.table = %v, want eit", group["table"])
	}
	if _, buried := group[logging.ICountKey]; buried {
		t.Errorf("%s was buried inside the demux group; a query for it would miss every grouped line", logging.ICountKey)
	}
}

func TestNestedGroupsKeepICountAtTheTop(t *testing.T) {
	t.Parallel()

	machine := clock.New()
	if err := machine.Advance(5); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var buf bytes.Buffer
	base, err := logging.New(&buf, "info", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logging.WithCounter(base, machine).
		WithGroup("demux").
		WithGroup("filter").
		Info("armed", "pid", 1010)

	got := line(t, &buf)
	if n, ok := got[logging.ICountKey].(float64); !ok || uint64(n) != 5 {
		t.Fatalf("%s = %v, want 5: %s", logging.ICountKey, got[logging.ICountKey], buf.String())
	}
	demux, ok := got["demux"].(map[string]any)
	if !ok {
		t.Fatalf("no demux group in %s", buf.String())
	}
	filter, ok := demux["filter"].(map[string]any)
	if !ok {
		t.Fatalf("no demux.filter group in %s", buf.String())
	}
	if filter["pid"] != float64(1010) {
		t.Errorf("demux.filter.pid = %v, want 1010", filter["pid"])
	}
}

func TestLevelFilteringStillApplies(t *testing.T) {
	t.Parallel()

	machine := clock.New()
	var buf bytes.Buffer
	base, err := logging.New(&buf, "warn", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger := logging.WithCounter(base, machine)

	logger.Info("this is below the threshold")
	if buf.Len() != 0 {
		t.Errorf("an info line survived a warn threshold: %s", buf.String())
	}
	logger.Warn("this is not")
	if buf.Len() == 0 {
		t.Error("a warn line was dropped at a warn threshold")
	}
}

func TestNewValidatesLevelAndFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   string
		format  logging.Format
		wantErr bool
	}{
		{name: "json info", level: "info", format: logging.FormatJSON},
		{name: "text debug", level: "debug", format: logging.FormatText},
		{name: "upper case level", level: "WARN", format: logging.FormatJSON},
		{name: "upper case format", level: "info", format: logging.Format("JSON")},
		{name: "unknown level", level: "chatty", format: logging.FormatJSON, wantErr: true},
		{name: "empty level", level: "", format: logging.FormatJSON, wantErr: true},
		{name: "unknown format", level: "info", format: logging.Format("yaml"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			got, err := logging.New(&buf, tt.level, tt.format)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("New(%q, %q) succeeded; a process that logs less than its operator asked for looks identical to one with nothing to say", tt.level, tt.format)
				}
				return
			}
			if err != nil {
				t.Fatalf("New(%q, %q): %v", tt.level, tt.format, err)
			}
			if got == nil {
				t.Fatal("New returned a nil logger and no error")
			}
		})
	}
}

// TestTheHandlerIsUsableAsAnySlogHandler keeps the wrapper honest about the interface it claims.
func TestTheHandlerIsUsableAsAnySlogHandler(t *testing.T) {
	t.Parallel()

	machine := clock.New()
	var buf bytes.Buffer
	base, err := logging.New(&buf, "debug", logging.FormatJSON)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := logging.WithCounter(base, machine).Handler()

	if !h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Enabled(debug) = false at a debug threshold")
	}
	if h.Enabled(context.Background(), slog.LevelDebug-1) {
		t.Error("Enabled said yes below the threshold")
	}
	if got := h.WithAttrs(nil); got != h {
		t.Error("WithAttrs(nil) allocated a new handler")
	}
	if got := h.WithGroup(""); got != h {
		t.Error(`WithGroup("") allocated a new handler`)
	}
}
