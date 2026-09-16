// Package logging builds the process logger.
//
// Structured JSON by default, because every question asked of this program is "what was the
// machine doing at instruction N" and that is a query, not a paragraph. The emulator-time field
// that makes such a query answerable is added by the core (see internal/logging's icount
// attribution); this file only decides where lines go and how they are encoded.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Format selects the encoding of a log line.
type Format string

const (
	// FormatJSON is the machine-readable encoding, and the default.
	FormatJSON Format = "json"
	// FormatText is for a human watching a terminal.
	FormatText Format = "text"
)

// New builds a logger writing to w.
//
// Level accepts slog's own names (debug, info, warn, error), case-insensitively. An unrecognised
// level or format is an error rather than a silent fall back to info: a process that logs less
// than its operator asked for looks identical to a process with nothing to say.
func New(w io.Writer, level string, format Format) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		return nil, fmt.Errorf("log level %q: %w", level, err)
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch Format(strings.ToLower(string(format))) {
	case FormatJSON:
		handler = slog.NewJSONHandler(w, opts)
	case FormatText:
		handler = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("log format %q: want %q or %q", format, FormatJSON, FormatText)
	}

	return slog.New(handler), nil
}
