# Module and architecture decisions

Why the choices are what they are. Not read at runtime — this exists so a future reader can answer
"why is there no database?" without git archaeology.

## Composition: none, deliberately

`~/.claude/modules/compositions/` offers saas, ecommerce, api-service, content-platform,
internal-tool, ai-product, realtime-ai, team-collaboration and mobile-app. **None fits.** Every one
assumes a product with users, records and CRUD. This is a hardware emulator: its "domain" is a MIPS
CPU and eight peripherals, and its "data" is a 2 MB ROM.

Forcing the nearest fit would import an ORM, a migration story, an auth module and a permissions
model that nothing here needs, and every later phase would inherit the ceremony.

## Modules in use: none

Checked against the catalogue. `auth` — the box has no accounts and the SPEC says viewers never
authenticate. `billing`, `notifications`, `analytics`, `file-storage`, `admin`, `audit-log`,
`feature-flags`, `search` — no product surface needs any of them. **Everything here is custom
build**, which means the custom-feature specs in the phase files carry the entire design weight
rather than being a footnote to module config.

If the listings-reconstruction phase returns (SPEC §3), it will bring a datastore with it and this
decision gets revisited then — not before.

## Architecture decisions

### Application structure
- **A single Go binary**, not a service split. The emulator core, the broadcast generator and the
  browser transport are one process because they share the machine and a process boundary between a
  CPU and its peripherals would be absurd.
- **Package-based boundaries** (`internal/cpu`, `internal/bus`, `internal/device/*`,
  `internal/broadcast`, `internal/oracle`, `internal/snapshot`, `internal/web`), with dependencies
  pointing inward: devices know the bus, the bus knows nothing of the web layer.

  **Two names changed when phase 1 built this, recorded here so the plan and the tree agree:**
  - **`internal/web` shipped as `internal/httpx`**, with `internal/app` holding the wiring. `httpx`
    is the ordinary Go name for "our additions to `net/http`", and it keeps the *server* plumbing
    distinct from the browser client that phase 5 will add — which is what a reader would expect
    `web/` to mean.
  - **`internal/snapshot` does not exist, deliberately.** Snapshot framing split along the line
    settled during phase 1: the generic part — versioned name→blob container, duplicate rejection,
    name-ordered encoding so equal sets produce identical bytes, and the completeness refusal — is
    `internal/platform/snapcodec`, because it is what `ARCH-SNAP-1` must inspect and one format is
    the whole point. The *policy* that "the expected set is my attached devices" stays in
    `internal/bus`, because what is attached to a bus is bus knowledge. A third package would have
    owned neither half.

### The core patterns
- **Devices behind one interface** (`Read(addr, size)`, `Write(addr, size, value)`, plus
  `Snapshot`/`Restore`) registered on an address-decoding bus. Uniformity is what makes snapshot
  completeness checkable rather than hopeful — spike 003.
- **Every device is serialisable from the first line written.** Retrofitting serialisation across
  eight device models is the expensive version, and a partial snapshot fails by producing a
  plausible machine (spike 003).
- **Errors are values.** `error` returns on anything that can fail; panics only for genuine
  programmer error. An emulator must not die on a bad guest instruction — it must report and halt
  visibly, the way the browser's `emulator-error` halt does.
- **No goroutine in the instruction loop.** The core is single-threaded and deterministic by
  construction; concurrency lives at the edges (the web transport, the broadcast ticker), which is
  what makes FR-7's byte-identical replay achievable at all.
- **The instruction counter is the clock.** Every scheduled thing — timers, the broadcast carousel,
  device pumps — is driven off `icount`, never wall time. Wall-clock scheduling is what made the
  browser emulator non-deterministic between runs and caused at least one wrong conclusion.
- **Application entry uses the oracle's declared host handoff.** The supplied ROM reaches an idle
  bootloader state but does not execute the flash entry stub in measured runs; its valid image
  descriptor instead selects a service loop. The browser oracle reaches the application by a
  one-time PC/ISA/RA change after 200,000 idle instructions and a flash-header check. The Go
  machine uses the same explicit policy, records its firing, and leaves decompression to guest
  instructions. This is an emulator intervention, not evidence of a physical Digibox handoff.
  The rejected alternative was claiming a guest-only transfer from a ROM branch that has not
  produced one through 200 million measured instructions. Evidence: `reference/digibox-boot.html`
  `bootloaderHandoff`, `docs/reference/digibox-emulation.md` and `gort-f3f.10`.
