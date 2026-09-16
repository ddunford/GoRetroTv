# Phase 1a: Architecture conformance

## Outcome
The decisions in `plan/module-decisions.md` stop being prose and start failing CI — while there is
still almost nothing to drift. Retrofitting a catalogue after five phases means auditing shipped
code instead of holding new code.

## Overview
Run `/architecture-conformance`: install the harness, seed the catalogue with every recorded
decision bound either to a rule or to an explicit "none, because…", and prove each detector can fire.

## Tasks (mirror — bd epic `gort-87m` is the source of truth; never hand-ticked)

- [x] `TASK-1a.1` Install the conformance harness (registry, corpus, probe runner) and wire `ctl.sh conformance` → `/go-engineer` [TC-1a.1]
- [x] `TASK-1a.2` `ARCH-DEV-1`: developer surfaces bind to localhost only — the gdb stub and instrument endpoints must never listen on a public interface. **The one real security control on a public demo host** → `/go-engineer` [TC-1a.2]
- [x] `TASK-1a.3` `ARCH-SNAP-1`: every type implementing `Device` also implements `Snapshot`/`Restore`, and every exported field is covered. A device added later that forgets is caught here, not by a plausible machine six phases on → `/go-engineer` [TC-1a.3]
- [x] `TASK-1a.4` `ARCH-DET-1`: no wall-clock source and no goroutine inside the instruction loop — `time.Now`, `time.Since` and `go` statements are refused in `internal/cpu` and `internal/device` → `/go-engineer` [TC-1a.4]
- [ ] `TASK-1a.5` `ARCH-LAYER-1`: dependencies point inward — `internal/device` may not import `internal/web`; the core may not import the transport → `/go-engineer` [TC-1a.5]
- [ ] `TASK-1a.6` `ARCH-FW-1`: no firmware bytes in the repository or in any built image → `/go-engineer` [TC-1a.6]
- [x] `TASK-1a.9` `ARCH-PLATFORM-1`: the shared layer stays shared — `internal/platform` may import nothing from `internal/cpu`, `internal/device`, `internal/broadcast` or `internal/web`. This is what stops domain logic drifting into it, which is the failure mode a shared kernel has → `/go-engineer` [TC-1a.8]
- [ ] `TASK-1a.7` Write one probe per detector and run the stop-gate: each rule must be caught **on its own**, not by a sibling → `/go-engineer` [TC-1a.7]
- [ ] `TASK-1a.8` ⫘ Record each decision's rule (or its explicit "none, because…") in the catalogue and generate the rules-index mirror → `/go-engineer` [no-test: catalogue verified by the stop-gate]

## Test Cases live in test-plan-phase-1a.md
