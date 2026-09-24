---
name: digibox-emulator
description: Working on the Pace 2500N Digibox emulator in frontend/public/digibox-boot.html — the NEC VR4111 CPU and MIPS16 reference, the firmware's device conventions, the page's debug API, and the measurement discipline a hardware model needs. Use when changing the emulator page or mips16.js, reading the firmware's disassembly, modelling a new peripheral, feeding it DVB sections, or diagnosing a boot that runs for ever doing something plausible.
---

# The Digibox emulator

<!-- anchor: none - title section; each part below declares its own subject -->

`frontend/public/digibox-boot.html` runs a Pace 2500N's real flash image in the browser.
**The established facts live in `docs/reference/digibox-emulation.md`** and that file is the
record; this is the working knowledge that keeps getting re-derived.

## The standing instruction: EVERY FINDING LANDS IN THE DEMO PAGE

<!-- anchor: none - working practice for this domain, not a claim about code -->

**From the owner, 15 Sep 2026: "We should ALWAYS be updating the demo page!"** -- and it was said
after the probes had spent a day learning how to give this box a channel line-up while the page a
visitor loads still broadcast nothing at all. That is the failure mode to design against: the
probes accumulate capability, the docs accumulate evidence, and the thing with a URL does not move.

So a finding is not finished when a probe proves it and a doc records it. If it changes what the
box can DO, it belongs in `frontend/public/digibox-boot.html` in the same piece of work.

**Two things make that safe rather than reckless, and both are load-bearing:**

- **Never change what an existing debug API emits by DEFAULT.** Every probe written before today
  measured some particular state, and "on a plain boot the match table is `0x40` and `0x73` only"
  is a statement about SILENCE. Add the new capability behind an option (`__siBAT(…, {lineup})`)
  and switch it on where the DEMO is assembled, not in the shared builder.
- **Keep the old state reachable from the URL.** `?si=0` silences the broadcast and `?u203=0`
  unmaps the second flash chip, for the same reason: "is this the broadcast's doing?" has to stay
  askable without editing the page. The harnesses pass `?si=0`, so their baselines never moved.

And then `./ctl.sh digibox`, and then LOOK at it.

## The first rule, because it is the whole genre

<!-- anchor: none - working practice for this domain, not a claim about code -->

**A hardware model does not fail with a stack trace. It fails by running for ever doing
something plausible.** A wrong register model is a boot that stops at 20 tasks; a wrong
constant is a link that silently runs at sixty baud; a stale read is a confident finding
about a machine that was not in the state you assumed.

So: **run `./ctl.sh digibox` after any change to the page.** It boots the machine in a real
browser and asserts the boot, a remote key and a DVB section delivery. `./ctl.sh
digibox:self-test` proves it can still go red. A cold boot takes ~96s, a warm one ~31s.

---

## Part 1 — The CPU

<!-- anchor: internal/cpu/core.go -->
<!-- anchor: internal/cpu/mips16.go -->
<!-- anchor: internal/cpu/mips32.go -->
<!-- fingerprint: sha256:98ab19062f94d7b916a96a1d193864720649c543abf56d6d91a26f844669d5c3 @ 2026-09-22 -->

