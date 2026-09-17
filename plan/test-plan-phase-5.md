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
  (`retired=1.1B`, completed handoff/Sky gates, state hash `04E99A24`); cold runs observe the
  application handoff and cannot claim Ready without measured acquisition. The live Playwright
  test asserts Ready before enabling the handset and again after a real firmware menu transition.
- [x] **TC-5.3: A key press from the page reaches the firmware** (covers: TASK-5.3, TASK-5.7).
  **Result:** `tests/live/firmware.spec.ts` starts the real Go server with private firmware and
  snapshot, clicks the page's Sky button, and waits for the firmware's Box Office menu on the
  canvas; `internal/web/transport_test.go` asserts the measured CSI wire frame from a live socket.
- [?] **TC-5.4: Press sky, get the menu** (covers: TASK-5.4, TASK-5.7) — end to end through the deployed path.
  **Blocked:** The real public browser path was walked and showed the Box Office menu, but a
  committed Playwright test against the deployed URL remains for TASK-5.7.
- [x] **TC-5.5: The demo host serves the page over TLS** (covers: TASK-5.5, TASK-5.7) — **checked against
  `goretrotv.demosrv.uk`, not localhost**, and every asset it fetches is verified to return its own
  content rather than the SPA fallback.
  **Result:** `./ctl.sh up-public` built and started the image behind the real Traefik router;
  `https://goretrotv.demosrv.uk/health` returned 200 over verified TLS. The public page, stylesheet,
  favicon and three JavaScript modules matched their built files byte for byte; an unknown module
  returned 404. Browser verification at the real URL showed Ready, 24 enabled buttons and the real
  firmware's Box Office menu after Sky was pressed. Light, dark and 390 px mobile screenshots were
  inspected; mobile had no horizontal overflow. `./ctl.sh conformance` passed 7/7 rules and 26/26
  probes, including the firmware-free image check.
- [x] **TC-5.6: Developer surfaces are unreachable publicly** (covers: TASK-5.6, TASK-5.7) — the gdb port and
  instrument endpoints refuse from outside. Asserted against the deployed host.
  **Result:** With `https://goretrotv.demosrv.uk/health` returning 200, TCP connections to
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
