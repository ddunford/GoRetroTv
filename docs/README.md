# The docs

Start at **`orientation.md`** if you have not worked on this before — human or agent.

## By what you are trying to do

<!-- anchor: none - navigation for this directory; its subject is the doc tree, not code -->

| You want to | Read |
|---|---|
| Understand what this project is at all | `orientation.md` |
| Model a device, read the firmware, feed it sections, touch the oracle | `reference/digibox-emulator-skill.md` — **not optional**, then `reference/digibox-emulation.md` |
| Know why the design is the way it is | `explanation/` |
| Inspect a running guest | `instruments.md`, then `debugger.md` |
| Save, restore or move a machine | `snapshots.md` |
| Put it on the public demo host | `deployment.md` |
| Know what was decided, and what was rejected | `plan/module-decisions.md`, `docs/decisions/` |
| Know what to work on next | `bd ready` — the tracker, not a file here |

## What each tree is for

<!-- anchor: none - navigation for this directory; its subject is the doc tree, not code -->

- **`reference/`** — measured evidence about the real firmware. `digibox-emulation.md` records how
  each fact was measured and which earlier readings were withdrawn; every address and constant
  claimed anywhere else traces back to it. Evidence, not prose: treat a disagreement with it as a
  bug in the other thing.
- **`explanation/`** — why the architecture is the way it is, joined up across packages. The
  per-package reasoning lives in the Go doc comments, which is the right home for it; these files
  carry only the arguments that span packages and are therefore in no single one.
- **`decisions/`** — ADRs. The fuller record with rejected alternatives is `plan/module-decisions.md`.
- **Top-level how-to files** — `debugger.md`, `instruments.md`, `snapshots.md`, `deployment.md`.

## What is deliberately not here

<!-- anchor: none - navigation for this directory; its subject is the doc tree, not code -->

Package listings, device tables, the register map, route and flag inventories. All derivable —
`grep`, `go doc`, or `-h` answers them, and a written copy would be wrong within a week. The
measured hardware facts have one home (`reference/digibox-emulation.md`) and the decisions have
another (`plan/module-decisions.md`); neither is restated here.

`internal/wire/wire.go` is generated from `schema/wire.json` and says so in its header — the
generator is the source of truth, not any prose about it.

## Keeping these true

<!-- anchor: ctl.sh -->
<!-- anchor: scripts/docs-anchors.py -->

Sections in `orientation.md` and `explanation/` carry anchors naming the code they describe:

```
<!-- anchor: internal/platform/clock/clock.go#^func \(c \*Clock\) Advance -->
```

The fingerprint is stamped on the day a human verified that section against that code. When the
anchored code changes, the section is flagged for re-reading rather than left to rot quietly — which
matters more than usual here, because agents read these files and act on them with full confidence.

**It is a gate, not a convention.** `./ctl.sh lint` runs it, so a change that moves code out from
under a doc fails the same check as a vet error:

```sh
./ctl.sh docs      # the gate on its own
./ctl.sh lint      # hooks, AGENTS.md, docs, vet, golangci-lint
```

`--strict` is on: a new section that makes a claim about code and names none fails too. That half is
what stops the tree drifting back to unanchored one section at a time while the gate reports green.

When it fails, re-read each section it lists against its anchor, fix what is no longer true, then
stamp it:

```sh
python3 scripts/docs-anchors.py --repo . --docs docs stamp --section '<heading>'
```

Stamping without re-reading is the one move that breaks this — a fingerprint nobody earned reads as
verified for ever and takes the section out of the queue permanently. A section that makes no claim
about code declares that instead, with a reason: `<!-- anchor: none - why -->`.
<!-- fingerprint: sha256:8beea1f0274d336c9a4de49d2c7486702dc033b4b44a748deb65e3ef36cec75e @ 2026-09-22 -->

## The site

<!-- anchor: scripts/docs-site.py -->
<!-- fingerprint: sha256:84e3ad889bec5ee4852d30459a969e711508da63c1f402b885e711a2fcd4fc4a @ 2026-09-22 -->

Read the docs in a browser, with search and navigation, instead of as raw markdown:

```sh
./ctl.sh docs-serve                # http://127.0.0.1:8098/
./ctl.sh docs-serve --lan          # also on this machine's LAN address, for another machine
./ctl.sh docs-serve 9000 --lan     # any port
```

It rebuilds the page whenever the docs have moved, so editing a file and refreshing is the whole
loop: no watcher, no restart. (Editing the *generator* needs a restart — the running process holds
the code it started with.)

**`--lan` is a deliberate exception to `ARCH-DEV-1`,** which records that developer surfaces bind
only to loopback. Without it nothing leaves this machine; with it the viewer is readable from
another machine on the same network. What that publishes is this repository's documentation — not
secret, but a detailed description of the box. It binds the machine's own LAN address rather than
`0.0.0.0`, so it is not also served on every container bridge and VPN interface the host has.

Underneath it is `docs/site.html`: the whole tree as one self-contained page, no dependencies and
no build step, each section showing what it is anchored to and when it was last verified. It is
generated and gitignored, so rebuild it rather than editing it:

```sh
python3 scripts/docs-site.py build --docs docs --title "GoRetroTV"
python3 scripts/docs-site.py check --docs docs    # exit 1 once it has fallen behind
```

`./ctl.sh docs` checks it is current whenever it exists, so a site built from older docs cannot sit
there looking authoritative.
