// Package instruments observes guest execution without changing machine state.
// Each result refuses an empty observation window; callers can require a known
// positive control before interpreting a zero at an address of interest.
package instruments

import (
	"fmt"
	"sort"

	"github.com/ddunford/goretrotv/internal/bus"
	"github.com/ddunford/goretrotv/internal/memory"
	"github.com/ddunford/goretrotv/internal/platform/hexfmt"
	"github.com/ddunford/goretrotv/internal/platform/instrument"
)

// Histogram counts retired guest instructions by their executable address.
type Histogram struct {
	examined uint64
	counts   map[uint32]uint64
}

// NewHistogram starts an empty PC census.
func NewHistogram() *Histogram { return &Histogram{counts: make(map[uint32]uint64)} }

// Observe records one guest instruction before it executes.
func (h *Histogram) Observe(pc uint32) {
	h.examined++
	h.counts[pc]++
}

// Exact counts executions of pc. A zero is meaningful only if instructions were examined.
func (h *Histogram) Exact(pc uint32) (uint64, error) {
	if err := requireObserved("PC histogram", "guest instructions", h.examined); err != nil {
		return 0, err
	}
	return h.counts[pc], nil
}

// RequirePC makes a known positive control part of the instrument's contract.
func (h *Histogram) RequirePC(pc uint32) error {
	return requireObserved("PC histogram", "guest PC "+hexfmt.Addr(pc), h.counts[pc])
}

// PCHit is one ranked address in a PC range.
type PCHit struct {
	PC    uint32
	Count uint64
}

// RangeSummary reports all executions in a half-open PC range.
type RangeSummary struct {
	Examined uint64
	Total    uint64
	Distinct int
	Hottest  []PCHit
}

// Range summarizes [lo, hi). It permits zero hits in the range after a real
// population was examined; RequirePC can assert that a control ran in that window.
func (h *Histogram) Range(lo, hi uint32, top int) (RangeSummary, error) {
	if lo >= hi || top < 0 {
		return RangeSummary{}, fmt.Errorf("invalid PC range or top count")
	}
	if err := requireObserved("PC histogram", "guest instructions", h.examined); err != nil {
		return RangeSummary{}, err
	}
	r := RangeSummary{Examined: h.examined}
	for pc, count := range h.counts {
		if pc < lo || pc >= hi {
			continue
		}
		r.Total += count
		r.Distinct++
		r.Hottest = append(r.Hottest, PCHit{PC: pc, Count: count})
	}
	sort.Slice(r.Hottest, func(i, j int) bool {
		if r.Hottest[i].Count != r.Hottest[j].Count {
			return r.Hottest[i].Count > r.Hottest[j].Count
		}
		return r.Hottest[i].PC < r.Hottest[j].PC
	})
	if len(r.Hottest) > top {
		r.Hottest = r.Hottest[:top]
	}
	return r, nil
}

// AccessKind selects guest loads or stores.
type AccessKind uint8

// Read and Write select the direction of a guest bus operation.
const (
	Read AccessKind = iota
	Write
)

// WatchConfig defines a half-open guest-address window and an optional
// half-open executing-PC window. A DRAM address window matches both KSEG aliases.
type WatchConfig struct {
	Kind         AccessKind
	Lo, Hi       uint32
	FromPC, ToPC uint32
	Max          int
}

// AccessHit records one matching bus operation and its guest instruction.
type AccessHit struct {
	ICount uint64
	PC     uint32
	Access bus.ObservedAccess
}

// WatchResult reports all matches and the bounded detailed log.
type WatchResult struct {
	Examined uint64
	Total    uint64
	Logged   []AccessHit
	Capped   bool
	ByPC     map[uint32]uint64
}

// Watch observes one bus access window without changing device behavior.
type Watch struct {
	config   WatchConfig
	physical bool
	lo, hi   uint32
	examined uint64
	total    uint64
	logged   []AccessHit
	byPC     map[uint32]uint64
}

