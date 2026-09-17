package instruments

import (
	"errors"
	"testing"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

func TestHistogramRefusesEmptyAndDistinguishesZeroFromMissingControl(t *testing.T) {
	h := NewHistogram()
	if _, err := h.Exact(0x8006EA04); !errors.Is(err, instrument.ErrNothingExamined) {
		t.Fatalf("empty histogram: %v", err)
	}
	h.Observe(0x800D35DC)
	h.Observe(0x8006EA04)
	h.Observe(0x8006EA04)
	if err := h.RequirePC(0x8006EA04); err != nil {
		t.Fatal(err)
	}
	if err := h.RequirePC(0x800297B0); !errors.Is(err, instrument.ErrNothingExamined) {
		t.Fatalf("missing positive control: %v", err)
	}
	zero, err := h.Exact(0x800297B0)
	if err != nil || zero != 0 {
		t.Fatalf("declared zero: %d, %v", zero, err)
	}
	r, err := h.Range(0x8006EA00, 0x8006EB00, 3)
	if err != nil || r.Examined != 3 || r.Total != 2 || r.Distinct != 1 || len(r.Hottest) != 1 || r.Hottest[0].PC != 0x8006EA04 {
		t.Fatalf("range: %+v, %v", r, err)
	}
}

func TestWatchMatchesBothDRAMAliasesAndPreservesActualAddress(t *testing.T) {
	w, err := NewWatch(WatchConfig{Kind: Write, Lo: 0x801072D8, Hi: 0x801072DC, Max: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Result(); !errors.Is(err, instrument.ErrNothingExamined) {
		t.Fatalf("empty watch: %v", err)
	}
	w.ObserveInstruction()
	w.ObserveAccess(0x80010000, 6, bus.ObservedAccess{Virtual: 0xA01072D8, Size: bus.Word, Fetch: true})
	w.ObserveAccess(0x80010000, 7, bus.ObservedAccess{Virtual: 0xA01072D8, Size: bus.Word, Value: 1, Write: true})
	w.ObserveAccess(0x80010004, 8, bus.ObservedAccess{Virtual: 0x801072D8, Size: bus.Word, Value: 2, Write: true})
	w.ObserveAccess(0x80010004, 8, bus.ObservedAccess{Virtual: 0x801072D8, Size: bus.Word, Value: 2})
	r, err := w.Result()
	if err != nil || r.Total != 2 || !r.Capped || len(r.Logged) != 1 || r.Logged[0].Access.Virtual != 0xA01072D8 || r.ByPC[0x80010004] != 1 {
		t.Fatalf("watch result: %+v, %v", r, err)
	}
	if err := w.RequireHit(); err != nil {
		t.Fatal(err)
	}
}

func TestWatchCatchesAccessCrossingRangeStart(t *testing.T) {
	w, err := NewWatch(WatchConfig{Kind: Read, Lo: 0x801072D8, Hi: 0x801072DC, Max: 2})
	if err != nil {
		t.Fatal(err)
	}
	w.ObserveInstruction()
	w.ObserveAccess(0x80010000, 1, bus.ObservedAccess{Virtual: 0x801072D6, Size: bus.Word})
	w.ObserveAccess(0x80010000, 1, bus.ObservedAccess{Virtual: 0x801072D8, Size: bus.Word, Fetch: true})
	r, err := w.Result()
	if err != nil || r.Total != 1 {
		t.Fatalf("cross-boundary data access or fetch filtering failed: %+v, %v", r, err)
	}
}

func TestWatchPCFilterAndFlashRange(t *testing.T) {
	w, err := NewWatch(WatchConfig{Kind: Read, Lo: OCodeStart, Hi: OCodeEnd, FromPC: OCodeFetch, ToPC: OCodeFetch + 1, Max: 2})
	if err != nil {
		t.Fatal(err)
	}
	w.ObserveInstruction()
	w.ObserveAccess(0x80069290, 1, bus.ObservedAccess{Virtual: OCodeStart, Size: bus.Byte})
	w.ObserveAccess(OCodeFetch, 2, bus.ObservedAccess{Virtual: OCodeEnd, Size: bus.Byte})
	w.ObserveAccess(OCodeFetch, 3, bus.ObservedAccess{Virtual: OCodeEnd - 1, Size: bus.Byte})
	r, err := w.Result()
	if err != nil || r.Total != 1 || r.Logged[0].Access.Virtual != OCodeEnd-1 {
		t.Fatalf("filtered watch: %+v, %v", r, err)
	}
}

func TestReadWatchSeesUncachedAliasAndRejectsWrongPC(t *testing.T) {
	w, err := NewWatch(WatchConfig{Kind: Read, Lo: 0x801072D8, Hi: 0x801072DC,
		FromPC: 0x800D35DC, ToPC: 0x800D35E0, Max: 2})
	if err != nil {
		t.Fatal(err)
	}
	w.ObserveInstruction()
	w.ObserveAccess(0x800D35E0, 1, bus.ObservedAccess{Virtual: 0xA01072D8, Size: bus.Word, Value: 3})
	if err := w.RequireHit(); !errors.Is(err, instrument.ErrNothingExamined) {
		t.Fatalf("wrong PC did not reject: %v", err)
	}
	w.ObserveAccess(0x800D35DC, 2, bus.ObservedAccess{Virtual: 0xA01072D8, Size: bus.Word, Value: 3})
	if err := w.RequireHit(); err != nil {
		t.Fatal(err)
	}
	r, err := w.Result()
	if err != nil || r.Total != 1 || r.Capped || len(r.Logged) != 1 {
		t.Fatalf("read alias: %+v, %v", r, err)
	}
}

func TestCallsCapturePreInstructionRegistersAndCap(t *testing.T) {
	c, err := NewCalls([]uint32{0x8006EA04}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Result(); !errors.Is(err, instrument.ErrNothingExamined) {
		t.Fatalf("empty calls: %v", err)
	}
	var gpr [32]uint32
	gpr[31], gpr[29] = 0x80010000, 0x80128E7C
	gpr[4], gpr[5], gpr[6], gpr[7] = 1, 2, 3, 4
	c.Observe(9, 0x8006EA04, gpr)
	c.Observe(10, 0x8006EA04, gpr)
	r, err := c.Result()
	if err != nil || r.Examined != 2 || r.Total != 2 || !r.Capped || len(r.Logged) != 1 || r.Logged[0].RA != gpr[31] || r.Logged[0].SP != gpr[29] || r.Logged[0].Args != [4]uint32{1, 2, 3, 4} {
		t.Fatalf("call trace: %+v, %v", r, err)
	}
	if err := c.RequirePC(0x8006EA04); err != nil {
		t.Fatal(err)
	}
	if err := c.RequirePC(0x800297B0); !errors.Is(err, instrument.ErrUnknownFinding) {
		t.Fatalf("unknown call target: %v", err)
	}
}

func TestOCodeTraceRequiresFetchAndMatchingRead(t *testing.T) {
	o, err := NewOCodeTrace(2)
	if err != nil {
		t.Fatal(err)
	}
	o.ObserveInstruction(0x800D35DC)
	if _, err := o.Result(); !errors.Is(err, instrument.ErrNothingExamined) {
		t.Fatalf("missing fetch: %v", err)
	}
	o.ObserveInstruction(OCodeFetch)
	if _, err := o.Result(); !errors.Is(err, instrument.ErrNothingExamined) {
		t.Fatalf("missing CODE read: %v", err)
	}
	o.ObserveAccess(OCodeFetch, 11, bus.ObservedAccess{Virtual: 0x9FC4A538, Size: bus.Byte, Value: 0x41})
	r, err := o.Result()
	if err != nil || r.Total != 1 || r.Logged[0].Access.Virtual != 0x9FC4A538 {
		t.Fatalf("o-code trace: %+v, %v", r, err)
	}
}
