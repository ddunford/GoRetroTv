# Test Plan: Phase 5 — Browser and deployment

## Test Cases
- [ ] **TC-5.1: Frames reach the browser** (covers: TASK-5.1) — the canvas matches the core's
  framebuffer hash.
- [ ] **TC-5.2: The status line tracks real machine state** (covers: TASK-5.2) — it must be driven by
  observed state, not a timer. The predecessor's said Ready twenty seconds early because it keyed on
  a task count that plateaus before the work starts.
- [ ] **TC-5.3: A key press from the page reaches the firmware** (covers: TASK-5.3).
- [ ] **TC-5.4: Press sky, get the menu** (covers: TASK-5.4) — end to end through the deployed path.
- [ ] **TC-5.5: The demo host serves the page over TLS** (covers: TASK-5.5) — **checked against
  `goretrotv.demosrv.uk`, not localhost**, and every asset it fetches is verified to return its own
  content rather than the SPA fallback.
- [ ] **TC-5.6: Developer surfaces are unreachable publicly** (covers: TASK-5.6) — the gdb port and
  instrument endpoints refuse from outside. Asserted against the deployed host.
