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
