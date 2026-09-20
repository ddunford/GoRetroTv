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
- [ ] **TC-6.2: The line-up builds service records** (covers: TASK-6.2, TASK-6.9) — N services, 18 bytes apart,
  with the four flag bits unpacked; a gate halfword other than `0xFFFF` must decode no entries.
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
