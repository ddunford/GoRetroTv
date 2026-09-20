# Test Plan: Phase 6 — The broadcast

## Test Cases
- [x] **TC-6.1: Sections are accepted** (covers: TASK-6.1, TASK-6.9) — CRC verified by the firmware, not by us.
  **Result:** `internal/broadcast/firmware_test.go` boots the real `FLASH_U202`/`U203` images from the
  post-acquisition snapshot and pushes a built NIT at the guest. Addressed to the network the guest's
  own match unit is asking for, the firmware reads the section and keeps fetching — 37 fetches, accepted.
  With the network id moved by one the section still reaches the parser and is discarded after its
  header — 9 fetches, not accepted — so the test proves the guest's own acceptance rather than ours.
  `internal/dvb/crc_test.go` anchors the CRC against the standard `123456789` check vector, and
  `internal/broadcast/sections_test.go` covers section length, the descriptor loops and MJD/BCD times.
  `go test ./internal/broadcast ./internal/dvb` passes with the private firmware installed and skips
  without it. TASK-6.9's cross-check of the Huffman encoder is a separate case and is not covered here.
- [x] **TC-6.2: The line-up builds service records** (covers: TASK-6.2, TASK-6.9) — N services, 18 bytes apart,
  with the four flag bits unpacked; a gate halfword the guest does not admit must decode no entries.
  *(The case used to say "other than `0xFFFF`". It is corrected rather than ticked as written: the
  box admits `0x0000` too, which nobody had tested — see the Result.)*
  **Result:** `internal/broadcast/lineup_firmware_test.go` pushes a real BAT at the restored
  post-acquisition snapshot and counts the guest's own parser PCs, so "did it decode N services"
  is answered by the firmware rather than by reading our own bytes back. Four services in,
  `0x800BF826` (entry field read) and `0x800BF814` (the `mult` by `0x12` that gives the eighteen-byte
  record stride) each run exactly four times, and so does each of the four flag-unpack stores
  `0x800BF860/6C/78/96` — all four are asserted separately, because three of them firing would look
  like success at any coarser granularity. The bouquet id and network id are read off the box's own
  match unit (unit 3 is `4a/ff 10/ff 00/ff`, so bouquet `0x1000` exactly); choosing them instead is
  how you broadcast a section the hardware never delivers, which from outside is indistinguishable
  from one the box ignored.
  **Negative controls, and two of them corrected the plan.** A gate of `0x1234`, `0x0001` or
  `0xFFFE` decodes nothing; specifier 9 in place of 2 decodes nothing. But sweeping nine gate values
  found **`0x0000` decodes as readily as `0xFFFF`**, by a different path through the firmware, and
  moving the `0x5F` *after* the `0xB1` still decoded everything — so on this box the specifier's
  value is load-bearing and its position is not. Both refine claims the record had drawn from a
  single control each, and both are written up in `docs/reference/digibox-emulation.md → gort-qbn.2`.
  The builder still emits the `0xFFFF` gate with the specifier first.
  **Falsified, not assumed:** reordering the descriptors in the builder was run against the real
  firmware to see whether the test would catch it — it did not, which is how the position finding
  surfaced. `TestOracleLineupDescriptorVector` pins the Go bytes to the untouched oracle's own
  `lineupDescriptors()` output (`5f0400000002b114ffff…`), so the builder matches an implementation
  a box has already been measured accepting. `go test -race ./...`, `ctl.sh lint` and the boot gate
  pass.
- [ ] **TC-6.3: The linkage makes the guide ask and be answered** (covers: TASK-6.3, TASK-6.9) — with it the
  answered arm runs; without it the not-answered arm does. Both directions asserted.
- [ ] **TC-6.4: Twelve records arrive as twelve** (covers: TASK-6.4, TASK-6.9) — and with the length field
  computed openTVtoXML's way, the box must read two. The wrong version has to be shown failing.
- [ ] **TC-6.5: Clock first changes the request** (covers: TASK-6.5, TASK-6.9) — with a clock the box asks for a
  real MJD; without one it asks for 40587.
- [ ] **TC-6.6: Sections are addressed as the box asks** (covers: TASK-6.6, TASK-6.9).
- [ ] **TC-6.7: The in-world clock face matches real time** (covers: TASK-6.7, TASK-6.9) — 1:1, London both ends.
- [ ] **TC-6.8: A live edit reaches the guide; a broken one does not break it** (covers: TASK-6.8, TASK-6.9).
- [ ] **TC-6.9: The guide shows correct now and next** (covers: TASK-6.10) — SPEC success criterion 4.
