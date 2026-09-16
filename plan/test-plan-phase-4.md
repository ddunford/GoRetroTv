# Test Plan: Phase 4 — Snapshot, replay and the debugger

## Test Cases
- [ ] **TC-4.1: The snapshot covers the inventory** (covers: TASK-4.1) — every device's state
  round-trips; a device with a deliberately omitted field must fail.
- [ ] **TC-4.2: Restore is indistinguishable from not having stopped** (covers: TASK-4.2) — snapshot
  at N, restore, run to N+10,000,000; the state hash equals an uninterrupted run's. Omitting the
  flash command-sequencer state must fail this.
- [ ] **TC-4.3: A post-acquisition snapshot reaches a pressable box in under a second** (covers:
  TASK-4.3) — SPEC success criterion 3.
- [ ] **TC-4.4: Replay is byte-identical** (covers: TASK-4.4) — two replays of one recording produce
  identical framebuffer hashes and identical instruction counts.
- [ ] **TC-4.5: gdb attaches and breaks** (covers: TASK-4.5) — break on a firmware address, read
  registers, step, continue. And it must **refuse a non-localhost bind**.
- [ ] **TC-4.6: An instrument refuses to report on a wrong state** (covers: TASK-4.6) — a census
  whose subject is absent reports a harness failure, not zero.
