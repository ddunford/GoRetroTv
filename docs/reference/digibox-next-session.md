# Digibox emulator — continuation prompt

*Current as of 2026-09-16. Read this first, then `docs/reference/digibox-emulation.md`, where every
claim here has its evidence.*

**Continuation prompt, short by design.** Read this file, then the emulation doc's last six
sections. Load the `digibox-emulator` skill before touching `frontend/public/digibox-boot.html`.
Open work is `bd list --parent=sky-02me`. Nothing is blocked on the user. `./ctl.sh digibox` is
green and the demo host draws the Sky interface on every load.

**A SIDE ERRAND IS PARKED AND IS NOT PART OF THIS WORK — `sky-02me.19`.** A standalone showcase of
the Sky interface (`docs/sky-recreation/`, two files, no network requests at all) was built for
Makeryard and never went live because that platform's build queue stopped picking up snapshots. It
is finished and verified; do not spend time on it unless asked. One thing it left behind on the demo
host IS live and is worth knowing: `frontend/docker/nginx.conf` now gives
`Access-Control-Allow-Origin` to `FLASH_U20[23].bin` **and to nothing else**, so those two paths are
cross-origin readable. That is noted on `sky-02me.15`, which is still the owner's call.

**THE BLOCKER IS GONE, AND IT WAS NEVER WHAT IT LOOKED LIKE.** `sky-02me.18` and `sky-02me.17`
are both answered (15 Sep 2026); the evidence is in `digibox-emulation.md` under their headings.
What follows replaces the whole "a line-up and a working interface cannot coexist" framing, which
was wrong.

## `sky-02me.18` ANSWERED: the box was busy, not refusing, and it recovers by itself

**Demonstrated end to end in a browser:** feed a NIT, wait, feed a BAT, watch the box say
*"Rebuilding the channel list"* for 74-140 s, watch it return to *Ready* on its own, then press
**tv guide** and get **256 widgets and the guide on screen** with the BAT still parsed
(`0x42/ext=0x0 0x4a/ext=0x1000` in the match table). **"Has a line-up" and "can draw" are not
mutually exclusive.** The five-line chain this file used to open with does not close.

**What is actually happening.** A parsed BAT sends the EPG's o-code into a COUNTED loop at
`0x9FC85F41`-`0x9FC85F93` -- decoded, `--check` validated, back edge `41 ad` provably targeting the
loop head, counter at `fp-248` tested against `DS[0x00019A20]`, body one `scall (2,0x28)` and one
`scall (2,0x2D)` per service. While it runs the interpreter **never returns to its event loop at
`0x9FC4A538`**, so a key press is not refused -- it is never LOOKED at. 258 bytecode instructions
per press against a healthy 41,895.

**And the 140 seconds is OUR CLOCK.** During the whole of it the hottest PC is `0x800D35DC`, the
same seven-instruction idle spin that dominates an idle box, at the same rate -- ~85% of everything
executed. The box is waiting, not working. This emulator runs ~3.1M instructions/s against a real
VR4111's ~81 MHz, so the same ~345M instructions is **four to five seconds on hardware**. A real
Digibox rebuilds its service list in a few seconds and shows a message while it does; the page now
does the same.

**Two suspects were measured and refused**, and both refusals are worth more than a guess: the
NVRAM mirror (whole-device `localStorage` saves cost **289 ms out of 128 s**, 0.2% -- batching was
implemented, measured and reverted) and every one of the eight earlier native-level hypotheses.

## `sky-02me.17` ANSWERED: the `0xA1` loop consumes exactly one tag, `0xB5`

All 256 tags swept with every pass reaching its last descriptor and both controls silent. The
consumer at `0x800C6B42` compares against `0xB5` literally and extracts two big-endian u16 from
`desc[2..3]` and `desc[4..5]`, then returns 1 -- which **ends the walk**, which is why the pass
carrying it covered only 53 of 64 and why that coverage failure was the finding rather than a gap.

**But `0xB5` is probably not the programmes, and that is the live thread.** Two u16s and stop is a
reference or a window. In the same neighbourhood: a descriptor DISPATCHER at `0x800C6930` switching
on `tag - 0xB3` (arms for `0xB3`..`0xB8` and beyond), and `0x800C6C66` handling tag **`0xBC`** by
computing `desc[1] / 9` -- **nine-byte entries**, the shape the BAT's `0xB1` uses for its
per-service records. That is what a programme list looks like. **`sky-02me.20`.**

## `sky-02me.21` IS DONE — the guide draws listings

*16 Sep 2026. Evidence: `digibox-emulation.md`, last three sections.*

