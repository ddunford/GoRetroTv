# Test Plan: Phase 1 — Foundation

## Prerequisites
- Go 1.22+, Docker, and `firmware/FLASH_U202.bin` present

## Test Cases

- [ ] **TC-1.1: Logging carries emulator time** (covers: TASK-1.2)
  **Steps:** emit a log line from the core after N instructions.
  **Expected:** JSON with both `icount` and a wall timestamp; `icount` matches the machine.

- [ ] **TC-1.2: A device round-trips through snapshot** (covers: TASK-1.5)
  **Steps:** a stub device with internal state → mutate → `Snapshot` → mutate again → `Restore`.
  **Expected:** state equals the snapshot exactly. **And a device that forgets a field must fail
  this** — add one deliberately to prove the test can catch it.

- [ ] **TC-1.3: Memory aliases agree** (covers: TASK-1.6)
  **Steps:** write through `0x80000000`, read through `0xA0000000`, and vice versa.
  **Expected:** identical bytes; the flash region refuses writes.

- [ ] **TC-1.4: A wrong ROM is refused** (covers: TASK-1.7)
  **Steps:** start with a flash image whose checksum does not match the manifest.
  **Expected:** refuses to start, naming the mismatch. Running a different ROM silently is the
  failure this prevents.

- [ ] **TC-1.5: Checkpoints are deterministic and cover the ISA bit** (covers: TASK-1.8, TASK-1.9)
  **Steps:** run the same program twice; then run it with the ISA mode bit flipped at a known point.
  **Expected:** identical streams for the first pair; a differing checkpoint for the second.

- [ ] **TC-1.6: The comparison localises an injected divergence** (covers: TASK-1.10)
  **Steps:** corrupt one register at instruction 4,500,000 in one stream.
  **Expected:** reports the divergence in the 4,500,000–4,501,000 window. **And over an empty or
  truncated stream it must report a harness failure, not success.**

- [ ] **TC-1.7: The boot gate can go red** (covers: TASK-1.11)
  **Steps:** run the gate against a deliberately broken build.
  **Expected:** non-zero exit and a readable reason. A gate nobody has seen fail is decoration.
