# What GoRetroTV is

A Sky Digibox from 1998 was a small computer whose only job was to turn a satellite signal into
pictures, menus and a TV guide. This project runs the **real program out of a real box's memory
chips**, in Go, by pretending to be the hardware that program expects — and puts the picture in a
browser window with the handset on screen. Nothing here imitates the Sky interface. The menus you
see are the ones the firmware draws, because the firmware is the thing running.

## The nouns

<!-- anchor: none - domain vocabulary; the words outlive any file that holds them -->

The words this codebase uses to mean something specific. Everything else is ordinary English.

| Word | In plain terms | Lives in |
|---|---|---|
| **the oracle** | The working browser emulator this port came from, kept as a measuring instrument. It is how we know the Go version is right. | `reference/digibox-boot.html` |
| **icount** | The instruction counter. It is the only clock — nothing consults the wall. | `internal/platform/clock/` |
| **state hash** | One number standing for "the machine is in exactly this state". Used to compare two runs. | `internal/platform/statehash/` |
| **snapshot** | The whole machine written to a file — 32 MB of RAM, both flash images, and every device's private state. | `internal/bus/`, `internal/machine/` |
| **handoff** | The moment the host steps in, once, to start the application after the box's own bootloader goes quiet. | `internal/machine/handoff.go` |
| **Sky gates** | An optional policy that answers two checks the application makes, so the menus appear. Off by default. | `internal/machine/sky_gates.go` |
| **boot gate** | The standing check that the box still boots. Run after anything that touches a device. | `./ctl.sh oracle-gate` |
| **instrument** | A counter that measures the running guest. It must fail loudly rather than report zero. | `internal/platform/instrument/` |
| **a wedge** | The box stops doing anything useful while still running. This is what a fault looks like here — not a crash. | `internal/multiplex/` |

## What happens between power-on and a picture

1. **The box's own bootloader runs**, out of flash, exactly as the real hardware would.
2. **It goes idle** — it has done its job and is spinning, waiting for something a real box gets
   from a chip we do not model yet.
3. **The host steps in once.** It checks the flash header matches the ROM, then sets three CPU
   registers so execution lands on the application's entry stub. → `internal/machine/handoff.go`
4. **The guest decompresses and runs its own application.** From here the host is a spectator again.
5. **The RTOS creates its tasks** — around forty-two of them when the box is healthy.
6. **Optionally, the Sky gates fire**, answering two application checks so the menus present.
7. **The firmware draws**, through the on-screen-display and blitter models, into a framebuffer.
8. **The browser gets pixels and sends back key presses**, over a WebSocket, outside the
   instruction loop so the emulation stays deterministic. → `internal/web/transport.go`

<!-- anchor: internal/machine/handoff.go -->
<!-- anchor: internal/web/transport.go -->
<!-- fingerprint: sha256:e3332f909f5cdc31e8d2145d5f1e1fc66785b8e50cc4bba9ea9746045c0ab65c @ 2026-09-24 -->

## A way to think about it

It is closer to running an old game cartridge in an emulator than to rewriting the game: nobody
reimplements the menus, because the original code draws them.

**Where the analogy breaks — twice, and both matter.** A cartridge is self-contained; a Digibox is
only half a system. The other half is a *live broadcast* arriving continuously and a *smartcard*
answering questions, so this emulator has to invent a plausible transmission and a plausible card as
well as a CPU — and a surprising share of the difficulty lives there rather than in the processor.
Second, most emulators are judged by whether they look right. This one is judged against the oracle
instruction for instruction, which means **looking right is not a pass**; two implementations can be
wrong in the same way and agree with each other.

## Five things that would surprise you

Full versions in `CLAUDE.md → Non-Obvious Domain Patterns`, which is the home of these; the
one-liners here exist so a newcomer knows they are coming.

1. **A wrong hardware model does not crash. It keeps going, plausibly.** A boot that stalls at
   twenty tasks, or a link silently running at the wrong speed.
2. **Recording what the firmware writes is safe; changing what a read returns is not.** Making one
   register file read back its own writes stopped the RTOS starting at all.
3. **"The box is not asking for it" is a symptom, not a finding.** Four different causes have
   produced it, one of them a genuine firmware defect.
4. **The oracle proves this port matches the browser emulator — not that either matches a real
   Digibox.** Inherited errors are only caught by the measured record.
5. **When a symptom correlates with the variable you are already holding, check the one you are
   not.** A guide fault that looked like a day-of-eight rotation was the hour on the clock.

<!-- anchor: none - each item is a pointer to CLAUDE.md, which owns the full version -->

## Where to go next

<!-- anchor: none - navigation for this directory; its subject is the doc tree, not code -->

- **Modelling any device, or reading the firmware:** `docs/reference/digibox-emulator-skill.md`,
  then `docs/reference/digibox-emulation.md`. Not optional.
- **Why the design is the way it is:** `docs/explanation/`
- **Doing a specific job:** `docs/debugger.md`, `docs/snapshots.md`, `docs/instruments.md`,
  `docs/deployment.md`
- **Decisions, with rationale and rejected alternatives:** `plan/module-decisions.md` and
  `docs/decisions/` — not restated here.
- **Open work:** `bd ready`. The tracker is the source of truth, not any file in this directory.
