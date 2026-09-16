# GoRetroTV — SPEC

> Draft, 16 September 2026. A real Pace 2500N Digibox, running its own firmware, fast enough to
> finish reverse-engineering and good enough to put in front of people.

## 1. Vision

**The problem.** A 1998 Sky Digibox is a specific, irreplaceable experience — the blue menus, the
guide, the way a channel change felt — and it exists now only as hardware in lofts. Emulating the
*look* is a web page anyone could write. Running the **actual firmware** is the only thing that
makes it true: the real menus, the real guide, the real bugs.

That already works. A browser emulator (`reference/digibox-boot.html`) boots the real flash image,
draws the real Sky interface, accepts a real handset, and — as of today — shows real programmes on
the guide, delivered as genuine Sky/OpenTV title sections over a modelled DVB multiplex. Roughly
6,000 lines of measured findings underpin it (`docs/reference/digibox-emulation.md`).

**Why port it.** The browser emulator has run out of road as a *research instrument*, and the
project is still reverse-engineering. Measured costs, from a full day's work:

| | Browser today | Why it matters |
|---|---|---|
| Cold boot | 447M instructions, **135 s** | every experiment pays it |
| Channel-list rebuild | **~200 s** | paid again on every run |
| One experiment | **~6 minutes** | ~15 experiments in a day |
| Re-reaching a known state | impossible | localStorage priming only restores *some* of it |
| Determinism | no | periodic timers changed event counts between runs and caused one wrong conclusion |
| Debugger | bespoke `__breakAt`/`__pcHits` | no breakpoints, no watchpoints, no stepping |
| MPEG-2 video | impossible | would need a decoder in WASM |

A native Go core plausibly runs 15–60× faster, can **snapshot and restore the whole machine**, can
be made **deterministic and replayable**, can expose a **gdb stub**, and can link **ffmpeg**. The
three unsolved screens have each cost multiple six-minute runs per hypothesis; this is the tooling
that finishes them.

**The pitch.** A Sky Digibox you can open in a browser, running the firmware it shipped with — and
underneath it, an emulator you can attach a debugger to.

## 2. Users & personas

- **The developer (primary).** Reverse-engineering the firmware. Needs speed, determinism,
  snapshots, a debugger, and instruments that refuse to report on a machine in the wrong state.
- **The visitor (secondary).** Opens a URL, waits for the box to boot, presses **sky**, and browses
  a 1998 guide. Needs it to work without explanation and without a local install.
- **Not a user:** anyone wanting a Sky-lookalike UI. That is explicitly not this product.

## 3. Scope

### In scope (v1)

- A Go emulator core for the NEC VR4111 (MIPS32 + MIPS16 via JALX) and the Pace/ST peripherals the
  firmware touches, **verified against the browser emulator as an oracle**.
- Framebuffer output to a browser, handset input from it.
- The modelled DVB multiplex: NIT, BAT, SDT, TDT/TOT and Sky/OpenTV title sections, driven from an
  editable data file.
- Snapshot/restore, deterministic replay, a gdb stub, and the probe/instrument suite ported or
  re-pointed.
- **The three unsolved screens: the ALL CHANNELS grid, the OpenTV object carousel, and menu audio.**

### Out of scope (v1)

| Deferred | Target |
|---|---|
| MPEG-2 video decode and the video plane | Phase: Video |
| Listings reconstruction from scanned magazines | Phase: Listings |
| HDMI / Raspberry Pi kiosk output | Phase: TV client |
| ErsatzTV / ffmpeg linear playout | Archived; returns only if Video lands |
| Telephone line and modem return path | Phase: Interactive |
| Box Office / Sky Active transactions | Phase: Interactive |
| Teletext, radio channels, per-viewer accounts | Rejected — the box is shared by design |

## 4. Functional requirements

### Emulator core

- **FR-1** Execute the Pace 2500N flash image from reset to a fully booted application (42 Nucleus
  tasks) without hand-holding. *Acceptance: the task list reaches 42 and the box reports Ready.*
- **FR-2** Implement MIPS32 and MIPS16 including `JALX`, the COP0 registers the scheduler needs
  (Count, Compare, Status, Cause, EPC, ErrorEPC), `ERET` semantics, and delay-slot-correct interrupt
  delivery. *Acceptance: the oracle comparison in FR-6 passes.*
