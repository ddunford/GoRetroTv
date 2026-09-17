# Test Plan: Phase 3c — Links and NVRAM

## Test Cases
- [ ] **TC-3c.1: A key press reaches the input layer** (covers: TASK-3c.1, TASK-3c.7) — the dispatcher and key
  event counters both move, as the predecessor's gate asserts.
- [ ] **TC-3c.2: NVRAM survives a restart** (covers: TASK-3c.2, TASK-3c.7) — write, stop, start, read back.
- [ ] **TC-3c.3: Acknowledgement policy** (covers: TASK-3c.3, TASK-3c.7) — the documented set boots; acking
  everything is shown to change where the boot stops, proving the model is the working one.
- [ ] **TC-3c.4: CA init proceeds past the card** (covers: TASK-3c.4, TASK-3c.7).
- [ ] **TC-3c.5: The demodulator reports locked** (covers: TASK-3c.5, TASK-3c.7) — register 75 with bits 0x17
  set and register 78 = 0x02, which is what the driver polls.
- [ ] **TC-3c.6: A full cold boot matches the oracle** (covers: TASK-3c.6) — 42 tasks, checkpoints
  matching end to end. **SPEC success criterion 1.**
- [ ] **TC-3c.7: Real application handoff** (covers: TASK-3c.9) — the bootloader decompresses
  the application to `0x800009F4` and transfers control by executing guest instructions. The
  test must fail if a host-side forced PC change or image copy is substituted for the handoff.
