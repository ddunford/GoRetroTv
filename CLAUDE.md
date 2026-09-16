# GoRetroTV

> A Pace 2500N Sky Digibox (1998–2002) running its own firmware, in Go, with a browser for a screen.

*This file is the project's instructions for **every** agent. `AGENTS.md` is a symlink to it, so
Codex CLI and Claude Code read the same text and cannot drift apart. Where something is specific to
one tool it says so and names the equivalent for the others.*

## Stack

| Layer | Technology |
|---|---|
| Emulator core | Go 1.22+, single binary, no CGo in the core |
| Browser client | One static page + a small TypeScript module — **no SPA framework** |
| Transport | WebSocket: framebuffer out, handset keys in |
| Persistence | Files only — NVRAM image, machine snapshots, recorded traces |
| Video (later phase) | ffmpeg via CGo or a subprocess — not in v1 |

Scaffolds: `/go-scaffold`. Agent: `/go-engineer`. Standards: `/go-testing`. *(Claude Code skills. An
agent without them builds to the conventions in this file and the recorded decisions in
`plan/module-decisions.md` and `docs/decisions/`, which is where the binding parts live anyway.)*

## Architecture Decisions

Full record with rationale and rejected alternatives: `plan/module-decisions.md`.

- **One binary, package boundaries inward** — devices know the bus, the bus knows nothing of the web.
- **Every device implements one interface including `Snapshot`/`Restore`, from the first line
  written.** Retrofitting serialisation across eight device models is the expensive version, and a
  partial snapshot does not fail — it produces a plausible machine whose faults read as firmware
  bugs (spike 003).
- **The instruction counter is the clock.** Timers, the broadcast carousel and device pumps are all
  driven off `icount`, never wall time. Wall-clock scheduling is what made the predecessor
  non-deterministic between runs and caused a wrong conclusion about event counts.
- **No goroutine in the instruction loop.** Single-threaded and deterministic by construction;
  concurrency lives at the edges. This is what makes byte-identical replay possible at all.
- **Errors are values; a bad guest instruction halts visibly.** It must never take the process down
  or, worse, continue plausibly.
- **No database, no auth, no tenancy, no queue, no cache** — see the record. Nothing here needs them.
### Shared / platform layer
Owner: `internal/platform/`. Five primitives, each with three or more real consumers — declared here
because the alternative is not "no shared layer", it is six improvised copies discovered by
`/plan-reconcile` after they have shipped.
- **`hexfmt`** — address and word formatting, one casing, one width. The predecessor's instruments
  built lookup keys with `hex32` (uppercase) and read them with `toString(16)` (lowercase), and every
  address containing a hex letter reported a plausible **zero**. That cost a day and two wrong
  findings; one function with one casing is the fix.
- **`statehash`** — the machine-state hash. It is the acceptance instrument for FR-6 (oracle), FR-7
  (replay) and FR-8 (snapshot) and is also what the framebuffer and boot-gate comparisons use. One
  definition, or the three requirements verify against three different hashes.
- **`snapcodec`** — the versioned encode/decode every `Device.Snapshot` writes into, so completeness
  is checkable by `ARCH-SNAP-1` rather than hopeful.
- **`instrument`** — the subject-assertion guard: an instrument that cannot find what it counts
  returns a harness failure, never zero. This project states that as a convention; a convention with
  no code owning it becomes twelve copies that each forget it differently.
- **`clock`** — the icount scheduler every timer, device pump and carousel wave is driven off.
**Not shared, deliberately:** the Huffman codec, MPEG CRC-32 and the section builders (broadcast
only); the MIPS16 decode tables (cpu only); the blitter's fill-bit and the demux's match-unit
semantics (their own devices). Domain knowledge stays in its package.

- **Developer surfaces bind to localhost only.** The gdb stub and instrument endpoints are the one
  real security control on a public demo host, and they carry a conformance rule.

## Non-Obvious Domain Patterns

*The firmware's own behaviour — six thousand lines of it — is in
`docs/reference/digibox-emulation.md`, and that file is evidence rather than documentation: it
records how each thing was measured and which earlier readings were withdrawn. **Read it before
modelling any device.** This section is only for patterns discovered while building the port.*

