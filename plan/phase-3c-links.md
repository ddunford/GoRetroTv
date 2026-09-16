# Phase 3c: The serial links and NVRAM

## Outcome
The box takes a key press, remembers its settings, and believes it has a tuner.

## Overview
CSI (the handset), I²C and the EEPROM (NVRAM), the smartcard link, and the satellite demodulator.

## Tasks (mirror — bd epic `gort-3c` is the source of truth; never hand-ticked)

- [ ] `TASK-3c.1` The CSI link and handset frames; `__key(raw, source)`'s equivalent. The 33 documented raw codes, with **sky = 0x7D, tv guide = 0x80, up = 0x58, down = 0x59, left = 0x5A, right = 0x5B, select = 0x5C** → `/go-engineer` [TC-3c.1]
- [ ] `TASK-3c.2` I²C plus the EEPROM, persisted to a file. **This is the NVRAM** and its contents are what make a boot warm → `/go-engineer` [TC-3c.2]
- [ ] `TASK-3c.3` The peripheral micro's command acknowledgements — which commands are answered is load-bearing: acking everything frees the link but stalls the boot elsewhere → `/go-engineer` [TC-3c.3]
- [ ] `TASK-3c.4` The smartcard link, enough for the CA init to proceed → `/go-engineer` [TC-3c.4]
- [ ] `TASK-3c.5` The satellite demodulator at I²C `0x18`: indirect register addressing, the microcode upload port, and **always locked** — there is no RF here and the point is to let the firmware open the demux → `/go-engineer` [TC-3c.5]
- [ ] `TASK-3c.6` **Oracle agreement to a full cold boot — 42 Nucleus tasks.** This is SPEC success criterion 1 → `/go-engineer` [TC-3c.6]
- [ ] `TASK-3c.7` ⫘ Tests → `/go-engineer` [TC-3c.1..TC-3c.5]
- [ ] `TASK-3c.8` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns
- **An asynchronous driver has nobody's stack.** Capturing a call chain at an I²C transaction was
  tried twice and both gave `ra=0`: the task that asked is blocked on a semaphore and its frame is on
  neither stack. Diff the PC histogram instead.
