# Lessons — GoRetroTV

Project-specific rules from corrections made while building this. The portable ones live in
`~/.claude/lessons.md` and the framework's `CLAUDE.md`; these reference this project's host,
firmware, `ctl.sh` and plan, so they stay here.

---

### Deploy every verified change to the demo host — do not ask each time

`goretrotv.demosrv.uk` is a **dev/demo host on a dev machine**, not production. There is no
customer, no SLA and no release ceremony, and the whole point of it is to see the box working.
So a change that has passed its checks gets deployed in the same turn that finishes it:

```bash
./ctl.sh up-public      # builds the image and restarts behind Traefik, then waits for health
```

**`./ctl.sh restart-public` is NOT a deploy** — it restarts the container on the image it already
has, which is the box power-cycle. Deploying with it leaves `/health` reporting the old commit
while you announce the new one. That mistake has been made here once already.

Asking for deploy approval on every change was the wrong default and the user said so plainly
(2026-09-20): *"this constant changing and not deploying is getting boring."* Ask only when the
change is genuinely risky — a schema or protocol break, something that could wedge the box for
other viewers, or anything the user has flagged as sensitive. Otherwise ship it and say what
`/health` reports, because the version string is the proof it actually landed.

### The handset's key codes are MEASURED, and most of the project's labels were never proved

Only four codes were ever verified against a screen, and one long-standing label was simply wrong:

| raw | what it actually does, measured 2026-09-20 from the post-acquisition snapshot |
|---|---|
| `0x7D` | Sky menu on the **BOX OFFICE** tab (frame `FE8D1CCC`) — the page calls this "sky" |
| `0x7E` | Sky menu on the **SERVICES** tab (`64AF0A8D`) — the record called this "the Box Office menu again" |
| `0x80` | **TV GUIDE** (`B424095B`) |
| `0xF5` | **INTERACTIVE** (`20DB8CF2`) |

`0x7E` was mis-attributed because the original walk pressed it while the box was *already* showing
Box Office, so the screen never changed and the sentence attached to the measurement described a
screen nobody had checked — this file's oldest failure mode, and the record names it as such.

**The real remote is the Sky Rev 9**, and it has `sky` and `box office` as SEPARATE buttons, so a
distinct sky code exists and has not been found. Establish a code by pressing it from a screen that
is *not* already its destination and comparing frame hashes; never from a public key table and never
from the label someone gave it.

### A coverage check greps for a TOKEN, and the token is narrower than the claim

Twice on 2026-09-21, in two unrelated places, a check reported clean because what it searched for
was narrower than what it claimed to cover.

`ARCH-FW-1` says "firmware bytes are absent from git and built images" and matches the three names,
sizes and digests in `firmware/MANIFEST.md` plus the U202 magic. The Sky Huffman dictionary carries
the *same* stated licence contract — `dictionaries/MANIFEST.md` says "never committed and never
baked into a published image" — and the rule has no concept of it, so two tracked copies sat on a
public remote from the first commit with 7/7 rules passing over them the whole time. The same hour,
auditing for exactly that, a sweep of `git ls-files` grepped for paths containing `dictionar` and
missed both, because they live in `tools/skyepg/` and `web/data/` and are named `skyuk.dict`.

**Search by the thing's own identity — extension, size, digest, magic — not by where you expect it
to live.** `git ls-files '*.dict'` finds both copies; `grep dictionar` finds neither. And when a
rule's invariant names a CLASS of thing ("firmware bytes", "non-redistributable data"), enumerate
every member of that class and check the scope against the list, because a rule that covers one
member and passes reads exactly like a rule that covers them all.

This is the project's own instrument rule wearing different clothes: *a census that cannot find the
thing it is counting is a harness failure, never a count of zero.* A green check whose subject was
never in scope is a zero dressed as a pass.

### A gate nobody can start looks exactly like a gate nobody broke

`tools/input-replay-gate.sh` and `tools/snapshot-runon-gate.sh` both used `rg`. On this machine
`rg` is a shell FUNCTION from an interactive profile, not a binary, so a `#!/usr/bin/env bash`
script gets `rg: command not found` and the gate dies on its first parse line — before it has
checked anything. Both had been in that state, and nothing said so, because a gate that cannot
start produces no failures.

They were found only because a change to the CSI card policy needed exactly those two gates to
verify it: the replay gate records a Sky key and compares two real-firmware framebuffer replays,
which is the very path that change touched. Had the change been worse, they would still have been
silent.

**Gate scripts run in a bare non-interactive shell and get none of your profile.** Use POSIX tools
(`grep -E`, not `rg`); and when a gate is the thing standing between you and shipping a behaviour
change, check that it RAN, not merely that it did not complain.

### Measure the multiplier before optimising for it

`internal/multiplex/firmwaretests` blew its thirty-minute race-detector cap. Two fixes looked
obviously right and both were wrong, and measuring took less time than either.

**A shared acquisition** — twelve tests each spend up to 120M instructions watching the same
fixture arrive on the same day, so acquire once and restore. It saved 26 seconds, not the 80 that
was expected, because `runUntil` already early-exits the moment the block registers and never
spends its budget. It also broke a test: the snapshot restores the BOX, but the transmitter is a
plain Go object that stays fresh, so the two end up out of step and the box behaves differently.

