package logging

import (
	"context"
	"log/slog"
)

// ICountKey is the field carrying emulator time. Every log line from a machine-aware logger has it
// alongside slog's wall-clock "time", because the two answer different questions and neither
// substitutes for the other: the wall says when a human saw something, icount says where the
// machine was. Only the second one is the same across two runs of the same firmware.
const ICountKey = "icount"

// Counter reports emulator time in instructions.
//
// Deliberately one method, so this package does not depend on the emulator. *clock.Clock satisfies
// it; so does any device or machine that can say where it is.
type Counter interface {
	// Now reports the current instruction count.
	Now() uint64
}

// WithCounter returns a logger that stamps every record with the machine's instruction count.
//
// The count is read at the moment the line is emitted, not when the logger was built, so a
// long-lived logger held by a device reports where the machine is now rather than where it was
// when the device was constructed.
//
// A nil counter returns the logger unchanged, and the resulting lines carry no icount field at
// all. That is the important half: an absent counter must not log icount=0, because zero is a
// real instruction and a line claiming it would be indistinguishable from one emitted at the very
// start of a boot. Absent means absent.
func WithCounter(logger *slog.Logger, counter Counter) *slog.Logger {
	if logger == nil || counter == nil {
		return logger
	}
	return slog.New(&icountHandler{base: logger.Handler(), counter: counter})
}

// icountHandler injects ICountKey at the top level of every record.
//
// It owns grouping rather than delegating it because of where the field has to land. slog applies
// a handler's open groups to whatever a record carries, so a handler that simply called
// AddAttrs would bury icount inside whichever group happened to be open — findable by a human
// reading one line, invisible to the query that asks what the machine was doing at instruction N,
// which is the only reason the field exists. Holding the groups here keeps icount at the top and
// everything else where the caller put it.
type icountHandler struct {
	base    slog.Handler
	counter Counter
	groups  []string
	attrs   []slog.Attr // attrs added after a group was opened, so they nest with it
}

func (h *icountHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *icountHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(slog.Uint64(ICountKey, h.counter.Now()))

	if len(h.groups) == 0 {
		r.Attrs(func(a slog.Attr) bool {
			out.AddAttrs(a)
			return true
		})
		return h.base.Handle(ctx, out)
	}

	nested := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	nested = append(nested, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		nested = append(nested, a)
		return true
	})
	for i := len(h.groups) - 1; i >= 0; i-- {
		nested = []slog.Attr{{Key: h.groups[i], Value: slog.GroupValue(nested...)}}
	}
	out.AddAttrs(nested...)

	return h.base.Handle(ctx, out)
}

func (h *icountHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	c := h.clone()
	if len(c.groups) == 0 {
		// No group open, so the base handler can hold these and pre-format them once.
		c.base = c.base.WithAttrs(attrs)
		return c
	}
	c.attrs = append(c.attrs, attrs...)
	return c
}

func (h *icountHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := h.clone()
	c.groups = append(c.groups, name)
	return c
}

func (h *icountHandler) clone() *icountHandler {
	return &icountHandler{
		base:    h.base,
		counter: h.counter,
		groups:  append([]string(nil), h.groups...),
		attrs:   append([]slog.Attr(nil), h.attrs...),
	}
}
