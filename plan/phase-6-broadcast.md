# Phase 6: The broadcast

## Outcome
The box acquires a line-up and shows real 1998 programmes on its guide, having asked for them itself.

## Overview
The modelled multiplex. Every rung of this was measured by the predecessor and must not be
re-derived — `docs/reference/digibox-emulation.md` and `digibox-next-session.md` are the spec.

## Tasks (mirror — bd epic `gort-qbn` is the source of truth; never hand-ticked)

- [x] `TASK-6.1` Section builders: NIT, SDT, TDT, TOT, with MPEG CRC-32 (poly `0x04C11DB7`, init all ones, no final inversion) → `/go-engineer` [TC-6.1]
- [x] `TASK-6.2` The BAT with the `0x5F` private-data-specifier (value **2**) ahead of a `0xB1` line-up descriptor whose gate halfword must be `0xFFFF` and whose entries are nine bytes → `/go-engineer` [TC-6.2]
- [x] `TASK-6.3` The `0x4A` linkage descriptor with **linkage_type `0x91`**, in **both** descriptor loops. Without it the guide's one database question fails and it draws nothing; in the transport loop alone it changes nothing → `/go-engineer` [TC-6.3]
- [x] `TASK-6.4` The Sky/OpenTV title-section builder with the Huffman codec. **The 12-bit length field counts the bytes AFTER the four-byte header and the reader advances `length + 4`** — openTVtoXML advances by the field alone, and following it makes twelve records arrive as two, silently → `/go-engineer` [TC-6.4]
- [x] `TASK-6.5` The carousel: clock tables first and repeating, with NIT/BAT/SDT held until the box has a clock. **The entire listings request is day-addressed and the box programs it once** — with no clock it asks for table `0xA1`, PID `0x33` and MJD 40587, the Unix epoch, and never re-subscribes → `/go-engineer` [TC-6.5]
- [x] `TASK-6.6` Address each section with the table id, PID and MJD **the box is currently asking for**, read from the match unit and the guide's notification slot. The table-id low bits are not constant between boxes → `/go-engineer` [TC-6.6]
- [x] `TASK-6.7` The in-world clock from a single authority, 1:1 with real London wall-clock time, looping over a 28-day window → `/go-engineer` [TC-6.7]
- [x] `TASK-6.8` Re-read the schedule file while running; a malformed edit keeps the last good line-up and says so → `/go-engineer` [TC-6.8]
- [ ] `TASK-6.9` ⫘ Tests, including a cross-check of the Huffman encoder against the reference decoder → `/go-engineer` [TC-6.1, TC-6.2, TC-6.3, TC-6.4, TC-6.5, TC-6.6, TC-6.7, TC-6.8]
- [ ] `TASK-6.10` ⫘ Playwright: press tv guide, read now and next → `/qa-test-engineer` [TC-6.9]
- [ ] `TASK-6.13` Five days in eight arm the listings PID and program no title filter. On MJD mod 8 in {1,3,6} the box programs a title match unit in ~430k instructions; on {0,2,4,5,7} it arms the right PID and programs none in 200M. The transmitter routes around it by deriving the addressing, and the programmes are then STORED — 67 of 67 registered — but the guide does not draw them, so the demo still has to pin a date in the set → `/go-engineer` [TC-6.13]
- [ ] `TASK-6.14` Huffman-encoded titles lose their spaces and gain a trailing character — `Dream Team` draws as `DreamTeams`. Present since the first title section went on air and invisible because every title tested stayed legible; the encoder's round-trip test against its own decoder cannot see it, because the two share the mistake → `/go-engineer` [TC-6.14]
- [ ] `TASK-6.11` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns (measured — do not re-derive)
- **`9E 8B` is the MJD.** Twice recorded as refuted, and both refutations moved the clock *after* the
  match unit was programmed, which is a fact about re-subscription rather than meaning. A third
  reading on 2026-09-20 found a simpler cause for some of it: those runs set the clock with a **TDT**,
  which this box does not filter for at all.
