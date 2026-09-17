# Test Plan: Phase 5 — Browser and deployment

## Test Cases
- [?] **TC-5.1: Frames reach the browser** (covers: TASK-5.1, TASK-5.7) — the canvas matches the core's
  framebuffer hash.
  **Blocked:** The transport has a live WebSocket integration test, but the browser canvas and
  machine wiring belong to TASK-5.2 and TASK-5.4; this hash comparison cannot run yet.
- [?] **TC-5.2: The status line tracks real machine state** (covers: TASK-5.2, TASK-5.7) — it must be driven by
  observed state, not a timer. The predecessor's said Ready twenty seconds early because it keyed on
  a task count that plateaus before the work starts.
  **Blocked:** The page and machine-state source are not built yet (TASK-5.2 and TASK-5.4).
- [?] **TC-5.3: A key press from the page reaches the firmware** (covers: TASK-5.3, TASK-5.7).
  **Blocked:** A live WebSocket key reaches the real CSI link with the measured Sky frame, but the
  instruction loop and browser are not connected yet (TASK-5.4); firmware receipt needs that path.
- [?] **TC-5.4: Press sky, get the menu** (covers: TASK-5.4, TASK-5.7) — end to end through the deployed path.
  **Blocked:** Browser wiring and the deployed path are not built yet (TASK-5.4 and TASK-5.5).
- [?] **TC-5.5: The demo host serves the page over TLS** (covers: TASK-5.5, TASK-5.7) — **checked against
  `goretrotv.demosrv.uk`, not localhost**, and every asset it fetches is verified to return its own
  content rather than the SPA fallback.
  **Blocked:** The page is not deployed and the DNS owner gate is open (TASK-5.5, gort-7g2.2).
- [?] **TC-5.6: Developer surfaces are unreachable publicly** (covers: TASK-5.6, TASK-5.7) — the gdb port and
  instrument endpoints refuse from outside. Asserted against the deployed host.
  **Blocked:** The deployed route is not available (TASK-5.5 and owner gates gort-7g2.1/.2).
- [x] **TC-5.7: The TS decoder is pinned to the Go encoder's actual bytes** (covers: TASK-5.7, TASK-5.9) — feed the decoder a fixture **captured from the running server** and assert the
  projection. A hand-authored fixture passes while the wire differs, which is the failure mode where
  the contract test exists, is green, and still ships the bug.
  **Proof:** `internal/web/wire_capture_test.go` captures palette and dirty-frame JSON from an HTTP-upgraded Go WebSocket transport and compares it byte for byte with `tests/fixtures/wire.jsonl`; `tests/wire.test.mjs` decodes the captured bytes and checks colour and pixel projection. `go test -race ./internal/web ./internal/wire`, `npm run wire:check`, and `npm run test:wire` pass.
- [x] **TC-5.8: The page survives losing the box** (covers: TASK-5.10, TASK-5.7) — kill the socket
  mid-session: the page says so, reconnects, and resumes. Halt the machine: a readable reason, not a
  frozen canvas. Press a key while disconnected: refused visibly, never swallowed.
  **Proof:** `tests/e2e/handset.spec.ts` closes a live routed WebSocket, checks the visible
  disconnect and disabled handset, confirms no key was sent, and verifies that the old canvas
  pixel stays drawn. A new socket supplies a full palette/frame and ready state; the new pixels
  appear and the handset sends a key. A second rapid drop waits at least 450 ms before reconnect,
  proving backoff. A separate halted-state test checks the guest reason, disabled input, and
  zero outbound keys. Browser screenshots were inspected in light, dark, and mobile views.
- [x] **TC-5.9: The handset acknowledges input and is operable by keyboard** (covers: TASK-5.2, TASK-5.7) — every button shows a pressed state, is reachable by **a real `Tab` press** (programmatic
  focus matches `:focus` but not reliably `:focus-visible`, so it measures the browser's heuristic
  rather than the stylesheet), shows a visible focus ring asserted **by colour** rather than by the
  presence of an outline, and is operable by touch at a usable target size.
  **Result:** `tests/e2e/handset.spec.ts` drove all handset buttons with real Tab navigation
  and checked each visible focus colour, 3 px outline and 44 px touch target. It exercised
  keyboard and pointer pressed states, confirmed a Sky press sends raw `0x7D`, verified the
  acknowledgement text, and tapped Sky in a mobile touch context. It also checked the
  disconnected disabled state and reduced-motion duration. The browser suite passed
  seven cases, including its dedicated reduced-motion project; the focus-colour assertion
  failed under a deliberately wrong expected colour before restoration.