Load the page, let the carousel run, press **sky** then **tv guide**, wait a few seconds, press it
again:

    0 Sky One                          12.10pm Thu 1
    NOW      AfternoonFilm
    2:00pm   ChildrensHour
      Search Time · Search Channel · Search Favourite

No probe and no pokes. Four things had to be true at once and three had been hardcoded — the record
length (counts the bytes AFTER the header; `openTVtoXML` is wrong for this box and tvheadend is
right), the clock going FIRST (the whole request is day-addressed and the box programs it once), a
`linkage_descriptor` of type `0x91` in BOTH of the BAT's descriptor loops, and — the one that took
longest — **the day and `tableId & 3` are the GUIDE'S to choose, not the match unit's**. The
acquisition subsystem asks for tomorrow on the unit's table; the guide subscribes for its own day
with its own low bits, and those bits are not even constant between boxes. `__siGuideSlot()` reads
the slot at `0x80165048` and `__siTitles()` defaults to it.

**START HERE INSTEAD: `sky-02me.22`** — `FOR YOUR INFORMATION / No satellite signal is being
received` sits above the rows. Drawn with **zero demodulator traffic**, appears only once the
linkage is present and before any listings, so it is the service-SELECTION half of the linkage
answer rather than the schedule half. `0x800A4040` walks `0x18`-byte service records for the
linkage's (service_id, tsid, onid); the records the BAT's `0xB1` descriptor builds are 18 bytes, so
that may be a different table entirely. `scripts/digibox-probes/where-the-linkage-takes-it.js`
already isolates the ~15k o-code instructions the linkage adds and lists the new code runs.

### Harness rules this task paid for

- **A LADDER ADDRESS OUT OF A POOL WORD IS ODD, AND `__pcHits` NORMALISES UPWARD ONLY.** It counts
  `a` and `a|1`, so `0x800C64CD` never looks at `0x800C64CC` where the hits are. Eleven rungs read
  zero while two hand-typed EVEN addresses counted perfectly. Refuse an odd address by name.
- **`& ~1` IN JAVASCRIPT YIELDS A SIGNED INT**, so a masked address compares unequal to its literal.
  `(x & ~1) >>> 0`.
- **A GENERATED CENSUS ENTRY MUST BE CROSS-CHECKED AGAINST A HAND-WRITTEN ONE.** The module-12 walk
  dropped a dereference — the array holds POINTERS TO the `{impl, descriptor}` records — and
  reported zero for all 51 entries, which reads as "the guide asks its database nothing".
- **THE O-CODE EVENT COUNT BETWEEN TWO WINDOWS MEANS NOTHING WITHOUT A SAME-LENGTH CONTROL.** A
  periodic timer landing in one eleven-second window and not the other looks exactly like causation.
  Compare by CONTENT.
- **OPEN THE SCREENSHOT.** Twice this task a hash change was the clock — `7.00pm Thu 1` → `Fri 2` —
  and once it was the known stripe artefact, not rows.
- **A RUNNING CAROUSEL HAS NO `running` FIELD.** `__siCarousel()` returns its state object when on
  and `{running:false}` only when off, so `.running === true` never holds and refuses a good box.

### Settled, do not re-derive

The record format and `scripts/skyepg/` (falsifiable self-test) · `9E 8B` is the MJD, and the table
id, the PID and the day bytes are ALL day addressing that moves with the clock · `FUN_800c95d0` and
its pool-word call chain · the data segment (`"RSRC"` above the app image at `0x800FC66C`; do NOT
gate on `DS[0x1ACB0]`) · the guide's linkage question and both arms · the notification model and the
slot layout at `0x80165048`. `sky-02me.17`, `.18`, `.20` and `.21` are closed.

## Instrument changes made this session

- **`__readWatch(lo, hi, {max})`** -- the cap is now the caller's to raise. **Default unchanged at
  4,000**, so no earlier probe's baseline moved. A `sky` press is 34,067 bytecode instructions and a
  `tv guide` press 40-50k, so the default keeps a tenth of one and a diff of two truncated openings
  reports "identical" for streams that part later.
- **`__i2cState().eeprom.saveMs`** -- time spent inside the whole-device save, so the refuted NVRAM
  hypothesis stays refuted without being re-derived.
- **`assertRedrawable()` in the o-code probes** -- throws on the same key pressed twice running. A
  box already on a screen rebuilds nothing, so its zero widgets read exactly like the fault; this
  caught two probes in one session, both of which had the screen they pressed FROM in their own
  output. The guard was run against a repeat to confirm it fires.
- **The box status line reports the rebuild** (`Rebuilding the channel list`), driven by observed
  EEPROM traffic rather than by guessing at the o-code. Without it a visitor who feeds a line-up
  watches an apparently dead box for two minutes.

## Where it stands in one paragraph