- **FR-3** Model every peripheral the firmware drives: the demux and its 32 section rings, the
  blitter, the DMA controller, the OSD/video registers, I²C + EEPROM, the CSI handset link, the
  smartcard link, the satellite demodulator, and both flash chips. *Acceptance: FR-6.*
- **FR-4** Render the framebuffer at the raster's own geometry and serve it to a browser, with
  handset key input returning over the same channel. *Acceptance: a visitor can press sky and
  navigate the menus.*
- **FR-5** Persist NVRAM between runs, and let a run start from blank NVRAM on request.

### Verification

- **FR-6 — THE ORACLE, in two tiers.** *(Shape set by spike 002; the naive version was
  impractical — a per-instruction trace of one boot is 1.8–59 GB.)*
  **Tier 1:** both implementations emit a 32-bit machine-state hash every 1,000 instructions
  (447,000 checkpoints, 3.6 MB) and the sequences must match across a full cold boot to 42 tasks.
  **Tier 2:** on the first differing checkpoint, both sides re-run that 1,000-instruction window
  with full per-instruction tracing, reporting the exact divergent instruction and both states.
  *Acceptance: tier 1 matches end to end, and tier 2 has been shown to localise a deliberately
  injected divergence.* This is the single most important requirement in the document — it is what
  makes the port verifiable rather than hopeful.
  **Its limit, stated:** where both implementations are wrong in the same way they will agree. The
  oracle catches porting regressions, not inherited ones; the independent evidence is
  `docs/reference/digibox-emulation.md`.
- **FR-7** Deterministic replay: a recorded input trace replays to a byte-identical framebuffer and
  identical instruction count. *Acceptance: two replays of the same trace produce the same hash.*
- **FR-8** Snapshot and restore the entire machine to a file. *(Inventory established by spike 003,
  read off the reference emulator's own `reset()`: CPU + 32 MB RAM + COP0 + the ISA mode bit, the
  interrupt and timer state, **both flash chips' command sequencer state**, and every peripheral —
  CSI, I²C/EEPROM (the NVRAM), VRAM, UART, smartcard, DMA, blitter, display.)*
  *Acceptance: snapshot at instruction N, restore, run to N+10,000,000, and the state hash equals
  that of an uninterrupted run to the same point.* "It restores" is not the test — a partial
  snapshot produces a plausible machine, and every resulting fault reads as a firmware bug.
  Peripherals are therefore written with serialisable state **from the start**; this is built
  alongside FR-6, whose checkpoint hash is the acceptance instrument.

### Broadcast

- **FR-9** Broadcast a modelled multiplex the firmware acquires by itself: NIT, BAT (with the
  `0xB1` line-up descriptor and the `0x4A`/type-`0x91` linkage), SDT, TDT and TOT, on the PIDs the
  firmware arms. *Acceptance: the box subscribes to its own listings without intervention.*
- **FR-10** Broadcast Sky/OpenTV title sections built at runtime from an editable schedule file,
  Huffman-compressed against Sky's published dictionary, addressed with the table id, PID and MJD
  the box is currently asking for. *Acceptance: the guide shows correct now and next.*
- **FR-11** Serve the in-world clock from one authority so the emulator and any UI cannot disagree.
  *Acceptance: the box's clock face matches real London wall-clock time, 1:1.*
- **FR-12** Re-read the schedule file while running, so an edit appears on the guide without a
  restart. *Acceptance: an edit is visible within two carousel waves; a malformed edit keeps the
  last good schedule and says so.*

### The unsolved screens (v1, and each is research before it is code)

- **FR-13** The **ALL CHANNELS** grid lists channels and their programmes. *Currently: it draws a
  time header and no rows.* See §13.
- **FR-14** The box accepts an **OpenTV object carousel** — DSM-CC modules carrying applications and
  resources. *Currently: never fed, and the box never asks. `0x800BECF0`, the Huffman decompressor,
  has never executed once.*
- **FR-15** **Menu audio** plays. *Currently: unknown whether the firmware drives audio at all; that
  is the first measurement, not an implementation task.*

### Instruments

- **FR-16** A gdb stub for the MIPS target: breakpoints, watchpoints, stepping, register and memory
  access.
