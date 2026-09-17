# Test Plan: Phase 2 — The CPU core

## Prerequisites
- Phase 1 complete; the oracle emits checkpoints

## Test Cases

- [x] **TC-2.1: Instruction families** (covers: TASK-2.1, TASK-2.10) — table-driven per family including the
  unaligned load/store path; expected values taken from the ISA, not from our own decoder.
  **Result:** `internal/cpu/mips32_test.go` covers arithmetic, shifts, loads/stores, HI/LO, branches and the big-endian unaligned quartet; `./ctl.sh test` passed.
- [x] **TC-2.2: Delay slots** (covers: TASK-2.2, TASK-2.10) — the slot executes before the transfer, for every
  branch and jump form including the likely/AL variants.
  **Result:** `internal/cpu/branch_test.go` drives branch, jump, likely, link and JALX forms and verifies both instruction addresses; a negative control that hid a pending branch failed.
- [x] **TC-2.3: COP0 survives a context restore** (covers: TASK-2.3, TASK-2.10) — write Status/EPC, restore,
  `ERET`, land where expected. **A stubbed COP0 must fail this.**
  **Result:** `internal/cpu/cop0_test.go` proves register writes/readback and Compare's timer request; `internal/cpu/eret_test.go` writes Status/EPC and executes ERET. Stubbed COP0 read and wrong ERET return mutations fail.
- [x] **TC-2.4: ERET picks its register from ERL** (covers: TASK-2.4, TASK-2.10) — both branches, and the ISA
  bit masked off the address.
  **Result:** `internal/cpu/eret_test.go` covers EPC, ErrorEPC, ISA bit and no delay slot; wrong-register and wrong-slot mutations fail.
- [x] **TC-2.5: No interrupt in a delay slot** (covers: TASK-2.5, TASK-2.10) — raise an interrupt while the slot
  is pending; expect it deferred by one instruction and the jump still taken. This is the one that
  cost a night: EPC in a delay slot returns to an orphaned instruction and the stack drifts.
  **Result:** `internal/cpu/interrupt_test.go` raises IP2 between branch and slot and checks EPC at the target, vector selection, timer request persistence and Status gates. Mutations permitting slot delivery or ignoring IE/IM fail.
- [x] **TC-2.6: MIPS16 decode and JALX** (covers: TASK-2.6, TASK-2.10) — mode switches both ways; the T register
  set by `cmpi` and by a bare `move $t8`.
  **Result:** `internal/cpu/mips16_test.go` executes immediate branches, `cmpi`, MOV32R to T, extended LI, MIPS16 JALX into MIPS32 and rejection of MIPS16e SAVE/RESTORE. `internal/cpu/mips16_families_test.go` covers arithmetic, T compare, shifts, loads and stores in tables. `internal/cpu/snapshot_test.go` verifies complete CPU state and malformed-state rejection. `./ctl.sh test` passed.
- [x] **TC-2.7: Shift operand order** (covers: TASK-2.7, TASK-2.10) — `sllv $rx,$ry` computes `ry << rx`.
  Reversing it produces a plausible number, so this asserts the value, not the absence of an error.
  **Result:** `TestMIPS16VariableShiftOperandOrder` asserts `5 << 3 == 40` in the destination and preserves the amount register; `./ctl.sh test` passed.
- [x] **TC-2.8: Flash command sequencer** (covers: TASK-2.8, TASK-2.10) — autoselect returns the part id; a
  snapshot taken mid-sequence restores mid-sequence.
  **Result:** `internal/memory/flash_command_test.go` verifies per-chip IDs, program bit clearing, bottom-boot sector erase, and restoring the array plus an in-progress unlock. `TestFlashHoldsTheDeviceContract` covers all mutable fields; `./ctl.sh test` passed.
- [x] **TC-2.9: Oracle agreement to the peripheral wall** (covers: TASK-2.9, TASK-2.11) — checkpoints match
  until the first access to an unmodelled device, and the divergence names that device.
  **Result:** `docs/reference/cpu-oracle-measurement.md` records browser and Go checkpoint runs. The first differing value is a read from the unmodelled video RAM data port `0xB00020B0` at instruction 3,204,424; PC and registers agree before it.
- [x] **TC-2.10: Built-binary firmware integration** (covers: TASK-2.11) — the built Go runner
  loads verified firmware, matches independent oracle checkpoint anchors through instruction
  3,204,400, and names the first unmodelled video RAM read at instruction 3,204,424. The
  integration check fails if the anchors or peripheral-wall diagnosis change. The guest-driven
  application handoff is retained as TC-3c.7 after the required device models exist.
  **Result:** `tools/cpu-gate.sh` builds and runs `cmd/firmwaretrace` with the verified local firmware. Eight anchors from `tests/fixtures/cpu-oracle-anchors.txt` agree through instruction 3,204,400; the MIPS16 load at `0xBFC0AF5A` names the unmapped video RAM port, and the first divergent sampled state is 3,204,500. `./ctl.sh cpu-gate` passed. A corrupted oracle anchor made the gate fail with exit 1.
