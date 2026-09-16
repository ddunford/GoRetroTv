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

### Observability
- **Structured JSON logging** with an emulator-time field (`icount`) alongside wall time, because
  every question here is "what was the machine doing at instruction N".
- **No Sentry.** A single-operator developer tool on a demo host; the boot gate and the oracle are
  the real error detection.

### Deployment and access
- **Public**, at `goretrotv.demosrv.uk`, TLS via the existing Traefik, as the predecessor did.
- **Developer surfaces are not exposed.** The gdb stub and the instrument endpoints bind to
  localhost only. This is the one real security control in the project and it gets a conformance
  rule.
