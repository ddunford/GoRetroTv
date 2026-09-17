# Test Plan: Phase 4 — Snapshot, replay and the debugger

## Test Cases
- [x] **TC-4.1: The snapshot covers the inventory** (covers: TASK-4.1, TASK-4.7) — every device's state
  round-trips; a device with a deliberately omitted field must fail. The central instruction
  clock's current count and next device-pump deadline round-trip, with the first post-restore
  pump at the same instruction as an uninterrupted run.
  **Result:** `internal/machine/snapshot_test.go` checks whole-machine roundtrip, exact set
  refusal and rollback after a member rejects restore; `internal/platform/clock/snapshot_test.go`
  checks event deadlines, registration order and corrupt-state refusal; the existing device
  contract tests check each attached device's full state. `cmd/firmwaretrace -snapshot-out`
  wrote a 38.8 MB private snapshot from the real firmware at 100,000 instructions, and the
  clock-driven runner retained 470,000/470,000 cold-oracle checkpoints and 42 tasks. A later
  650M-instruction run exposed a missing child: the I²C-bound demodulator was not bus-attached,
  so its indirect pointer was absent from the machine image. I²C snapshot v2 now includes both
  demodulator and EEPROM child state. `internal/device/i2c/controller_test.go` proves mid-read
  and mid-page-write continuation, refuses old incomplete images, and checks corrupt-child
  atomicity. A fresh v2 image resumes a nine-section SI acquisition to 680M and matches the
  unchanged browser oracle across six samples, nine guest PC counts, four match units and all
  14 demodulator read deltas; the old image missed the first read's register.
- [x] **TC-4.2: Restore is indistinguishable from not having stopped** (covers: TASK-4.2, TASK-4.7) — snapshot
  at N, restore, run to N+10,000,000; the state hash equals an uninterrupted run's. Omitting the
  flash command-sequencer state must fail this.
  **Result:** `./ctl.sh snapshot-gate` saves the real machine at one million retired instructions
  and compares direct versus restored execution at 11 million; both produce the exact state hash
  `B24C2CD2`. `internal/machine/snapshot_test.go` completes a flash program command after a
  machine restore, then deliberately replaces the pending unlock state with the pristine flash
  state and proves the run-on state hash changes.
- [x] **TC-4.3: A post-acquisition snapshot reaches a pressable box in under a second** (covers: TASK-4.3, TASK-4.7) — SPEC success criterion 3.
  **Result:** `./ctl.sh snapshot seed` built a private 38.8 MB v2 image from the real firmware:
  verified cold EEPROM, warm 42-task boot, three TDT/TOT pairs, NIT/BAT/SDT and the finite
  service-list rebuild. The independently seeded image was byte-identical to the earlier named
  `post-acquisition` image (SHA-256 `4d12dab14717b558e3fc0827dbac5ba3fd0528f5491811920cb444b8cae12e0a`).
  Restoring the named image with the built binary took **0.74 seconds**, including firmware
  loading and state-hash calculation; it restored at instruction 1.1B with hash `04E99A24`.
  Sky `0x7D` at that instruction reached guest PC `0x8006EA04` twice and drew the measured
  Box Office surface by 1.12B (raw hash `F3634409`, 37 distinct bytes, final state hash
  `F51114FC`). The seed command refuses an existing name, and both library directory and image
  are private (`0700`/`0600`). Evidence: `.artifacts/si-warm-ready-v2.log`,
  `.artifacts/post-acquisition-named-key.log`, and `docs/snapshots.md`.
- [x] **TC-4.4: Replay is byte-identical** (covers: TASK-4.4, TASK-4.7) — two replays of one recording produce
  identical framebuffer hashes and identical instruction counts.
  **Result:** `./ctl.sh replay-gate` records a Sky key from the real post-acquisition
  snapshot, then replays the trace twice. All three runs retire exactly
  1,120,000,000 guest instructions and produce the same 414,720-byte framebuffer
  (SHA-256 `1bffc82b335571138a8c80c8589a0da52a4dcb317b634183311e2c4a7a3f0b74`,
  raw hash `F3634409`, 37 distinct bytes) and state hash `F51114FC`. A recording
  with an altered expected framebuffer digest fails replay; the decoder also
  rejects malformed input events and trailing JSON. Evidence:
  `.artifacts/sky-key-record.log` and `.artifacts/sky-key-replay-{a,b}.log`.
- [ ] **TC-4.5: gdb attaches and breaks** (covers: TASK-4.5, TASK-4.7) — break on a firmware address, read
  registers, step, continue. And it must **refuse a non-localhost bind**.
- [ ] **TC-4.6: An instrument refuses to report on a wrong state** (covers: TASK-4.6, TASK-4.7) — a census
  whose subject is absent reports a harness failure, not zero.
