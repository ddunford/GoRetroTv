# Reconciliation — 2026-09-17

## Phase 2

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

## Phases 3b and 3c

Scope: `SPEC.md` FR-1, FR-6, FR-7 and FR-8, both phase and test plans, the Go drawing and link devices, the diagnostic runner, and the browser oracle. Read-only backend, frontend and wiring passes were made for each phase, followed by an architecture pass.

### Wiring checked

The Go application currently registers `/health` and optional local pprof routes. There are no product browser routes, pages, WebSocket handlers or frontend services; Phase 5 owns those. The device packages are wired through `cmd/firmwaretrace`, which uses the real firmware and records checkpoint, guest-PC and surface evidence. `cmd/goretrotv` currently loads and verifies firmware but does not run the machine; its integration is also Phase 5 work. The browser oracle is a measurement tool, not a product page.

### Findings and disposition

| Finding | Disposition |
|---|---|
| Packed 1-, 2- and 4-bit OSD rows advanced by `width*depth/8`, overlapping the next row when the row did not fill a byte | Fixed to round up the row stride; regression covers partial-byte rows at all three depths. `gort-omj.5` and TC-3b.5 were reopened, corrected and verified. |
| Phase 3b plan described a DMA channel signature and display return shape that differed from code, and counted 14 scanned descriptors | Corrected plan to `DMA.Run(channel uint8)`, `Display.Compose() (*image.Paletted, error)` and 13 scanned descriptors plus an unused slot. |
| Phase 3c claimed that the guest polls demodulator lock registers 75/78 | A full 470-million-instruction cold run observed 11 demodulator reads at registers 0–5, 14 and 1025, and none at 75/78. TC-3c.5 now states the observed coverage. `gort-l14.7` / TC-3a.7 tracks later SI-acquisition reads. |
| The cold-boot oracle gate used 100,000-instruction checkpoints; SPEC FR-6 requires every 1,000, plus deliberate-divergence localisation | The 1,000-cadence comparison exposed six isolated differing checkpoints. Exact-instruction traces found a missing readback word at `0xB200A000`. The measured board latch model resolved all six; `./ctl.sh oracle-gate` now matches 470,000/470,000 checkpoints and 42 tasks. `tools/oracle-tier2-gate.sh` localizes a deliberately injected Status bit to instruction 100,123. |
| The runner counts its own 16-instruction device-pump phase while `internal/platform/clock` has no production caller | `gort-lbl.1` / TC-4.1 now requires a central schedulable instruction clock, snapshot of its deadline, and removal of the runner-local phase. |
| The Sky menu host gate policy was implemented but absent from the architecture decision record | Added to `CLAUDE.md` and `plan/module-decisions.md`, distinguishing the clean hardware oracle from the optional warm-menu presentation policy. |
| SPEC FR-1 says the box reports Ready; the current command-line server has no such status | Retained in Phase 5 TASK-5.2/5.4. `gort-4sx.2` records that 42 tasks alone does not prove Ready; the status must come from observed guest state. |
| Local pprof routes share the application mux | Phase 5 public security gate must prove they cannot be reached from the public URL. |

No new shared abstraction was justified by three or more duplicated production implementations. The device/bus import direction, snapshot interface, single-thread instruction loop and error-value policy match the recorded decisions. Phase 3b's drawing path and Phase 3c's link path were exercised with real firmware. Phase 5 still owns the product browser and public URL; Phase 4 owns the central clock and complete snapshots.

Task/test link check: every named TC-3b and TC-3c appears in its phase plan, and every implementation task appears in its test plan. TASK-3b.8 and TASK-3c.8 are security audits with explicit `[no-test]` annotations and their own audit reports.

Closeout evidence: `./ctl.sh test` passed under the race detector; `./ctl.sh lint` reported zero issues; `./ctl.sh image` built the firmware-free image; `./ctl.sh conformance` passed all seven architecture rules and caught all 21 negative probes. `./ctl.sh oracle-gate` matched 470,000/470,000 clean cold-boot checkpoints, `./ctl.sh links-gate` proved the warm handset/surface path and demodulator read histogram, and `tools/oracle-tier2-gate.sh` localized its deliberate mutation exactly. The unchanged browser oracle's SHA-256 is recorded with the fixture in `internal/platform/statehash/testdata/README.md`.
