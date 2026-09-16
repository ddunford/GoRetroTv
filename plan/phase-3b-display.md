# Phase 3b: The drawing path

## Outcome
The Sky interface appears. The single most visible milestone in the project.

## Overview
Blitter, DMA, VRAM and the OSD registers. The predecessor found two of these the hard way and both
findings are load-bearing.

## Tasks (mirror — bd epic `gort-3b` is the source of truth; never hand-ticked)

- [ ] `TASK-3b.1` VRAM and the OSD/display register block; the framebuffer at `0x80584048`, 720×576 → `/go-engineer` [TC-3b.1]
- [ ] `TASK-3b.2` The blitter: fills and copies. **Bit 24 is the fill bit, not bit 23** — bit-24-clear commands are copies from a source packed at the blit width (stride 480, measured against 1,593 candidates) → `/go-engineer` [TC-3b.2]
- [ ] `TASK-3b.3` The DMA controller at `0xB0009000`: 13 channels, 40-byte descriptors at `0x80108A60 + 40*ch`, completion bits, the write-1-to-clear acknowledge, and `+0x010` which **must read back** because the LISR read-modify-writes it → `/go-engineer` [TC-3b.3]
- [ ] `TASK-3b.4` The plane/window model: the 100-byte records at `*0x80105E9C`, the produce/consume indices at `+0x50`/`+0x54`, the background flag and colour. **Window 0 is a trap** — the validator errors when the id is 0 while the gate reads 0, and the error handler does not return → `/go-engineer` [TC-3b.4]
- [ ] `TASK-3b.5` Palette/CLUT and bit depth (2, 4 or 8 bpp per window) → `/go-engineer` [TC-3b.5]
- [ ] `TASK-3b.6` Oracle comparison to a drawn menu; then compare the **framebuffer** itself, not just checkpoints → `/go-engineer` [TC-3b.6]
- [ ] `TASK-3b.7` ⫘ Tests → `/go-engineer` [TC-3b.1..TC-3b.5]
- [ ] `TASK-3b.8` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns
- **Before the application programs the OSD the panel reads video RAM at a guessed bit depth and
  paints stripes** (predecessor bug `sky-02me.16`). Decide deliberately what to show pre-programming
  rather than inheriting the artefact.
