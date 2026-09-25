# Test Plan: Phase 7 — The unsolved screens

## Test Cases
- [?] **TC-7.1: The grid's row loop is located and its behaviour known** (covers: TASK-7.1, TASK-7.2, TASK-7.7)
  — a named address, and evidence for whether the body executes.
  *2026-09-22, SECOND ENTRY, AND IT WITHDRAWS THE FIRST. The note below concluded that the four
  inherited flash addresses "are the browser oracle's" because they neither executed nor were read.
  **That was measured on a screen that had given up.** The grid gates on a transport record whose
  state climbs 4 -> 6 -> 7 about 56 million instructions after the last programme registers; every
  measurement of this screen pressed select after a fixed settle and caught it at 4, when the row
  loop runs ONCE and leaves. Waited for properly, the loop runs all six channels — and all four
  oracle addresses ARE read, 26 times between them.*

  *The loop is located and its behaviour is known, so the case's own question is answered:
  `FUN_800A4A90`, loop head `0x800A4B60`, and three screens that do draw rows reach it zero times.
  The body executes six times, 4,000+ instructions each. It is still `[?]` because rows still do not
  draw: the body never enters the MIPS listings module, so the decision is in the o-code. The row
  callback is now named — `0x800CB7B8`, entry 1 of the six-descriptor table at `0x80164978` — and it
  is invoked once per channel and produces no pixels. Instruments:
  `rowcallback_firmware_test.go`, `divergence_firmware_test.go`, `nativecensus_firmware_test.go`.
  Full state of the hunt: bd memory `the-all-channels-grid-hunt`.*

  *2026-09-22, FIRST ENTRY, SUPERSEDED: BLOCKED, and the blocker is that the addresses this case
  inherited are not in this port. `0x9FC75F2F`, `0x9FC75F7B`, `0x9FC762FD` and `0x9FC77D41` neither
  execute nor are read — they are the browser oracle's. Artefact:
  `.artifacts/grid-reads-all-channels.png`.*
- [?] **TC-7.2: PID 0x52's consumer is identified** (covers: TASK-7.3, TASK-7.7) — by letting the firmware name
  it, with a control proving the instrument works.
  *2026-09-22: NOT RUN. TASK-7.3's own premise was corrected in the phase file — `0x52` is the
  box's own transient subtable registration, and the live question is application signalling
  instead. The case needs rewriting with the task before it can be walked.*
- [?] **TC-7.3: The trigger for a carousel request is found** (covers: TASK-7.4, TASK-7.7) — `carouselAcquire`
  runs, with its arguments captured. A census showing everything cold is a valid result and must be
  reported as one.
  *2026-09-22: NOT RUN. Depends on TC-7.2's question being settled first; nothing has fed the
  carousel, and `0x800BECF0` has still never executed.*
- [?] **TC-7.4: A module is accepted** (covers: TASK-7.5) — `0x800BECF0` executes for the first time
  in this project's history.
  *2026-09-22: NOT RUN. Blocked behind TC-7.3 — a module cannot be accepted before a request is
  triggered.*
- [?] **TC-7.5: Whether audio is driven at all** (covers: TASK-7.6) — a yes or a no, both acceptable.
  *2026-09-22: NOT RUN. Untouched this phase; no instrument exists for it yet.*
- [?] **TC-7.6: ALL CHANNELS lists channels with programmes** (covers: TASK-7.8) — SPEC success
  criterion 5.
  *2026-09-22: FAILS TODAY, and it is the phase's headline. Measured on the live demo and in a
  repeatable test: the grid draws its header, the in-world date and clock and correctly spaced
  half-hour columns, and NO CHANNEL ROWS. Marked `[?]` rather than `[!]` because the product is
  not yet claimed to implement it — TC-7.1 is the work that would.*
- [x] **TC-7.7: The number of services the grid iterates is known, and whether it matches the
  broadcast** (covers: TASK-7.10) — the count read off the running machine rather than off the
  record, with the schedule's own service count beside it. A disagreement is the finding; so is
  agreement, because it closes the cheapest explanation for the empty grid.
  *2026-09-22: PASS. Six service records in the box, one per announced service, read off the
  running machine by content (service id at +4, channel at +10) with the 18-byte stride taken from
  the commonest gap. AGREEMENT — so the cheapest explanation is closed, exactly as this case
  anticipated. The grid does not iterate them in any case: zero reads of those records during the
  draw. Instrument: `internal/multiplex/firmwaretests/lineupcount_firmware_test.go`, which fails
  unless it finds every announced service.*
- [ ] **TC-7.8: The browser handset matches the original remote without inventing controls**
  (covers: TASK-7.8) — compare the rendered handset with the July 1999 manual fold-out and a
  contemporary standard Sky Digital remote photograph. The dark-blue tapered body, control groups,
  labels, blue navigation keys, coloured keys and telephone-letter number pad must be recognisable
  at desktop and mobile widths in both colour schemes. Every enabled key must still travel through
  the existing WebSocket-to-CSI path with a measured raw code; TV-only and unmeasured controls must
  be visibly and accessibly unavailable. All enabled targets remain at least 44 by 44 CSS pixels.
  References: `bskyb2400_userguide.pdf`, Version 2.0 July 1999; Chigwell Satellite's photographed
  blue standard remote.
