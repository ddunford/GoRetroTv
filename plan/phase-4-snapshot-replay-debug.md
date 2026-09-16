# Phase 4: Snapshot, replay and the debugger

## Outcome
**The experiment loop drops from six minutes to seconds.** This is the phase that pays for the port:
everything after it is cheaper, and the three unsolved screens become tractable.

## Overview
Snapshot/restore, deterministic replay and a gdb stub. Every device already implements
`Snapshot`/`Restore` (phase 1) — this phase makes it whole, proves it, and adds the tooling.

## Tasks (mirror — bd epic `gort-4` is the source of truth; never hand-ticked)

- [ ] `TASK-4.1` Snapshot format and writer covering the spike-003 inventory: CPU, 32 MB RAM, COP0, the ISA bit, interrupt/timer state, **both flash chips' command sequencer state**, and every device → `/go-engineer` [TC-4.1]
- [ ] `TASK-4.2` Restore, and the acceptance test that matters: snapshot at N, restore, run to N+10,000,000, and require the state hash to equal an uninterrupted run. **"It restores" is not the test** → `/go-engineer` [TC-4.2]
- [ ] `TASK-4.3` A named snapshot library and `ctl.sh` verbs, with a documented "post-acquisition" snapshot every probe can start from → `/go-engineer` [TC-4.3]
- [ ] `TASK-4.4` Input recording and deterministic replay to a byte-identical framebuffer and identical instruction count → `/go-engineer` [TC-4.4]
- [ ] `TASK-4.5` gdb remote serial protocol stub: registers, memory, breakpoints, watchpoints, step, continue — **bound to localhost only** (ARCH-DEV-1) → `/go-engineer` [TC-4.5]
- [ ] `TASK-4.6` Port the instrument suite: PC histograms, range/read/write watches, call tracing, section injection, and the o-code trace — **each asserting its own subject**, so a census that cannot find what it counts is a harness failure and never a zero → `/go-engineer` [TC-4.6]
- [ ] `TASK-4.7` ⫘ Tests → `/go-engineer` [TC-4.1..TC-4.6]
- [ ] `TASK-4.8` ⫘ Security audit — especially that the stub and instruments cannot be reached from outside → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns
- **A partially-correct snapshot does not fail; it produces a plausible machine.** Restore without
  the demux ring pointers and sections land at the wrong offset; without the I²C transaction state
  the next EEPROM read returns the wrong byte. Every such fault reads as a firmware bug, which is why
  TC-4.2 compares state hashes rather than checking that a file loaded.
