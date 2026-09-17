# Phase 4: Snapshot, replay and the debugger

## Outcome
**The experiment loop drops from six minutes to seconds.** This is the phase that pays for the port:
everything after it is cheaper, and the three unsolved screens become tractable.

## Overview
Snapshot/restore, deterministic replay and a gdb stub. Every device already implements
`Snapshot`/`Restore` (phase 1) — this phase makes it whole, proves it, and adds the tooling.

## Tasks (mirror — bd epic `gort-lbl` is the source of truth; never hand-ticked)

- [x] `TASK-4.1` Snapshot format and writer covering the spike-003 inventory: CPU, 32 MB RAM, COP0, the ISA bit, interrupt/timer state, **both flash chips' command sequencer state**, and every device → `/go-engineer` [TC-4.1]
- [x] `TASK-4.2` Restore, and the acceptance test that matters: snapshot at N, restore, run to N+10,000,000, and require the state hash to equal an uninterrupted run. **"It restores" is not the test** → `/go-engineer` [TC-4.2]
- [x] `TASK-4.3` A named snapshot library and `ctl.sh` verbs, with a documented "post-acquisition" snapshot every probe can start from → `/go-engineer` [TC-4.3]
- [x] `TASK-4.4` Input recording and deterministic replay to a byte-identical framebuffer and identical instruction count → `/go-engineer` [TC-4.4]
- [x] `TASK-4.5` gdb remote serial protocol stub: registers, memory, breakpoints, watchpoints, step, continue — **bound to localhost only** (ARCH-DEV-1) → `/go-engineer` [TC-4.5]
- [x] `TASK-4.6` Port the instrument suite: PC histograms, range/read/write watches, call tracing, section injection, and the o-code trace — **each asserting its own subject**, so a census that cannot find what it counts is a harness failure and never a zero → `/go-engineer` [TC-4.6]
- [ ] `TASK-4.7` ⫘ Tests → `/go-engineer` [TC-4.1, TC-4.2, TC-4.3, TC-4.4, TC-4.5, TC-4.6]
- [ ] `TASK-4.8` ⫘ Security audit — especially that the stub and instruments cannot be reached from outside → `/security-reviewer` [no-test: audit produces its own report]

**Reconcile carry-over for TASK-4.1:** the Phase 3 diagnostic runner currently counts its own
16-instruction device-pump phase. When the central machine and snapshot writer are assembled,
schedule timer and device pumps through `internal/platform/clock`, include that clock's state in
the snapshot, remove the runner-local phase, and preserve the measured oracle cadence. This is
tracked in `gort-lbl.1` and `plan/reconcile-2026-09-17.md`.

## Key patterns
- **A partially-correct snapshot does not fail; it produces a plausible machine.** Restore without
  the demux ring pointers and sections land at the wrong offset; without the I²C transaction state
  the next EEPROM read returns the wrong byte. Every such fault reads as a firmware bug, which is why
  TC-4.2 compares state hashes rather than checking that a file loaded.

## Custom Feature: snapshot, replay and the gdb stub

**Purpose:** The phase that pays for the port. Nothing off the shelf snapshots this machine, and the
inventory was established by spike 003 off the reference emulator's own `reset()`.

**State it covers** — the whole machine, and the acceptance test is what proves it:

| Group | Contents |
|---|---|
| CPU | GPRs, HI/LO, PC, COP0, **the ISA mode bit**, delay-slot state |
| Memory | 32 MB RAM, **both flash chips' command sequencer state** |
| Devices | CSI, I²C/EEPROM, VRAM, UART, smartcard, DMA, blitter, display, demux ring pointers |
| Scheduling | interrupt and timer state, `icount` |

**Interfaces:**
- `Machine.Snapshot(io.Writer) error` / `Restore(io.Reader) error` — via `platform/snapcodec`
- `Recorder.Record(input) ` / `Replay(trace) (frameHash, icount)`
- `GDB.Serve(addr string) error` — **must refuse a non-localhost bind** (ARCH-DEV-1)

**Key patterns (non-obvious):**
- **A partially-correct snapshot does not fail; it produces a plausible machine.** Restore without
  the demux ring pointers and sections land at the wrong offset; without the I²C transaction state
  the next EEPROM read returns the wrong byte. Every such fault reads as a firmware bug — which is
  why the acceptance test is a **state-hash match after running on**, never "the file loaded".
- Every instrument ported here carries the subject-assertion guard from `platform/instrument`: a
  census that cannot find what it counts is a harness failure, never a zero.

**Test checklist:**
- [ ] A device with a deliberately omitted field fails the round-trip
- [ ] Omitting the flash command-sequencer state fails the run-on comparison
- [ ] Two replays of one recording produce identical framebuffer hashes and instruction counts
- [ ] The gdb stub refuses `0.0.0.0`
