# Phase 5: The browser, and the demo host

## Outcome
A visitor opens `goretrotv.demosrv.uk`, waits for the box to boot, presses **sky**, and navigates the
real Sky interface. The product becomes something you can show someone.

## Overview
Framebuffer transport, handset input, the page, and a real deployment.

## Tasks (mirror — bd epic `gort-4sx` is the source of truth; never hand-ticked)

- [x] `TASK-5.1` WebSocket framebuffer transport — dirty-region updates at ~10 fps; 720×576 at 8 bpp is trivial bandwidth and must not be sent as video → `/go-engineer` [TC-5.1]
- [x] `TASK-5.2` The page: a canvas at the raster's own aspect, a Sky-shaped handset, and a status line saying **in words** what the box is doing (booting / checking its flash / rebuilding the channel list / ready). **Every handset button ships its interaction states as part of this task** — pressed, disabled-while-disconnected, keyboard-operable and focus-visible — not as a later polish task → `/go-engineer` [TC-5.2, TC-5.9]
- [x] `TASK-5.3` Key input from the page to the CSI link, with the documented raw codes → `/go-engineer` [TC-5.3]
- [x] `TASK-5.4` Wiring task — verify the page's canvas renders frames the core produced, a key press reaches the input layer, and the status line reflects real machine state rather than a timer → `/go-engineer` [TC-5.4]
- [x] `TASK-5.5` Production Dockerfile, compose overlay, Traefik labels and TLS at `goretrotv.demosrv.uk` → `/devops-deployment-engineer` [TC-5.5]
- [x] `TASK-5.6` **Developer surfaces must not be published** — the gdb stub and instrument endpoints bind to localhost; assert it against the deployed host, not only in config → `/devops-deployment-engineer` [TC-5.6]
- [x] `TASK-5.9` The wire contract: one versioned schema for the framebuffer and key messages, generated or shared rather than written twice, with the TS decoder's fixture **captured from the running server** — a hand-written fixture matches the author's mental model, not the wire → `/go-engineer` [TC-5.7]
- [x] `TASK-5.10` Failure states on the page: the socket drops and reconnects with backoff, the box halting shows a readable reason rather than a frozen canvas, and the keypad refuses input while disconnected instead of swallowing it → `/go-engineer` [TC-5.8]
- [x] `TASK-5.7` ⫘ Playwright specs driving the deployed page → `/qa-test-engineer` [TC-5.1, TC-5.2, TC-5.3, TC-5.4, TC-5.5, TC-5.6, TC-5.7, TC-5.8, TC-5.9]
- [x] `TASK-5.8` ⫘ Security audit of the public surface → `/security-reviewer` [no-test: audit produces its own report]
- [x] `TASK-5.11` **Reset the box from the page** — a host-level control that restores the state the process started from, so a viewer meeting a stuck or halted box recovers it without an operator restarting the container. Restores the verified post-acquisition snapshot where one is configured and cold-boots from flash where none is, and **says which of the two it did**: `power_control` is a device name in this box's inventory and not evidence of standby, so this is a host intervention and is reported as one, never dressed up as guest behaviour. It is delivered through the wire schema and drained inside the instruction loop like a key, because `Runtime` admits only its loop owner. It sits **away from the handset** — the handset's existing Standby key (`0xCC`) is a real guest key and must not be confused with this — and every viewer is told, because the box is shared by design. **Ships its interaction states as part of this task** — pressed, disabled while disconnected, no double-submit, outcome announced, keyboard and touch operable → `/go-engineer` [TC-5.13]
- [x] `TASK-5.12` **Keep the screen in line with the handset while scrolling** — on a desktop window, scrolling down to the lower keys took the picture off the top, so you could watch the box or press its keys but not both. The screen, its status line and the reset pin as one stage, capped from the viewport height (as a width, so the 5:4 ratio holds and the bezel shrinks with the picture) because a stage taller than the window would be clipped, which is worse than the scrolling. **Nothing may sit below a pinned stage** — anything left there scrolls under it → `/go-engineer` [TC-5.14]
- [x] `TASK-5.13` **Fit the box and the whole handset on screen at once** — pinning the screen was half the fix; the remote still scrolled, so the sky key and the number pad were never visible together. On a desktop window the page IS the window: the handset compresses through its spacing (never below 44px targets), the standing copy gives way on short windows, and the set is sized from the row's leftover height with container-query units rather than a swept constant. Below the handset's floor the page scrolls with the picture pinned → `/go-engineer` [TC-5.15]

## Closing gates

Beyond the security audit, this epic carries four gates, each edged behind the build tasks so none of
them is claimable before there is anything to review: **every test-plan case proved**, **`/ux-review`
of the page**, **an accessibility audit** (both colour schemes, the handset keyboard-operable, the
canvas carrying a text alternative for the current screen) and **a mobile audit**. This is the only
phase that ships a surface this project authors — phase 7 records why it has none.

## Key patterns
- **A status line is not decoration.** The predecessor's box spent 74–200 s rebuilding its channel
  list looking completely dead, and visitors read that as broken. The line exists because the box's
  real behaviour needs narrating.
- **Verify against the deployed host, not localhost.** The predecessor reported "verified end to end"
  three times from a dev server while the demo host was broken — twice because an asset was not
  served there at all, and the host answers **200 with the SPA's HTML** rather than 404.

## Custom Feature: the browser transport and page

**Purpose:** The browser is a display and a keypad, not a participant. No module covers it and no SPA
framework earns its place around `putImageData`.

**The wire contract** — one versioned schema, generated or shared, never written twice:

| Message | Direction | Shape |
|---|---|---|
| `frame` | server → page | version, seq, dirty rect (x, y, w, h), palette epoch, 8bpp bytes |
| `palette` | server → page | version, 256 RGB entries, epoch |
| `state` | server → page | version, phase enum (booting / flash-check / channel-list / ready / halted), reason |
| `key` | page → server | version, raw code, source |

**Interfaces:** `Transport.Serve(ws)` · `Transport.PushFrame(dirty image.Rectangle)` ·
`Transport.OnKey(func(raw, source uint8))`

**Key patterns (non-obvious):**
- **A status line is not decoration.** The predecessor's box spent 74–200 s rebuilding its channel
  list looking completely dead, and visitors read that as broken. And the line must be driven by
  observed machine state, not a timer — the predecessor's said Ready twenty seconds early because it
  keyed on a task count that plateaus before the work starts.
- **720×576 at 8 bpp is trivial bandwidth and must not be sent as video.** Dirty rectangles at ~10 fps.
- **Verify against the deployed host, not localhost.** The predecessor reported "verified end to end"
  three times from a dev server while the demo host was broken — twice because an asset was not
  served there at all, and because the host answers **200 with the page's HTML** rather than 404, so
  a missing file looks like a working one.
- **The decoder's fixture is captured from the running server.** A hand-written fixture is a snapshot
  of the author's wishlist; it passes while the wire differs.

**Test checklist:**
- [ ] The canvas matches the core's framebuffer hash
- [ ] The status line is shown to track real state, not elapsed time
- [ ] Every asset the deployed page fetches returns its own content, not the page's HTML
- [ ] The gdb port and instrument endpoints refuse from outside, asserted against the deployed host
- [ ] A dropped socket reconnects; a halted box says so; a key pressed while disconnected is refused
