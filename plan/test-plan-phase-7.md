# Test Plan: Phase 7 — The unsolved screens

## Test Cases
- [ ] **TC-7.1: The grid's row loop is located and its behaviour known** (covers: TASK-7.1, TASK-7.2, TASK-7.7)
  — a named address, and evidence for whether the body executes.
- [ ] **TC-7.2: PID 0x52's consumer is identified** (covers: TASK-7.3, TASK-7.7) — by letting the firmware name
  it, with a control proving the instrument works.
- [ ] **TC-7.3: The trigger for a carousel request is found** (covers: TASK-7.4, TASK-7.7) — `carouselAcquire`
  runs, with its arguments captured. A census showing everything cold is a valid result and must be
  reported as one.
- [ ] **TC-7.4: A module is accepted** (covers: TASK-7.5) — `0x800BECF0` executes for the first time
  in this project's history.
- [ ] **TC-7.5: Whether audio is driven at all** (covers: TASK-7.6) — a yes or a no, both acceptable.
- [ ] **TC-7.6: ALL CHANNELS lists channels with programmes** (covers: TASK-7.8) — SPEC success
  criterion 5.
- [ ] **TC-7.7: The number of services the grid iterates is known, and whether it matches the
  broadcast** (covers: TASK-7.10) — the count read off the running machine rather than off the
  record, with the schedule's own service count beside it. A disagreement is the finding; so is
  agreement, because it closes the cheapest explanation for the empty grid.
