# Test Plan: Phase 5 — Browser and deployment

## Test Cases
- [x] **TC-5.1: Frames reach the browser** (covers: TASK-5.1, TASK-5.7) — the canvas matches the core's
  framebuffer hash.
  **Result:** `internal/board/runtime_test.go` pins the verified private snapshot's composed indexed
  framebuffer hash to `A6A21DC5` and the post-Sky menu to `FE8D1CCC`. `tests/live/firmware.spec.ts` captures the full frame from the
  running Go WebSocket, checks that same hash, expands its real palette into RGBA, and compares a
  SHA-256 digest against every pixel in the browser canvas before pressing Sky. It then waits for
  the exact post-Sky menu hash through the WebSocket. The live test passes.
- [x] **TC-5.2: The status line tracks real machine state** (covers: TASK-5.2, TASK-5.7) — it must be driven by
  observed state, not a timer. The predecessor's said Ready twenty seconds early because it keyed on
  a task count that plateaus before the work starts.
  **Result:** The server sends Ready only after restoring the exact post-acquisition snapshot
  (`retired=1.1B`, completed handoff/Sky gates, state hash `8B2A7E0B`). Cold runs remain Booting
  through handoff and TASK20's initial one-schedule plateau; active TASK20 bank validation reports
  Flash check; the channel-list label requires TASK20's event wait after running and all three
  firmware-programmed SI PIDs (`0x14`, `0x11`, `0x10`). Its text says the box is *waiting* for the
  list because the demux request alone does not prove rebuilding has begun. `cmd/goretrotv/status_test.go`
  rejects premature transitions, and `internal/board/task_state_test.go` proves task status comes
  from the live guest list rather than a stale TASK signature. The live Playwright test asserts
  Ready from the verified snapshot before enabling the handset and after a real menu transition.
- [x] **TC-5.3: A key press from the page reaches the firmware** (covers: TASK-5.3, TASK-5.7).
  **Result:** `tests/live/firmware.spec.ts` starts the real Go server with private firmware and
  snapshot, clicks the page's Sky button, and waits for the firmware's Box Office menu on the
  canvas; `internal/web/transport_test.go` asserts the measured CSI wire frame from a live socket.
- [x] **TC-5.4: Press sky, get the menu** (covers: TASK-5.4, TASK-5.7) — end to end through the deployed path.
  **Result:** `tests/public/firmware.spec.ts` run by `npm run test:e2e:public` restarts the public container from the private snapshot,
  opens `https://goretrotv.demosrv.uk/` in Chromium, captures the real `wss://` frame and checks
  its indexed hash `A6A21DC5` against the entire browser canvas. A click on Sky sends raw
  `0x7D` over that WebSocket and yields the exact composed Box Office menu hash `FE8D1CCC`.
  The spec passed twice, including after an explicit public restart; it saves light, dark and
  mobile screenshots under `.artifacts/playwright-public-results/`.
- [x] **TC-5.5: The demo host serves the page over TLS** (covers: TASK-5.5, TASK-5.7) — **checked against
  `goretrotv.demosrv.uk`, not localhost**, and every asset it fetches is verified to return its own
  content rather than the SPA fallback.
  **Result:** `tests/public/firmware.spec.ts` and `./ctl.sh up-public` verified the image behind the real Traefik router;
  `https://goretrotv.demosrv.uk/health` returned 200 over verified TLS. The public page, stylesheet,
  favicon and four JavaScript modules matched their built files byte for byte; an unknown module
  returned 404. Browser verification at the real URL showed Ready, 24 enabled buttons and the real
  firmware's Box Office menu after Sky was pressed. Light, dark and 390 px mobile screenshots were
  inspected; mobile had no horizontal overflow. `./ctl.sh conformance` passed 7/7 rules and 26/26
  probes, including the firmware-free image check.
- [x] **TC-5.6: Developer surfaces are unreachable publicly** (covers: TASK-5.6, TASK-5.7) — the gdb port and
  instrument endpoints refuse from outside. Asserted against the deployed host.
  **Result:** `tests/public/firmware.spec.ts` asserts the HTTPS developer-route boundary;
  with `https://goretrotv.demosrv.uk/health` returning 200, TCP connections to
  `goretrotv.demosrv.uk:23457` (GDB test port) and `:8099` timed out; direct connections to the
  origin LAN address `192.168.1.12` on both ports were refused. Public requests for
  `/debug/pprof/`, `/debug/pprof/profile`, `/debug/pprof/cmdline`, `/instruments`, `/metrics`,
  and `/trace` all returned 404. The merged public compose config has no host ports and forces
  `GORETROTV_ENABLE_PPROF=false` even when the caller exports `true`. `internal/app` also refuses
  to start a pprof-enabled server outside explicit development mode, so bypassing the overlay
  cannot put pprof on the public mux. `./ctl.sh test` passed under the race detector, including
  the production refusal test; repeatable network commands are in `docs/deployment.md`.
