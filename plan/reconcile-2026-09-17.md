# Phase 2 reconciliation — 2026-09-17

Scope: `SPEC.md` FR-2, FR-6 and FR-18; `plan/phase-2-cpu-core.md`; `plan/test-plan-phase-2.md`; and the actual CPU, flash, trace, oracle and control-script paths. This is a single-repository, CPU-phase reconciliation. Phase 2 adds no frontend route, page, provider, service or HTTP endpoint; the existing app serves `/health` only.

## Wiring checked

`./ctl.sh cpu-gate` builds `cmd/firmwaretrace`, which loads the operator's verified firmware images, attaches DRAM and two flash devices to the bus, steps `internal/cpu.Core`, and emits `internal/platform/statehash` checkpoints. The gate compares eight values captured from the unchanged browser oracle, checks the browser and Go hashes differ after the first video RAM read, and verifies the bus names that missing port. `./ctl.sh gate` independently starts the built HTTP process and probes `/health`. The browser oracle remains unchanged.

## Requirement and test result

| Requirement | Plan and code evidence | Verdict |
|---|---|---|
| MIPS32 instruction families, delay slots, COP0, ERET, interrupts | `internal/cpu/mips32.go`, `core.go`; TC-2.1–2.5 tests | Implemented and race-tested |
| Original MIPS16 ASE, JALX in both directions, implicit T, shift operand order | `internal/cpu/mips16.go`; TC-2.6–2.7 tests | Implemented and race-tested |
| Stateful flash protocol and snapshots | `internal/memory/flash.go`, `internal/cpu/snapshot.go`; TC-2.8 and CPU snapshot tests | Implemented and race-tested |
| Oracle agreement while only Phase 2 devices exist | `cmd/firmwaretrace`, `tools/cpu-gate.sh`, browser measurements in `docs/reference/cpu-oracle-measurement.md`; TC-2.9–2.10 | Verified through the first unmodelled video RAM read |
| Full application boot and 42 tasks | SPEC FR-1/FR-6; Phase 3c TASK-3c.9/TC-3c.7 requires genuine bootloader handoff before TASK-3c.6/TC-3c.6 checks full cold boot | Open in Phase 3c |

## Drift corrected and work retained

- The Phase 2 plan's CPU state table described T as a separate bool and gave a writer-argument snapshot signature. The implementation uses `GPR[24]` and `Snapshot() ([]byte, error)`; the plan now records the actual complete state and signature.
- The original Phase 2 handoff test depended on peripherals outside that phase. The user authorised moving true guest-driven handoff to Phase 3c. `gort-f3f.10` / TC-3c.7 retains it, and `gort-f3f.6` depends on that task. Phase 2's TC-2.10 now runs the real firmware through the measured peripheral wall.
- Public CI has no redistributable firmware, so the local firmware gates are not currently run by GitHub Actions. `gort-4sx.17` tracks a private CI path before release; local `./ctl.sh gate` and `./ctl.sh cpu-gate` both pass now.

Quality evidence: `./ctl.sh test`, `./ctl.sh lint`, `./ctl.sh gate`, `./ctl.sh cpu-gate`, `./ctl.sh vuln` and `./ctl.sh conformance` passed. All seven architecture rules passed and all 21 negative probes were caught. The Phase 2 test plan has ten checked cases and ten named results. The independent read-only review found two gate/documentation gaps; both were corrected before closeout.