**The box shows the Sky interface.** Load `https://retrotv.demosrv.uk/digibox-boot.html`, let it
boot, press the handset button marked **sky** (raw `0x7D`), and the real Box Office menu appears --
the TV GUIDE / BOX OFFICE / SERVICES / INTERACTIVE tab bar with icons, and six menu rows with
MOVIES BY START TIME highlighted. No probe and no pokes;
`scripts/digibox-probes/plain-key-press.js` is the reproduction that does nothing but boot, press
and look. Getting there took finding two application gates that made the box clear its own screen
(`sky-eluc.28`..`.32`), one misread opcode bit in our blitter (`sky-eluc.35`), and a declared
fiction that answers the gates after the RTOS is up (`sky-eluc.37`).

## The page is a Digibox now (`sky-02me.14`, done)

The framebuffer is the page: centred, at the raster's own aspect, with a Sky-shaped handset beside
it and **every** instrument moved — not deleted — into an `Instruments` modal that opens in one
click. A status line under the screen says what the box is doing in words (booting / checking its
own flash / ready / N keys sent). Load it, wait for the light to go green, click **sky**, and
navigate with the pad.

**What the probes depend on is unchanged and is now asserted rather than hoped for.** `[data-raw]`
is still on all 33 handset buttons and on nothing else, `#sp-max`, `#icount` and `#fb-base` are
still there, and a closed `<dialog>` is `display:none` — which every panel survives, because they
all write text and draw canvases rather than measure layout, and `.click()` fires on a hidden
control.

**It found a live defect in the gate, and that is the part to carry.** *"BGLOAD stopped being
scheduled"* was satisfied by BGLOAD **not having started**: `TASK20.runs` reads 1 for the first
two seconds after the 42nd task, then climbs to 2589 over eighteen more. At one-second sampling
the first plateau fires the test, so the page said *Ready* twenty seconds early and a click on
`sky` got no menu. Both the gate and the page now track the PEAK and only count stillness once it
has risen. `./ctl.sh digibox` prints the run count with the time for exactly that reason —
*"BGLOAD ran 2588 times and then stopped, after 22s"*, where it used to say *"after 2s"*.
Evidence and the sampled curve: `docs/reference/digibox-emulation.md`.

Then: `bd list --parent=sky-02me`. Ready now are screenshot-every-screen, the telephone-line mock,
the Interactive/Box Office cost question, the menu music (first establish whether the firmware drives
audio at all), a visual regression harness, and two defects this task filed rather than fixed —
**`.15`** (the demo host serves `FLASH_U202.bin` publicly while the page's own comment says it never
does; which way to resolve it is the owner's call) and **`.16`** (before the application programmes
the OSD the panel reads video RAM at a guessed bit depth and paints stripes).

## The acquisition ladder — table `0xA1` on PID `0x33`, and what the two header bytes are not

Feeding a real line-up makes the box ask for its own listings, and the chain is measured end to end
(`docs/reference/digibox-emulation.md`; probes in `scripts/digibox-probes/`):

1. **One NIT** widens the match table to `0x4A/ext=0x1000`.
2. **A BAT** carrying `[0x5F, 4, specifier 2]` then a `0xB1` whose gate halfword is `0xFFFF` and
   whose entries are nine bytes builds **4 of 4 service records**, 18 bytes apart, with the four
   flag bits reading back exactly as sent.
3. The box then **subscribes to `0xA1/ext=0xBBB` by itself** — and `0xBBB` is the value fed in
   entry field `+3..4`, so **that field is the per-service listings reference**. `0x42` (SDT)
   appears at the same moment, having never appeared before.
4. Listings arrive on **PID `0x33`** (not `0x52`). The match unit requires
   **`payload[0]=0x9E`, `payload[1]=0x8B`** and `(payload[3] & 0x70)==0`; `tableIdMask 0xFE` accepts
   `0xA0` too — measured as the SAME table, so the mask is width, not a second table — and
   `extensionMask 0xFFFC` covers all four services in one unit.
   **THOSE TWO BYTES ARE THE MJD, and this file spent weeks calling them a signature and then twice
   calling the date reading refuted.** Both refutations moved the clock AFTER the match unit was
   programmed and observed that the bytes did not follow — true, and a fact about the box not
   re-subscribing rather than about meaning. Set the clock FIRST and they move to the day's MJD, the
   table id moves `0xA1`→`0xA0` and the PID `0x33`→`0x37`+`0x36`. `0x9E8B` is MJD 40587, 1 Jan 1970:
   the date a box with no clock thinks it is. **Read all three off `__siMatches()` and stamp them.**
