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