- **The guide is a subscriber, not a reader.** It registers a 44-byte notification slot; the notify
  fires only when service, day key and `tableId & 3` all match.
- **A warm box does not re-subscribe to listings.** It restores its line-up and title PID from NVRAM
  but programs the listings match unit only during a fresh acquisition.

## Custom Feature: the modelled multiplex

**Purpose:** Make the box acquire a line-up and ask for listings **by itself**. Every rung was
measured by the predecessor; `docs/reference/digibox-emulation.md` and `digibox-next-session.md` are
the specification and none of it should be re-derived.

**Data it owns:**

| Artefact | Shape | Notes |
|---|---|---|
| `listings/` | one JSON per date (`1998-12-24.json`) plus `default.json` | the editable schedule (FR-12); a real listings page keeps the date it was printed for |
| section builders | NIT `0x40`, SDT `0x42`, BAT `0x4A`, TDT `0x70`, TOT `0x73`, titles `0xA0`–`0xA3` | MPEG CRC-32, poly `0x04C11DB7`, init all ones, no final inversion |
| carousel | wave scheduler driven off `icount` | clock tables first and repeating |
| in-world clock | one authority | 1:1 with London wall-clock, 28-day loop |

**Interfaces:**
- `Schedule.Load(path) (Schedule, error)` — a malformed edit **keeps the last good schedule and says so**
- `Carousel.Wave(icount uint64) [][]byte` · `Clock.Now() time.Time` (the single authority, FR-11)
- `Titles.Build(day, tableID, pid, services) []byte`

**Key patterns (measured — do not re-derive):**
- **The 12-bit record length counts the bytes AFTER the four-byte header and the reader advances
  `length + 4`.** openTVtoXML advances by the field alone; follow it and twelve records arrive as
  two, silently. This was the single most important fix of the predecessor's last day.
- **The `0x4A` linkage with type `0x91` must be in BOTH descriptor loops.** In the transport loop
  alone it changes nothing; absent entirely, the guide's one database question fails and it draws
  nothing.
- **One listings request covers a SET of channels.** The table-id extension carries a mask as well
  as a value: the value is the bitwise OR of the listings ids the box wants and the mask clears the
  bits that differ. Six channels arrive as one `bf/f8`, and the value alone (`0x0BBF`) is nobody's
  id — so a transmitter that reads it literally sends nothing and reports nothing.
- **The guide's row number is the listings id**, not the line-up's channel field, so a channel's
  listings id must be the number a viewer expects to see.
- **The box applies its time offset to programme times as well as the clock**, so the wire carries
  UTC and the editable schedule is in local time.
- **The handset answers five codes and no others**, swept 0x00–0xFF: `0x0C`/`0x80` tv guide,
  `0x7D` box office, `0x7E` services, `0xCC` standby, `0xF5` interactive. There is no key that
  opens the menu on TV GUIDE, and no separate sky/home key — `0x7D` opens the menu and the tab is
  remembered, which is why it can look like either. The firmware names its own keys on the
  SERVICES → *Using Your Sky Digibox* help page.
- **Clock first, and the clock table is the TOT.** The listings request is day-addressed and the box
  programs it **once**. Measured 2026-09-20: the box's match units carry `0x73` and nothing matches
  `0x70`, so a TDT is never delivered — a carousel that sends only a TDT leaves the box on the day it
  woke with and reports nothing. With a TOT the whole request moves together: the requested MJD is
  the clock's **own** day (not the day after, which the record had) and the listings PID is
  `0x30 | (MJD mod 8)`.
- **The guide is a subscriber, not a reader** — a 44-byte notification slot that fires only when
  service, day key and `tableId & 3` all match.
- **A warm box does not re-subscribe.** It restores line-up and title PID from NVRAM but programs
  the listings match unit only during a fresh acquisition.

**Test checklist:**
- [ ] The length field computed openTVtoXML's way is shown to deliver two records instead of twelve
- [ ] The linkage present and absent are both asserted, and the transport-loop-only case too
- [ ] Without a clock the box is shown asking for MJD 40587
- [ ] The Go Huffman encoder round-trips against the reference decoder over the whole schedule
