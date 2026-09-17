package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/instruments"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

type instrumentRange struct{ lo, hi, fromPC, toPC uint32 }
type instrumentRanges []instrumentRange

func (r *instrumentRanges) String() string { return fmt.Sprintf("%d ranges", len(*r)) }

// Set accepts lo:hi or lo:hi:fromPC:toPC. All bounds are half-open.
func (r *instrumentRanges) Set(spec string) error {
	fields := strings.Split(spec, ":")
	if len(fields) != 2 && len(fields) != 4 {
		return fmt.Errorf("range %q needs lo:hi[:fromPC:toPC]", spec)
	}
	var values [4]uint32
	for i, field := range fields {
		n, err := strconv.ParseUint(field, 0, 32)
		if err != nil {
			return fmt.Errorf("range bound %q: %w", field, err)
		}
		values[i] = uint32(n) // #nosec G115 -- ParseUint limits to 32 bits.
	}
	if values[0] >= values[1] || len(fields) == 4 && values[2] >= values[3] {
		return fmt.Errorf("range %q is empty or reversed", spec)
	}
	*r = append(*r, instrumentRange{values[0], values[1], values[2], values[3]})
	return nil
}

type instrumentRun struct {
	hist        *instruments.Histogram
	histRanges  instrumentRanges
	controls    []uint32
	reads       []*instruments.Watch
	readRanges  instrumentRanges
	writes      []*instruments.Watch
	writeRanges instrumentRanges
	calls       *instruments.Calls
	callTargets []uint32
	ocode       *instruments.OCodeTrace
}

