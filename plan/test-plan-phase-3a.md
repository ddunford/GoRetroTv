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
- [x] **TC-3a.5: A pushed section is read by the firmware** (covers: TASK-3a.5, TASK-3a.8) — including the
  appended byte; without it the task walks off the end of each section.
  **Result:** With the real firmware and the unchanged browser oracle at instruction 469,000,000,
  an 8-byte TDT on PID 0x14 was accepted by the guest-programmed filter 22. In both machines the
  firmware's length reader at `0x800044BC` ran twice and the filter record's last-read pointer
  advanced from `0xA07E2000` to `0xA07E2009` (section plus the hardware's status byte). The
  same 468M Go snapshot without a feed left the pointer unchanged and the reader unexecuted.
  `TestPushWritesSectionAndHardwareByteToBoardDRAM` proves two consecutive sections are separated
  by that byte. Evidence: `.artifacts/oracle-si-tdt-470m.json`,
  `.artifacts/go-si-tdt-470m-even.log`, `.artifacts/go-si-control-470m.log`.
- [x] **TC-3a.6: The demux interrupt reaches its handler** (covers: TASK-3a.6, TASK-3a.8).
  **Result:** For the same TDT, both machines executed the guest LISR `0x800041B4` once, the
  section task `0x80004520` once, and its tail `0x8000494A` once. The oracle recorded one demux
  IRQ; status was acknowledged back to zero in both. Without a feed the Go run hit none of those
  PCs. The odd Ghidra function addresses carry the MIPS16 ISA bit; these PC probes use the even
  executing addresses. Same artefacts as TC-3a.5.
- [x] **TC-3a.7: Oracle agreement through acquisition** (covers: TASK-3a.7) — the box programs the
  same filters as the oracle: TDT, SDT and NIT, enable `0xFFC00000`. Capture the guest demodulator
  read histogram during acquisition and report whether it ever polls lock registers 75/78; the
  Phase 3c cold boot observed neither.
  **Result:** With the real firmware and unmodified `reference/digibox-boot.html?si=0`, three
  TDT/TOT clock pairs at 650M, 654M and 658M instructions followed by NIT at 662M, BAT at 663M
  and SDT at 664M. An uninterrupted Go run from the clean 468M snapshot and the browser oracle
  agreed at all six scheduled instruction samples and at 680M on demux enable/status, armed PIDs,
  filter 22–24 read pointers, nine guest PC counts, all four programmed NIT/BAT/SDT/TOT match
  units, and all 14 demodulator register read deltas. Each register, including lock/identity
  register 11, was read 13 times during acquisition; registers 75 and 78 were never read. Both
  machines armed PID `0x52` after BAT and executed the service-list dispatcher five times.
  The oracle retires a MIPS32 branch pair together, so its 654M and 663M deliveries occurred
  one instruction after the requested boundary; the comparator records requested and actual
  instruction counts and compares the resulting guest states. Wrong-ID NIT at 650M parsed only
  its header; correct network ID `0x0020` at 651M registered the BAT filter in both machines.
  The comparator's negative control, a changed filter 22 read pointer, failed as expected.
  Evidence: `.artifacts/oracle-si-clock-acquisition-verified.json`,
  `.artifacts/go-si-clock-direct.log`, `.artifacts/go-si-650m.log`,
  `.artifacts/oracle-si-id-control.json`, `.artifacts/go-si-id-from-468.log`;
  `python3 tools/compare-si-acquisition.py --oracle .artifacts/oracle-si-clock-acquisition-verified.json
  --go-log .artifacts/go-si-clock-direct.log --go-baseline-log .artifacts/go-si-650m.log
  --require-acquired` passed. A separate snapshot run-on check exposed that the I²C-bound
  demodulator state was omitted from whole-machine snapshots. I²C snapshot v2 now saves it and
  the EEPROM child; the regenerated 650M image also passes this oracle comparison without a
  demodulator histogram difference. The uninterrupted run remains the acquisition proof.
