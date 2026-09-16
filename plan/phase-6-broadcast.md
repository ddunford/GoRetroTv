# Phase 6: The broadcast

## Outcome
The box acquires a line-up and shows real 1998 programmes on its guide, having asked for them itself.

## Overview
The modelled multiplex. Every rung of this was measured by the predecessor and must not be
re-derived — `docs/reference/digibox-emulation.md` and `digibox-next-session.md` are the spec.

## Tasks (mirror — bd epic `gort-6` is the source of truth; never hand-ticked)

- [ ] `TASK-6.1` Section builders: NIT, SDT, TDT, TOT, with MPEG CRC-32 (poly `0x04C11DB7`, init all ones, no final inversion) → `/go-engineer` [TC-6.1]
- [ ] `TASK-6.2` The BAT with the `0x5F` private-data-specifier (value **2**) ahead of a `0xB1` line-up descriptor whose gate halfword must be `0xFFFF` and whose entries are nine bytes → `/go-engineer` [TC-6.2]
- [ ] `TASK-6.3` The `0x4A` linkage descriptor with **linkage_type `0x91`**, in **both** descriptor loops. Without it the guide's one database question fails and it draws nothing; in the transport loop alone it changes nothing → `/go-engineer` [TC-6.3]
- [ ] `TASK-6.4` The Sky/OpenTV title-section builder with the Huffman codec. **The 12-bit length field counts the bytes AFTER the four-byte header and the reader advances `length + 4`** — openTVtoXML advances by the field alone, and following it makes twelve records arrive as two, silently → `/go-engineer` [TC-6.4]
- [ ] `TASK-6.5` The carousel: clock tables first and repeating, with NIT/BAT/SDT held until the box has a clock. **The entire listings request is day-addressed and the box programs it once** — with no clock it asks for table `0xA1`, PID `0x33` and MJD 40587, the Unix epoch, and never re-subscribes → `/go-engineer` [TC-6.5]
- [ ] `TASK-6.6` Address each section with the table id, PID and MJD **the box is currently asking for**, read from the match unit and the guide's notification slot. The table-id low bits are not constant between boxes → `/go-engineer` [TC-6.6]
- [ ] `TASK-6.7` The in-world clock from a single authority, 1:1 with real London wall-clock time, looping over a 28-day window → `/go-engineer` [TC-6.7]
- [ ] `TASK-6.8` Re-read the schedule file while running; a malformed edit keeps the last good line-up and says so → `/go-engineer` [TC-6.8]
- [ ] `TASK-6.9` ⫘ Tests, including a cross-check of the Huffman encoder against the reference decoder → `/go-engineer` [TC-6.1..TC-6.8]
- [ ] `TASK-6.10` ⫘ Playwright: press tv guide, read now and next → `/qa-test-engineer` [TC-6.9]
- [ ] `TASK-6.11` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns (measured — do not re-derive)
- **`9E 8B` is the MJD.** Twice recorded as refuted, and both refutations moved the clock *after* the
  match unit was programmed, which is a fact about re-subscription rather than meaning.
- **The guide is a subscriber, not a reader.** It registers a 44-byte notification slot; the notify
  fires only when service, day key and `tableId & 3` all match.
- **A warm box does not re-subscribe to listings.** It restores its line-up and title PID from NVRAM
  but programs the listings match unit only during a fresh acquisition.