**Sampling the framebuffer less often** in the press loop, on the theory that hashing 400KB every
65,536 instructions dominated. It made the test SLOWER — 507s to 593s — because a coarser sample
means a press takes longer to be seen as settled, so each one runs more guest instructions. The
cost was execution, not hashing.

**The number that mattered took one command:** the same test timed plain and under the detector,
60s against 507s, an 8.5x multiplier — against the "twelve" the Makefile had been guessing with.
That multiplied out to show the package was already over the cap BEFORE anything was added to it,
which changed the problem from "trim the new tests" to "this package should not be under the race
detector at all".

### Do not filter a command's stderr through a grep built for its stdout

Immediately after writing the lesson above, I ran the whole race suite in a background shell as
`./ctl.sh test 2>&1 | grep -E "^(FAIL|---|panic|# )"`. It came back with no output and an exit
status of 127. No output looked like "everything passed" — the grep matches only failures — and the
127 was the truth: `make` is not on `PATH` in a non-interactive background shell here, so
`ctl.sh test` died instantly and printed `make: command not found`, which the grep swallowed
because it does not start with FAIL.

**A filter chosen for the success path hides the failure path.** If the pattern cannot match
"command not found", a run that never started is indistinguishable from a run that passed. Either
widen the pattern to cover how the thing fails, or send the raw output to a file and filter what you
read rather than what you capture — and always read the exit status, which was sitting there saying
127 the whole time.

### A hash proves the screen changed, not which screen it is

Three instruments — one of them committed and carrying three sets of findings — measured the wrong
screen for weeks. The route is `box office` → LEFT to the TV GUIDE tab → select. One LEFT from box
office still draws the BOX OFFICE menu, so a check of "did the hash change" accepts it as the tab,
selects box office's first entry, and measures MOVIES BY START TIME. Behind that sits a second
trap: after select, a hash cannot tell the grid from the same menu with its highlight moved,
because both differ from the screen before.

Nothing caught it. Not a PC census, not a twelve-iteration analysis, not the notes — each was
internally consistent about the wrong screen. **What caught it was opening the PNG the instrument
had been writing all along.**

**Pin the screens you require by hash, and verify each pinned hash by eye once.** "Different from
where I was" is not an identification, and an instrument that cannot say which screen it is on
produces findings that are worse than no findings, because they read as measurements.

### A uniform answer across independent things is a bug in the instrument

Dumping what the box filters for, six armed channels each came back "NO RULES AT ALL — takes any
table on its PID". Six independent filters agreeing perfectly is not a finding, it is a tell. The
cause: there are 32 section channels and 16 match units, `Match` refuses any index from 16 up, and
I was passing channel numbers 19–24 into it. Every call returned "not set", which the instrument
rendered as "unfiltered".

Two habits come out of it. **When every member of a set answers identically, suspect the question
before believing the answer.** And **an accessor that returns `(value, ok)` is telling you
something with `ok`** — dropping it on the floor turns "I could not ask" into "the answer is no".

Also, from the same run: printing a `[]uint16` with `%v` gives decimal. The box "armed 82", which
is `0x52` — a PID this project already knows well — and it read for a moment as a new discovery.
Format identifiers the way the rest of the record writes them.

### The settle returns mid-paint frames — but two screens that photograph alike are not therefore one

`screenNow` + "four identical samples 65,536 instructions apart" calls a screen done after about a
quarter of a million instructions of stillness. A menu painting under a busy carousel holds a
HALF-DRAWN frame still for longer than that, so the detector can return a real framebuffer of a
screen that has not finished drawing. That part is measured and it is worth knowing.

**What was then inferred from it was wrong, and the inference is the lesson.** `0xDDBC18E9` and
`0x43779DC8` both photograph as the ten-entry TV GUIDE menu with ALL CHANNELS highlighted, so
`route_test.go` was changed to accept either, on the reading that one was mid-paint and the other
finished. Then the suite said otherwise: **SELECT from `0xDDBC18E9` opens the grid, and SELECT from
`0x43779DC8` moves to the next TAB.** Two screens that behave differently under the same key are not
the same state, whatever they look like. The change was reverted and the claim withdrawn; what
actually distinguishes them is unmeasured, and "focus on the tab row versus focus in the menu" is
another guess, which is exactly what produced the wrong one.

So: **the picture is necessary and it is not sufficient.** This project's rule has always been that
only the artefact proves which screen you measured — the corollary it did not say out loud is that
two identical artefacts do not prove you are in the same STATE. Where a hash is load-bearing, prove
the equivalence by BEHAVIOUR (press the key and see where it goes), not by eye.

`pressAndLetItFinish` (`rununtil_test.go`) — settle, then run a ten-million-instruction tail and
re-read — is still the right tool for a route of your own, and the A-Z walk runs with no swallowed
press using it. Three other things wear the same hat and are real:

- **Do not wait for transport state ≥ 6 before pressing keys.** It leaves the box mid-animation, so
  nothing settles and every press reports `00000000` — which reads exactly like a box that has
  stopped taking input, and was chased as one.
- **Do not crop the tab strip out of the screen hash** to dodge the animation. The strip is the only
  thing that distinguishes one tab from another, so a body-only hash makes navigation *worse*.
- **Pin the screen you select FROM, not the one you land on**, whenever the destination is the thing
  under test. ALL PROGRAMMES A-Z opens empty with no index and already filled with one, so pinning
  it cost a run; the category menu before it is stable.