func newInstrumentRun(hist bool, histRanges, readRanges, writeRanges instrumentRanges, controls, targets pcHits, ocode bool, max int) (*instrumentRun, error) {
	if max <= 0 || max > 100000 {
		return nil, fmt.Errorf("instrument-max must be in 1..100000")
	}
	r := &instrumentRun{histRanges: histRanges, readRanges: readRanges, writeRanges: writeRanges}
	if hist || len(histRanges) != 0 || len(controls) != 0 {
		r.hist = instruments.NewHistogram()
	}
	for _, control := range controls {
		r.controls = append(r.controls, control.address)
	}
	for _, target := range targets {
		r.callTargets = append(r.callTargets, target.address)
	}
	if len(targets) != 0 {
		var err error
		r.calls, err = instruments.NewCalls(r.callTargets, max)
		if err != nil {
			return nil, err
		}
	}
	for _, spec := range readRanges {
		watch, err := instruments.NewWatch(instruments.WatchConfig{
			Kind: instruments.Read, Lo: spec.lo, Hi: spec.hi, FromPC: spec.fromPC, ToPC: spec.toPC, Max: max,
		})
		if err != nil {
			return nil, err
		}
		r.reads = append(r.reads, watch)
	}
	for _, spec := range writeRanges {
		watch, err := instruments.NewWatch(instruments.WatchConfig{
			Kind: instruments.Write, Lo: spec.lo, Hi: spec.hi, FromPC: spec.fromPC, ToPC: spec.toPC, Max: max,
		})
		if err != nil {
			return nil, err
		}
		r.writes = append(r.writes, watch)
	}
	if ocode {
		var err error
		r.ocode, err = instruments.NewOCodeTrace(max)
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *instrumentRun) requested() bool {
	return r.hist != nil || len(r.reads) != 0 || len(r.writes) != 0 || r.calls != nil || r.ocode != nil
}

func (r *instrumentRun) observeAccesses() bool {
	return len(r.reads) != 0 || len(r.writes) != 0 || r.ocode != nil
}

func (r *instrumentRun) ObserveInstruction(icount uint64, pc uint32, gpr [32]uint32) {
	if r.hist != nil {
		r.hist.Observe(pc)
	}
	for _, watch := range r.reads {
		watch.ObserveInstruction()
	}
	for _, watch := range r.writes {
		watch.ObserveInstruction()
	}
	if r.calls != nil {
		r.calls.Observe(icount, pc, gpr)
	}
	if r.ocode != nil {
		r.ocode.ObserveInstruction(pc)
	}
}

func (r *instrumentRun) ObserveAccess(icount uint64, pc uint32, access bus.ObservedAccess) {
	for _, watch := range r.reads {
		watch.ObserveAccess(pc, icount, access)
	}
	for _, watch := range r.writes {
		watch.ObserveAccess(pc, icount, access)
	}
	if r.ocode != nil {
		r.ocode.ObserveAccess(pc, icount, access)
	}
}

func (r *instrumentRun) Report(out io.Writer) error {
	if !r.requested() {
		return nil
	}
	if r.hist != nil {
		for _, pc := range r.controls {
			if err := r.hist.RequirePC(pc); err != nil {
				return err
			}
		}
		ranges := r.histRanges
		if len(ranges) == 0 {
			ranges = instrumentRanges{{lo: 0, hi: ^uint32(0)}}
		}
		for _, spec := range ranges {
			result, err := r.hist.Range(spec.lo, spec.hi, 16)
			if err != nil {
				return err
			}
			if result.Total == 0 {
				return instrument.MustFind("PC range histogram", "executed PCs in requested range", 0, 1)
			}
			if _, err := fmt.Fprintf(out, "pc-range lo=%s hi=%s examined=%d total=%d distinct=%d\n", wireWord(spec.lo), wireWord(spec.hi), result.Examined, result.Total, result.Distinct); err != nil {
				return err
			}
			for _, hit := range result.Hottest {
				if _, err := fmt.Fprintf(out, "pc-range-hit pc=%s count=%d\n", wireWord(hit.PC), hit.Count); err != nil {
					return err
				}
			}
		}
	}
	for i, watch := range r.reads {
		if err := reportWatch(out, "read-watch", r.readRanges[i], watch); err != nil {
			return err
		}
	}
	for i, watch := range r.writes {
		if err := reportWatch(out, "write-watch", r.writeRanges[i], watch); err != nil {
			return err
		}
	}
	if r.calls != nil {
		for _, pc := range r.callTargets {
			if err := r.calls.RequirePC(pc); err != nil {
				return err
			}
		}
		result, err := r.calls.Result()
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "call-trace examined=%d total=%d logged=%d capped=%t\n", result.Examined, result.Total, len(result.Logged), result.Capped); err != nil {
			return err
		}
		for _, call := range result.Logged {
			if _, err := fmt.Fprintf(out, "call at=%d pc=%s ra=%s sp=%s a0=%s a1=%s a2=%s a3=%s\n", call.ICount, wireWord(call.PC), wireWord(call.RA), wireWord(call.SP), wireWord(call.Args[0]), wireWord(call.Args[1]), wireWord(call.Args[2]), wireWord(call.Args[3])); err != nil {
				return err
			}
		}
	}
	if r.ocode != nil {
		result, err := r.ocode.Result()
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "ocode-trace examined=%d total=%d logged=%d capped=%t\n", result.Examined, result.Total, len(result.Logged), result.Capped); err != nil {
			return err
		}
		for _, hit := range result.Logged {
			if _, err := fmt.Fprintf(out, "ocode at=%d pc=%s address=%s size=%d value=%s\n", hit.ICount, wireWord(hit.PC), wireWord(hit.Access.Virtual), hit.Access.Size, wireWord(hit.Access.Value)); err != nil {
				return err
			}
		}
	}
	return nil
}

func reportWatch(out io.Writer, name string, spec instrumentRange, watch *instruments.Watch) error {
	if err := watch.RequireHit(); err != nil {
		return err
	}
	result, err := watch.Result()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%s lo=%s hi=%s examined=%d total=%d logged=%d capped=%t\n", name, wireWord(spec.lo), wireWord(spec.hi), result.Examined, result.Total, len(result.Logged), result.Capped); err != nil {
		return err
	}
	for _, hit := range result.Logged {
		if _, err := fmt.Fprintf(out, "%s-hit at=%d pc=%s address=%s size=%d value=%s\n", name, hit.ICount, wireWord(hit.PC), wireWord(hit.Access.Virtual), hit.Access.Size, wireWord(hit.Access.Value)); err != nil {
			return err
		}
	}
	return nil
}
