---
name: digibox-emulator
description: Working on the Go port of the Pace 2500N Digibox emulator — the NEC VR4111 CPU and MIPS16 reference, the firmware's device conventions, the oracle and its debug API, and the measurement discipline a hardware model needs. Use when writing the CPU core or a peripheral model, reading the firmware's disassembly, feeding it DVB sections, touching the browser oracle, or diagnosing a boot that runs for ever doing something plausible.
---

# The Digibox emulator

This project ports a Pace 2500N's real flash image to Go. **The established facts live in
`docs/reference/digibox-emulation.md`** — roughly six thousand lines — and that file is the record;
this is the working knowledge that keeps getting re-derived. Read the record before modelling any
device, and read it as *evidence*: it says how each thing was measured and which earlier readings
were withdrawn.

## The oracle, and the one thing you must not do to it

`reference/digibox-boot.html` is the predecessor: the same firmware, running in JavaScript, correct
enough to draw the Sky interface and show real programmes on the guide. It is kept deliberately and
**it is the only independent check this port has** (SPEC FR-6).

**So it is a measuring instrument, not a sibling implementation.** Changing it to agree with the Go
port is the one move that destroys its value, and it will look reasonable at the time — the two
disagree, the JavaScript is old, the fix is one line. Where they disagree, **the measured record
decides**, and if the record is silent the answer is to go and measure rather than to pick.

Two rules make editing it safe when it is genuinely necessary (TASK-1.9 adds its checkpoint
emitter):

- **Never change what an existing debug API emits by DEFAULT.** Every probe ever written measured
  some particular state, and "on a plain boot the match table is `0x40` and `0x73` only" is a
  statement about SILENCE. Add capability behind an option and switch it on at the call site.
- **Keep the old state reachable from the URL.** `?si=0` silences the broadcast and `?u203=0`
  unmaps the second flash chip, so "is this the broadcast's doing?" stays askable without editing
  the page.

**And the oracle's limit is stated rather than discovered later: where both implementations are
wrong in the same way, they will agree.** It catches porting regressions, not inherited ones.

## The first rule, because it is the whole genre

**A hardware model does not fail with a stack trace. It fails by running for ever doing
something plausible.** A wrong register model is a boot that stops at 20 tasks; a wrong
constant is a link that silently runs at sixty baud; a stale read is a confident finding
about a machine that was not in the state you assumed.

So: **run the boot gate after any change to the core or a device** — it boots the machine and
asserts the boot, a remote key and a DVB section delivery, and it can be shown to go red (SPEC
FR-18; built by TASK-1.11). Two changes in one day of the predecessor's life looked inert and
stopped the boot: one multiplied a threshold that counted CALLS rather than instructions, the other
let a register file read back its own writes into an interrupt handler that spins until the bit
reads clear. Neither raised anything.

