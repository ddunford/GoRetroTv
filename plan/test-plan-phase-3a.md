# Test Plan: Phase 3a — Demux and section delivery

## Test Cases
- [x] **TC-3a.1: Ring geometry** (covers: TASK-3a.1, TASK-3a.8) — filter f's ring is where the firmware expects.
  **Result:** `internal/device/demux/section_test.go` asserts first, middle and last ring and five-word record addresses, bus aliasing, reset and snapshot restore; `./ctl.sh test` passed.
- [x] **TC-3a.2: Enable and status semantics** (covers: TASK-3a.2, TASK-3a.8) — write-one-to-set and
  write-zero-to-clear, each asserted in the direction it actually works. Swapping them must fail.
  **Result:** `internal/device/demux/registers_test.go` verifies cumulative enable bits, complement
  acknowledgement, reset and snapshot restore; `./ctl.sh test` passed.
- [x] **TC-3a.3: The LISR handshake completes** (covers: TASK-3a.3, TASK-3a.8) — and a register file that reads
  back its own writes at `+0x124` must hang, proving the model is the working one.
  **Result:** `TestLISRPointerHandshake` exercises two selected filter pointers and a bounded LISR
  spin; an overlay that echoes the busy command fails at the spin limit. `./ctl.sh test` passed.
- [x] **TC-3a.4: PID channels and match units are separate index spaces** (covers: TASK-3a.4, TASK-3a.8).
  **Result:** `TestPIDChannelsAndMatchUnitsAreIndependent` programs PID channel 22 while match
  unit 22 cannot exist, then programs unit 0 and verifies the armed PIDs are unchanged. Snapshot
  coverage, `./ctl.sh test` and `./ctl.sh lint` passed.
- [ ] **TC-3a.5: A pushed section is read by the firmware** (covers: TASK-3a.5, TASK-3a.8) — including the
  appended byte; without it the task walks off the end of each section.
- [ ] **TC-3a.6: The demux interrupt reaches its handler** (covers: TASK-3a.6, TASK-3a.8).
- [ ] **TC-3a.7: Oracle agreement through acquisition** (covers: TASK-3a.7) — the box programs the
  same filters as the oracle: TDT, SDT and NIT, enable `0xFFC00000`. Capture the guest demodulator
  read histogram during acquisition and report whether it ever polls lock registers 75/78; the
  Phase 3c cold boot observed neither.

**Guest acceptance dependency:** TC-3a.5–3a.7 remain open until the real application handoff in
TASK-3c.9. The hardware boundary already has committed tests in
`internal/device/demux/push_test.go` and `internal/device/irq/controller_test.go`; the CPU gate
matches eight oracle anchors through instruction 3,204,400 and identifies the video RAM read at
3,204,424 as the current wall. Those results do not prove guest section consumption or handler entry.
