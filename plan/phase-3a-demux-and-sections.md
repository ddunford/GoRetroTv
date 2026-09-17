# Phase 3a: The demux and section delivery

## Outcome
The box acquires SI by itself — the first phase where the emulated machine does something
recognisably Digibox-shaped.

## Overview
Thirty-two section rings, the filter records, the LISR handshake and the interrupt. Every fact here
is measured; `docs/reference/digibox-emulation.md` Part 5 is the specification.

**Execution dependency discovered 2026-09-17:** the Go CPU reaches an unmodelled video RAM read at
instruction 3,204,424, before the application programs section filters. Phase 3a's hardware model,
unit tests and security review can finish first, allowing Phase 3b to model video RAM. Guest firmware
consumption, handler entry and SI oracle comparison stay in TASK-3a.7/TC-3a.5–3a.7 and depend on
Phase 3c's real guest-driven handoff (TASK-3c.9); they are not inferred from unit tests.

## Tasks (mirror — bd epic `gort-l14` is the source of truth; never hand-ticked)

- [x] `TASK-3a.1` Section RAM: thirty-two 12 KB rings, filter `f` at `0xA07A0000 + f*0x3000`, and the records at `0x80142D34 + f*20` = `{start, end, current, last-read, context}` → `/go-engineer` [TC-3a.1]
- [x] `TASK-3a.2` The enable at `+0xD8` is **write-one-to-set**; the status at `+0xB8` is **write-zero-to-clear**. They were modelled the other way round for months — do not re-derive → `/go-engineer` [TC-3a.2]
- [x] `TASK-3a.3` The LISR handshake: write `0x4000|(f<<2)` to `+0x124`, spin until bit 14 clears, read a 21-bit byte offset from `+0x128`. **`+0x124` must NOT read back its own writes** — making it do so stopped the RTOS starting → `/go-engineer` [TC-3a.3]
- [x] `TASK-3a.4` PID channel programming (`0x14 + 4*ch`) and the match units. **16 match units against 32 PID channels — never join them by index**; doing so produced a confident artefact → `/go-engineer` [TC-3a.4]
- [x] `TASK-3a.5` Section injection API, appending the extra byte after each section that the hardware appends (the task advances by `section_length + 4` where DVB's total is `+ 3`) → `/go-engineer` [TC-3a.5]
- [x] `TASK-3a.6` Demux interrupt wiring and its dispatch row → `/go-engineer` [TC-3a.6]
- [x] `TASK-3a.7` Oracle comparison through SI acquisition → `/go-engineer` [TC-3a.7]
- [x] `TASK-3a.8` ⫘ Demux unit and bus integration tests → `/go-engineer` [TC-3a.1, TC-3a.2, TC-3a.3, TC-3a.4, TC-3a.5, TC-3a.6]
- [x] `TASK-3a.9` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Custom Feature: the section demux

**Purpose:** The hardware that decides which DVB sections the firmware ever sees. Nothing models it;
every fact below was measured off the running box.

**State it owns** (`internal/device/demux`):

| Field | Shape | Notes |
|---|---|---|
| rings | 32 x 12 KB at `0xA07A0000 + f*0x3000` | filter `f`'s buffer |
| records | 32 x `{start, end, current, lastRead, context}` at `0x80142D34 + f*20` | the firmware's view |
| `enable` | uint32 at `+0xD8` | **write-one-to-set** |
| `status` | uint32 at `+0xB8` | **write-zero-to-clear** — the LISR's complement write is what clears it |
| PID channels | 32, at `0x14 + 4*ch` | the channel index **is** the filter index |
| match units | 16 | **16 units against 32 channels — never join them by index** |

**Interfaces:**
- `Demux.Push(pid uint16, section []byte) error` — appends the extra byte the hardware appends
- `Demux.ArmedPIDs() []uint16` — what the box is actually asking for, for instruments to read

**Key patterns (non-obvious, measured):**
- **`+0x124` must not read back its own writes.** The LISR writes `0x4000|(f<<2)` and spins for bit
  14 to clear; a register file that echoes its writes is an infinite loop, and making it "correct"
  stopped the RTOS starting at all.
- The task advances by `section_length + 4` where DVB's total is `+ 3`, so **the hardware appends a
  byte after each section**. Write one or the reader walks off the end of every section.
- A PID and a filter index cannot share an argument: PID `0x0014` is 20, which is also a real filter.

**Acquisition observed (`gort-l14.7`):** With the real firmware and the unchanged browser oracle,
clock sections followed by NIT, BAT and SDT made both machines register the same four match units
and arm transient PID `0x52` after BAT. A wrong-network-ID NIT parsed only its header; network ID
`0x0020` registered the BAT filter. The six sampled guest states agreed through instruction 680M,
including section read pointers and nine guest PC counts. The diagnostic feeder supplied these
sections at recorded instruction counts; autonomous carousel delivery remains Phase 6 work.

**Test checklist:**
- [ ] Swapping set/clear semantics on `+0xD8`/`+0xB8` fails
- [ ] A `+0x124` that echoes its writes hangs, proving the model is the working one
- [ ] Joining match units to channels by index is shown to produce the artefact it produced before
- [ ] Omitting the appended byte fails section delivery
