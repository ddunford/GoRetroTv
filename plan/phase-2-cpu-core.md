# Phase 2: The CPU core

## Outcome
The Go machine executes the real firmware and **agrees with the oracle**, instruction for
instruction, as far as the absence of peripherals allows. First real proof the port is faithful.

## Overview
NEC VR4111: MIPS32, MIPS16 via `JALX`, COP0, exceptions and interrupts. The measured reference is
`docs/reference/digibox-emulation.md` Part 1 and `digibox-emulator-skill.md` — **read them before
writing the decoder**; they record the traps, each of which cost the predecessor real time.

## Tasks (mirror — bd epic `gort-2` is the source of truth; never hand-ticked)

- [ ] `TASK-2.1` MIPS32 decode and execute: ALU, shifts, loads/stores including the unaligned `lwl`/`lwr`/`swl`/`swr` that memset's path uses, branches, jumps, HI/LO with `MTHI`/`MTLO` → `/go-engineer` [TC-2.1]
- [ ] `TASK-2.2` Delay slots modelled as hardware does: a jump arms the slot and the slot executes before the transfer → `/go-engineer` [TC-2.2]
- [ ] `TASK-2.3` COP0: Count, Compare (writing it clears the interrupt request), Status, Cause, EPC, ErrorEPC, BadVAddr, Config. **Stubbing COP0 destroys every context restore** → `/go-engineer` [TC-2.3]
- [ ] `TASK-2.4` `ERET` picking its return register from `Status.ERL`, with no delay slot, and bit 0 of EPC carrying the ISA mode → `/go-engineer` [TC-2.4]
- [ ] `TASK-2.5` Interrupt delivery and the vector; **an interrupt must not be taken in a branch delay slot** — defer by one instruction (the predecessor lost a night to this) → `/go-engineer` [TC-2.5]
- [ ] `TASK-2.6` MIPS16 decode: the original ASE, not MIPS16e — no `SAVE`/`RESTORE`. The implicit T register, `JALX` mode switching, and the extended forms → `/go-engineer` [TC-2.6]
- [ ] `TASK-2.7` MIPS16 shift operand order — the FIRST printed operand is the shift amount; getting it backwards yields a plausible wrong answer rather than an error → `/go-engineer` [TC-2.7]
- [ ] `TASK-2.8` Flash device: read paths plus the command sequencer (autoselect, program, erase) with its state, which is real state a snapshot must carry → `/go-engineer` [TC-2.8]
- [ ] `TASK-2.9` Run the oracle comparison from reset and drive the first divergence to zero, iterating until the two agree as far as missing peripherals permit → `/go-engineer` [TC-2.9]
- [ ] `TASK-2.10` ⫘ Write the CPU test suite, table-driven per instruction family → `/go-engineer` [TC-2.1..TC-2.8]
- [ ] `TASK-2.11` ⫘ Write and run the phase's integration check: boot to the documented handoff point → `/qa-test-engineer` [TC-2.9, TC-2.10]
- [ ] `TASK-2.12` ⫘ OWASP security audit of phase 2 → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns (from the measured record — do not re-derive)
- **MAME cannot decode MIPS16 and QEMU has no board.** There is no prior art to borrow; the
  reference is the NEC manual plus the predecessor's findings.
- **One deliberate inaccuracy is inherited on purpose:** Count advances per instruction, not per
  clock. Monotonic but wrongly scaled — enough for ordering, not for measuring real time.