// NewWatch validates and arms a guest access watch.
func NewWatch(config WatchConfig) (*Watch, error) {
	if config.Kind != Read && config.Kind != Write || config.Lo >= config.Hi || config.Max <= 0 {
		return nil, fmt.Errorf("invalid watch kind, range or cap")
	}
	if (config.FromPC != 0 || config.ToPC != 0) && config.FromPC >= config.ToPC {
		return nil, fmt.Errorf("invalid watch PC range")
	}
	w := &Watch{config: config, lo: config.Lo, hi: config.Hi, byPC: make(map[uint32]uint64)}
	// Only a wholly DRAM window gets normalized. Other addresses remain virtual,
	// preserving the oracle's read-watch behavior for flash and MMIO.
	if inDRAM(config.Lo) && inDRAM(config.Hi-1) {
		lo, _ := bus.Physical(config.Lo)
		hi, _ := bus.Physical(config.Hi - 1)
		if hi >= lo {
			w.physical, w.lo, w.hi = true, lo, hi+1
		}
	}
	return w, nil
}

func inDRAM(addr uint32) bool {
	phys, ok := bus.Physical(addr)
	return ok && phys < memory.DRAMSize
}

// ObserveInstruction must be called once before each monitored guest step.
func (w *Watch) ObserveInstruction() { w.examined++ }

// ObserveAccess must be called only for accesses made by the current guest
// instruction, with that instruction's pre-execution PC and icount.
func (w *Watch) ObserveAccess(pc uint32, icount uint64, access bus.ObservedAccess) {
	if access.Fetch {
		return
	}
	if (w.config.Kind == Write) != access.Write {
		return
	}
	if w.config.ToPC != 0 && (pc < w.config.FromPC || pc >= w.config.ToPC) {
		return
	}
	addr := access.Virtual
	if w.physical {
		var ok bool
		addr, ok = bus.Physical(addr)
		if !ok {
			return
		}
	}
	if access.Size != 0 && uint64(addr) < uint64(w.hi) && uint64(addr)+uint64(access.Size) > uint64(w.lo) {
		w.total++
		w.byPC[pc]++
		if len(w.logged) < w.config.Max {
			w.logged = append(w.logged, AccessHit{ICount: icount, PC: pc, Access: access})
		}
	}
}

// Result refuses an empty instruction window and returns a detached report.
func (w *Watch) Result() (WatchResult, error) {
	if err := requireObserved("access watch", "guest instructions", w.examined); err != nil {
		return WatchResult{}, err
	}
	byPC := make(map[uint32]uint64, len(w.byPC))
	for pc, n := range w.byPC {
		byPC[pc] = n
	}
	return WatchResult{Examined: w.examined, Total: w.total,
		Logged: append([]AccessHit(nil), w.logged...), Capped: w.total > uint64(len(w.logged)), ByPC: byPC}, nil
}

// RequireHit asserts that the configured bus range was reached. Use this for
// a positive control before interpreting silence in a narrower watch.
func (w *Watch) RequireHit() error {
	return requireObserved("access watch", "configured guest address range", w.total)
}

// Call records a target PC's pre-execution argument and frame registers.
type Call struct {
	ICount     uint64
	PC, RA, SP uint32
	Args       [4]uint32
}

// CallResult reports call counts and bounded detailed records.
type CallResult struct {
	Examined uint64
	Total    uint64
	Logged   []Call
	Capped   bool
	ByPC     map[uint32]uint64
}

// Calls records executions of declared exact guest PCs.
type Calls struct {
	pcs             map[uint32]struct{}
	max             int
	examined, total uint64
	logged          []Call
	byPC            map[uint32]uint64
}

// NewCalls validates and arms an exact-PC call trace.
func NewCalls(pcs []uint32, max int) (*Calls, error) {
	if len(pcs) == 0 || max <= 0 {
		return nil, fmt.Errorf("call trace requires PCs and a positive cap")
	}
	c := &Calls{pcs: make(map[uint32]struct{}, len(pcs)), max: max, byPC: make(map[uint32]uint64)}
	for _, pc := range pcs {
		c.pcs[pc] = struct{}{}
	}
	return c, nil
}