- **FR-17** Carry over the probe suite's capabilities — PC histograms, range and read/write watches,
  call tracing, section injection, the o-code trace — with the guards that make them refuse to
  report on an unexpected machine state.
- **FR-18** A boot gate equivalent to `./ctl.sh digibox`: prove a change did not break the boot, the
  handset, or section delivery. *Acceptance: it can be shown to go red.*

## 5. Key user flows

1. **Visitor.** Open the URL → the box boots with a status line saying what it is doing → press
   **sky** → the Box Office menu → left to **TV GUIDE** → a guide with real 1998 programmes.
2. **Developer, fast loop.** Restore a post-acquisition snapshot → press a key → read an instrument
   → change one thing → repeat. Target: seconds, not six minutes.
3. **Developer, deep loop.** Attach gdb → break on an address → inspect → step.
4. **Developer, verification.** Run the oracle comparison → see either "identical to N instructions"
   or the exact index where the two machines diverged.
5. **Editing the schedule.** Edit the data file → the running box shows it within two waves.

## 6. Non-functional requirements

| Property | Target | Why this number |
|---|---|---|
| Emulation speed | **≥ 15M instructions/s** sustained (stretch 40M) | **Measured, spike 001:** a plain switch interpreter in Go does **44.2M/s** against the browser's 3.1M — but that is an upper bound with no MIPS16 decode, COP0, interrupt checks or peripheral dispatch. 15M/s puts a 447M cold boot at ~30 s against 135 s. |
| Snapshot restore | **< 1 s** | replaces a ~200 s rebuild |
| Oracle agreement | state-hash checkpoints every 1,000 instructions across a **full cold boot**, matching end to end | **Measured, spike 002:** a per-instruction trace is 1.8–59 GB per run and unusable; checkpoints are 3.6 MB and run in CI. |
| Determinism | byte-identical framebuffer across replays | removes the noise that caused a wrong conclusion |
| Browser latency | interactive on a LAN; 720×576 8bpp at ~10 fps is trivial bandwidth | the OSD is not a video stream |
| Boot gate | runs in CI and can be shown to fail | a gate nobody can see fail is decoration |

**Security.** The demo host is public and has no accounts, no user data and nothing to protect. The
gdb stub and the instrument API are **developer surfaces and must not be exposed publicly** — that
is the one real control.

## 7. Integrations & external dependencies

- **The flash images** (`firmware/`) — the Pace 2500N's own ROM. Not redistributable; gitignored,
  with a manifest recording sizes and checksums.
- **Sky's published Huffman dictionary** — for title text.
- **ffmpeg** — Video phase only.
- **archive.org** — Listings phase only.
- No accounts, no payments, no LLMs.

## 8. Domain concepts

**Box** the emulated Digibox · **Flash image** its firmware · **Application** the OpenTV bytecode
program the firmware runs · **o-code** that bytecode · **Native** a firmware function the bytecode
calls as `scall(module, function)` · **Section** one DVB table fragment · **PID** the stream it
arrives on · **Match unit** the hardware filter deciding which sections are kept · **Line-up** the
channels, from the BAT's `0xB1` descriptor · **Listings reference** the per-service id the box turns
into the table-id extension it requests listings with · **Broadcast day** the in-world date, as an
MJD · **Notification slot** the 44-byte record the guide registers to be told new data arrived.

## 9. Architecture sketch

```
   browser  ──websocket──  Go service  ──  emulator core (CPU + peripherals)
   framebuffer + keys                   │
                                        ├── broadcast: schedule file → DVB sections
                                        ├── snapshot / replay / gdb stub
                                        └── instruments (histograms, watches, traces)
```

One Go binary. The browser is a display and a keypad, not a participant.

## 10. Repository topology

Single repo, `github.com/ddunford/GoRetroTv`, working at
`/opt/workspaces/development/goretrotv.demosrv.uk`.

## 11. Constraints & preferences

Go for the core (stated). Browser output first, HDMI/Pi later (stated). The browser emulator is
**kept in-tree as the oracle**, not deleted — it is the only independent check the port has.

## 12. Deployment & access

`goretrotv.demosrv.uk`, public, TLS via the existing Traefik. A **demonstration host with no real
users**: findings there are real, incidents there are not. Developer surfaces are not exposed.

## 13. Risks & open questions

**The three unsolved screens are research, and v1 includes them. What is already known:**