**NEC VR4111, big-endian, mixed MIPS32 and MIPS16 reached through `JALX`.** Reference: NEC
VR4111 User's Manual, µPD30111, U13137EJ2V0UM00 2nd ed., April 1998, 775pp —
`http://bitsavers.trailing-edge.com/components/nec/mips/Vr4111-um_199804.pdf`. Not vendored
(it is NEC's document and 2.9 MB), so what cost time is written out here.

**The manual documents the CPU and the bus and nothing else we need.** Zero hits for CLUT,
palette, MPEG, demux, video encoder or framebuffer — those are a Pace/ST ASIC at `0xB0000000`
(physical `0x1000_0000`, the external system bus) that no public manual covers. Useful
chapters: 4 and 29 (MIPS16 encodings), 30 (COP0 hazards), 11 (BCU), 15 and 19 (interrupts,
GPIO).

### COP0 — three things are load-bearing and all three look optional

| # | Register | Why it matters |
|---|---|---|
| 8 | BadVAddr | |
| 9 | **Count** | Must advance. With a static Count no time passes: no tick, no preemption, delay loops spin for ever. |
| 11 | **Compare** | When Count == Compare a timer interrupt is requested, and **writing Compare clears the request**. |
| 12 | Status | IE bit 0, EXL 1, ERL 2, IM 15..8, BEV 22 |
| 13 | Cause | IP bits 15..8, timer is IP7 |
| 14 | EPC | |
| 16 | Config | |
| 30 | ErrorEPC | |

**COP0 is not optional.** Stubbing `mfc0`/`mtc0` as "reads 0, writes discarded" silently
destroys every context restore, because the scheduler puts a task's Status and EPC into COP0
and `ERET`s to it.

**`ERET` picks its return register from `Status.ERL`** and has **no delay slot**:

    if Status.ERL:  PC = ErrorEPC(30);  Status.ERL = 0
    else:           PC = EPC(14);       Status.EXL = 0

**Bit 0 of EPC carries the ISA mode** (MIPS16 vs MIPS32) and must be masked off the address.
Implementing only the EPC branch is correct until the first error exception.

**Interrupt delivery:** only when `Status.IE` is set, `EXL`/`ERL` clear, and the matching
`Status.IM` bit set. Then `Cause.ExcCode = 0`, set the IP bit, `EPC = PC | isa`, set EXL,
vector to `0x80000180` (or `0xBFC00380` when BEV is set). This boot runs with BEV clear.

**Easy to miss:** `MTHI`/`MTLO` (SPECIAL fn `0x11`/`0x13`). The reads are obvious; the writes
appear only in context restores, so dropping them corrupts HI/LO across task switches and
nowhere else. Also `lwl`/`lwr`/`swl`/`swr` — memset's unaligned path uses them, and without
them the boot aborts somewhere that looks nothing like memset.

**One deliberate inaccuracy in our model:** Count advances per instruction, not per clock.
Monotonic but wrongly scaled — enough for ordering, not for anything measuring real time.

### MIPS16 — the original ASE, not MIPS16e

`SAVE`/`RESTORE` appears nowhere and a VR4111 would not have it. MAME cannot decode MIPS16 at
all; QEMU implements MIPS16e but has no board. There is no prior art to borrow.

**AN INTERRUPT MUST NOT BE TAKEN IN A BRANCH DELAY SLOT.** The MIPS16 executor models delay
slots as hardware does: a jump arms `cpu.delayed` and advances PC *past* the jump, and the
next step runs the slot and only then applies the jump. Between those two steps the PC *is* a
delay slot, and taking an interrupt there records EPC as the SLOT's address. Real MIPS backs
EPC up to the branch and sets `Cause.BD`; we defer the interrupt by one instruction instead,
which is equivalent and needs no BD. **This cost a night**: EPC landed in the delay slot of a
`jr $a0`, ERET returned to an orphaned `addiu $sp,16` without the jump, `$sp` ended sixteen
bytes high, and a semaphore epilogue read `lw $a0,52($sp)` from the wrong word and ran the PC
to zero. The saved frame was never corrupted — the pointer into it was.

**Operand order in the shift instructions is the reverse of how it reads**, and getting it
backwards produces a plausible wrong answer rather than an error. With the project's
disassembler (`periph/m16.py`), `sllv $rx, $ry` means **`ry = ry << rx`** — the FIRST printed
operand is the shift amount. Confirmed twice in the demux LISR, where `li $v1,1; sllv $v0,$v1`
builds `1 << f` for a filter number in `$v0`.

**The T register is implicit.** `cmpi $rx, imm` sets T = (rx != imm); `bteqz` branches when
T == 0, `btnez` when T != 0. `move $t8, $rx` also sets T, so a bare `move $t8, $v0` followed
by `bteqz` is "branch if v0 == 0".

---

## Part 2 — This firmware's conventions

<!-- anchor: internal/bus/device.go -->
<!-- fingerprint: sha256:1f7dbb560757532aab32daed9468bd55c735b7bcffe239a08aed885b7d7966a0 @ 2026-09-22 -->

**Two programs.** The bootloader is `0x0-0x1FFFF` with its own Nucleus and ONE task; the
application is a separate image at flash `0x20000` with entry `0xBFC2048C`. `FETask`,
`SMHKTask`, `SMNTask`, `SMTTask`, `SCTask`, `EVTTask` are the APPLICATION's. Boot the
bootloader alone and exactly one `TASK` magic exists. Time has been lost diagnosing "why are
no tasks created" while running the wrong program.

**Nucleus PLUS task control block** (derived from a live box, checked field by field):

    +0x00 previous   +0x04 next   +0x0C id "TASK"  +0x10 name[8]
    +0x18 status  +0x19 delayed-suspend  +0x1A priority  +0x1B preemption
    +0x1C times scheduled  +0x20 time slice
    +0x24 stack start  +0x28 stack end  +0x2C stack pointer  +0x30 size  +0x34 minimum free

Status: **0 ready, 2 sleep, 3 mailbox, 4 queue, 5 pipe, 6 semaphore, 7 event.** `__tasks()`
walks the created list and names all of it.

**Reading a blocked task's stack for return addresses is the fastest diagnosis in this
project.** Scan from `+0x2C` to `+0x28` for odd words in `0x80000000-0x80120000` and you have
the call chain. It is how the key deadlock was found in one step after a day of theories.

**Its "queues" are semaphore + pipe pairs.** `0x8011B80C` is semaphore `EVQS0002` and
`0x8011B834` pipe `EVQP0002`; `0x8011B928`/`0x8011B950` are `EVQS0001`/`EVQP0001`. So a task
"blocked on a queue" is status 5, pipe.

---

## Part 3 — The page's debug API

<!-- anchor: internal/oracle/page.go -->
<!-- fingerprint: sha256:7dfbe35e54b09849da78f1229d42d18c8dbd756359be1d7fe786ec8d8e264063 @ 2026-09-22 -->

Always `__profile(true)` before anything that reads PC hits or breaks.

| Call | What it gives |
|---|---|
| `__tasks()` | the Nucleus task list — name, status, priority, runs, stack pointer |
| `__pcHits(a, …)` | execution counts at exact addresses — **keyed by `hex32`, which UPPERCASES; a lower-case lookup returns `undefined`, not a count** |
| `__rangeHits(lo, hi, n)` | total and hottest within a range |
| `__hotExcluding(lo, hi, n)` | hottest, minus a known idle loop |
| `__breakAt([pcs])` / `__regs()` / `__step(n)` / `__resume()` | live registers at an address |
| `__peek(a, n)` | memory. **An MMIO word is `__peek(a,1)[0]`, not four bytes** |
| `__find(str\|bytes, lo, hi, max)` | scan DRAM |
| `__key(raw, source)` | a real handset frame on the CSI link |
| `__ackSet([codes]\|"all")` | which peripheral commands the micro answers |
| `__cardRate(traffic, idle)` | link byte time, in instructions |
| `__pumpEvery(n)` | how often the device pumps run |
| `__dispState()` / `__dmxLog()` | demux status, enable, PID map, register writes |
| `__siPush(pid, bytes)` / `__siPushFilter(f, bytes)` / `__siTDT(…)` | feed DVB sections |
| `__csiRxLog()` / `__cardFrames()` / `__csiState()` | both halves of the peripheral wire |
| `__blitLog()` / `__dmaLog()` / `__vramState()` | the drawing path |
| `__demod()` / `__eeprom()` / `__i2cState()` | tuner and NVRAM |

---

## Part 3b — Reading the firmware: Ghidra

<!-- anchor: none - external tooling procedure -->

`./ctl.sh decompile 0x8005141C` gives the disassembly and the decompiled C of any address;
`./ctl.sh ghidra:import` rebuilds the project when it is missing or a new MIPS16 seed is added.
The language is **`MIPS:BE:32:16e`**, which is exactly this CPU, and the decompiler handles
MIPS16 — a page of it becomes a dozen lines of C.

**The project is two blocks, because the running program is not one file.** The application
executes from a RAM image the bootloader DECOMPRESSES to `0x800009F4`; its strings and the pool
words it loads with `lw $rx,n($pc)` live in the flash at `0x9FC00000`. Import one without the
other and half of every function is unreadable.

**ISA_MODE is SEEDED, never blanketed.** The image is mixed MIPS32 and MIPS16 reached through
`JALX`, so setting MIPS16 across it decodes every MIPS32 function into plausible nonsense.
`scripts/ghidra-import.sh` seeds entry points this project reached by TRACING the running
machine — each one known to be MIPS16 and known to be a function — and lets flow analysis
expand. Add a seed there when you prove a new one.

**IT READS; THE EMULATOR IS THE GROUND TRUTH, and that ordering is not deference.** Ghidra's
decode was checked instruction-for-instruction against a function the emulator executes before
any of it was trusted. But the static reading of this firmware has been **backwards twice** in
one session — a tag comparison whose branch skipped on MATCH (so the value it tests for means
"accept", not "redirect"), and a record id that is a halfword where the surrounding fields are
words. Both looked settled on the page and were only corrected by running them. So: read here
to find the shape fast, then prove the shape by driving it.

The project lives in gitignored `ghidra/` — **not** `.ghidra/`, because Ghidra refuses any path
component starting with a dot, which costs one confusing error before you notice.

## Part 4 — The measurement discipline

<!-- anchor: internal/platform/instrument/instrument.go -->
<!-- fingerprint: sha256:af00007f115e309f9e87b5febf843a476a726e1d7ea2894325e16fd6230cdc03 @ 2026-09-22 -->

Every one of these cost real time here. They are ordered by how often they recur.

**1. Read state only from a machine in the state you assume.** `__tasks().n === 42` is the
cheap check. `[0x80105C44]` reads `0xFFFFFFFF` before the application runs and `20` after —
read from a page reloaded for a screenshot, it reported a device-registration failure that
had never happened. This happened three times in one session, including once inside the gate
written to catch it.

**2. A LOOKUP MUST BUILD ITS KEY THE WAY THE PRODUCER DID, and a missing key must THROW.**
`__pcHits` returns an object keyed by `hex32(addr)`, which uppercases the hex digits. A census
that looks up `'0x'+addr.toString(16)` therefore misses **every address containing a hex letter**
and reports a perfectly plausible **zero** for it, while all-digit addresses count correctly. That
put `(1,0xE4)` (`0x80081C58`) and `(1,0xC7)` (`0x80081B40`) at "never called" across two runs of a
236-entry census with specific, varied, reproducible counts — and it was broken only because an
independent instrument disagreed, the icount trace placing `(1,0xE4)` 204k instructions before the
fills it had supposedly never caused. **A census that cannot find its own subject is a harness
failure, not a count of zero.**

**3. `__pcHits` accumulates ONLY while profiling is on.** A gap in profiling looks exactly
like a function that stopped being called, and produced a confident, wrong "SMTTask is stuck
in the dispatcher".

**4. Recording what the firmware WRITES is inert. Answering a read it used to get zero for is
not.** Making the demux register file read back its own writes — which sounds like an
improvement — stopped the RTOS starting at all. `+0x124` is the one to remember: the LISR
spins until its bit 14 reads CLEAR, so "reads back what was written" is an infinite loop,
while the old undefined-becomes-zero was accidentally correct. **Log writes freely; change
reads only with a reason and a boot afterwards.**

**5. Ask whether a constant counts CALLS or INSTRUCTIONS.** They are identical until
something changes how often the call happens. Batching the device pumps multiplied
`bootloaderHandoff`'s idle threshold by sixteen and the handoff never happened.

**6. Verify against a fixed point, not against a number you produced.** The MJD conversion
was briefly "wrong" because the expected value in the check was invented. 1995-10-10 is MJD
50000; anchor on that.

**7. A boot measurement is of a box that has booted before.** Cold 447M instructions / ~96s,
warm 140M / ~31s, because the NVRAM persists to localStorage. Say which you measured.

**8. When a fix changes nothing, the diagnosis is wrong.** Go and look again rather than
trying a bigger version of it.

**9. RUN EMULATOR PROBES ONE AT A TIME, and the temptation is strongest while waiting.** Two
concurrent probes starve each other and the loser returns **zeros that look like measurements** —
here `*0x80105E9C` read back as `0x00000000` and a window record came out entirely zero on a box
whose plane was plainly blue. It happened by launching a second probe to fill the wait for the
first. Two more traps in the same waiting: a wait condition must key on something only the NEW
state has (`until grep -q '"tasks"'` matched the harness's own `BOOT {"tasks":42,…}` banner and
returned instantly), and node's stdout to a file is block-buffered, so an empty output file is not
evidence that a probe died.

**10. One command per question.** `ls a; ls b | wc -l` printing four lines has been read as one
answer more than once.

---

## Part 5 — Feeding it a stream

<!-- anchor: internal/broadcast/sections.go -->
<!-- anchor: internal/dvb/crc.go -->
<!-- fingerprint: sha256:3ceb9c592320a4e8695fdb5b667ff0e76afe4667e9ccf487601358dd42017d7d @ 2026-09-24 -->

During initial SI acquisition the box asks for exactly three PIDs and says so: `__dispState().pids`
gives **filter 22 → PID 0x0014 (TDT, the clock), 23 → 0x0011 (SDT, the line-up), 24 → 0x0010
(NIT)**, with the enable at `+0xD8` reading `0xFFC00000` — filters 22..31, matching the ten
that carry contexts in the filter records. **The channel index IS the filter index.** Later guest
states also arm viewing EIT and OpenTV carousel PIDs; measure the state whose behavior you are
explaining rather than treating this acquisition snapshot as a permanent census.

Delivery, read off the firmware rather than the DVB spec:

- Section RAM is **thirty-two 12 KB rings**, filter f at `0xA07A0000 + f*0x3000`. Records at
  `0x80142D34 + f*20` = `{start, end, current, last-read, context}`.
- `+0xD8` is the **enable** (write-one-to-set, one bit per filter); `+0xB8` is the **status**
  (write-zero-to-clear, which is what the LISR's complement write does). They were modelled
  the other way round for months.
- The LISR writes `0x4000|(f<<2)` to `+0x124`, spins for bit 14 to clear, reads a 21-bit byte
  offset from `+0x128`.
- The task advances by `section_length + 4` where DVB's total is `+ 3`, so **the hardware
  appends a byte after each section**. Write one.

A PID and a filter index cannot share an argument — PID `0x0014` is 20, a real filter.

Next after TDT: SDT `0x42` and NIT `0x40` need a correct MPEG CRC-32 (poly `0x04C11DB7`, init
all ones, no final inversion); `withCrc()` on the page does it. Then the OpenTV carousel
(`0xA0`-`0xB1`, parsers `0x800C95D0`/`0x800C9CA0`, Huffman `0x800BECF0`).

**There is no MPEG-2 audio/video decoder here yet.** The current browser colour bars and tone are
a declared presentation substitute triggered by the guest's own selected-service MPEG callback;
they are not decoded broadcast media. Component signalling is now real: a successful front-end
tune makes the guest open PAT, CAT and then the PMT PID announced by the PAT; a PMT carrying MPEG-2
video and MPEG audio executes the firmware's component rebuild and reaches its audio stop decision.
The PMT channel is PID-only—none of the sixteen match units describes table `0x02`—so do not route
it through an unrelated unit merely because byte-nine bits overlap its filter number. Phase 8 still
adds elementary-stream delivery and decoding; until the guest changes the audio decision from stop
to start, any test card remains a prototype rather than decoded broadcast media.
