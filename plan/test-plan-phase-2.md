# Test Plan: Phase 2 — The CPU core

## Prerequisites
- Phase 1 complete; the oracle emits checkpoints

## Test Cases

- [x] **TC-2.1: Instruction families** (covers: TASK-2.1, TASK-2.10) — table-driven per family including the
  unaligned load/store path; expected values taken from the ISA, not from our own decoder.
  **Result:** `internal/cpu/mips32_test.go` covers arithmetic, shifts, loads/stores, HI/LO, branches and the big-endian unaligned quartet; `./ctl.sh test` passed.
- [ ] **TC-2.2: Delay slots** (covers: TASK-2.2, TASK-2.10) — the slot executes before the transfer, for every
  branch and jump form including the likely/AL variants.
- [ ] **TC-2.3: COP0 survives a context restore** (covers: TASK-2.3, TASK-2.10) — write Status/EPC, restore,
  `ERET`, land where expected. **A stubbed COP0 must fail this.**
- [ ] **TC-2.4: ERET picks its register from ERL** (covers: TASK-2.4, TASK-2.10) — both branches, and the ISA
  bit masked off the address.
- [ ] **TC-2.5: No interrupt in a delay slot** (covers: TASK-2.5, TASK-2.10) — raise an interrupt while the slot
  is pending; expect it deferred by one instruction and the jump still taken. This is the one that
  cost a night: EPC in a delay slot returns to an orphaned instruction and the stack drifts.
- [ ] **TC-2.6: MIPS16 decode and JALX** (covers: TASK-2.6, TASK-2.10) — mode switches both ways; the T register
  set by `cmpi` and by a bare `move $t8`.
- [ ] **TC-2.7: Shift operand order** (covers: TASK-2.7, TASK-2.10) — `sllv $rx,$ry` computes `ry << rx`.
  Reversing it produces a plausible number, so this asserts the value, not the absence of an error.
- [ ] **TC-2.8: Flash command sequencer** (covers: TASK-2.8, TASK-2.10) — autoselect returns the part id; a
  snapshot taken mid-sequence restores mid-sequence.
- [ ] **TC-2.9: Oracle agreement to the peripheral wall** (covers: TASK-2.9, TASK-2.11) — checkpoints match
  until the first access to an unmodelled device, and the divergence names that device.
- [ ] **TC-2.10: Boot reaches the documented handoff** (covers: TASK-2.11) — the bootloader
  decompresses the application to `0x800009F4` and transfers control.