5. `FUN_800c64cc` then walks the payload from offset 6. It is NOT a generic descriptor loop — it is
   the Sky title record layout: `+0..1` event id, `+2..3` packet_length, `+4` tag `0xB5`, `+5`
   descriptor length, `+6..7` start, `+8..9` duration, `+13..` Huffman title. The earlier reading
   (tag `0x46` len 71 → tag `0x8F` len 144) was the walker imposing a tag/length shape on filler
   bytes, which is what a generic iterator does to anything.

**That tag is `0xB5`, and it is answered** (`sky-02me.17`): all 256 swept, exactly one consumed,
and the caller asks for it by name (`li a2,0xb5`). See the START HERE section — the box parses our
records and the open question is no longer the format but where the guide's rows come from.

**Not yet on screen**, and now for a understood reason rather than an open one: the `0xA1` path
writes into a stack local that feeds a comparison, so it is a time LOOKUP consulted by the guide
rather than the store the guide draws from.

**A RUNNING BROADCAST DOES NOT STOP THE BOX DRAWING — that reading is WITHDRAWN.**
Thirty seconds was not long enough to watch. See the `sky-02me.18` section at the top of
this file: the box is rebuilding its channel list and finishes on its own. The carousel
stays opt-in (`?si=1`) because two minutes of an apparently dead box is a poor first
impression, not because a broadcast breaks anything.

## The older framing: what is missing is DATA, not UI

Every screen reached says *"Searching for listings"* or *"Further schedule information is not
available"*. With the carousel running the box acquires cleanly — 79 sections, **0 refused**, NIT,
SDT and BAT all accepted — and the guide **concludes** its search rather than waiting for ever.

**Feeding PID `0x52` delivers and cannot fill the guide, and the reason is measured.** 16 match units
against 32 PID channels means channel 21 cannot have one, so nothing filters that PID and every
section reaches the firmware. The consumer — found by read-watching the ring and letting the firmware
name it, not by guessing — is `0x800A857A`, which tests **`0x40` NIT, `0x42` SDT, `0x70` TDT and
nothing else**. The OpenTV carousel parser addresses in this project's own notes are **cold** (0 hits,
neighbourhoods too, inside a region taking 42.8M), so that note does not describe the live path. The
open question is *which code consumes schedule events*; the method that worked transfers.

## What must not be re-derived, and what must not be moved

- **`skyGatesTick()` is a DECLARED FICTION, not a fix**, answering the two screen gates once the task
  list reaches 42. `__skyGates(false)` disables it. The races behind it are real and open:
  `sky-eluc.29` (the AV flag is raised 592,817 instructions after the decision reads it) and
  `sky-eluc.32` (`obj+0x14` is written after the first key press has cleared the screen).
- **DO NOT MOVE THE GATE ANSWERS EARLIER.** Before the RTOS is up the boot stops at **28 tasks
  instead of 42** — tried twice, caught both times by `./ctl.sh digibox`, reverted both times.
- **Bit 24 is the blitter's fill bit, not bit 23.** Bit-24-clear commands are copies from a source
  packed at the blit width (stride 480 measured against 1,593 candidates).
- **The demux match table is 16 units for 32 PID channels**, so `__siMatches()` (match unit) and
  `__siFilters()`/`__dispState().pids` (PID channel) are different index spaces. **Never join them by
  index** — doing so produced a confident artefact.
- **A single-file bind mount pins an inode and `git checkout` is a rename.** `./ctl.sh restart:prod
  frontend` re-resolves it; the demo host is the **`retrotv-prod`** project, not the dev stack.
- **Settle before pressing a key, and press from a state you have checked.** Three wrong findings
  came from naming a screen nobody had verified. The flat ~80 s wait in `keymap.js` is generous
  and still correct; the measured figure is that BGLOAD finishes about **20–25 s** after the 42nd
  task on this host, and **the page's own status line is now a usable signal** — wait for it to
  say *Ready* rather than for a timer. Do not substitute a bare "TASK20 stopped moving" test for
  it: that fires before BGLOAD starts. See `sky-02me.14` in the emulation doc.

## Background: the ring, and what is supposed to enqueue

The window record is 100 bytes, array at `*0x80105E9C`, count at `*0x80106F24`, gate at
`*0x80106F20`:

    +0x00  kind (1 = live)      +0x40  bit depth (2, 4 or 8)
    +0x50  PRODUCE index        +0x54  CONSUME index
    +0x58  background set flag  +0x5C  background colour, replicated across the word

**Window 1 is the visible plane and reads `0xDCDCDCDC`** — exactly the palette index the four
720×144 fills wrote. Driving `(1,0xD2)` on it by hand with colour `0x2A` returns 1, sets `+0x5C` to
`0x2A2A2A2A` and advances **produce 1 → 2**; nothing consumes it until a drain runs. Attributed by
instruction count, the boot draw is unambiguous:

    196111684  (1,0xD2)(window=1, colour=0xDCDCDCDC)   the producer
    196316013  (1,0xE4)(window=1)                      the drain
    196328206  BLIT fill 8bpp 0x80584048 @0,0 720x144 value 0x000000DC   (and three more)

