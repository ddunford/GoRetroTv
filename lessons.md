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

### The settle detector returns mid-paint frames, and everything downstream of it was wrong

`screenNow` + "four identical samples 65,536 instructions apart" calls a screen done after about a
quarter of a million instructions of stillness. A menu painting under a busy carousel holds a
HALF-DRAWN frame still for longer than that, so the detector returns a real framebuffer of a screen
that has not finished drawing — and the next press lands in a painting menu, which is exactly when
a press is swallowed.

**Every screen pin in the firmware suite was therefore a photograph of a menu mid-draw.** Re-taken
on 2026-09-23 with a press that runs a ten-million-instruction tail after the settle, and verified
by eye against `.artifacts/route-*.png`:

    box office        0x1CBD8D51 -> 0xFE8D1CCC   six entries, MOVIES BY START TIME highlighted
    the TV GUIDE menu 0xDDBC18E9 -> 0x43779DC8   ten entries, ALL CHANNELS highlighted
    ALL CHANNELS                  0x144CF59D     "Searching for listings", filling to 0x71A6DFE8

and the route is **ONE LEFT from box office, not two** — the second LEFT was compensating for the
first press being read mid-paint, and with the tail it walks past the menu to SERVICES.

**`0xDDBC18E9` and `0x43779DC8` are one menu at two moments of its paint**, and how that was
established is the part worth keeping. Both photograph as the ten-entry TV GUIDE menu, so the route
was changed to accept either — and the suite appeared to refute it: *SELECT from `0xDDBC18E9` opens
the grid, SELECT from `0x43779DC8` moves to the next TAB*. The change was reverted and the claim
withdrawn on the strength of that. It was the right call on the evidence and the evidence was bad:
both of those presses were themselves reading mid-paint, so what differed was WHEN in the paint the
key landed, not WHICH screen it landed on. With the tail, SELECT from `0x43779DC8` opens ALL
CHANNELS, photographed.

So the rule survives its own example, which is the useful shape of it:

- **The picture is necessary and not sufficient.** Two identical artefacts do not prove two
  identical states; where a hash is load-bearing, prove the equivalence by BEHAVIOUR.
- **And a behavioural difference is only as good as the instrument that pressed the key.** A
  difference measured through a broken press is a fact about the press. Before concluding that two
  states differ, check that the thing distinguishing them is not the harness.

**It is now a conformance rule rather than a paragraph.** `ARCH-PRESS-1` holds that a screen read
with a counter incremented and compared against the stability threshold in the same window may
exist only in `rununtil_test.go`. It exists because the rule had been written in prose twice and
was broken in forty-eight places anyway: broadcasting the `0xB2` guide-row descriptor made every
menu take longer to paint, and thirty-one probes stopped reaching the TV GUIDE tab in a single
suite run, all reporting `drew 00000000` for screens that were drawing perfectly well.

Three other things wear the same hat and are real:

- **Do not wait for transport state ≥ 6 before pressing keys.** It leaves the box mid-animation, so
  nothing settles and every press reports `00000000` — which reads exactly like a box that has
  stopped taking input, and was chased as one.
- **Do not crop the tab strip out of the screen hash** to dodge the animation. Measured: below the
  strip the picture churns as hard as the whole frame, because what keeps changing is the menu's
  own content — and the strip is the only thing that distinguishes one tab from another, so a
  body-only hash makes navigation *worse* while fixing nothing.
- **Pin the screen you select FROM, not the one you land on**, whenever the destination is the thing
  under test. ALL PROGRAMMES A-Z opens empty with no index and already filled with one, so pinning
  it cost a run; the category menu before it is stable.
### Continue means pursue the visible outcome

When the owner says to keep going toward test video and audio on a channel, an investigation boundary is not a handoff boundary. Keep the bead active and continue through the measured implementation and browser proof; do not offer to stop merely because the next missing predecessor has been identified.

### Bars behind the OSD do not prove satellite playback

A browser-generated test card and tone triggered at the MPEG selection callback are not a working channel while the guest still renders *No satellite signal is being received* and the TV Guide search overlay. Acceptance for media must prove the firmware enters its real playback state and dismisses those overlays; a host presentation layered behind them is only an instrument, never the product outcome.
### A test source is programme configuration, not a channel identity

The presentation substitute must never become a global “Test channel” selected by any decoder PID request. Resolve the firmware-programmed EIT service id through the current guide programme, and activate only that programme's override or its service's explicit default source; otherwise channel changes inherit plausible but false video.

### “Playing” is not acceptable when tuning still feels like acquisition

Channel playout acceptance must measure the user-visible latency from SELECT to both the guide overlay clearing and the first decoded frame. A screenshot eventually containing video does not prove a broadcast-like channel change; the public box must cut promptly to the already-running programme and must not spend tens of seconds draining transport or waiting for an over-conservative UI settle.

### A lossy browser queue must be non-blocking across the whole replacement sequence

Checking that a one-slot channel is full and then receiving its old value is racy because the WebSocket writer can drain it between those operations, leaving the emulator blocked on the receive. Latest-frame, audio, media-state, and machine-state publication must use non-blocking receive and send steps, with a concurrent publisher/consumer regression; a single-threaded “slow client” test cannot expose this stall.