// Observe inspects the registers before the guest instruction executes.
func (c *Calls) Observe(icount uint64, pc uint32, gpr [32]uint32) {
	c.examined++
	if _, ok := c.pcs[pc]; !ok {
		return
	}
	c.total++
	c.byPC[pc]++
	if len(c.logged) < c.max {
		c.logged = append(c.logged, Call{ICount: icount, PC: pc, RA: gpr[31], SP: gpr[29], Args: [4]uint32{gpr[4], gpr[5], gpr[6], gpr[7]}})
	}
}

// Result refuses an empty instruction window and returns a detached report.
func (c *Calls) Result() (CallResult, error) {
	if err := requireObserved("call trace", "guest instructions", c.examined); err != nil {
		return CallResult{}, err
	}
	byPC := make(map[uint32]uint64, len(c.byPC))
	for pc, n := range c.byPC {
		byPC[pc] = n
	}
	return CallResult{Examined: c.examined, Total: c.total, Logged: append([]Call(nil), c.logged...), Capped: c.total > uint64(len(c.logged)), ByPC: byPC}, nil
}

// RequirePC asserts that a declared target was actually executed.
func (c *Calls) RequirePC(pc uint32) error {
	if _, ok := c.pcs[pc]; !ok {
		return &instrument.HarnessError{Instrument: "call trace", Subject: "declared call PCs", Detail: "PC " + hexfmt.Addr(pc) + " was not declared", Err: instrument.ErrUnknownFinding}
	}
	return requireObserved("call trace", "declared PC "+hexfmt.Addr(pc), c.byPC[pc])
}

// The firmware's OpenTV CODE chunk and its native interpreter fetch site.
const (
	OCodeStart uint32 = 0x9FC4A400
	OCodeEnd   uint32 = 0x9FCA07A4
	OCodeFetch uint32 = 0x80069298
)

// OCodeTrace records addresses read from the OpenTV CODE chunk by the main
// interpreter opcode fetch. The fetch PC must execute in the window.
type OCodeTrace struct {
	watch   *Watch
	fetches uint64
}

// NewOCodeTrace watches the bytecode reads made by the native interpreter fetch.
func NewOCodeTrace(max int) (*OCodeTrace, error) {
	w, err := NewWatch(WatchConfig{Kind: Read, Lo: OCodeStart, Hi: OCodeEnd,
		FromPC: OCodeFetch, ToPC: OCodeFetch + 1, Max: max})
	if err != nil {
		return nil, err
	}
	return &OCodeTrace{watch: w}, nil
}

// ObserveInstruction records one guest instruction and checks the fetch control.
func (o *OCodeTrace) ObserveInstruction(pc uint32) {
	o.watch.ObserveInstruction()
	if pc == OCodeFetch {
		o.fetches++
	}
}

// ObserveAccess records a bus operation from the current guest instruction.
func (o *OCodeTrace) ObserveAccess(pc uint32, icount uint64, access bus.ObservedAccess) {
	o.watch.ObserveAccess(pc, icount, access)
}

// Result requires both the interpreter fetch PC and a CODE read in the window.
func (o *OCodeTrace) Result() (WatchResult, error) {
	if err := requireObserved("o-code trace", "interpreter fetch PC", o.fetches); err != nil {
		return WatchResult{}, err
	}
	result, err := o.watch.Result()
	if err != nil {
		return WatchResult{}, err
	}
	if err := requireObserved("o-code trace", "CODE chunk reads from interpreter fetch", result.Total); err != nil {
		return WatchResult{}, err
	}
	return result, nil
}

func requireObserved(name, subject string, count uint64) error {
	if count != 0 {
		return nil
	}
	return instrument.MustFind(name, subject, 0, 1)
}