- **The ALL CHANNELS grid (FR-13).** Reached `sky` → left → select, *not* the `tv guide` key. It
  draws a header and no rows, runs 14,956 o-code instructions against a working menu's 34,796,
  builds 16 widgets and returns cleanly — it finds nothing to list. **Ruled out by measurement:**
  listings (there are no rows to fill), the line-up (all twelve services are present with channel
  numbers and flags), the `0xB1` flag bits, the notification model (the grid registers no slot), and
  the 18-byte service record array (nothing reads it while drawing). `DS+0x02E20C` feeds
  `add -12; switch` and is a **view selector, not a channel count** — an earlier reading of it as a
  count was wrong. Next: the executed reads at `0x9FC75F7B` and `0x9FC762FD`.
- **The object carousel (FR-14).** Every carousel-side function is cold through the grid press —
  `carouselAcquire`, `getCodeModule`, `appById`, `appStart`, `registryInsert`, the Huffman
  decompressor, the carousel parser — with a live control proving the census works. **The box never
  asks.** So the prior question is what makes it ask. The one armed PID nobody feeds is **`0x52`**.
- **Menu audio (FR-15).** Unmeasured. Establish whether the firmware drives audio at all first.

**Risks.**

- **Porting silently loses correctness.** This domain fails by running plausibly, not by crashing —
  a wrong MIPS16 shift operand order or the blitter's fill bit gives a working-looking box. FR-6 is
  the mitigation and it is not optional.
- **The oracle is itself only a model.** Where both are wrong they will agree. The measured facts in
  `docs/reference/digibox-emulation.md` carry their own provenance; anything not measured there is
  not evidence.
- **v1 contains genuine unknowns.** FR-13 to FR-15 have no guaranteed completion date. Three
  hypotheses on the grid have already been killed. If they prove unbounded, the honest move is to
  ship parity and reopen them, not to redefine them as done.
- **The firmware is not redistributable.** It must stay out of the repo and out of the image.

## 14. Success criteria

1. A cold boot to 42 tasks agrees with the oracle: matching state-hash checkpoints end to end, with tier-2 localisation demonstrated against an injected divergence.
2. That boot takes **under 30 seconds** (spike 001), and a snapshot restore makes it a once-per-session cost rather than a per-experiment one.
3. A snapshot restores to a pressable, acquired box in under a second.
4. A visitor at `goretrotv.demosrv.uk` reaches a guide showing correct now-and-next, with no
   explanation and no local install.
5. The ALL CHANNELS grid lists channels and programmes.
6. `gdb` attaches and breaks on a firmware address.
7. The boot gate runs in CI and has been demonstrated failing.

## Spikes run before planning

| Spike | Question | Verdict |
|---|---|---|
| `001-go-interpreter-throughput` | Is Go fast enough to justify the port? | **Proven, target revised.** 44.2M/s against the browser's 3.1M — a 14× floor. The first measurement said 83M/s and was measuring a runaway PC executing unmapped zeros; the PC histogram caught it. |
| `002-oracle-comparison` | Is FR-6 practical? | **Rewritten.** Per-instruction traces are 1.8–59 GB; two-tier checkpoint hashing is 3.6 MB and localises to 1,000 instructions. |
| `003-snapshot-completeness` | What must a snapshot capture? | **Proven tractable.** The inventory is the reference emulator's `reset()`; the acceptance test must be a state-hash match after restore, not "it restored". |

## References

- `docs/reference/digibox-emulation.md` — the measured record. Every claim above traces to it.
- `docs/reference/digibox-next-session.md` — where the listings work stopped.
- `docs/reference/digibox-emulator-skill.md` — CPU reference, firmware conventions, measurement rules.
- `docs/reference/lessons-from-the-browser-emulator.md` — mistakes already paid for.
- `reference/digibox-boot.html` — the oracle.
- Full history of the predecessor: `/opt/workspaces/development/archive/skytv.demosrv.uk-2026-09-16`
  (610 commits, the FastAPI backend, the listings pipeline, ErsatzTV playout, 464 tests, the
  conformance harness).
- NEC VR4111 User's Manual, µPD30111, U13137EJ2V0UM00 2nd ed., April 1998.
- tvheadend `src/epggrab/module/opentv.c`; `dave-p/openTVtoXML`; `jcdutton/loadepg`.
