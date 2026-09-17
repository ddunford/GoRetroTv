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
- [x] **TC-3b.3: A DMA transfer completes and acknowledges** (covers: TASK-3b.3, TASK-3b.7) — and `+0x010` reads
  back, without which the LISR's read-modify-write corrupts the enable set.
  **Result:** `internal/device/dma/controller_test.go` drives descriptor channel 12 through the
  blitter and row-two IP2, checks halfword completion, enable readback and write-one-to-clear
  acknowledgement; channel 8 uploads a plane and bootloader channel 5 completes without IP2.
  A zero-readback mutant fails the test.
- [x] **TC-3b.4: Window 0 is refused safely** (covers: TASK-3b.4, TASK-3b.7) — driving window 0 while the gate
  reads 0 must not wedge the machine.
  **Result:** `internal/device/osd/windows_test.go` builds two firmware-owned 100-byte records in
  DRAM, verifies window 0 returns an immediate error with gate zero, reads the visible window's
  depth, indices and background fields without mutation, and rejects an out-of-bounds table.
- [x] **TC-3b.5: Bit depths render** (covers: TASK-3b.5, TASK-3b.7) — 2, 4 and 8 bpp against known pixels.
  **Result:** `internal/device/osd/compose_test.go` programs field buffers at each depth, checks
  known pixels in both interlaced fields, decodes a packed CLUT entry, and checks the unprogrammed
  display returns black. `TestComposeAdvancesPastPartialByteAtEndOfRow` proved the former floor
  stride wrong for two-row, non-byte-aligned 1/2/4bpp surfaces and passes with the rounded-up
  byte stride. OSD package and race tests pass.
- [x] **TC-3b.6: The menu matches the oracle** (covers: TASK-3b.6) — press sky; the framebuffer hash
  equals the oracle's for the same instruction count. 37 distinct surface bytes is the known-good shape.
  **Result:** a 470M-instruction cold boot produced a persistent 16 KiB EEPROM image and 42 guest
  tasks. Both `tools/oracle-warm-surface.mjs` and `cmd/firmwaretrace` loaded copies of that same
  image, queued raw Sky key `0x7D` at instruction 200,000,000, and stopped at exactly 230,000,000.
  Before the key both raw 720×576 DRAM surfaces hashed to FNV-1a `9825B318` with 12 distinct bytes;
  after the key both hashed to `F3634409` with 37. The oracle made 11 blits after the key; Go reached
  the guest input-event PC `0x8006EA04` twice. The oracle screenshot in
  `.artifacts/oracle-warm-surface.png` was inspected and shows the Box Office menu with six readable
  rows. Both raw framebuffers also have SHA-256
  `1bffc82b335571138a8c80c8589a0da52a4dcb317b634183311e2c4a7a3f0b74`.
  The capture report is `.artifacts/oracle-warm-surface.json`; source oracle SHA-256 and EEPROM
  SHA-256 are recorded there. The key path required CSI receive pacing on the guest transmit receipt,
  verified by `internal/device/csi/link_test.go`.