- **Sky menu presentation uses the oracle's declared post-boot gates, separately from hardware.**
  A cold boot must first create all 42 Nucleus tasks. Only then may the optional policy answer
  the application checks at RAM `0x80054F84` and flash offset `0x72FA1`, once, with the exact
  expected flash byte checked before modification. The browser oracle does this by default; Go
  keeps it explicit behind `-sky-gates`. Applying either answer before the RTOS boot was measured
  to stop at 28 tasks, while leaving both unanswered makes the application unbind its screen.
  A warm boot with persisted EEPROM and the declared policy draws the Box Office menu after Sky
  key `0x7D`. This is a host presentation intervention, not evidence of a physical Digibox register
  behavior. The clean no-gates checkpoint stream verifies hardware parity; the gated warm surface
  comparison verifies the menu path. Rejected alternatives were an early firmware patch and
  treating the menu answers as guest-produced hardware values. Evidence: `internal/machine/sky_gates.go`,
  `tools/oracle-warm-surface.mjs`, `plan/test-plan-phase-3b.md` TC-3b.6, and the measured record
  in `docs/reference/digibox-emulation.md`.

### The shared / platform layer
`internal/platform/` holds `hexfmt`, `statehash`, `snapcodec`, `instrument` and `clock` — the full
rationale and the litmus each passes is in `CLAUDE.md → Shared / platform layer`.

**Why it is decided before a line is written.** This is a greenfield tree with no duplication to
find, so the reuse question is not "what already exists" but "what will six packages each invent".
Two of the five are named because the predecessor already paid for their absence: `hexfmt` (a casing
mismatch between a key's producer and its consumer reported zero for every address containing a hex
letter) and `instrument` (the subject-assertion rule existed as prose and was broken by three agents
in one afternoon). `statehash` is here because it is the acceptance instrument for three separate
functional requirements and three copies would verify three different things.

**The boundary is enforced, not hoped for.** `ARCH-LAYER-1` (phase 1a) already refuses outward
imports; `internal/platform` may import nothing from `internal/device`, `internal/cpu`,
`internal/broadcast` or `internal/web`, which is what stops domain logic drifting into it.

### Not applicable, and why
- **No database.** No persistent product data. NVRAM is a byte array in a file.
- **No auth, no tenancy.** One shared box, no accounts — the same decision the predecessor made.
- **No API envelope, no pagination.** The HTTP surface is a framebuffer stream, a key channel and a
  handful of developer endpoints. `/architect api` has nothing to decide here.
- **No cache layer, no queue.** Nothing to cache; nothing to defer.

### Frontend
- **No SPA framework.** The browser is a canvas, a keypad and a websocket. React, a router and a
  state library would all be ceremony around `putImageData`. A single static page and a small
  TypeScript module, served by the Go binary.
- This is a **rejection, not an omission**: if the page later grows real UI (instrument panels,
  a debugger front-end), revisit it then.

### WebSocket transport dependency
- **Adopt `github.com/coder/websocket@v1.8.15`** at the outward browser transport boundary. Go's
  standard library has the HTTP server but no WebSocket protocol implementation. The tagged module
  has no transitive module requirements, so this adds one reviewed dependency while the emulator
  core remains standard-library-only. ADR 0001 records the decision and `ARCH-MODULE-1` enforces
  the exact path and version. Last web-verified: 2026-09.

### Observability
- **Structured JSON logging** with an emulator-time field (`icount`) alongside wall time, because
  every question here is "what was the machine doing at instruction N".
- **No Sentry.** A single-operator developer tool on a demo host; the boot gate and the oracle are
  the real error detection.

### Host interventions — the box is driven through its inputs — 2026-09-21

**The decision: a host write to guest state is an INSTRUMENT, never a MECHANISM.** The shipped
emulator reaches its screens because the modelled inputs — the transport stream, the viewing card,
the handset — satisfied the firmware, not because the host wrote to memory or flash. Reading *and
writing* guest state from tests, spikes, the gdb stub and firmwaretrace probes stays entirely
permitted, as does Ghidra and any static tool on the images: that is how this port learns anything.
The test for any change is **if every instrument were removed, would the box still reach this
screen?**