## Part 1 — The CPU

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
disassembler (the predecessor's `periph/m16.py`, in the archive), `sllv $rx, $ry` means **`ry = ry << rx`** — the FIRST printed
operand is the shift amount. Confirmed twice in the demux LISR, where `li $v1,1; sllv $v0,$v1`
builds `1 << f` for a filter number in `$v0`.

**The T register is implicit.** `cmpi $rx, imm` sets T = (rx != imm); `bteqz` branches when
T == 0, `btnez` when T != 0. `move $t8, $rx` also sets T, so a bare `move $t8, $v0` followed
by `bteqz` is "branch if v0 == 0".

---

## Part 2 — This firmware's conventions

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

## Part 3 — Instruments: what the oracle has, and what the port owes

The browser oracle carries the instrument suite every measurement in the record was made with.
**Porting it is FR-17 / TASK-4.6**, and until that lands the oracle is where a measurement gets
made — so this table is both a reference for driving it and the specification for the port.

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
| `__siTitles(pid, opts)` / `__siGuideSlot()` / `__siBAT(…, {linkage})` | the listings path |
| `__csiRxLog()` / `__cardFrames()` / `__csiState()` | both halves of the peripheral wire |
| `__blitLog()` / `__dmaLog()` / `__vramState()` | the drawing path |
| `__demod()` / `__eeprom()` / `__i2cState()` | tuner and NVRAM |

**The Go port gets two instruments the oracle never had, and they change the method rather than
just the speed:** a **gdb stub** (FR-16) and **snapshot/restore** (FR-8). A hypothesis that cost the
predecessor six minutes to test — a cold boot plus a 200-second channel-list rebuild — should cost
seconds from a post-acquisition snapshot. That is the whole reason this port exists; if an
investigation in phase 7 is still paying boot costs, something in phase 4 was not finished.

**Every instrument asserts its own subject.** A census that cannot find the thing it is counting is
a **harness failure**, never a count of zero. This is not a style note — see Part 4 rule 2, where it
cost two runs of a 236-entry census and a wrong conclusion. `internal/platform/instrument` owns the
guard so that each probe cannot forget it independently.

## Part 3b — Reading the firmware: Ghidra

The import and decompile scripts came across with the rest of the tooling:
`tools/ghidra/ghidra-import.sh` builds the project, `tools/ghidra/ghidra-decompile.sh <addr>` gives
the disassembly and the decompiled C of any address. `ctl.sh` should wire both as verbs
(`decompile`, `ghidra:import`) when TASK-1.1 writes it, because that is how every note in the record
cites them. The language is **`MIPS:BE:32:16e`**, which is exactly this CPU, and the decompiler
handles MIPS16 — a page of it becomes a dozen lines of C.

The project directory is gitignored and must be called `ghidra/` — **not** `.ghidra/`, because
Ghidra refuses any path component starting with a dot, which costs one confusing error before you
notice.

**The project is two blocks, because the running program is not one file.** The application executes
from a RAM image the bootloader DECOMPRESSES to `0x800009F4`; its strings and the pool words it
loads with `lw $rx,n($pc)` live in the flash at `0x9FC00000`. Import one without the other and half
of every function is unreadable. This is why `firmware/application-ram-image.bin` is kept as a
capture rather than regenerated — every Ghidra seed and every address in the record is expressed
against that exact copy.

**ISA_MODE is SEEDED, never blanketed.** The image is mixed MIPS32 and MIPS16 reached through
`JALX`, so setting MIPS16 across it decodes every MIPS32 function into plausible nonsense. The
import script seeds entry points that were found by TRACING the running machine — each one known to
be MIPS16 and known to be a function — and lets flow analysis expand. Add a seed when you prove one.

**IT READS; THE EMULATOR IS THE GROUND TRUTH, and that ordering is not deference.** Ghidra's decode
was checked instruction-for-instruction against a function the emulator executes before any of it
was trusted. But the static reading of this firmware has been **backwards twice** in one session — a
tag comparison whose branch skipped on MATCH (so the value it tests for means "accept", not
"redirect"), and a record id that is a halfword where the surrounding fields are words. Both looked
settled on the page and were only corrected by running them. So: read to find the shape fast, then
prove the shape by driving it.

## Part 4 — The measurement discipline

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

**11. The oracle proves agreement, not correctness — and a green comparison over a short run is the
easiest vacuous pass in this project.** Tier 1 compares state hashes end to end across a full cold
boot for a reason: two implementations agree trivially for the first few thousand instructions,
because nothing has happened yet. Before believing a comparison, check how far it ran and that it
ran to the point you claim.

**12. Report the URL, not the command.** The predecessor reported "verified end to end" three times
from a dev server while the demo host was broken. Every probe defaults to localhost; the person
looking at the work has the public host open. The tell is that you can name the command but not the
URL it hit.

## Part 5 — Feeding it a stream

The box asks for exactly three PIDs and says so: `__dispState().pids` on a booted machine
gives **filter 22 → PID 0x0014 (TDT, the clock), 23 → 0x0011 (SDT, the line-up), 24 → 0x0010
(NIT)**, with the enable at `+0xD8` reading `0xFFC00000` — filters 22..31, matching the ten
that carry contexts in the filter records. **The channel index IS the filter index.**

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

**There is no MPEG-2 video decoder here and none is planned.** The target is the menus and the
guide, which the OSD and blitter already draw.
