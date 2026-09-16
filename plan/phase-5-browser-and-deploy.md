# Phase 5: The browser, and the demo host

## Outcome
A visitor opens `goretrotv.demosrv.uk`, waits for the box to boot, presses **sky**, and navigates the
real Sky interface. The product becomes something you can show someone.

## Overview
Framebuffer transport, handset input, the page, and a real deployment.

## Tasks (mirror — bd epic `gort-5` is the source of truth; never hand-ticked)

- [ ] `TASK-5.1` WebSocket framebuffer transport — dirty-region updates at ~10 fps; 720×576 at 8 bpp is trivial bandwidth and must not be sent as video → `/go-engineer` [TC-5.1]
- [ ] `TASK-5.2` The page: a canvas at the raster's own aspect, a Sky-shaped handset, and a status line saying **in words** what the box is doing (booting / checking its flash / rebuilding the channel list / ready) → `/go-engineer` [TC-5.2]
- [ ] `TASK-5.3` Key input from the page to the CSI link, with the documented raw codes → `/go-engineer` [TC-5.3]
- [ ] `TASK-5.4` Wiring task — verify the page's canvas renders frames the core produced, a key press reaches the input layer, and the status line reflects real machine state rather than a timer → `/go-engineer` [TC-5.4]
- [ ] `TASK-5.5` Production Dockerfile, compose overlay, Traefik labels and TLS at `goretrotv.demosrv.uk` → `/devops-deployment-engineer` [TC-5.5]
- [ ] `TASK-5.6` **Developer surfaces must not be published** — the gdb stub and instrument endpoints bind to localhost; assert it against the deployed host, not only in config → `/devops-deployment-engineer` [TC-5.6]
- [ ] `TASK-5.7` ⫘ Playwright specs driving the deployed page → `/qa-test-engineer` [TC-5.1..TC-5.6]
- [ ] `TASK-5.8` ⫘ Security audit of the public surface → `/security-reviewer` [no-test: audit produces its own report]

## Key patterns
- **A status line is not decoration.** The predecessor's box spent 74–200 s rebuilding its channel
  list looking completely dead, and visitors read that as broken. The line exists because the box's
  real behaviour needs narrating.
- **Verify against the deployed host, not localhost.** The predecessor reported "verified end to end"
  three times from a dev server while the demo host was broken — twice because an asset was not
  served there at all, and the host answers **200 with the SPA's HTML** rather than 404.