This is the mainstream accuracy position rather than a preference, and it is worth citing as such:
emulators accumulate title-specific hacks to paper over unmodelled behaviour, and the accepted path
is to retire them as accuracy improves — ZSNES's maintainers explicitly removed its hacks,
"implementing proper code that tricky games relied on"
(<https://emulation.gametechwiki.com/index.php/Emulation_Accuracy>), and the HLE/LLE tradeoff is a
recognised axis (<https://alexaltea.github.io/blog/posts/2018-04-18-lle-vs-hle/>).

**Three tiers, and every intervention is classified into one.**

| Tier | Rule | What is in it today |
|---|---|---|
| Mechanism | Forbidden in the product | The Sky menu gates |
| Declared policy | Only where no guest path is established; explicit, reported, never silent | Application entry |
| Instrument | Always allowed, read and write | gdb stub, firmwaretrace, read/write watches, tests, spikes |

**What the survey found, and it is worse than the idea assumed.** `cmd/goretrotv/main.go:125` calls
`board.New(images, true)` — the Sky gates are hard-coded on in the product with no flag and no
config. `acquiredSnapshot()` then *requires* `SkyGates.Done()` and `Handoff.Done()`, so the
post-acquisition snapshot the live demo restores is a machine that only exists because of the pokes.
Retiring a gate therefore invalidates that snapshot; the naive acceptance test "reach the same state
hash" cannot work, because the recorded hash is the hash of a poked machine.

**The gates are not card checks, and that is settled by a primary period source.** The official
Pace/BSkyB Digibox user guide tells a customer with no viewing card to open the Services screen for
the helpdesk number, and handles card absence as a per-programme overlay rather than a menu lockout
(<https://drew1440.com/wp-content/uploads/2020/11/bskyb2400_userguide.pdf>, p.4 and pp.35-36). The
card hypothesis this decision started from is dead. The emulator's own record agrees from the other
direction: a card answers one frame and `0x8002A534` releases `Periph` unconditionally.

**What they probably ARE, and the evidence is circumstantial but coherent.** A real box's first
power-on acquires its channel list from a default transponder (11778 V 27.5 2/3), showing
"SEARCHING FOR LISTINGS - PLEASE WAIT" until channel 998 populates; boxes that received every other
transponder still booted to "No Satellite Signal Being Received" when 11778 alone was weak
(<https://www.satandpcguy.com/satellite-help/sky-default-transponder-frequency-setting-for-sky-digiboxes/>).
A RAM word going `0xFFFFFFFF`->0 and a persisted flash byte incrementing read as *acquired /
initialised* state. Both messages the box shows are acquisition states, and both are things the
owner has seen on the demo. INFERRED, and the spike below is what settles it.

**The carousel trigger is documented, and it is not DSM-CC.** OpenTV 1.x uses module "flows", not a
DSM-CC object carousel: "an OpenTV flow always contains a directory module, which is automatically
downloaded (before any other module) by a STB when an application is signalled in the programme
stream", with a flow watcher launching on a new application id
(<http://www.diva-portal.org/smash/get/diva2:1032128/FULLTEXT01.pdf>, Fagerqvist & Marcussen, LTU
2000:075, §3.3-3.6). We signal no application anywhere. This supersedes TASK-7.5's premise.

**The decisions.**
- **`ARCH-POKE-1`**: no host write to guest state is reachable from the server binary's default
  configuration, with probes that fail when one is. The constraint is mechanised rather than
  written down, in the shape of `ARCH-DEV-1` and `ARCH-FW-1`.
- **The Sky gates survive only as an oracle mode.** A flag is added and the product default is
  inverted; the oracle keeps its existing gated baseline and its 470,000 checkpoints, because
  re-baselining the only independent check against behaviour we just changed would spend the
  instrument to buy the result.
- **Application entry remains a declared policy** and is out of scope here. The record states that
  no normal guest path to the application has been established and that the oracle's own injection
  cannot prove the firmware transferred control; retiring it is a much larger claim and gets its own
  question.
- **The video plane and a test image are a separate phase**, deliberately re-opening the v1
  out-of-scope decision, once the gates work has run.
- **Acceptance for any retirement**: delete the intervention, cold boot, reach the same screen and a
  newly recorded state hash, while the gated oracle path still matches its existing baseline.

**Rejected, with revisit triggers.**
- *Building a DSM-CC object carousel* — wrong middleware; revisit only if measurement shows this
  firmware speaks DSM-CC after all.
- *Re-baselining the oracle now* — revisit once an ungated boot is proven and stable.
- *Assuming PAT/PMT is the signalling mechanism* — we send neither, but the record mentions PAT and
  PMT **zero times** in 6,000+ lines and no observation shows a PID `0x00` filter, so the box
  wanting them is unproven. It is the cheap standard-DVB first attempt, not a diagnosis.
- *Feeding PID `0x52`* — withdrawn. The record establishes `0x52` is transient, opens in response to
  our own NIT and closes again, and is the box's own subtable registration for network `0x20`. An
  earlier "nothing feeds PID 0x52" conclusion is named in the record as an artefact of a join.

**Spikes, in order.** (1) What do RAM `0x80054F84` and flash `0x72FA1` actually ask — Ghidra plus
write-watches, against the acquisition hypothesis. (2) Signal an application in the programme stream
and watch `0x800BECF0`: one execution confirms the trigger, silence falsifies it cheaply. (3) Log
every access to `0xB200A000`-`0xB200A0F4` by offset, size and PC inside handler `0x8000665D`, and
model what the 160 bytes do — today `boardlatch` models the first word as an echo and its own
comment admits the wider identity is unestablished.

### Deployment and access
- **Public**, at `goretrotv.demosrv.uk`, TLS via the existing Traefik, as the predecessor did.
- **Developer surfaces are not exposed.** The gdb stub and the instrument endpoints bind to
  localhost only. This is the one real security control in the project and it gets a conformance
  rule.
