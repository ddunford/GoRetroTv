# Test Plan: Phase 3c — Links and NVRAM

## Test Cases
- [?] **TC-3c.1: A key press reaches the input layer** (covers: TASK-3c.1, TASK-3c.7) — the dispatcher and key
  event counters both move, as the predecessor's gate asserts.
  The CSI register, paced receive byte, interrupt acknowledge, and escaped handset frame are
  covered by `internal/device/csi/link_test.go`; guest dispatcher and event counters still need a
  completed application boot before this case can pass. **Blocked:** the real application handoff
  and guest dispatcher are not yet available in the Go trace; reopen after TASK-3c.9/3c.6.
- [x] **TC-3c.2: NVRAM survives a restart** (covers: TASK-3c.2, TASK-3c.7) — write, stop, start, read back.
  **Result:** `internal/device/i2c/controller_test.go` writes EEPROM bytes over I²C, stops,
  constructs a new controller and store, loads the image from disk, and reads the same bytes
  over a fresh I²C transaction. `internal/device/eeprom/store_test.go` checks the file image
  independently. The I²C test also proves START alone raises no interrupt, while address-byte
  completion does and `+0x50` clears it.
- [?] **TC-3c.3: Acknowledgement policy** (covers: TASK-3c.3, TASK-3c.7) — the documented set boots; acking
  everything is shown to change where the boot stops, proving the model is the working one.
  `internal/device/csi/peripheral_test.go` proves the default set answers only 0x52 and 0x18,
  echoes the guest's sequence, and that acknowledging everything changes the wire replies.
  **Blocked:** comparing the resulting 42-task and 19-task guest boots requires the real
  application handoff and remaining Phase 3c devices. Reopen after TASK-3c.9/3c.6.
- [?] **TC-3c.4: CA init proceeds past the card** (covers: TASK-3c.4, TASK-3c.7).
  `internal/device/smartcard/port_test.go` drives the real six-byte boot command through six
  separate TX completion interrupts, checks interrupt acknowledgement and the paced six-byte
  empty-slot response. **Blocked:** proving that guest CA init advances requires the real
  application handoff and complete Phase 3c boot; reopen after TASK-3c.9/3c.6.
- [x] **TC-3c.5: The demodulator reports locked** (covers: TASK-3c.5, TASK-3c.7) — register 75 with bits 0x17
  set and register 78 = 0x02, which is what the driver polls.
  **Result:** `internal/device/demod/model_test.go` checks the measured lock, identity, BER and
  register-11 responses plus a 1 KiB port-5 microcode upload without changing readback;
  `internal/device/i2c/controller_test.go` sets the indirect register pointer through the
  vbus-0 I²C path and reads register 75 as 0x17 through the controller data register.
- [ ] **TC-3c.6: A full cold boot matches the oracle** (covers: TASK-3c.6) — 42 tasks, checkpoints
  matching end to end. **SPEC success criterion 1.**
- [ ] **TC-3c.7: Real application handoff** (covers: TASK-3c.9) — the bootloader decompresses
  the application to `0x800009F4` and transfers control by executing guest instructions. The
  test must fail if a host-side forced PC change or image copy is substituted for the handoff.
  **Open evidence:** the ROM demux self-test now passes through a guest-driven DMA channel-5
  transport transfer, setting `[0x800081F8]` to zero and flash scan step `[0x800050BC]` to
  `0x10000` without host mutation. BOOTMain then waits for CSI command `0x44`; a diagnostic
  synthetic peripheral reply reaches scanner PC `0x9FC122B6`. Both JB image scans and CRCs pass;
  BOOTMain enters a sleep/service loop and reaches the browser oracle's idle handoff boundary
  (`ready=0x100`, `current=0`) by 100 million
  instructions. The guest still never reaches flash entry `0x9FC2048C` or loader `0x9FC20618`,
  and the application image at `0x800009F4` remains zero. See `gort-f3f.10` for trace PCs.