On a key press `(1,0xE4)` runs **twice** and `(1,0xD2)` does not run at all: the paint is called and
the queue it drains is empty. **That is now explained rather than open** — nothing enqueues because
`apply()` has no window to damage, because the tree is bound to no plane. `(1,0xCB)` ×26 and
`0x80082D50` were the two leads out of this section and both are retired; the answer was one level
up, at the binding.

`(1,0xE3)` appears in the periodic burst list above, and that observation was correct and was read
one way too weakly: it is not background noise, it is the binding call itself, running twice — once
to attach and once to detach.

**The plane family is derived soundly, not by proximity.** Every member validates its window id
against `*0x80105E9C`, so the test is which code actually RESOLVES to a literal pool word holding
that address — scan for MIPS16 `lw rx,off(pc)` whose computed target is the pool word, then take the
enclosing native. (The base is the instruction word-aligned, except in a jump delay slot where it is
the jump's address; the `0x39/0x3A/0x3B` shims only make sense under that rule.) An earlier version
of this list attributed pool words to the nearest preceding function within `0x400` and wrongly put
`(1,0x3A)` in the family — it is a list iterator taking a callback in `$a1`, nothing to do with
planes. The real membership:

`(1,0x0E) (1,0x13) (1,0x1F) (1,0x20) (1,0x27) (1,0x29) (1,0x2A) (1,0x9A) (1,0x9E) (1,0xC3)
(1,0xC4) (1,0xCB) (1,0xD0) (1,0xD1) (1,0xD2) (1,0xD3) (1,0xD4) (1,0xD7) (1,0xD8) (1,0xDA)
(1,0xDC) (1,0xDD) (1,0xDE) (1,0xDF) (1,0xE0) (1,0xE1) (1,0xE2) (1,0xE3) (1,0xE4) (1,0xE5)
(1,0xE6) (1,0xE7) (1,0xE9) (1,0xEA)`

`(1,0xE9)` and `(1,0xEA)` take five arguments each and are the shape of a blit.

**Window 0 is a trap.** The validator errors when the id is 0 while the gate reads 0, and the error
handler does not return — a hand-call burns its whole instruction budget and leaves the machine
wedged, which then reads as "the drain did nothing". Only drive window 1.

**`__call` cannot complete `(1,0xE4)`**: the function tail-jumps through a jump table, losing the
harness's sentinel return address (`PC left mapped memory at 0x00000000`). Forcing the drain needs a
different lever.

## Retired — do not rebuild anything on these

- **The two families are not disjoint**, though the older reading was close. `(1,0x2F) (1,0x33)
  (1,0x37) (1,0x52) (1,0x56) (1,0x57) (1,0x75) (1,0xBB) (1,0xBC) (1,0xD5)` appear in both windows.
  The *pre-blit-only* set is real and its load-bearing member is `(1,0xD2)`.
- **Much of the pre-blit burst is periodic background work** that runs on a box nobody has touched —
  `(1,0xC7)` ×100 and `(1,0x2D)` ×49 land in the first six seconds of a sixty-second timeline with
  no key pressed at all. `(1,0xC7)` is confirmed **not** key-driven.
- **`(1,0x2D)` is not a raster primitive.** It inserts a `{u16,int,int}` triple into a growable
  registry of 16-slot blocks, so it was the wrong one of the two leads to chase.
- **`0x9FC4D4F6` is TIMER REGISTRATION**, not a widget populator, and `DS[0x025310]` is a timer's
  `userData`, not a draw list. The "drawing gate" at `0x9FC4DB3F` is a timer-expiry check.
- **`DS[0x018868]` is the remote-live byte and is NOT the blocker.**
- **NVRAM does not hold the configuration**; a blank one still boots to `INST = 1`. The installed
  flag is a static initialiser in the flash DATA chunk and is not the installation switch.
- **Resources are never named by literal id**, so searching for a screen by its id finds nothing.
- **Post-boot pokes cannot test boot-time state.**

## Tools and levers that exist

- `scripts/opentv-natives.py` — resolve any module-1 native to its firmware function, argument
  count and the natives it shares an implementation with. `--shims` emits the address list a
  `__pcHits` census needs.
- `scripts/digibox-probes/module1-census.js` — a complete per-native census counted at the shims.
  Every thunk is at a distinct address, so the histogram is exact and needs no operand decoding.
- `scripts/digibox-probes/module1-timeline.js` — the same census in two-second buckets across a
  minute with no key, then a key. **Use this before believing any census difference**, because the
  box's module-1 activity is bursty and periodic.
- `scripts/digibox-probes/raster-willing.js` — the window record, and the producer driven by hand.
- `scripts/digibox-probes/who-drew-the-blue.js` — natives and blits interleaved by icount.
- `scripts/ocode-disasm.py` — built on OpenTV's own opcode table from their GNU SDK.
- `__readWatch(lo, hi[, {fromPc:[lo,hi]}])` — a PC filter; pointed at the interpreter's opcode
  fetch it is an o-code execution trace. Log caps at 4000 and says `capped`.
- `__flashPoke` on the DATA chunk — the only lever that changes state the application establishes
  at startup. The verified DATA chunk base is **`0x9FCA07AC`**.
- `__breakWhen(addr, val)` — stalls inside the store itself; more reliable than polling.
- `__writeWatch(lo, hi)` / `__writeWatchLog()` — DRAM writes with the PC of each. **Useless for an
  EEPROM offset**, which never touches DRAM.
- `__eeTx()` / `__eeTxClear()` — every EEPROM transaction with its address and bytes. **It caps at
  400 and the boot alone fills it**, so clear it before the window you care about or you will read
  the boot's transactions and think they are yours. Its `driverPc`/`driverRa`/`driverStack` are the
  DRIVER's, never the caller's — see the warning below.
- `__breakAt([pcs])` + `__regs()` — arm BOTH `addr` and `addr | 1`; MIPS16 PCs carry the ISA bit.
- **The histogram diff**, `scripts/digibox-probes/what-the-dead-press-does-instead.js` — snapshot
  `__rangeHits(lo, hi, 1000000)` before and after, subtract, and compare two runs. It found the whole
  dead-press call path without needing any prior idea what to look for, and it is the most generally
  useful instrument built this session.

## Instrument warnings earned the hard way

- **WHEN A VARIANT OF A WORKING PROBE GOES QUIET, RE-RUN THE ORIGINAL BEFORE ADDING HYPOTHESES.**
  A hand-rolled variant of the `0xA1` probe reported *"copied and NOT parsed"* for its own
  known-good control across FOUR runs. The cause was the harness: it never located the parse buffer
  at `0x802A76xx` and sat watching a first-stage copy at `0x801A85xx` that nothing re-reads. The
  original, re-run verbatim, parsed first time. Four runs measured my own harness rather than the
  box. Prefer a surgical copy of the working probe with ONE thing changed.
- **THERE ARE THREE COPIES AND ONLY ONE IS THE PARSE BUFFER.** `0x801A85xx` is a first-stage copy,
  `0x802A76xx` is where the walk happens, `0x807DCxxx`/`0x807DFxxx` is where refused sections land.
  Take the copy at or above `0x80200000`, and report the watch's TOTAL read count alongside the
  per-offset list — "the watch logged nothing" and "it logged plenty, none in the payload" are
  different answers and collapsing them wastes a run.
- **VERIFY A PATCH BY RE-READING THE WRITTEN FILE, NEVER BY ASSERTING YOUR INTENT.** A regex meant
  to install new records matched `// 12 records` while the file held a literal `%d` from an earlier
  formatting bug; the substitution silently failed, the previous payload ran, and the result was
  reported as refuting a hypothesis that had never been tested. The guard was `'RECORDS = [' in
  src`, true whether or not anything changed — a check that cannot fail. Compare the actual byte
  count in the file against the generator and exit non-zero on a mismatch.
- **OPEN THE SCREENSHOT.** A run reported *"THE GUIDE SCREEN CHANGED"* whose baseline presses had
  built ZERO widgets at 12 colours — the box was mid-NVRAM-rebuild, so the change was the rebuild
  ending rather than the data arriving. Baselines must wait the rebuild out and THROW if they did
  not draw.

- **RUN EMULATOR PROBES ONE AT A TIME, and this bites hardest when you are waiting.** Two
  concurrent probes starve each other and the loser returns **zeros that look like measurements** —
  here `*0x80105E9C` read as `0x00000000` and a window record came back all zero on a box whose
  plane is plainly blue. It happened because a second probe was launched to fill the wait while the
  first was still running.
- **A wait condition must key on something only the NEW state has.** `until grep -q '"tasks"'`
  matched the harness's own `BOOT {"tasks":42,…}` banner and returned instantly, reporting a probe
  finished that had not started its measurement.
- **Node's stdout to a file is block-buffered**, so a probe redirected to a file shows nothing until
  it exits. An empty output file is not evidence that it died.
- **`__pcHits` KEYS ITS RESULT WITH THE PAGE'S `hex32`, WHICH UPPERCASES.** A census that builds the
  lookup key with a plain `toString(16)` misses every address containing a hex letter and returns a
  perfectly plausible **zero** for it, while all-digit addresses count correctly. That is how
  `(1,0xE4)` and `(1,0xC7)` were reported as never called, out of a 236-entry census with specific,
  varied, reproducible counts that survived two runs. It was broken only by an **independent
  instrument disagreeing** — the icount trace put `(1,0xE4)` 204k instructions before the fills. The
  probes now build the key the page's way and **throw on a missing key**: a census that cannot find
  its own subject must fail as a harness error, never report a count.
- **Pick a control that actually executes.** A trace armed on `(1,0x26)` as a "must fire constantly"
  control logged nothing, which read as a broken instrument — that native simply does not run in an
  idle window. A control that is itself cold proves nothing either way.
- **DRAM is 32 MB and the framebuffer is at `0x80584048`.** A scan of `0x80000000`–`0x80400000`
  answers about a subset while looking like an answer about memory.
- `__pcHist` has **no clear function** — successive `__rangeHits` calls are cumulative and must be
  differenced.
- **AN ASYNCHRONOUS DRIVER HAS NOBODY'S STACK.** Capturing a call chain at an I2C transaction was
  tried twice — at the STOP (`pc=0x80006F30`, the completion interrupt) and at the arm-START
  (`pc=0x80006C7E`, inside the driver) — and both gave `ra=0` with an empty chain, because the task
  that asked for the transfer is blocked on a semaphore and its frame is on neither stack. Two
  attempts was the right number to stop at; the third would have been iterating a technique already
  shown not to apply. **Diff the PC histogram instead.**
- **READ THE NUMBERS, NOT THE PROBE'S OWN VERDICT.** Three times this session a convenient one-line
  summary was cruder than the table printed beside it, and three times the table was right: *"both
  devices went up"* when the EEPROM had gone from **exactly zero** to hundreds; *"a read-back that
  never matches"* from inspecting only the hottest address when the log showed a five-transaction
  cycle; and *"it draws on a warm boot with the line-up stored"* when the `tables` line in the same
  output said the line-up was gone. A verdict string is a convenience and gets quoted later; the
  table is the evidence.

## The channel line-up (`sky-eluc.12`): the descriptor is `0xB1`, behind specifier 2

Still not the blocker for the interface appearing — but the chain is now measured end to end, in
four probe stages each carrying its own control. Evidence and the full listings:
`docs/reference/digibox-emulation.md`; probes in `scripts/digibox-probes/`.

- **The ladder.** On a settled plain boot the match table is `0x40` (NIT, network `0x20`) and
  `0x73` (TOT) **only**, for 220M instructions. Push **one NIT** and `0x4A/ext=0x1000` appears and
  the bouquet id moves `0x0` → `0x1000`. Three runs. `0x42` (SDT) never appeared in any of them,
  which contradicts the older note inside `__siIds()`.
- **Delivery is by PID.** `siPush()` writes into the ring and sets the status bit; the hardware
  table-id match is not in that path. "No match unit for `0x4A`" is a *subscription* fact.
- **Sections are parsed on a COPY.** A ring watch reports every byte read by `0x800FA958` — the
  byte loop inside `memcpy`. It reported all 128 private tags as consumed; they were the copier.
- **The loop walker is generic:** `FUN_800ad590(loop, off, wantedTag, cb)` reads tag and length
  only, terminates on tag `0`, and the **callback** reads bodies.
- **Sweeping `0x01..0x7F` consumes exactly one tag: `0x5F`**, the DVB
  `private_data_specifier_descriptor` — its handler compares the 32-bit value against **2** and
  **5**. That explains the `0x80..0xFF` negative: a private descriptor means nothing until a
  namespace is declared.
- **Specifier 2 unlocks `0xB1`** (`0x800BF7DA`). Specifier 0 and 9 unlock nothing. The box walks
  the loop a **second** time once the namespace is declared — 261 descriptors against 131.
- **`0xB1` is `[tag][len][16-bit field, `0xFFFF` sentinel][N × 9-byte entries]`**, `N = (len−2)/9`,
  each entry becoming an 18-byte record seeded from the transport context.

- **The nine bytes are measured too** (`sky-eluc.38`, closed). Entry = `u16, u8, u16, u16, u16`,
  nine bytes, `N = (len−2)/9`, each becoming an 18-byte record:

      entry +0..1 -> rec[4..5]   +2 -> rec[12]   +3..4 -> rec[6..7]   +5..6 -> rec[8..9]
      entry +7..8 -> rec[10..11] = value >> 4, with bits 3,2,1,0 unpacked to rec[13..16]
      rec[0..1] and rec[2..3] come from the transport the descriptor arrived on

  So the last field is **packed: 12 bits of value plus four flag bits**.
- **The `u16` at body`[0..1]` is a GATE and must be `0xFFFF`.** With `0xFFFF` all 27 entry bytes
  are read; with `0x1234` only the length and those two bytes are, and **no entry is decoded** —
  a well-formed descriptor the box accepts and silently ignores.

**Open:** the field SEMANTICS. Widths and destinations are measured; "service id, type, channel
number" is not. Name them by feeding a real line-up with a distinguishable value per field and
reading which number appears where on screen — not from a public BAT table.

The regional keying still stands: `0x1001` England/Wales SD (the 1998 line-up), `0x1002` Scotland,
`0x1003` regional, `0x1004` Ireland; sub-bouquet 1–72, **1 = London**.

## Open issues

    sky-02me.21  P1   START HERE -- the box PARSES our title records; the guide will not draw
                      them. The 0xA1 path is a time LOOKUP writing to a stack local, so the rows
                      come from elsewhere. O-code trace a guide press; do not guess a fifth format
    sky-02me.20  --   ANSWERED: 0xBC walked-not-consumed, 0xA0 is the same table as 0xA1, and a
                      well-formed 0xB5 opens no new rung. Three negatives, each with controls
    sky-eluc.12  P1   feed a real Sky line-up -- everything it needs is measured, and it is NO
                      LONGER BLOCKED: .18 is answered, and a box with a parsed BAT draws the guide
    sky-02me.18  --   ANSWERED: the box was busy for 74-140s, not refusing, and recovers by itself
    sky-02me.17  --   ANSWERED: the 0xA1 loop consumes exactly one tag, 0xB5 -- but see .20
    sky-eluc.27  P1   what calls the drain
    sky-02me.15  P2   the demo host serves FLASH_U202.bin publicly while the page says it does not
    sky-02me.16  P2   before the OSD is programmed the panel paints video RAM as stripes
    sky-02me.11  P2   does --carousel need --ack-all to boot? (partly answered: on a WARM box,
                      --carousel alone does not boot -- 22 tasks)
    sky-eluc.2   P2   what sends #CONTROL error 0x19 between the IEPG search and the app start

`sky-eluc.10`, `.14`, `.25`, `.26` and now `sky-02me.17` are closed, each with its answer in the
close reason. `sky-02me.18` is answered in `digibox-emulation.md` and retitled.

## House rules for this file's subject

- **EVERY FINDING LANDS IN THE DEMO PAGE.** From the owner, 15 Sep 2026: *"We should ALWAYS be
  updating the demo page!"* A finding is not finished when a probe proves it and this file records
  it — if it changes what the box can DO, it goes into `digibox-boot.html` in the same piece of
  work. Add it behind an option on the shared builder and switch it on where the demo is assembled,
  so no existing probe's baseline moves, and keep the old state reachable from the URL (`?si=0`,
  `?u203=0`).