- [x] **TC-5.7: The TS decoder is pinned to the Go encoder's actual bytes** (covers: TASK-5.7, TASK-5.9) — feed the decoder a fixture **captured from the running server** and assert the
  projection. A hand-authored fixture passes while the wire differs, which is the failure mode where
  the contract test exists, is green, and still ships the bug.
  **Result:** `internal/web/wire_capture_test.go` captures palette and dirty-frame JSON from an HTTP-upgraded Go WebSocket transport and compares it byte for byte with `tests/fixtures/wire.jsonl`; `tests/wire.test.mjs` decodes the captured bytes and checks colour and pixel projection. `go test -race ./internal/web ./internal/wire`, `npm run wire:check`, and `npm run test:wire` pass.
- [x] **TC-5.8: The page survives losing the box** (covers: TASK-5.10, TASK-5.7) — kill the socket
  mid-session: the page says so, reconnects, and resumes. Halt the machine: a readable reason, not a
  frozen canvas. Press a key while disconnected: refused visibly, never swallowed.
  **Result:** `tests/e2e/handset.spec.ts` closes a live routed WebSocket, checks the visible
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
- [x] **TC-5.10: The screen remains visible while using the handset on a phone** (covers: gort-4sx.19) — handset presses need visible firmware feedback at 320 px without covering focused keys.
  **Result:** `tests/e2e/handset.spec.ts` tabs through every handset button at 320 × 700 and
  asserts that the canvas remains in the viewport above the focused button. It taps Sky and Select
  in a mobile touch context and checks the screen stays above Select. The screenshot of
  the final key shows the screen and key together. Playwright passed 9/9; the public mobile view
  is inspected after deployment.
- [x] **TC-5.11: Short landscape keeps screen and handset together** (covers: gort-4sx.21) — a 568 × 320 viewport must show the guest display beside a usable handset during navigation.
  **Result:** `tests/e2e/handset.spec.ts` passes locally and checks that screen and Select are
  visible side by side with no horizontal overflow. The deployed URL at 568 × 320 reported
  screen bounds `121.875..288.265625`, Select bounds `136.9375..183.9375`, `scrollWidth=568`,
  and the screenshot `.artifacts/public-landscape-568x320.png` was inspected.
- [?] **TC-5.12: The canvas describes the guest screen as it changes** (covers: gort-4sx.22) —
  the text alternative must name the visible menu and selected option from rendered firmware
  output, and must not retain a known-screen label after an unrecognised change.
  **Open:** `tests/live/firmware.spec.ts` passed against real private firmware: it verified the
  blue screen, all six Box Office selections, and negative controls changing one palette,
  menu-row, or outside-row byte. `tests/e2e/screen-description.spec.ts` passed with a dirty
  frame changing a known blue screen to an unknown one; the old label was removed. The
  classifier covers those measured frames only. Other guest screens and interactive choices
  still lack verified semantic extraction, so full screen-reader access is not yet proved.
