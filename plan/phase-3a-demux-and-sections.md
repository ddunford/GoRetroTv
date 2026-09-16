# Phase 3a: The demux and section delivery

## Outcome
The box acquires SI by itself — the first phase where the emulated machine does something
recognisably Digibox-shaped.

## Overview
Thirty-two section rings, the filter records, the LISR handshake and the interrupt. Every fact here
is measured; `docs/reference/digibox-emulation.md` Part 5 is the specification.

## Tasks (mirror — bd epic `gort-3a` is the source of truth; never hand-ticked)

- [ ] `TASK-3a.1` Section RAM: thirty-two 12 KB rings, filter `f` at `0xA07A0000 + f*0x3000`, and the records at `0x80142D34 + f*20` = `{start, end, current, last-read, context}` → `/go-engineer` [TC-3a.1]
- [ ] `TASK-3a.2` The enable at `+0xD8` is **write-one-to-set**; the status at `+0xB8` is **write-zero-to-clear**. They were modelled the other way round for months — do not re-derive → `/go-engineer` [TC-3a.2]
- [ ] `TASK-3a.3` The LISR handshake: write `0x4000|(f<<2)` to `+0x124`, spin until bit 14 clears, read a 21-bit byte offset from `+0x128`. **`+0x124` must NOT read back its own writes** — making it do so stopped the RTOS starting → `/go-engineer` [TC-3a.3]
- [ ] `TASK-3a.4` PID channel programming (`0x14 + 4*ch`) and the match units. **16 match units against 32 PID channels — never join them by index**; doing so produced a confident artefact → `/go-engineer` [TC-3a.4]
- [ ] `TASK-3a.5` Section injection API, appending the extra byte after each section that the hardware appends (the task advances by `section_length + 4` where DVB's total is `+ 3`) → `/go-engineer` [TC-3a.5]
- [ ] `TASK-3a.6` Demux interrupt wiring and its dispatch row → `/go-engineer` [TC-3a.6]
- [ ] `TASK-3a.7` Oracle comparison through SI acquisition → `/go-engineer` [TC-3a.7]
- [ ] `TASK-3a.8` ⫘ Tests → `/go-engineer` [TC-3a.1..TC-3a.6]
- [ ] `TASK-3a.9` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]