- **Run `./ctl.sh digibox` after any change to `frontend/public/digibox-boot.html`.** A hardware
  model does not fail with a stack trace; it fails by running for ever doing something plausible.
- **Edit spike pages in place.** A write-then-rename strands the demo host's file bind mount.
- Verify against `https://retrotv.demosrv.uk/digibox-boot.html`.
- There is **no MPEG-2 video decoder** and none is planned. The target is the menus and the guide.

## Useful external references

- Sky bouquet/region ids: <https://github.com/iptv-org/epg/issues/1133>
- BAT structure worked example: <https://dvbsnoop.sourceforge.net/examples/example-bat.html>
- A recreation of the classic Sky EPG, useful as a visual reference if we ever build the UI
  ourselves rather than waiting for the 1998 one: <https://olddigibox.github.io/sky-web-epg/>
- NEC VR4111 User's Manual (not vendored, 2.9 MB):
  <http://bitsavers.trailing-edge.com/components/nec/mips/Vr4111-um_199804.pdf>

---

# PROJECT MOVED — this file is the predecessor's handoff

*16 September 2026.* The emulator is now its own project in Go, at
`github.com/ddunford/GoRetroTv` (`/opt/workspaces/development/goretrotv.demosrv.uk`). The browser
emulator this file describes is kept as `reference/digibox-boot.html` and is now the port's
**correctness oracle**, not a dead end.

**Everything above remains true and is still the specification** for the firmware's behaviour. The
phase plan references it rather than re-deriving it. What changed is only where the code lives and
what language it is in.

The predecessor, whole — 610 commits, the FastAPI backend, the listings pipeline, ErsatzTV playout,
464 tests and the conformance harness — is at
`/opt/workspaces/development/archive/skytv.demosrv.uk-2026-09-16`.