- [x] **TC-5.13: The reset control recovers the box, and says what it did** (covers: TASK-5.11) —
  the state matrix and the recovery, both. Pressed on a running box, the canvas returns to the
  startup frame and the status line says the box was restored; pressed twice quickly, the second
  press is refused rather than queued; pressed while disconnected, it is disabled and sends
  nothing. The control is reachable and operable by keyboard and by touch, and its outcome is
  announced rather than only drawn. **A halted box is the case that matters**: halt the machine,
  then reset, and the box must run again — today `haltMachine` returns and nothing restarts it.
  A second viewer's page must show the same reset, because the box is shared.
  **Result:** `cmd/goretrotv/reset_test.go` runs the real firmware and the private snapshot.
  `TestResetRestartsAHaltedBoxAndSaysWhatItDid` halts the guest for real — an illegal instruction
  written at its own restored program counter — watches the browser be told `halted`, sends a reset
  on that same socket, and gets `ready` back with *"The box was reset and restored to its startup
  state."* Falsified: with the halt path returning as it did before this task, the test fails after
  91 s having never been answered. `TestResetRebuildsARunningBoxAndItKeepsRetiring` resets a working
  box, checks a **second connected viewer** is told too, then presses sky and waits for the guest to
  draw — an idle box publishes nothing at all, so drawing is the only honest liveness signal.
  `internal/web/transport_test.go` proves a reset is accepted in the halted phase where a key is
  still refused (falsified against the reset behind the same gate), the minimum interval folds a
  second press (falsified against the interval removed — the first version of that test passed
  without it, because the queue's one slot was doing the folding), and malformed or unknown client
  messages close the socket. `tests/e2e/reset.spec.ts` covers the state matrix in a browser: one
  request per press with the control held through the host's own cooldown, enabled on a halted box
  where every handset key is disabled, disabled and silent while disconnected, keyboard-operable
  with a visible focus ring, and a token-driven transition that reduced motion shortens. The
  control's accessible description distinguishes it from the handset's standby key, and it is
  asserted to live outside `#handset`. Light, dark and 390 px screenshots were inspected
  (`.artifacts/reset-light.png`, `reset-dark.png`, `reset-mobile-dark.png`); `scrollWidth` was 390
  at 390 px and the button measured 140 × 44. `go test -race ./...`, `./ctl.sh lint`,
  `npm run wire:check`, typecheck, CSS lint and the 17-test local Playwright suite all pass.
- [x] **TC-5.14: The screen stays with the handset while scrolling** (covers: TASK-5.12) — on a
  desktop window you must be able to watch the box and press its lower keys at the same time,
  and nothing may be covered by the pinned screen.
  **Result:** `tests/e2e/handset.spec.ts` drives 1280 × 620 and 1440 × 900, scrolls to the far end
  and asserts the canvas, the status line, the reset and the number pad are all fully in the
  viewport, that the reset is the element at its own coordinates rather than something painted over
  it, that the page never scrolls horizontally, and that the canvas is **pinned at the sticky
  offset** rather than merely still on screen. That last assertion is the one that matters:
  capping the stage's height alone shortens the page enough that everything happens to fit at the
  bottom, so removing `position: sticky` left the first version of this test passing. With the
  assertion added, the same mutation fails at 1280 × 620 (canvas top 2 px, not ≥ 16). Light and
  dark were inspected at 1280 × 620 and 1440 × 900 scrolled to the end
  (`.artifacts/final-620-light.png`, `.artifacts/stage-1440-bottom-dark.png`), which is how the
  help text was caught overprinting the reset note — anything left below a pinned stage scrolls
  under it, so that copy moved above the television. Mobile was re-checked at 390 × 780: the stage
  is `display: contents` there and the television sticks against the grid as it did before, with
  `scrollWidth` 390. Full local Playwright suite 19/19.
- [x] **TC-5.15: The box and the whole handset fit the window** (covers: TASK-5.13) — on a desktop
  window nothing scrolls: the screen, its status line, the reset, and the handset from the sky key
  to the 0 key are all on screen together, with every key still at 44px.
  **Result:** `tests/e2e/handset.spec.ts` asserts it at 1366 × 768, 1440 × 900 and 1920 × 1080 —
  `maxScroll <= 0`, no horizontal scroll, sky/0/number-pad/screen/status/reset all fully in the
  viewport, the reset not covered by the pinned stage, and the smallest key ≥ 44px. Falsified:
  deleting the fit-to-window block fails all three. A fourth case covers the honest fallback at
  1280 × 620, where two columns of 44px keys cannot fit beside a picture: there the page scrolls
  and the test asserts the picture is **pinned** at the sticky offset rather than merely still on
  screen. Measured across eight window sizes: zero overflow at every window ≥ 760px tall
  (768/800/900/1050/1080), scrolling below it.
  Three mechanisms were tried and two discarded, which the CSS records: a constant subtracted from
  `100vh` fitted 1440 × 900 exactly and overflowed everywhere else, giving a 138px picture at
  1280 × 620; `width: fit-content` could not shrink the bezel because a canvas contributes its
  intrinsic 720px as max-content width whatever height it is drawn at. The set is now sized from
  the stage's own height with container-query units, so there is nothing to re-sweep when the copy
  changes. Light and dark inspected at 1366 × 768 (`.artifacts/fit3-1366x768.png`,
  `fit-dark-1366x768.png`); mobile re-checked at 390 × 780 (`fit-mobile-390.png`) and unchanged.
  Full local Playwright suite 21/21.