- **A hardware model does not fail with a stack trace. It fails by running for ever doing something
  plausible.** A wrong register model is a boot that stops at 20 tasks; a wrong constant is a link
  silently running at sixty baud. Every change gets the boot gate afterwards.
- **Recording what the firmware WRITES is inert; changing what a read RETURNS is not.** Making the
  demux register file read back its own writes — which sounds like an improvement — stopped the RTOS
  starting at all. Log writes freely; change reads only with a reason and a boot afterwards.
- **The oracle proves this port matches the browser emulator, not that either matches a Digibox.**
  Where both are wrong in the same way they agree. Inherited errors are caught only by the measured
  record.

## The emulator

**Read the emulator reference before writing the CPU core, modelling a peripheral, reading the
firmware's disassembly, feeding it DVB sections, or touching the oracle.** In Claude Code that is the
`digibox-emulator` skill; every other agent reads the same text at
`docs/reference/digibox-emulator-skill.md`. It is not optional in either case. The NEC VR4111 and
MIPS16 reference, the firmware's device conventions, the instrument suite and the measurement rules
are there rather than here, because only some sessions need them — and each rule in it cost the
predecessor real time.

The oracle is `reference/digibox-boot.html`. It is a **measuring instrument, not a sibling
implementation**: editing it to agree with the Go port is the one move that destroys its value, and
it will look reasonable at the time. Where the two disagree the measured record decides, and where
the record is silent the answer is to measure.

## Conventions

- **Firmware** lives in `firmware/`, is gitignored and is not redistributable. See its MANIFEST.
- **The oracle** is `reference/digibox-boot.html`. It is kept deliberately and must not be deleted:
  it is the only independent check the port has.
- **Testing:** Go's own test framework; table-driven where the shape suits. The boot gate and the
  oracle comparison are the integration-level checks and both run in CI.
- **Every instrument asserts its own subject.** A census that cannot find the thing it is counting
  is a harness failure, never a count of zero — this rule is written in blood upstream.
- **Git hooks:** `.githooks/pre-commit` (credential guard + bd mirror check); `git config
  core.hooksPath .githooks`. That setting is LOCAL config a clone does not carry, and **git runs no
  hooks at all, silently and with exit 0, when it points at a directory that does not exist** — so a
  fresh clone is unguarded and looks identical to a guarded one. `./ctl.sh hooks` asserts it, and
  `./ctl.sh lint` runs that assertion.
- **Naming:** no phase numbers, ticket ids or plan metadata in code.
- **Shell commands must be non-interactive.** `cp`, `mv` and `rm` are aliased to `-i` on some
  systems, which hangs an agent for ever on a y/n prompt nobody can answer: use `cp -f`, `mv -f`,
  `rm -f`, `rm -rf`. Likewise `-o BatchMode=yes` for `ssh`/`scp` and `-y` for `apt-get`. The same
  failure wearing its other hat: **never read an exit code through a pipe** (`cmd | tail` gives you
  `tail`'s status), and never `pgrep`/`pkill -f` a pattern your own command line contains.

## Tracker

Open work: **beads (`bd`)** — `bd ready` / `bd blocked` are the single source of truth. No
`plan/TODO.md`. Claim before building (`bd update --claim`), close with a verification reason
(`bd close --reason`). Phase-file `[ ]` / `[x]` boxes are a **generated mirror**
(`scripts/bd-mirror-phases.py`) and are never hand-ticked. In Claude Code, execute with
`/team-execute`, stating scope in plain words; any agent can work the same graph directly with the
`bd` commands above.

## Out of Scope (v1)

MPEG-2 video and the video plane (phase: Video) · listings reconstruction from scanned magazines
(phase: Listings) · HDMI / Raspberry Pi kiosk (phase: TV client) · ErsatzTV and ffmpeg linear
playout (archived) · telephone line and modem return path (phase: Interactive) · Box Office and Sky
Active transactions (phase: Interactive) · teletext, radio channels, per-viewer accounts (rejected —
the box is shared by design).


<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:970c3bf2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
