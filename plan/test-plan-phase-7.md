# Test Plan: Phase 7 — The unsolved screens

## Test Cases
- [?] **TC-7.1: The grid's row loop is located and its behaviour known** (covers: TASK-7.1, TASK-7.2, TASK-7.7)
  — a named address, and evidence for whether the body executes.
  *2026-09-22: BLOCKED, and the blocker is that the addresses this case inherited are not in this
  port. `0x9FC75F2F`, `0x9FC75F7B`, `0x9FC762FD` and `0x9FC77D41` neither execute (6,488,065
  instructions during the draw, all in RAM) nor are read (40,980 flash reads across 50 pages,
  none of them these) — they are the browser oracle's. Substantial evidence gathered and recorded:
  the grid reads the line-up zero times in 488,551 reads and never handles our channel numbers,
  and the structures it does walk are named. No row-loop address yet, so this is `[?]` and not
  `[x]`. Artefact: `.artifacts/grid-reads-all-channels.png`; instrument:
  `internal/multiplex/firmwaretests/gridreads_firmware_test.go`.*
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
