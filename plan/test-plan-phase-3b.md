# Test Plan: Phase 3b — The drawing path

## Test Cases
- [x] **TC-3b.1: Framebuffer geometry and addressing** (covers: TASK-3b.1, TASK-3b.7).
  **Result:** `internal/device/osd/video_test.go` verifies the 720×576 DRAM framebuffer alias,
  1 MB video RAM port readback, auto-increment, wrap and plane-register readback. The real firmware
  reproduced nine unchanged-browser checkpoints through the video RAM read at instruction 3,204,424
  (`./ctl.sh cpu-gate`).
- [x] **TC-3b.2: Fill versus copy** (covers: TASK-3b.2, TASK-3b.7) — a bit-24-set command fills; a bit-24-clear
  command copies at the blit width (480 pixels for the measured menu command). Asserting bit 23 instead must fail this test.
  **Result:** `internal/device/blitter/blitter_test.go` verifies 8-bit fills, packed-source copy
  with a source stride of the blit width rather than destination pitch, 16-bit cell fill, planar
  chroma copy and snapshot restore. A bit-23 mutant fails the copy assertion.
- [ ] **TC-3b.3: A DMA transfer completes and acknowledges** (covers: TASK-3b.3, TASK-3b.7) — and `+0x010` reads
  back, without which the LISR's read-modify-write corrupts the enable set.
- [ ] **TC-3b.4: Window 0 is refused safely** (covers: TASK-3b.4, TASK-3b.7) — driving window 0 while the gate
  reads 0 must not wedge the machine.
- [ ] **TC-3b.5: Bit depths render** (covers: TASK-3b.5, TASK-3b.7) — 2, 4 and 8 bpp against known pixels.
- [ ] **TC-3b.6: The menu matches the oracle** (covers: TASK-3b.6) — press sky; the framebuffer hash
  equals the oracle's for the same instruction count. 62 widgets, 37 colours is the known-good shape.
