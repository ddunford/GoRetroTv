# The oracle's boot gate — what it measured, and what a replacement must do

`tools/check-digibox-boot.mjs` was the predecessor's boot gate: a Playwright harness that drove
`digibox-boot.html` in a browser and asserted the real firmware still booted, took a key and
accepted a section. **It was retired in this repository rather than re-pointed** (gort-6ar.23); the
reasoning is at the bottom. Everything it had measured is here, because the numbers are evidence
and the code was only the thing that checked them.

The structural half of it — the only check that needed no browser — was not retired. It lives on as
a Go test in `internal/oracle`, runs in CI on every push, and is described under
[What survived](#what-survived).

---

## Why it existed

A hardware model does not fail with a stack trace. It fails by running for ever doing something
plausible. Twice in one session a change that looked inert stopped the boot, and was found only by
watching a task count by hand fifty seconds later:

- **Batching the per-instruction device pumps** silently multiplied `bootloaderHandoff`'s idle
  threshold by sixteen, because that constant counted *calls* and not instructions. 706 million
  instructions in, five tasks, `BOOTMain` still polling the peripheral link — which reads exactly
  like a firmware hang and is not one.
- **Letting the demux register file read back its own writes** — which sounds like an improvement,
  and was the code comment's stated intent — stopped the RTOS starting at all. The LISR spins until
  `+0x124`'s bit 14 reads *clear*, so "reads back what was written" is an infinite loop.

Neither was caught by anything, because nothing was watching. Both would have been caught by this
gate in under a minute. *An instruction to be careful competes for attention; a check that goes red
does not.*

## What it asserted

Each is a different layer that has actually broken.

1. The boot completes — 42 Nucleus tasks, the full application.
2. It completes in a sane number of instructions, so a change that keeps the boot but throws away
   the link-rate fix (340M instead of 140M) is still a failure.
3. The demux is programmed — three section filters on the PIDs the box asks for, and an enable mask
   matching the filter records.
4. A remote key reaches the application's input layer.
5. A DVB section reaches the firmware's own section handler.

Plus two structural checks: no uncaught page errors, and every `window.__x()` the page calls is also
defined by it.

## The measured constants

These were measured, not chosen. A replacement that picks different numbers is measuring a different
machine.

| Quantity | Value |
|---|---|
| Full task list | 42 Nucleus tasks |
| Warm boot (NVRAM written) | ~140M instructions, ~31 s — 20 tasks @56M → 42 @140M |
| Cold boot (NVRAM blank) | ~447M instructions, ~96 s — 20 tasks @56M → 21 @284M → 42 @447M |
| Warm ceiling used by the gate | 220M instructions |
| Cold ceiling used by the gate | 600M instructions |
| Demux section filters | channel 22 → PID `0x14` (TDT), 23 → `0x11` (SDT), 24 → `0x10` (NIT) |
| Filter enable mask | `0xFFC00000` (filters 22..31) |
| Key-path probes | `0x800297B0` (dispatcher), `0x8006EA04` (key event) |
| Section-path probes | `0x800041B4` (LISR), `0x80004520` (section task) |

**The boot costs three times as much the first time.** The box's NVRAM is a 24C128 persisted to
`localStorage`, so a machine that has booted before starts from a written one. The 228 million
instructions between 20 tasks and 21 are the box doing its first-time initialisation, and they are
real work rather than an artefact — wiping the NVRAM in a headed browser reproduces the headless
figure to three significant figures. **A single ceiling would either miss a regression on warm boots
or cry wolf on every fresh profile**, so the gate had to know which boot it was watching.

## The traps it fell into, and how each was fixed

These are the expensive part of the record. Every one produced a confident wrong answer first.

- **Reading the NVRAM after the boot reports every boot as warm.** The box writes NVRAM *during* the
  boot. Read it before.
- **`domcontentloaded` is not ready.** The page builds its NVRAM array empty and only fills it from
  `localStorage` (or with `0xFF` for a fresh one) in its own init. Reading it in that window reports
  16,384 written bytes on a blank device, which labelled a cold boot warm and failed the instruction
  ceiling with a number that was correct for the boot it actually was. The signal is the NVRAM array
  itself — `eeLoad()` fills it with `0xFF`, as a 24C128 leaves the fab, so an array still entirely
  `0x00` is one nobody has touched. **The Run button is not the signal**: it stays disabled while
  the page loads its own image and starts, so waiting on it times out on a machine that is booting
  perfectly well.
- **Mapping the second flash chip gave the box real work to do after the boot.** The resident content
  manager validates the partition in bank 1 by CRC-ing 1.5 MB of it — a quarter of a second on a real
  box, about twenty-five seconds in the emulator — and while it runs the machine is saturated and key
  frames on the CSI link are dropped. Measured: three presses inside that window produced *one* key
  event between them; eight presses outside it produced two events each, every time. **A key check
  that fires as soon as 42 tasks exist is a coin toss.**
- **And the stillness test has to wait for BGLOAD to have run.** On a cold boot `TASK20.runs` reads 1
  for about two seconds after the 42nd task appears, climbs to 2589 over the next eighteen, then never
  moves again. "The count did not change twice running" is satisfied by the plateau *before* the work
  as readily as by the one after it — so the first version reported "BGLOAD stopped being scheduled
  after 2s" on a box that had not started, which is the exact saturated state the wait exists to avoid,
  reported as the absence of it. The key check passed anyway, because a key reaching the input layer is
  a weaker claim than the application redrawing. **It was a coin toss dressed as a measurement.** The
  fix is to track the peak and only count stillness once the peak has risen above where it started.
- **The baseline depends on `?si=0`.** The page broadcasts by default. The 42 tasks, the three armed
  section filters and the instruction counts were all measured on a box with nothing on the air, and
  an acquiring box adds filters. A gate whose baseline moves silently is worse than no gate.

## Proving it had teeth

`--self-test` aimed a section push at a filter the firmware had **not** armed, which must be refused,
and required assertion 5 to go red. The run was correct when the check *failed*. A gate nobody has
watched fail is decoration, and this one said so in its own header.

## What survived

The structural symbol check needed no browser, so it was ported rather than retired. It lives in
`internal/oracle` and runs under `go test` in CI.

It exists because **a patch deleted `window.__siGuideSlot` while leaving both of its call sites, and
nothing caught it**: the file still parsed, because a deleted assignment is not a syntax error; the
gate still passed, because it runs with `?si=0` and the caller is on the broadcast path, minutes into
a run that never happens there; and the demo host served it for an hour. The box halted with
`window.__siGuideSlot is not a function` the moment it first opened a title PID.

Two properties carried over deliberately:

- It is a **structural** check — does a name that is called also exist — never a semantic one. That is
  the only kind of judgement a pattern match is allowed to make (CLAUDE.md → Code Accuracy Rules).
- It **asserts its own subject count**. A pattern that matches nothing reports a clean page exactly as
  it reports one with no faults.

## Why it was retired here rather than re-pointed

Three things were broken, and fixing them honestly meant building other tasks' deliverables:

1. It resolved Playwright through `../frontend/package.json`. There is no `frontend/` here, and
   **`playwright` is not resolvable in this environment at all** — only a browser cache exists. This
   repository has no `package.json` and no Node dependency (ADR 0001).
2. It read the page from `frontend/public/digibox-boot.html`. Ours is `reference/digibox-boot.html`.
3. It defaulted to `http://localhost:5173/digibox-boot.html`. Nothing serves that. And `file://` is
   not a substitute: the page `fetch`es `FLASH_U202.bin` and `FLASH_U203.bin` as siblings, plus
   `digibox/listings.json`, `digibox/skyuk.dict` and `/api/v1/broadcast/now`.

So a working harness needs a static server placing the oracle page next to the firmware images, a
broadcast endpoint, and a Node/Playwright toolchain. Those are TASK-1.11's, phase 5's and phase 6's
deliverables, and guessing at their shape now would produce a gate that encodes guesses.

**A half-fixed gate is worse than a visibly stale one**, because a visibly stale gate is obviously not
running while a half-fixed one looks live. Nothing operational referenced this file — the only three
mentions anywhere point at `scripts/check-digibox-boot.mjs`, a path it had not occupied for some time,
which is its own evidence of how long it had been unmaintained.

## Three assets this repository does not have

`reference/digibox-boot.html` references `digibox/sky-huffman.js`, `digibox/listings.json` and
`digibox/skyuk.dict`. **None of them is present here.** They are harmless to the boot and fatal to a
gate: anything that keys on console errors — as the retired gate's "no uncaught page errors" check
did — goes red for a reason nobody can act on, and a gate that fails for an unactionable reason is
one people learn to ignore.

Handle it deliberately, and write down which way you chose: serve the three, tolerate their absence
by name, or do not key on console errors at all. Discovering them later as noise is the outcome to
avoid. `?si=0` already avoids the broadcast path that wants the listings pair, so the decision is
mostly about `sky-huffman.js`.

## The recipe that is known to work

Proved while adding the checkpoint emitter (TASK-1.9), so it does not need rediscovering: serve
`reference/` **together with the two `.bin` images** on a private loopback port and drive it with the
plugin Playwright MCP, with `?si=0` on the URL. The Node `playwright` package is not resolvable in
this environment; the plugin MCP is a different thing and does work. Both facts are true at once,
and conflating them is how someone concludes the browser cannot be driven at all.

**The oracle is deterministic with `?si=0`** — two cold boots produced byte-identical checkpoint
streams over 461 million instructions. That is what makes a boot gate's numbers stable enough to
assert on, and it is **not** established with the broadcast on. Do not build a gate that depends on
a broadcasting box.

## What a replacement must do

For whoever builds the oracle harness (TASK-1.9's checkpoint emitter needs driving, and TASK-1.11
needs a boot gate):

- Serve `reference/digibox-boot.html` with the firmware images as siblings, and honour `?si=0`.
- **Anchor every baseline on a number the record states independently** — `docs/reference/digibox-emulation.md`
  and the emulator skill — and never on the page's own self-report. A self-reported baseline agrees
  with itself no matter what changed, which is a gate that cannot fail by construction.
- Wait on the NVRAM array, not on the Run button; read NVRAM **before** booting.
- Pick the ceiling from the warm/cold determination, never a single number.
- Wait for BGLOAD to have run *and then* stopped, tracking the peak — never just a plateau.
- Ship a self-test that deliberately breaks the machine and requires a named check to go red, and
  run it. Assert the subject count of anything that counts.
- Report milestones as well as the total. "The boot costs more than it used to" and "the boot is
  stuck somewhere new" look identical in a single final number, and the first thing anyone wants to
  know is where the extra went.
