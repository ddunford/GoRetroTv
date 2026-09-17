# Phase 3b: The drawing path

## Outcome
The Sky interface appears. The single most visible milestone in the project.

## Overview
Blitter, DMA, VRAM and the OSD registers. The predecessor found two of these the hard way and both
findings are load-bearing.

**Execution dependency discovered 2026-09-17:** the drawing hardware and its unit/integration
tests can finish before a real application menu exists. The complete drawn-menu framebuffer
comparison remains TASK-3b.6/TC-3b.6 and depends on Phase 3c's 42-task cold boot and handset
link. TASK-3b.7 and the security review cover the drawing hardware now, allowing Phase 3c to
start without claiming the menu was seen.

## Tasks (mirror — bd epic `gort-omj` is the source of truth; never hand-ticked)

- [x] `TASK-3b.1` VRAM and the OSD/display register block; the framebuffer at `0x80584048`, 720×576 → `/go-engineer` [TC-3b.1]
- [x] `TASK-3b.2` The blitter: fills and copies. **Bit 24 is the fill bit, not bit 23** — bit-24-clear commands are copies from a source packed at the blit width (stride 480, measured against 1,593 candidates) → `/go-engineer` [TC-3b.2]
- [x] `TASK-3b.3` The DMA controller at `0xB0009000`: 13 channels, 40-byte descriptors at `0x80108A60 + 40*ch`, completion bits, the write-1-to-clear acknowledge, and `+0x010` which **must read back** because the LISR read-modify-writes it → `/go-engineer` [TC-3b.3]
- [x] `TASK-3b.4` The plane/window model: the 100-byte records at `*0x80105E9C`, the produce/consume indices at `+0x50`/`+0x54`, the background flag and colour. **Window 0 is a trap** — the validator errors when the id is 0 while the gate reads 0, and the error handler does not return → `/go-engineer` [TC-3b.4]
- [x] `TASK-3b.5` Palette/CLUT and bit depth (2, 4 or 8 bpp per window) → `/go-engineer` [TC-3b.5]
- [ ] `TASK-3b.6` Oracle comparison to a drawn menu; then compare the **framebuffer** itself, not just checkpoints → `/go-engineer` [TC-3b.6]
- [ ] `TASK-3b.7` ⫘ Drawing hardware integration tests → `/go-engineer` [TC-3b.1, TC-3b.2, TC-3b.3, TC-3b.4, TC-3b.5]
- [ ] `TASK-3b.8` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns
- **Before the application programs the OSD the panel reads video RAM at a guessed bit depth and
  paints stripes** (predecessor bug `sky-02me.16`). Decide deliberately what to show pre-programming
  rather than inheriting the artefact.

## Custom Feature: the drawing path

**Purpose:** The Pace/ST ASIC's blitter, DMA and OSD. No public manual covers any of it — the NEC
manual has zero hits for CLUT, palette, MPEG, demux, video encoder or framebuffer.

**State it owns** (`internal/device/{blitter,dma,osd}`):

| Field | Shape | Notes |
|---|---|---|
| VRAM + framebuffer | 720x576 at `0x80584048` | |
| blitter command | src, dst, w, h, flags | **bit 24 is the fill bit, not bit 23** |
| DMA descriptors | 13 x 40 bytes at `0x80108A60 + 40*ch` | at `0xB0009000` |
| DMA `+0x010` | uint32 | **must read back** — the LISR read-modify-writes it |
| plane/window records | 100 bytes each at `*0x80105E9C` | produce/consume indices at `+0x50`/`+0x54` |
| CLUT | per window, 2/4/8 bpp | |

**Interfaces:**
- `Blitter.Execute(cmd) error` · `DMA.Run(ch int) error` · `OSD.Compose() *image.Paletted`
- `Display.FrameHash() uint32` — via `platform/statehash`, so the framebuffer comparison and the
  oracle checkpoint use one definition

**Key patterns (non-obvious, measured):**
- A bit-24-clear command is a **copy from a source packed at the blit width** (stride 480, measured
  against 1,593 candidates), not a fill of something else.
- **Window 0 is a trap:** the validator errors when the id is 0 while the gate reads 0, and the error
  handler does not return. Model the refusal, do not wedge.
- Before the application programs the OSD, the panel reads video RAM at a guessed bit depth and
  paints stripes. Decide deliberately what to show pre-programming rather than inheriting that.

**Test checklist:**
- [ ] Asserting bit 23 as the fill bit fails TC-3b.2
- [ ] `+0x010` not reading back corrupts the enable set via the LISR's read-modify-write
- [ ] Driving window 0 while the gate reads 0 does not wedge the machine
- [ ] The drawn menu's framebuffer hash equals the oracle's at the same instruction count
