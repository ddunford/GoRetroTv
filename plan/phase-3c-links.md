# Phase 3c: The serial links and NVRAM

## Outcome
The box takes a key press, remembers its settings, and believes it has a tuner.

## Overview
CSI (the handset), I²C and the EEPROM (NVRAM), the smartcard link, and the satellite demodulator.

## Tasks (mirror — bd epic `gort-f3f` is the source of truth; never hand-ticked)

- [x] `TASK-3c.1` The CSI link and handset frames; `__key(raw, source)`'s equivalent. The 33 documented raw codes, with **sky = 0x7D, tv guide = 0x80, up = 0x58, down = 0x59, left = 0x5A, right = 0x5B, select = 0x5C** → `/go-engineer` [TC-3c.1]
- [x] `TASK-3c.2` I²C plus the EEPROM, persisted to a file. **This is the NVRAM** and its contents are what make a boot warm → `/go-engineer` [TC-3c.2]
- [x] `TASK-3c.3` The peripheral micro's command acknowledgements — which commands are answered is load-bearing: acking everything frees the link but stalls the boot elsewhere → `/go-engineer` [TC-3c.3]
- [x] `TASK-3c.4` The smartcard link, enough for the CA init to proceed → `/go-engineer` [TC-3c.4]
- [x] `TASK-3c.5` The satellite demodulator at I²C `0x18`: indirect register addressing, the microcode upload port, and **always locked** — there is no RF here and the point is to let the firmware open the demux → `/go-engineer` [TC-3c.5]
- [ ] `TASK-3c.6` **Oracle agreement to a full cold boot — 42 Nucleus tasks.** This is SPEC success criterion 1 → `/go-engineer` [TC-3c.6]
- [ ] `TASK-3c.7` ⫘ Tests → `/go-engineer` [TC-3c.1, TC-3c.2, TC-3c.3, TC-3c.4, TC-3c.5]
- [ ] `TASK-3c.8` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]
- [x] `TASK-3c.9` ⫘ Apply the declared oracle handoff after the bootloader reaches idle and the flash header checks; then prove guest instructions decompress the application to `0x800009F4` and enter it → `/go-engineer` [TC-3c.7]

## Key patterns
- **An asynchronous driver has nobody's stack.** Capturing a call chain at an I²C transaction was
  tried twice and both gave `ra=0`: the task that asked is blocked on a semaphore and its frame is on
  neither stack. Diff the PC histogram instead.

## Custom Feature: the serial links and NVRAM

**Purpose:** The handset, the settings store, the smartcard and the tuner. The box will not boot
past CA init without them and will not remember anything without the EEPROM.

**State it owns** (`internal/device/{csi,i2c,eeprom,smartcard,demod}`):

| Field | Shape | Notes |
|---|---|---|
| CSI rx/tx | frame queues + link timing | `__cardRate`'s byte time, in instructions |
| EEPROM | byte array, file-backed | **this is the NVRAM**; its contents are what make a boot warm |
| I²C | bus state + per-device transaction state | restore without it and the next read returns the wrong byte |
| ack policy | set of answered command ids | which commands are answered is load-bearing |
| demodulator | indirect register file at I²C `0x18` | reg 75 bits `0x17`, reg 78 = `0x02` |

**Interfaces:**
- `CSI.Key(raw uint8, source uint8)` — sky `0x7D`, tv guide `0x80`, up `0x58`, down `0x59`,
  left `0x5A`, right `0x5B`, select `0x5C`
- `EEPROM.Persist(path string) error` / `Load` · `Demod.Locked() bool` — always true

**Key patterns (non-obvious, measured):**
- **Acking everything frees the link and stalls the boot elsewhere** (at 19 tasks, in NDS CA init).
  The set that boots is the documented one; this is a policy, not a completeness exercise.
- The demodulator is **always locked**. There is no RF here; the point is to let the firmware open
  the demux.
- **An asynchronous driver has nobody's stack.** Capturing a call chain at an I²C transaction gives
  `ra=0` both times it was tried — the asking task is blocked on a semaphore and its frame is on
  neither stack. Diff the PC histogram instead.

**Test checklist:**
- [ ] Acking everything is shown to change where the boot stops
- [ ] NVRAM survives a full restart
- [ ] The demodulator's polled registers are what the driver actually reads
- [ ] A cold boot reaches 42 tasks and matches the oracle end to end
- [ ] The declared host handoff fires only after the validated idle boundary; guest instructions then decompress the image to `0x800009F4`
