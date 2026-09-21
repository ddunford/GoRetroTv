# Emulating the Digibox — what is established

Facts about `FLASH_U202.bin` (md5 `7541fb4884d72b03858f7175217c2177`, Colibri's published JTAG
dump of a Pace 2500N) established by disassembly and by running the code. **Every claim here was
either verified twice by independent methods or is marked otherwise.** Where a method is named it
is because the method is the evidence.

## The machine

**NEC VR4111, big-endian MIPS, mixed MIPS32 + MIPS16 reached via `JALX`.** RTOS is **Nucleus
PLUS** — the image says `Copyright (c) 1993-1998 ATI - Nucleus PLUS - NEC4111`. MIPS16 is the
ORIGINAL ASE, not MIPS16e: `SAVE`/`RESTORE` appears nowhere, and a VR4111 would not have it.

**The VR4111 is a Windows CE handheld part and documents none of the AV hardware.** Its manual
(NEC U13137EJ2V0UM00, April 1998, 775pp, on bitsavers) has zero hits for CLUT, palette, MPEG,
demux, video encoder or framebuffer. Its on-chip registers live at physical `0x0B00_xxxx`, i.e.
KSEG1 `0xAB00_0000`. **Our peripherals at `0xB0000000` map to physical `0x1000_0000` — ISA-MEM,
the EXTERNAL system bus.** They are a separate Pace/ST ASIC and no public manual documents them.
~~The VR4111's on-chip modem (HSP) and keyboard (KIU) units are the Digibox's phone line and front
panel, which fits.~~ **WITHDRAWN 2026-09-21. It does not fit, and the firmware's own behaviour says
so.** The VR4111's on-chip HSP lives at physical `0x0C00_xxxx` (KSEG1 `0xAC00_xxxx`) and its KIU at
`0x0B00_xxxx` (`0xAB00_xxxx`) -- and **the oracle's complete address census contains zero hits for
either range**. The modem this firmware drives is an external UART at `0xB2001000`, which is
physical `0x12001000`, inside ISA-MEM. A firmware arming three interrupts on a discrete VR4111 could
not avoid ICU1/ICU2 and never touch them, so the chip is very likely NOT a discrete VR4111: drew1440
records that the 2500N (9F04, 2000) "replaced the ST processor with a NEC EMMA processor, which was
MIPS based", and the VR4111 manual's own §1.1 says the VR4111 is a VR4110 core plus seventeen
peripheral units -- so the Nucleus `NEC4111` port string names a core family and does not prove the
part. **The specific part number is NOT established**: no source found names a µPD61030 or ties a
VR4110 core to the 2500N. Treat `0xB0xxxxxx`/`0xB2xxxxxx` as the peripherals of an unidentified
NEC set-top SoC rather than as glue around a handheld CPU.

## The board's other chips — what is known, and the one lead worth chasing

**The CPU is settled from the image itself**: the Nucleus PLUS port string reads
`Copyright (c) 1993-1998 ATI - Nucleus PLUS - NEC4111 GHM 1.1.G1.3`, which names the core, and the
emulator runs the code. Everything else on the board is inference, so it is marked as such.

| Part | Evidence | Status |
|---|---|---|
| NEC VR4111 (or a VR4100-family sibling), big-endian MIPS | the RTOS port string above; the code executes | **SOURCED** — NEC manual on bitsavers |
| A media ASIC at KSEG1 `0xB0000000` → physical `0x1000_0000` (ISA-MEM, the external bus), carrying the OSD, blitter, DMA and demux | mapped register by register by driving the firmware; see the peripheral tables below | **MEASURED, part unidentified** |
| NDS conditional access | build paths `Y:\p0052_sw\ndsca\drivers\{ecm,emm,emma_nds_block,descrambler,ndsca_driver}.c` | **SOURCED** |
| OpenTV, release "Cougar" 124Bj | `/nfs/release/Cougar/124Bj/otv/src/malloc/farheap.c` | **SOURCED** |
| Sky application build tree | `H:/Sky_309/src/lstfirst.c`, `H:/Sky_309/src/stv.c` | **SOURCED** |

**`emma` in this image is EMM-A — conditional access — and NOT the NEC EMMA media processor.** It
is worth stating because the coincidence is a good one and it wasted time here: `emmAccessSemaphore`
sits directly beside `ecmAccessSemaphore`, and `emma_nds_block.c` is a sibling of `ecm.c` and
`emm.c` in the NDS CA driver directory. EMM and ECM are the entitlement and control messages of a
CA system. Nothing in the flash names a media part.

**THE LEAD, and it is unverified.** The only public account of this board's chipset — drew1440's
Sky Digibox history, <https://drew1440.com/sky/> — says the **2500N (9F04)**, released 2000,
"replaced the ST processor with a NEC EMMA processor, which was MIPS based", the later 2500S3/S4/S5
(9F05–9F07) going to ST's STi5512. A 2500S5 board photo confirms that half: its main chip is
marked `OMEGA / STi5512SWE`, so the S-series is a **different generation and its teardowns say
nothing about ours**. What is NOT confirmed is the EMMA claim for the 2500N, and it does not sit
easily beside the RTOS naming a 4111 — NEC's EMMA1 (µPD61050) integrates a VR4120 core rather than
pairing with a discrete VR4111.

**The reasoning in that last sentence is WEAKENED, 2026-09-21, and the part number is unsourced.**
The VR4111 manual's own §1.1 states the VR4111 *is* a VR4110 core plus seventeen peripheral units,
so an RTOS port string of `NEC4111` names a CORE FAMILY and is perfectly consistent with a VR411x
-core SoC — it does not argue against EMMA at all. Two research passes could not source the
`µPD61050`/VR4120 pairing given here, and one found NEC's set-top part of that era described as the
`µPD61030` with a VR4110 core (EE Times, 9 Oct 1998) while the other could corroborate neither part
number. What IS sourced is drew1440's statement that the 2500N (9F04) "replaced the ST processor
with a NEC EMMA processor, which was MIPS based". **So: EMMA for the 2500N is now better supported
than when this paragraph was written, the specific NEC part number is unestablished in either
direction, and `µPD61050`/VR4120 should not be relied on.** The decisive evidence remains the one
named below — a board photo — plus the census point now recorded at the head of this file: the
firmware never touches a VR4111's internal I/O ranges.

It is worth settling because of what it would buy: if the media ASIC is an EMMA, its OSD and
blitter have vendor documentation and a Linux port (`arch/mips/emma`), which is a register map for
the one part of this machine that has been mapped entirely by hand. Two things would settle it and
neither has been done: **a photograph of a 2500N (9F04) board** with the large NEC part legible, and
a comparison of our measured register offsets at `0xB0000000` against a published EMMA map. Until
one of them exists, the ASIC is unidentified and the tables below are the only description of it
there is.

## The CPU — what an emulator must implement

**Reference: NEC VR4111 64/32-bit Microprocessor User's Manual, µPD30111, document
U13137EJ2V0UM00 2nd edition, April 1998, 775pp.**
`http://bitsavers.trailing-edge.com/components/nec/mips/Vr4111-um_199804.pdf` (2,951,472 bytes).
Siblings in the same directory: `Vr4111-ds_199908.pdf` (datasheet), `Vr41xx-um_200206.pdf` (later
family). Not vendored here — it is NEC's document and 2.9 MB — so the facts that cost time are
written out below and the URL is recorded above.

**It documents the CPU and the bus and nothing else we need.** Zero hits for CLUT, palette, MPEG,
demux, video encoder or framebuffer; its 150 `LCD` hits are bus timing for an EXTERNAL controller.
Useful chapters: 4 and 29 (MIPS16 encodings), 30 (COP0 hazards), 11 (BCU — the external-bus
windows our bring-up programs), 15 and 19 (interrupts, GPIO).

Three CPU features were missing from our emulator and **each one looked like a hardware problem
until it was read**. None of them is exotic; all three are load-bearing for an RTOS.

**COP0 is not optional.** `mfc0`/`mtc0` must be a real 32-register file. Stubbing them
("reads 0, writes discarded") silently destroys every context restore, because the scheduler puts
a task's `Status` and `EPC` into CP0 and `ERET`s to it.

**`ERET` picks its return register from `Status.ERL`** (manual 7.3.12 and the exception chapter):

    if Status.ERL:  PC = ErrorEPC (CP0 30);  Status.ERL = 0
    else:           PC = EPC      (CP0 14);  Status.EXL = 0

and it has **no delay slot**. The manual is explicit that *"ERET loads the ISA mode from bit 0 of
the EPC or error EPC register"* — so bit 0 carries MIPS16-vs-MIPS32 and must be masked off the
address. Implementing only the EPC branch is correct until the first error exception and silently
wrong after it.

**`Count` (CP0 9) must advance and `Compare` (CP0 11) must fire** (manual 7.3.3/7.3.4). Count
increments with the MasterOut clock; when it equals Compare a timer interrupt is requested; and
**writing Compare clears that request**. With a static Count, no time passes: the RTOS never gets
a tick, never preempts, and delay loops spin for ever. The bootloader task sits in a routine that
reads a counter and multiplies by ten, which is what that looks like from the outside.

Interrupt delivery: take it only when `Status.IE` is set and `EXL`/`ERL` are clear and the
matching `Status.IM` bit is set; then `Cause.ExcCode = 0`, set the `IP` bit, `EPC = PC | isa`,
set `EXL`, and vector to `0x80000180` (or `0xBFC00380` when `Status.BEV` is set). The boot
installs handlers at the normal vectors and runs with BEV clear.

**Relevant CP0 numbers:** 8 BadVAddr, 9 Count, 11 Compare, 12 Status, 13 Cause, 14 EPC,
16 Config, 30 ErrorEPC. **Status bits:** IE 0, EXL 1, ERL 2, IM 15..8, BEV 22.

**Instructions that are easy to miss:** `MTHI`/`MTLO` (SPECIAL fn `0x11`/`0x13`) — the reads are
obvious, the writes appear only in context restores, so dropping them corrupts HI/LO across task
switches and nowhere else. With those, the unknown-opcode count over a 194-million-instruction
boot is **zero**.

**One deliberate inaccuracy in our emulator, recorded so nobody trusts it:** Count is advanced
per instruction rather than per clock. Monotonic but wrongly scaled, which is enough for the
firmware's ordering and not enough for anything that measures real time. A faithful divider needs
the measured MasterOut ratio.

## Two programs, and this caused a long detour

**The bootloader** occupies `0x0-0x1FFFF`, is write-protected, and has its own Nucleus. It starts
with BOOTMain, then creates SMNTask, SMTTask, SMHKTask and EVTTask when the board timer wakes its
event loop. **The application** is a separate image at flash `0x20000` (`JB` header, length `0x17F62C`,
manufacturer `0x9F`, version `0x2D`, `SIGN` at `0x19F648`) with its entry at **`0xBFC2048C`**.

Its loader at `0xBFC20618` walks a `(dst, src, len)` table at flash **`0x211DC`**:

    a0000400 <- 9fc20d00  0x000d0   trampoline
    80000800 <- 9fc20dd0  0x001f4   vectors
    800fbe10 <- 9fcc6278  0x09b78   data blob 1  (contains the EPG Huffman dictionary)
    80105988 <- 9fccfdf0  0x00900   data blob 2
    800009f4 <- 9fcd06f0  0xfb418   THE MAIN IMAGE, compressed

`FETask`, `SMHKTask`, `SMNTask`, `SMTTask`, `SCTask`, `EVTTask` are the APPLICATION's tasks. Boot
the bootloader to its first scheduler turn and one Nucleus `TASK` exists; let its timer run and
five exist; enter the application and there are **eleven** at the measured checkpoint. Time was lost diagnosing "why are no tasks created" while running the wrong
program entirely.

## The inner codec — solved

The dense ~812 KB is **not compressed in any standard sense and no LZ test could ever have found
it.** It is a halfword-dictionary code, unpacked by a routine at **`0xBFC206A0`**:

    0xD06F0   u32 block count = 32,161
    0xD06F4   256-entry dictionary of big-endian u16
    0xD08F4   16-bit control words, MSB first; each bit selects either a
              ONE-BYTE dictionary index or a TWO-BYTE literal halfword

847,553 bytes in, **1,029,144 out**, loaded at `0x800009F4`. Accounting closes exactly:
`268,139*2 + 246,437 + 32,161*2 + 516 = 847,553`.

**Three independent confirmations.** An offline decode written from the algorithm description, a
second written by transliterating the disassembly, and *the firmware's own decompressor executing
under emulation* all produce the same 1,029,144 bytes (976,383 non-zero, fnv1a `49bfde42`). And
the dictionary corroborates itself: sort the decoded output's halfwords by frequency and the
ranking reproduces the dictionary's own order — slots 0-19 are frequency ranks 0-19 exactly. That
cannot happen if the decode is wrong.

**`COMP` is a different thing and is dead code at boot.** The vendor-documented OTA format (a
nibble-aligned LZ77, validator at `0xBFC11BC8` enforcing a 17-byte header) is the over-air upgrade
path. ZERO `COMP` containers exist in the image; the flash is already that format's output.

## Graphics — reachable, and proved by running it

The bootloader draws the UPDATING SYSTEM SOFTWARE screen with two self-contained MIPS16 calls,
no RTOS task, no stream, no viewing card:

    jal 0xBFC02970   display / OSD init
    jal 0xBFC02A4C   build CLUT, program the OSD, blit 229 rows into both fields

**`0xB0004200` is an OSD display-list ROOT POINTER, not a framebuffer base** — written
read-modify-write as `(old & 0xFE000000) | ((phys | 0x01000000) & 0x01FFFFFF)`, pointing at a
descriptor at `0x80013C0C` whose `+0x10`/`+0x14` hold the field buffers `0x801D97F8` and
`0x801E6278` (2bpp, 150-byte stride, 229 rows). A value-based hunt for "a register holding a DRAM
address" finds `0x01013C0C` and concludes there is no framebuffer.

**The BMRL footer stores ABSOLUTE DRAM ADDRESSES**, not flash offsets — the resource is in `.data`
and is memcpy'd to DRAM by a table at flash `0x1AD24` during reset, before any drawing. This is why
searching the image for `0x1F004` or `0x177F0` finds nothing: the code has no use for them.

Running those two calls under emulation writes 34,350 bytes byte-identical (md5
`45bbbc599e10f22f8059ac1cce84b3b5`) to an independent offline decode.

## The hardware to model — the firmware's own device inventory

A `{base, 0, extent}` table at flash **`0x1AB68`** lists every region the machine has: six DRAM
regions, then **30 peripheral blocks**, then the three reset memcpy entries, and then — run on in
the same area — the Nucleus object names (`PACEHEAP`, `BOOTMain`, `sysheap`, `sempool`, `rtosq`,
`partpool`, and `MIKE PEARCE WOZ HERE`) and the flash device table. Two independent readings agree
on 30.

| base | size | what it is | how we know |
|---|---|---|---|
| `0xB0000000` | `0x74` | chip ID / config straps; `+0x10` is a 2-bit clock strap read once | SOURCED |
| `0xB0000030`, `0xB0000040` | — | **interrupt status** — the dispatcher masks against both | **SOURCED**: raising IP2 makes the firmware read exactly these two |
| `0xB0000200` | `0x94` | — | table only |
| `0xB0000400` | `0x54` | memory / SDRAM controller, written first, before stack setup | PARTIAL |
| `0xB0000600` | `0x24` | clock / PLL; `+0x10`'s value is selected by the measured clock | PARTIAL |
| `0xB0000800` | `0x14` | — | table only |
| `0xB0002000` | `0x4DC` | touched during bus bring-up | GUESS |
| `0xB0004000` | `0x620` | **video scaler + OSD**. `+0x100..0x17F` is a 16-phase x 4-tap polyphase coefficient table; **`+0x200` is the OSD display-list root** | **SOURCED** — the bootloader draws through it |
| `0xB0006000` | `0x44` | — | table only |
| `0xB0008000` | `0x98` | touched by MIPS16 bus bring-up (`+0x18`) | PARTIAL |
| `0xB0009000` | `0x244` | `+0x120` is a completion flag polled after command `0x08031F00` | SOURCED (mechanism) |
| `0xB000A000` | `0x160` | **DEMUX**. `PIDREG[ch] = +0x14 + 4*ch`, ch 0..31, bits 12:0 = PID, 15:14 = mode, `0x1FFF` = null | **SOURCED** |
| `0xB000B000` | `0x1000` | largest block — a buffer window rather than registers | GUESS |
| `0xB000D000` | `0xE4` | board timer; channel 0 status at `+0xD0`, acknowledge at `+0xE0`, IRQ mask `0x40` | SOURCED — bootloader EVTTick advances when this interrupt is modelled |
| `0xB1000000` | `0x38` | — | table only |
| `0xB2000000`–`0xB2009000` | various | ten blocks of identical shape (`0x74`/`0x94`/`0x134` repeating) — a bank of like devices | GUESS |
| `0xB200A000` | `0xF4` | **flash controller** — the flash device descriptor in DRAM holds `0xB200A000` and `0xB200A010` | SOURCED |
| `0xB4000000`, `0xB4080000`, `0xB4080100`, `0xB4180000`, `0xB4800000` | `0x10`/`0x4` | tiny blocks — latches or single registers | table only |

The oracle's word at `0xB200A000` retains writes for read-modify-write use. At retired
instruction 28,307,998, guest PC `0x80006780` reads that address into `a2`; the following
oracle state has `a2=1`, while an unmapped Go bus returned zero. The difference lasted two
instructions and disappeared before the next 100,000-step checkpoint. A one-word readback
model resolves this and five other isolated differences: the full 470-million-instruction cold
boot now matches all 470,000 browser checkpoints at 1,000-step cadence. This observation
establishes the word's readback behavior, not the identity of the whole `0xF4`-byte block.

**The technique that identifies a device without guessing: make the firmware say which one it
means.** Raise an interrupt line with every register answering zero and log what it reads while
servicing. It read `0xB0000040` and `0xB0000030` — so those are the status registers, told to us
by the code rather than chosen by us. The same trick works for any device whose handler can be
reached.

**And the interrupt dispatch table is data, so it can be read live.** The bootloader's is at
`0x800050D8`: 8-byte `{mask, handler}` rows, terminator `0xFFFFFFFF`, 19 entries of which only two
are armed (`0x2000` -> `0x9FC053E9`, `0x40` -> `0x9FC0434D`). Reading it out of DRAM gives the
device-to-handler map directly. The application has its own table at an address not yet located —
a scan for the same shape returned two candidates at `0x80101018` and `0x80101100` and **both are
false positives**: their masks repeat (`0x2`, `0x4`, `0xFF`) where real masks would be distinct
bits, and the handler column reuses a handful of addresses.

## Feeding it a stream — the interface

**Demux register block `0xB000A000`, size `0x160`**, declared in a 30-block SoC peripheral map at
flash **`0x20FD8`** — which is the register map the NEC manual could never provide.

    PIDREG[ch] = 0xB000A014 + 4*ch,  ch = 0..31
    bits 12:0 = PID,  bits 15:14 = channel mode,  0x1FFF = null / off

`demux_init` at `0x80002938`, `demux_hw_reset` at `0x800028EC`, `demux_reset` at `0x80004F28`.

### Standard DVB SI — builds the service list

| table_id | PID | what it gives us |
|---|---|---|
| `0x40` NIT actual | `0x0010` | network / transport layout |
| `0x42` SDT actual | `0x0011` | the channel line-up |
| `0x70` TDT | `0x0014` | **wall-clock time — this is how the box is told it is 1998** |
| `0x73` TOT | `0x0014` | time offset |
| `0x4A` BAT | — | bouquet (Sky's channel grouping) |
| `0x4E` / `0x4F` EIT p/f | — | present/following event |

Service-list builders: `ASTRA_SDT_SVL` `0x800A6454`, `OTV_NIT_SVL` `0x800A7588`,
`SKY_BAT_SVL` `0x800C0510`.

### OpenTV EPG carousel — the listings themselves

Section parsers at `0x800C95D0` (table_ids `0xA0-0xA3` kind 0, `0xA4-0xA7` kind 2, `0xB0` kind 1)
and `0x800C9CA0` (`0xB1`). Event record parser `0x800CA028` dispatches on a record tag; tag `0xB9`
is Huffman-compressed text.

**Huffman decoder `0x800BECF0(src, srclen, dst, dstmax)`; dictionary at flash `0xC8400` →
runtime `0x800FDF98`.** Verified by reading it: `(Including`, `(New Series)`, `(Part `, `(Repeat)`,
`(Stereo)`, `(Stereo) (Teletext)`, `(Teletext)`, `(Widescreen)`, then `Action`, `Adventures`,
`America`, `Animated`, `Australia`, `Baby`, `Best`, `Bill`, `Black`, `Blue`, `Breakfast`,
`Britain`, `British`, `Business`, `Call`, `Cartoon`, `Channel`, `Children`, `Clock`... the
vocabulary of 1990s British TV listings, alphabetised.

**`tvheadend/src/epggrab/module/opentv.c` (GPL, 1,153 lines) decodes this format, and the firmware
confirms its field offsets byte for byte.** That upgrades the prior art from "available" to
"validated against the original implementation".

## Prior art — none for the emulator

No emulator of a Sky Digibox, Pace STB, OpenTV receiver or Nucleus-PLUS MIPS box exists publicly.
MAME has no VR41xx core and **cannot decode MIPS16 at all**. QEMU implements MIPS16e but has no
board. The emulation community's own set-top box category is a stub. No public OpenTV o-code VM,
disassembler or specification exists — though a 2000 Luleå master's thesis on OpenTV→MHP migration
documents the build side, and notes that **o-code binds to ROM libraries at RUNTIME**, which means
the interpreter we are already emulating is where those calls land.

## The application's boot, traced to a single missing mechanism

Every link below is sourced, and the chain was traced from both ends — statically from the
decompressed image, and by measurement on a running emulator — with the two agreeing.

    timer channel 0 (interrupt mask 0x40)
      -> ISR 0x8000C020: reads status 0xB000D0D0, tests bit 0, acknowledges by
         writing the channel bit to 0xB000D0E0
      -> callback at [0x8010987C] = 0x80034E5D = EVT_Activate_HISR
      -> EVTHISR -> EVT_Wake 0x80034E70 -> releases semaphore 0x8011E444
      -> EVTTask (TCB 0x8011E374) wakes and drains the work list at *0x80105CCC

**The interrupt infrastructure works end to end.** Set the device bit in the PENDING register
`0xB0000030`, raise IP2, and the application's own LISR at `0x80024828` reads its registers, walks
its dispatch table at DRAM `0x800FC68C` (19 `{mask, handler}` rows, `0xFFFFFFFF` terminator,
seeded at runtime in BSS so only readable live), matches, and calls the handler. Measured 51 ISR
hits and 51 callback hits, one for one.

**`0xB0000040` is the interrupt ENABLE register**, read-modify-written by the firmware to arm and
disarm devices, so it must read back or every previously armed device is silently disarmed.
**`0xB0000030` is the raw PENDING register.** The dispatcher returns after the FIRST match, so one
IP2 assertion services exactly one device.

**Nucleus control-block layout, proven from both directions.** Every object carries a four-byte
magic at `+0x0C` and an inline eight-byte name at `+0x10` — `"TASK"`, `"SEMA"`, `"HISR"`, `"EVNT"`
— which makes the whole object graph walkable from a memory dump. For a TCB, `+0x68` is the
cleanup routine and `+0x6C` the suspend block; for a semaphore, `+0x18` is the count and `+0x20`
the number of tasks waiting, with `+0x24` the head of the suspension list. Matching a task's
suspend block against an object's suspension head is what identifies **which** object a task is
blocked on, and it is exact rather than circumstantial.

**The cleanup routine names the WAIT TYPE**, which is the cheapest way to classify a blocked task:
`0x800D0FE9` is the semaphore wait, `0x800CD9C9` the queue/mailbox wait, `0x800CF0A1` the
event-group wait.

**There are six tasks, and counting magics overcounts badly.** A DRAM sweep for `0x5441534B`
returns seventeen hits, eleven of which are that constant appearing in code or data. The six real
ones are `TASK0`, `TASK1`, `SMHKTask`, `SMNTask`, `SMTTask` and `EVTTask`. The sound test is not
the magic but the neighbouring fields: a real TCB has a plausible cleanup routine and a suspend
block that some object's suspension list points back at.

### The scheduler's idle loop, and a retracted finding

`0x800D35DC` is Nucleus's idle loop and it is worth knowing by sight, because when the machine is
doing nothing this is where every instruction goes:

    800d35dc  lui $1,0x8010 ; lw $4,0x72d8($1)     -> [0x801072D8]
    800d35e4  lui $1,0x8010 ; lw $8,0x72b0($1)     -> [0x801072B0]  TCD_Execute_Task
    800d35ec  bne $4,$0,+3
    800d35f4  beq $8,$0,-7                          loop while nothing is runnable

So **`[0x801072B0]` non-zero means a task is actually running**, and it is the single cheapest
health check on the whole system. Nineteen million hits at `0x800D35E4` means every task is
blocked.

**A retracted finding belongs here, because it cost a day and the shape recurs.** This section
previously reported that HISRs were activated but never dispatched, and blamed the emulator's
interrupt delivery for bypassing a Nucleus entry shim. Both halves were wrong. The evidence was
`*0x80105CCC` holding a queued HISR while `*0x801072D8` read zero — but those were **two
independent polls of a transient**, sampled between a debugger's steps rather than observed at the
moment either changed. A watchpoint on the same addresses showed the enqueue/dequeue cycle
completing on every tick and task status cycling 6 -> 0 -> 6. Nothing was broken. **Polling a
transient is not observing an event**, and two polls that happen to disagree will manufacture a
mechanism that does not exist.

## The smartcard link — why the box stops

**The application boots, creates all six of its tasks, and then every one of them blocks. The
thing it is waiting for is the viewing card.** This is the gate; nothing downstream of it — video,
the OSD, the EPG — can be reached until the card answers.

    TASK0 (TCB 0x8017314C) blocked obtaining semaphore "Periph" (0x8011B3CC)
      its stack carries a return into 0x8002A44F, inside the smartcard driver
      SMNTask holds Periph, having sent a command, and waits on event group
        "SMNEvts" (0x8011B650) for a reply that never arrives

The subsystem names itself in flash, in the same block as `Periph`: `SMART0`, `smart_card`,
`SCTask`, `SCSV%d`, beside `SMTEvts`, `SMTTask`, `SMTAck`, `SMTMutex`, `SMHKTask`, `SMNEvts`,
`SMNInit`, `SMNTask`, `CSIHISR`. **`SM` is the smartcard module**, and `CSI` is the clock
synchronous serial interface it talks over.

### The device

Read off the firmware's own ISR rather than guessed. Base `0xB2009000`:

| offset | |
|---|---|
| `+0x00` | control; the firmware writes `0x80` to enable |
| `+0x10` | data, one byte, read = receive, write = transmit |
| `+0x20` | status; **bit 0 = receive ready**, and the acknowledge is writing `status & 0xFFFFFFFE` back |
| `+0x30` | interrupt enable; the firmware writes `1` |

Interrupt **mask `0x2000`**, dispatch row 11. The firmware's initialisation, captured in order,
is `+0x00 <- 0x80`, `+0x20 <- 0`, `+0x10 <- 0`, `+0x30 <- 1`, and it then writes `0x2044` to the
interrupt enable — exactly the three masks it has registered handlers for (`0x2000` smartcard,
`0x40` timer, `0x4` the block at `0xB200A000`).

    byte arrives -> ISR 0x8002ADE5 -> tests status bit 0, reads the byte, stores it at
                    0x801063EC, acknowledges, activates CSIHISR (0x8011B434)
                 -> CSIHISR entry 0x8002AE0D -> sets flag 1 in event group SMNEvts
                 -> SMNTask wakes

### The wire protocol

    payload = [LEN] [SEQ] [CODE] [args...]        LEN counts the bytes after itself
    0x00 terminates a frame
    0x1B escapes: the next byte is taken verbatim, so a payload 0x00 or 0x1B goes out as 1B <byte>

`SEQ` is stamped by SMTTask at `0x8002B294` from a counter at `0x800FD1C8`, incrementing and
wrapping to 1 after 127. **A reply echoes the sequence of the request it answers.**

**The length field is the oracle for the escape rule**, and it is worth using rather than trusting
the rule: `05 02 53 20 1b 00 01` on the wire de-escapes to `05 02 53 20 00 01`, whose LEN of 5
matches the five bytes after it. De-framed without the escape rule the lengths stop matching.

### Two things that are ours, not the firmware's, and both were measured the hard way

**The card is the clock master.** The ISR receives without ever writing the data register, and
SMNTask shifts its own frame out one byte per byte received — so if nothing clocks the link the
box falls silent with its message half-sent. Modelled the other way round, transmit-clocked, the
box sent one command frame and stopped dead.

**The link has a baud rate and it is load-bearing.** The card interrupts the CPU on every byte, so
a link faster than the box can do work between bytes starves it, and the failure looks exactly
like a protocol that stalled. Measured on the same firmware and the same card: at one byte per
timer tick, **three** frames; at one byte per sixteen ticks, **117**, including the whole opening
sequence. Unthrottled, the box never got far enough to send a single byte.

### The handshake, which now completes

The card answers one frame — `0x52`, with `[0x03, <echoed seq>, 0x52, 0x02]` — and that is enough.
The handler at `0x8002A534` reads `frame[3]`, masks it to three bits, stores it, and releases
`Periph` **unconditionally**; the arg is compared nowhere, so any value 0–7 works. The sequence
number must be echoed from the request rather than fixed.

    box   03 01 11 07       box   03 06 41 08       box   03 14 51 00
    box   05 02 53 20 00 01 box   03 07 42 00       box   03 15 10 00
    box   03 03 54 02       box   03 08 17 01       box   03 16 22 01
    box   03 04 41 00       box   03 09 52 00       box   03 17 22 05
    box   03 05 43 87       card  03 09 52 02

`0x11 0x53 0x54 0x41 0x43 0x42 0x17` go unanswered and the box proceeds past all of them, so they
do not block — **and that is a trap, because it means "the box moved on" is not evidence a reply
was accepted.** Advancement is driven by SMTTask's 50-tick timeout. `0x18` is not a retry either:
it is SMHKTask's periodic heartbeat, reached from the descriptor at flash `0x9FC237A0`, with no
retry counter and nothing that will ever stop it. The honest success signal is **a frame after the
`0x52` that is not `0x18`** — the four on the right above — or the semaphore's waiter count
falling to zero.

Each command has a four-byte template `LEN 00 CMD 00` in flash — `0x11` at `0x9FC23840`, `0x52` at
`0x9FC2383C` — reached from this driver's literal pools, with the `00` in byte 1 as the sequence
placeholder SMTTask stamps.

### A reply must be PACED, and this cost most of a day

**The ISR stores every received byte to a single location, `0x801063EC`.** There is no receive FIFO.
A byte that arrives before SMNTask has consumed the previous one overwrites it — so a reply sent
as fast as the box will acknowledge it loses all but its last byte, the frame reaches the sink with
`len == 0`, and is dropped silently at `0x8002AEC0`. Replies are paced at the link rate like
everything else.

It was hard to see because every part of the receive path was healthy and said so. The sink
pointer at `0x801063F8` held a valid `0x8011B928` throughout; the validator was reached; the
framing was right. The only thing that named it was counting instructions at the sink call
`0x8002AED2` and finding **zero** while the pointer it would have used was perfectly good — which
points at the byte rather than at the frame.

**The receive path has exactly two gates, both silent drops**, and they are worth knowing before
debugging anything on this link: `len == 0` at `0x8002AEC0`, and a NULL sink at `0x8002AEC6`.
Beyond them, every branch chooses *which* dispatcher runs, not *whether* one runs — so validator
entries with no dispatcher entries can only mean the validator rejected the frame, and the
validator tests only `len == buf[0]+1`, `len < 32` and `len >= 3`.

## The blitter — how the application draws, and why the first picture was a wait

Past the card, the clock, the modem and the AV probe, the box drew nothing and slept. The graphics
client `0x8000AD15` allocates two planes from the `nocache` pool (KSEG1 DRAM at
`0xA0575788`–`0xA079FF88`), plants a sentinel in the LAST TWO BYTES of each, asks for a fill, and
`NU_Sleep`s in a loop at `0x8000AE2E` until those bytes read back as the fill colour. There is no
status bit and no interrupt on the client side: **completion is the engine having written DRAM.**
An emulator whose blitter does not write memory leaves the box asleep for ever, which is exactly
what 425M reads of a register that was not even the operand looked like.

The chain, every hop of which is now measured:

| hop | where | what |
|---|---|---|
| request | `0x800027CD` | pure software: `NU_Send_To_Queue` on **`BlitReq`** (`0x80184250`, 60-byte messages). Returns 0, or 6 if the send fails — and on 6 the client falls back to a CPU `memset` |
| driver | TASK6, body `0x80002805` | receives into `0xA079FF88 + 60n`, spins while `*(0xB0006040) & 1` (blitter busy), then submits DMA **channel 12** with `0x80005900(12, slot, 0, 60)` |
| DMA | `0xB0009000` | 13 channels. Descriptor (40 bytes at `0x80108A60 + 40*ch`: +12 src, +16 dst, +20 len\|flags, +32 last byte) → physical address written to `+0x040 + 0x10*ch`, then `+0x220 = 1`, then bit `ch` set in `+0x010`. Channel destinations are hardwired; `dst` is unused. Completion: bit `ch` in status `+0x120` (read as a halfword at `+0x122`; second bank `+0x140/+0x142`), interrupt **mask `0x2`** (dispatch row 2, LISR `0x80005E29`), acknowledged write-1-to-clear at `+0x130/+0x150`, and the LISR clears the enable bit itself. `+0x010` must read back — the LISR read-modify-writes it |
| blitter | `0xB0006000` | channel 12 lands the 60 bytes in **fifteen command registers `+0x00..+0x38`**; `0x80002851` zeroes exactly those and sets bits 0 and 4 of `+0x3C` around it; `+0x40` bit 0 is busy |

The bootloader submits a DMA channel-5 descriptor for 1,524 bytes from flash. The oracle models
completion but no byte transfer behind that channel. The guest runs it **with the ASIC enable
register `0xB0000040` at zero**, polling status bit 5 instead. Raising IP2
for that completion sent the bootloader into a handler it had not armed and it never came back
(30M spins at `0x9FC0DE4C`). So the line is gated on the enable register, which is what the ASIC
does. Bit 5 of `0xB0009120` is channel 5's completion and not a global ready flag, which retires
the constant the emulator used to return there.

**The command format is the hardware's, and only the code that builds it can say what it means.**
The one operation decoded so far, FILL, opcode word 0 = `0x418F0000`: word 7 destination
(physical), word 11 pitch − 1, word 13 `(rows − 1) << 16 | (width − 1)`, word 14 colour in the
top byte. **Width and pitch are in HALFWORDS**, and that was measured, not read: the first fill
was `0x167+1` units by `0x11F+1` rows into a plane the client had sized at 720×576/2 bytes. As
bytes that is half the plane, and the sentinel never moved; as halfwords it is exactly 720×288,
the last two bytes went `0x0000 → 0x1010`, and the poll released. The unit stays a knob on the
page (`__blitUnit`, `__blitRedo`) for the next operation that needs the same experiment.

**The whole command set, from every builder in the image** (eight pool words name the kick;
the report is `scratchpad/blit/BLITTER_FORMAT.md` in the session that produced it, and the
executor in `digibox-boot.html` carries the same table): words 1–6 are the source and 7–12 the
destination, in the same order — address (field 0), address (field 1), chroma (field 0), chroma
(field 1), pitch − 1 in pixels, `(y << 16) | x`. Word 13 is `(h − 1) << 16 | (w − 1)`, word 14
a fill pattern. Word 0: bit 23 = fill, bit 24 = pattern from word 14 (else the value is word 1),
bits 18–20 = format (0..3 → 1/2/4/8-bit CLUT surface, 6 → 4:2:2 planar, 7 → 4:2:0 planar;
`0x418F0000` with bits 16–19 = 0xF is the 16-bit-cell fill), bits 21/22 = field-pair addressing
(even frame lines in the field-0 buffer, odd in field 1, buffer row = line >> 1). No colour key,
ROP or scaling exists in any builder. Unwritten words are stack garbage and the hardware ignores
them. `0xB0006040` bit 0 is the only status bit anyone reads — busy — and surface create/destroy
and frame-store free all wait on it before touching memory.

**Channel 8 is the plane upload into the decoder's memory.** `0x8000B12A` (4 bpp) and
`0x8000B1E0` (2 bpp) read a plane base back from the table the firmware programmed at
`0xB0002098..0xB00020A4` (4 bpp A/B, 2 bpp A/B; `+0x80000` selects the second bank), write it to
`0xB00020D0`, set mode `0xB00020C0 = 0x12`, and submit channel 8 with the plane as source. Those
table registers must read back: answering 0 sent every plane to VRAM offset 0. Raster
`0xB0002090 = 576 << 16 | 720`, and `720 × 576 / 2 = 207360` and `/ 4 = 103680` are the two
DMA lengths to the byte.

What the first four fills are is worth knowing before anyone looks for a menu in them: 720×288
of `0x10` and 720×144 of `0x80` — **Y = 16, Cb = Cr = 128, one field of a black 4:2:0 video
frame**, not an OSD. After each pair the firmware submits **DMA channel 8** with the plane as
source (207360 and 103680 bytes), which is the path into the decoder's frame memory behind
`0xB0002000` — the next device to model, and the one that actually puts pixels on a screen.

## The second card slot — a framed link the main task blocks on

After the planes are up the main task writes NVRAM, then sends `60 00 01 11 04 74` on the UART at
`0xB2002000` and blocks on semaphore **`STXD0`** with no timeout. That port was modelled as a
16550 console and it is neither: the driver (`0x8002C600`–`0x8002D900`, strings `SMART0`,
`smart_card`, `SCSV%d`, `SCTask`, `TXUART1`, `RXUART1`, ISO 7816 Fi/Di tables) talks a framed
command protocol to the controller behind the box's other card slot. Register map, read off
every access the driver makes: `+0x00` control (write-only; `0xC0`, `|= 0x08`, `&= ~0x03`),
`+0x20` error flags, `+0x30`/`+0x40` a `{0x42, 0x00}` pair from a flash table, `+0x50` TX data
(word store of the byte), `+0x53` RX data (byte read of the same word), `+0x60` interrupt status
(bit 0 error, bit 1 TX done, bit 2 RX ready; the driver acknowledges by writing 0), `+0x70`
interrupt enable, same layout. The line is dispatch mask `0x00010000`, LISR `0x8002CCA1`.

**The task writes only byte 0.** The LISR writes bytes 1..N−1 itself, one per TX-done
interrupt, and after the last it clears `+0x70` bit 1 and activates HISR `TXUART1`
(`0x8002CD65`) — the only thing that releases `STXD0`. Six interrupts per six-byte frame, so a
port that raises none holds the main task for ever. Frames are `[0x60][len16][cmd][payload][XOR]`;
the boot frame is cmd `0x11` with clock class 4 (the 3.6864 MHz crystal). The reply is waited on
`SRXC0` for 100 ticks and is optional — a timeout marks the slot absent and the boot continues —
but the honest answer for an empty slot is the controller's "no card", `E0 00 01 11 C0 30`, which
is what the model sends, one byte per interrupt with a gap, because the LISR stores into
`rxbuf[count]` and only the HISR advances `count`. Measured: frame exchanged at 388M instructions,
`STXD0` released, the main task moved on to the next subsystem.

## The I2C bus and the EEPROM — the box's memory, and why the CA task never finished

The I2C controller at `0xB2006000` is a byte-at-a-time master with a seven-state LISR
(`0x80006DA1`, demux mask `0x00200000`). `+0x00` control takes four values — `0x99` idle/STOP,
`0x9A` arm START (the next data write emits START and the address byte), `0xAC` receive and ACK,
`0xA8` receive and NACK; `+0x10` status bit 2 is "the slave acknowledged" and is the only bit the
LISR tests; `+0x40` data; `+0x50` interrupt acknowledge; `+0x60` interrupt enable. **The line
rises only after a data byte has been shifted out and its ACK sampled, or after a byte has been
received** — never on the control writes. The old model fired on the `0x9A` write, before the
driver had set its state, and answered reads with the last byte written.

One physical bus, multiplexed four ways by a latch at `0xB4180000` (bits 4–5 the channel, bit 3
disable) that the firmware sets before and clears after every transfer. On it: **vbus 3, the
NVRAM — a single 24C128 at 8-bit address `0xA0`** (16 KB, two address bytes, 64-byte pages;
`0x80029508`); vbus 0, a register-paged front-end module at `0x18`; vbus 1, the UHF modulator PLL
at `0xCA`; vbus 2, a diagnostic pass-through. Write-cycle waiting is a probe (`START + 0xA0 +
STOP`) every 20 ms with no upper bound, so a device that never ACKs hangs the box there.

**The EEPROM's first 0x1800 bytes are the system area** the CA task verifies at start-up: a
16 KB DRAM shadow at `0x801170E0` is filled once by a single 16384-byte sequential read, writes
go to the chip in 64-byte pages and are read back into the shadow and compared. The CA stores are
two regions of it (`0x400` "ECM", `0x1400` "EMM"), each of which must begin `01 01` or gets
formatted. With a controller that forgot every write, the shadow read `0xFF` end to end, the
format never landed, and the ECM task — priority 4, and its retry has no sleep — spun for ever
and starved the boot task (measured: 89M passes of the byte-sum at `0x800E1418`, TASK0's sleep
loop frozen for minutes with the RTOS clock advancing). So the array **persists in the browser**
(`localStorage`, saved on every STOP after a write), which is what a power cycle does to a real
box. Measured with the model in place: 16,408 reads for the shadow, the format writes stick
(`01 01 01 08 d2 …` at offset 0), the CA loop has zero passes, and the boot-init task runs to its
final `for(;;)` — the boot sequence is complete.

## The boot aborted, and the reason was an instruction nobody had implemented

With every device answering, the boot "finished" into a `for(;;)` at `0x8003A929` that read as
an idle loop and is nothing of the kind: it is the die-hook of the fatal-error routine
`0x8001ECF9`, which first prints `!!!!! ABORT !!!!!! -> <code>` with the task handle, entry,
return address and name to **console channel 0 — the modem UART**, where the page's serial
capture had it all along. Code `0x00` from return address `0x8007EEC3`: `ABORT(0)` at
`0x800365BC` in `0x800365A7`, taken when `0x80062A91("/eeprom")` — a chdir onto the block
partition the NVRAM driver registers at `0x8003001A` — fails to resolve.

The partition was registered, formatted and mounted; the eeprom-side stat was sound (root
record kind `0x1F`, mode `0x8333`, uid 1). The VFS walk (`0x80061469` → `0x80061305`) failed
to find `/eeprom` and, before that, `/eeprom/blacklist`, whose directory the firmware had just
created — and it went on creating one per boot. The write log from a blank chip showed the
root entry written as `blacklist ff ff ff 80 00 00 00`, and the lookup (`0x80061428`) wants a
`0x00` after the name. The three `0xFF` bytes were not the chip's: they were stack that
`strncpy` (`0x8007E2ED`) should have zeroed through `memset` (`0x80000D34`), whose
**unaligned path writes its words with `swl`/`swr`** — and the MIPS32 executor did not
implement `lwl`/`lwr`/`swl`/`swr`. The emulator's own ledger had the answer: 25 skipped op 42
and 25 skipped op 46, every one at `0x80000DE4`/`0x80000DE8`. (An hour went on the wrong
theory, that production EEPROMs ship zero-filled; a zero-filled chip aborted identically.)

The quartet is implemented with big-endian semantics, checked against a byte-level reference
over 64 cases, and proved with the firmware's own memset: an unaligned six-byte clear now
lands (`ff 00 00 00 00 00 00 ff`). Measured after: `/eeprom` resolves to node `0x01001001`,
no abort, and **the first OSD commands reach the blitter** — four field-pair 8-bpp fills of a
720×576 surface at `0x80584048` with colour index `0xDC`. The page gained the tools this
needed: `__breakAt([pcs])`, `__regs()`, `__step(n)`, `__resume()`, `__call(addr, isa, a0..a3)`
(run a firmware routine and read `$v0`), and an EEPROM write-transaction log `__eeTx()`.

## The first screen the application put up, and the interrupt that turned out not to matter

With the abort gone the interpreter drew — four field-pair 8-bpp fills of a 720×576 surface at
`0x80584048`, colour index `0xDC` — and then sat in the OS-abstraction wait `0x8007FED1`. The
one enabled ASIC line the emulator never raised was mask `0x01000000`: dispatch row 0, LISR
`0x800041B5`, whose device is the block at `0xB000A000`. A model that fired every armed bit of
that block once per field was added, the interpreter was seen to leave the wait, and for a
few hours the block was written up here as "the display interrupt".

**Both halves of that were wrong, and the second was measured.** The block is the **EMMA
transport demultiplexer** (the tuner mapping, below, reads the LISR: `+0xD8/+0xB8` are
section-filter done bits for 32 filters, each finished filter's write pointer fetched through
`+0x124/+0x128` into `0x80107588[f]` and pushed on the ring at `0x801074C0`; `+0xE0/+0xC0` are
general events, bit 31/30 toggling bits 5/6 of `+0x00` and the decoder register `0xB0002004`;
`+0xD4/+0xB4`, `+0xDC/+0xBC` and `+0xD0/+0xB0` are the PES and section groups whose survivors
land in `0x801076BC/C4/B8`; the HISR `0x8000446A` releases the semaphores at `[0x801062B8]`
and `[0x801062B4]`). Firing its bits with no section behind them handed the SI manager empty
sections. And the wait was not on it: a control boot with the block **completely silent** —
enables never armed, zero interrupts from `0xB000A000` — programmed the OSD root
(`0xB0004200 = 0x01583FE8`) at about 250 M instructions, exactly as before. `0x8007FED1` is the
VM loop's own idle wait (its frame is on TASK30's stack under `0x80084077`), which the interpreter
enters and leaves on its own events. The model now asserts the line only as a level over
`status & enable`, and nothing sets status until a real section is delivered.

**The application then programmed the OSD hardware itself**: `0xB0004200 = 0x01583FE8` — a
display-list descriptor at `0x80583FE8`, 720×576 at 8 bpp, field buffers `0x80584048` and
`0x805B6A58`, CLUT at `0x805E9468`, field size `0x32A00`. The bootloader-derived descriptor
decoder on the page reads it unchanged; the panel had simply been preferring the decoder's frame
memory (the black video field) over the display list, and now does the reverse.

**The picture is the application's.** The page renders the display list the way the hardware
does: indices through the surface's CLUT, both field buffers woven back into a 720×576 frame.
The CLUT entry is 16 bits, two to a word, high half first, read off the firmware's own packer
at `0x80008C30`: `Y[7:2]` in bits 13..8, `Cb[7:4]` in bits 7..4, `Cr[7:4]` in bits 3..0, from
Y/Cb/Cr byte triples. Index `0xDC` decodes to Y 20, Cb 160, Cr 112 — RGB (0, 5, 69), the Sky
EPG's dark blue — and that is what the screen shows. The descriptor decoder had been rejecting
every descriptor the application built, because it checked the KSEG0 window only and the
application stores `phys | 0xA0000000`.

**Where it stands: the EPG is idle.** `0x80084051` is the o-code VM's event loop — `0x80083E21`
dequeues the next event, `0x800835B1` scans the module table for one whose event queue
(`+80`/`+84`) is non-empty and returns −1 when none is, and the VM then waits for a new event.
APPMAN is in its own message loop (`0x800502B9`). The screen is the application's cleared
background, and what it draws next depends on an event it has not received — a timer, a key, or
a signal-state change from a tuner that has no stream behind it. That is the next thing to map.

## The remote control: how a key reaches the EPG

Mapped from the firmware by a subagent (listings under a session scratchpad, report
`inputs/REPORT.md`) and then measured in the emulator.

**Keys do not reach the CPU as IR.** A front-panel microcontroller decodes the handset and
reports keys over the **same CSI link at `0xB2009000`** that carries the card-slot traffic
(mask `0x2000`, LISR `0x8002ADE5` → HISR `CSIHISR` → SMNTask `0x8002AE21` → pipe `EVQP0001`
→ SMTTask `0x8002B1AD` → dispatcher `0x800297B1`). Frames are `[len][seq][type][payload…]`,
`0x00`-terminated, `0x1B`-escaped; seq bit `0x80` marks an unsolicited frame. Type `2` is an
IR key from the front eye: `frame[3] == 0`, `frame[4]` bits 7:6 clear, bits 5:4 the source
(the decoder at `0x8002AAB4` **rejects source 2**; 0 is the handset), raw command
`((frame[4] & 0x0F) << 4) | (frame[5] >> 4)`. The decoder `0x8002A811` looks the raw byte up in
the 256-entry table at `0x800FCEC0` and posts `0x8006EA05(1, device, logical)`, which mails a
12-byte record `{u16 class 2, u16 kind 7 press / 8 release / 9 repeat, u16 key, u16 device}` to
the listener's mailbox. Release comes from a 20-unit watchdog; auto-repeat at 500/250 ms is
generated in the input layer. Front-panel buttons (type 6) are hooked away by the NDS init in
this image and never become key events.

**THE SIX RANGES AT `0x800FCEB4` ARE THE SKY KEYBOARD, NOT THE HANDSET, and several of the
obvious raw codes fall inside them.** `0x8002A811` turns device 4 into device 5 for raws in
`0x48`, `0x5D..0x69`, `0x87..0x91`, `0x96..0xC9`, `0xD1..0xF2`, `0xF6..0xFF`. So back up is
**`0x3C`** and not `0x63`, and the handset's colour keys are **`0x6D..0x70`** and not
`0x66..0x69` — in each pair the lower code is a keyboard key carrying the same logical code,
which is exactly what makes the mistake invisible. Two of the codes being injected by hand
here were the keyboard's before the page started displaying the resolved device.

### The blocker was the link's baud rate, and it was sixty

The frames were arriving all along. Measured end to end with a continuous profiling record:
the LISR takes the bytes, SMNTask assembles the frame intact (`__csiRxLog` shows
`05 80 02 1B 00 05 C0 00` on the wire), the sink at `0x8002AED2` fires — and the **first** key
after boot goes the whole way to `0x8006EA05` and the mailbox. Every key after it stops dead at
the sink with the dispatcher at zero.

**SMTTask's stack says why**, and it is a self-deadlock rather than a protocol fault:

    0x8002B351  SMTTask's dispatcher call
    0x800297DD  dispatcher -> RAM table entry 2
    0x8002AACB  type-2 decoder -> 0x8002A811
    0x8002A90D  new-key path
    0x8002A645  0x8002A635 — FLASH THE FRONT-PANEL LED
    0x8002998B  build the LED command frame
    0x8002B45D  0x8002B449 send a command frame
    0x800348AB  0x800348A1 -> 0x80034829 with NU_SUSPEND

Decoding a key sends a command, on the pipe SMTTask is the ONLY drainer of. The pipe
(`EVQP0002` at `0x8011B834`) read `available = 0` with a suspension list — full, and full of
identical `03 02 18 00` messages. Command `0x18` is a **periodic software timer**: break at its
sender `0x8002A0DD` and `$ra` is `0x8002A313`, called from the timer service task `0x80034E55`,
which is why nothing in the image statically references it. It was firing faster than a round
trip could complete.

**Because the emulated link ran at sixty baud.** On this page's own time base — `TIMER_PERIOD`
= 20,000 instructions standing in for a 10 ms Nucleus tick — the old "16 ticks per byte" is
160 ms per byte. A period micro link is 9600 baud or better. So the rate is now counted in
instructions and split in two: the continuous idle filler is OURS, invented to keep SMNTask's
loop turning, and stays at the rate the boot was measured with; a real exchange runs at 2,100
instructions, which is 9600 baud on the same arithmetic.

**Making the link fast exposed the byte-loss race the slow one was hiding**, and that is worth
keeping separate from the rate: the box's ISR clears the ready flag long before SMNTask copies
the byte out of the single holding location `0x801063EC`, so "the interrupt is done" is not
"someone has read it". SMNTask reads before it writes, exactly once per byte, so **a write to
`+0x10` is the receipt for the previous byte** — payload bytes are gated on that and cannot be
lost at any rate. Without the gate, at 9600 baud, the `0x52` reply never reached its handler,
`Periph` stayed at count 0 with TASK0 suspended on it, and the boot sat at six tasks.

The acknowledge policy is a runtime dial (`__ackSet`) so two boots can be compared with nothing
else changed. The default is `0x52` and `0x18`: `0x52` is the card status the SM init blocks on,
and `0x18` has to be answered or every poll costs SMTTask a 50-tick timeout during which it
dequeues nothing. Answering **everything** is a different boot and a worse one — 19 tasks held
for 313 million instructions, TASK0 inside the NDS CA init waiting for SI sections we do not
broadcast, which is a real thing to wait for rather than a bug to be acked away.

### Where it stops now: the application answers keys and has nothing to draw

With the link fixed every key dispatches, and the o-code interpreter (TASK30) is genuinely
running application code for them — measured as instructions executed in `0x80083000-0x80090000`
per press: standby `0xCC` **53,483**, Sky `0x21` **33,130**, `0x108` **40,238**, `0x100`
**17,273**, `0x900` **6,743**, while `0x7F`, `0xF5` and `0x7D` run **zero**. So the application
is discriminating between keys rather than ignoring them.

**And the framebuffer hash does not change on any of them.** The box has no service list, no
channel line-up and no clock, so every screen it could draw is empty. That is the reading the
rest of the evidence supports rather than one this measurement proves on its own, and the next
section is the thing to do about it.

An on-screen handset lives on the page. Its `data-key` carries the RAW byte rather than a name,
because the native side translates raw bytes to logical codes and stops there — the names live
in the o-code. Keys whose meaning is established are named and the rest carry their logical
code; the readout resolves both the logical code and the input device from the live tables at
the moment of the press, which is what makes a keyboard-range mistake visible.

## The tuner: an always-locked demodulator

Mapped by a second subagent (`tuner/`). The satellite demodulator is on I²C vbus 0 at
address `0x18`, indirect-addressed: `[0x00, lo]` `[0x01, hi]` set a 16-bit register index,
then a port byte (2/3/4/5/7) moves data — port 5 takes the 1 KB microcode. The driver's lock
test is register 75 (port 3 offset 11) reading `0x17`; 78 → `0x02`, 34 → `0x03` to the identity
probe, 64..69 the BER counters (0 = clean). The emulator answers exactly that table
(`__demod()` logs every transaction; measured 1,119 register writes and the microcode upload
during boot, 11 reads). `0x8000665D` (mask `0x4`) is the GPIO block at `0xB200A000`, not the
demux; the demux is EMMA at `0xB000A000/0xB000B000` with section RAM at `0xA07A0000`.

**FETask never receives a command, and the gate is now located and is a single byte.** The
driver IS registered and live — walk the device list at `0x80105ECC` on a booted box and the
box's whole inventory is there, `demodulator` as **device 20** with open `0x8002F0B1` and close
`0x8002F1A1` installed, alongside `video`, `av`, `audio`, `osd`, `osd driver`, `image`, `jpeg`,
`font`, `inputs`, `opentv`, `box`, `boxid`, `modem`, `rtc`, `beep`, `medium`, `serial`, `fd`,
`audio_encoder`. So nothing is missing; nothing asks.

The asker is `0x80032D0D`, which maps a tuner state to a command and issues it — and it returns
immediately unless **`[0x80106220] == 1`**. That byte is read from thirty sites across the
front-end and card code and **written from exactly two**, `0x800F5ADE` and `0x800F5B3C`, both
inside the NDS module whose init is `0x800F5AD1`. On a fully booted box it reads **0**.

**So the tuner is gated behind the NDS module, and the NDS module is gated behind SI sections** —
TASK13 is parked in `0x800F50E1` on `NDSCASectionNotifyQueue` (status 4, queue suspend). Sections
come first and unlock both; the demodulator does not lead, it follows. That also retires the older
reading that a key or the installation state decides it.

**Read these globals on a BOOTED box or they will lie to you.** `[0x80105C44]` reads
`0xFFFFFFFF` before the application runs — its initialised value, meaning "not open" — and `20`
afterwards. It was read once here from a page that had been reloaded for a screenshot and not yet
booted, and reported as a registration failure that had not happened. `__tasks()` reaching 42 is
the cheap check that the box is actually up.

## Feeding it sections — the delivery path, proved

The box asks for exactly three PIDs and says so. On a booted machine `__dispState()` reports
**filter 22 → PID `0x0014` (TDT, the clock), 23 → `0x0011` (SDT, the line-up), 24 → `0x0010`
(NIT)**, and the enable at `+0xD8` reads `0xFFC00000` — filters 22..31, the same ten that
carry context pointers in the filter records at `0x80142D34`. Those two facts were measured
separately and agree. **The channel index IS the filter index**; there is no second mapping.

**Status and enable were the wrong way round for months, and the firmware's own programming
order is what says so.** It arms each filter by writing a SINGLE BIT to `+0xD8` immediately
before programming that filter's PID — `0x01000000` then `+0x74 ← 0x00014010`, and so on down.
A write-one-to-clear status register is not written a bit at a time just before the thing it
would report on exists. So `+0xD8` is the enable and `+0xB8` the status, which also makes the
LISR ordinary: it reads enable & status and acknowledges by writing the complement back to the
status register, rather than — as the old reading had it — writing a near-all-ones value into
the enable register on every interrupt.

**What a delivery has to look like**, read off the firmware rather than from the DVB spec,
because where they differ the firmware wins:

- Section RAM is **thirty-two 12 KB rings**, filter `f` at `0xA07A0000 + f*0x3000`, confirmed
  against the live records `{start, end, current, last-read, context}` at `0x80142D34 + f*20`.
- The LISR `0x800041B5` writes `0x4000|(f<<2)` to `+0x124`, **spins until bit 14 reads clear**,
  then takes a 21-bit byte offset from `+0x128` and stores it at `0x80107588 + f*4`.
- The task `0x80004521` walks from the record's last-read pointer to that write pointer,
  taking each length from bytes 1 and 2 at the cursor (`0x800044BD`, which also handles the
  two ring-wrap cases) — and then advances by **`section_length + 4`** where DVB's own total is
  `+ 3`. **The extra byte is the hardware's**: a section-filter engine of this era appends a
  status byte, and the firmware's stride is the only evidence of it we have.

**It works.** One TDT runs the LISR once, the section task once, the length reader twice, and
the firmware acknowledges by writing the complement back — leaving the status clean, which is
independent corroboration of the swap above. The task runs its **entire tail**, `0x80004700`
through `0x8000494A`, so the section is processed and handed on rather than merely lifted.

**Three tables provoke real work.** Measured against a two-sided control — a quiet window
before and after, counting only regions that beat both — `__siBroadcast()` adds about 260,000
instructions in a subsystem around `0x80069000`, plus the demux and a region at `0x8001E000`.
And **`0x80032D0C`, the function that maps a tuner state to a demodulator command, now gets
called**, having been zero for the whole of this machine's existence. It still returns early
because `[0x80106220]` is 0.

**Validate a table before feeding it, because a bad one is dropped in silence.** Each section's
declared length is checked against its real byte count and the MPEG CRC-32 is run over the
whole section *including its own CRC*, which comes out zero only if the checksum is right — a
property the builder cannot fake by agreeing with itself. The SDT is then decoded back by a
separately written parser.

**A PID and a filter index cannot share an argument.** The feeder first took "a PID if above
31, else a filter" and failed on the very first table: TDT is PID `0x0014`, which is 20, a real
filter and the wrong one.

**Two leads for whoever picks this up.** The filter record's `+0x10` is a per-filter CONTEXT
pointer, and it is distinct for exactly the three SI filters while 25..31 share one value — so
it is the natural way to find each table's own handler, and following it from filter 23 should
land on the SDT path without having to guess at `ASTRA_SDT_SVL`. (The addresses themselves are
heap and change per boot; read them off a live box rather than quoting them.) And `+0x140`,
`+0x144`, `+0x148` are the section-filter MATCH and MASK programming, written in a
select-value-commit rhythm — `+0x148 ← value`, `+0x144 ← 0xC000|index` — which is how the box
says which `table_id` each filter accepts. Neither is modelled; nothing has needed them yet,
because we choose which filter to deliver to rather than letting a match decide.

## The firmware narrates its own boot, and reading it is one printf

**This application was shipped with its diagnostics compiled in.** The format strings are
still in flash — `"Subtable: allocated %d bytes for %d nodes."`, `"[BGLOAD] Searching for
%s"`, `"#CONTROL[%s] app status : MAXIMIZED"` — and **373 call sites** reach them through a
single function at **`0x8007D06C`**. Its prologue spills `$a0..$a3` **and** `$t0..$t3`, which
is how a MIPS o32 varargs function takes its arguments before it has to touch the stack: the
format is `$a0` and the first seven arguments are `$a1, $a2, $a3, $t0, $t1, $t2, $t3` in that
order. It is the printf; the next most-referenced string-taking function is a name-registering
constructor with 17 sites.

**Where that output physically went on a real box is not established and does not matter,
because the CALL is the observable.** `__traceCalls([{pc, name, fmt}])` logs any registered
address with its live registers and `__debugPrintf()` registers that one and renders the
format. Nothing the firmware reads changes, which is the only kind of change to the page that
is safe without a boot behind it; it costs one Map lookup per instruction and only while
profiling, so the boot is untouched.

**The verbosity mask is a single word at `0x80105D14` and it reads zero on this box.** Set it
to all ones and the gated lines appear (`#CONTROL`'s application messages are bit 3; bits 4,
16 and 32 also appear at other sites). Only **5 of the 373 sites are gated at all** — the rest
are unconditional and simply never fire, so a quiet machine is a fact about the box's state
rather than about the switch. **The mask is BSS and the application clears it when it
relocates**, so setting it once before the boot sets nothing; re-poke it while the boot runs
or you get only the tail.

**Profiling is off during the boot by default, so the boot narrative needs both switches on
from the start.** With them on, a warm boot says this, and every line is the firmware's:

    Subtable: allocated 95360 bytes for 5960 nodes / 145056 for 3022 leaves /
              352 for 22 hardware filters / 124 for 31 channels / 264 for 22 filters
    [BGLOAD] Initialisation of the resident content manager.
    [BGLOAD] dump: bank0 = 0xbfc20000, bank1 = 0xbf820000,
                   block_size = 0x10000, block_number = 0x1e
    [BGLOAD] analizing bank 0.
    [BGLOAD] partition 'OB'(0x4f42) validated (app_id=4/prod_id=159/version=45)
    [BGLOAD] partition UNUSED (nb_block=6) .
    BIB-ONLSEC / HTTP-Corelib initialisation
    #CONTROL[init] init 2.      #APPMAN[RESET] booting.
    gdbo raw port not initialized.
    #APPMAN[RESET] waiting for synchro.  syncronization done.
    Cannot program connect to com_0.passive.
    #INTPRT[RESET] starting.    #INTPRT[READY] init.
    [BGLOAD] analizing bank 1.  partition UNUSED (nb_block=30) .
    [BGLOAD] Second runtime not here. Mirroring.  starting mirroring...
    [BGLOAD] Searching for IEPG.
    #CONTROL[running] unknown control error=0x19.
    #CONTROL[running] starting application=0x2.
    #CONTROL[running] user memory allocated (Kbytes)=0x180000.
    #CONTROL[running] app status : MAXIMIZED.
    #INTPRT[RUNNING] running:0x2.
    XSI: XSI Cache Status 80495288.

and then **nothing, for ever**. A cold boot differs by one line (`blacklist: created &opened
-> 0.`) and by nothing else.

## The box's whole interface is an OpenTV application, and it is already running

**This is the finding that redirects the work, and it retires the older open question of
whether the o-code would draw a guide from SI alone.** The menus and the guide are not C code
in this image waiting for data. They are an **o-code application**, and the firmware's
application manager has already started one.

**The application-id table is at `0x80036B60`** and maps an id to `{provider, name, flags,
number}`: **1 → OpenTV / STARTUP**, **2 → NDS XSG / IEPG**, **3 → OpenTV / GUIDE**, **4 → NDS
XGS**, 5 → NDS XSG again, default **BUILT-IN**. The box starts **application `0x2`, the
IEPG**, the interpreter reports it `RUNNING`, and `#CONTROL` reports it `MAXIMIZED`. So the
guide application is in the foreground, full screen, and drawing nothing.

**And it HAS drawn, once.** The OSD display list is fully programmed — root at `0xB0004200`,
one descriptor, **720×576 at 0,0, 4 bpp**, two field buffers 288 rows at a 360-byte stride,
and a CLUT — and on a cold boot the surface comes up **uniformly filled with a non-zero
index**, sampled 14,812 bytes of it with not one zero. That is the **blue screen**: the
application painted its background and stopped. On a warm boot the same surface decodes
**black**, which is a difference in the box's own state rather than in the model, and is worth
knowing before anyone reports "the screen went black" as a regression. Either way `__blitLog()`
holds **8 entries for the whole boot**, so nothing is being drawn ON the background — this is
not a drawing bug, it is an application with nothing to say.

**THIS PARAGRAPH USED TO SAY THERE IS NO EPG APPLICATION IN THE FLASH. THAT WAS WRONG, and
the section below — *The EPG is in the flash, and it is running* — is the correction.** What is
true and was measured: bank 0 holds one PARTITION, `'OB'`, app_id 4, `0x17F62C` bytes, and
`FLASH_U203.bin` is byte-identical to U202 but for the terminating header. What does not follow
is that the partition holds only a runtime. It holds the EPG too, as a separate module inside
itself, and "one partition" was read as "one thing". `[BGLOAD] Searching for IEPG.` is followed
by neither `Found %s in partition %d` nor `No autorun` — which says the BGLOAD download path
finds nothing, and says nothing about what is already resident.

**Both chips are now mapped, and that divergence is closed.** The emulator modelled only
U202 at `0xBFC00000`; `bank1 = 0xBF820000` was unmapped, so those reads answered **0, not
`0xFF`** — the firmware read thirty blocks of zeros, called the partition UNUSED and set about
mirroring 1.5 MB into a chip that was not there, which made its verdict on bank 1 a fact about
our address decoder. U203 now sits at `0xBF800000` (soft dependency, exactly as U202 is) and
the command-cycle state machine is per chip, because an unlock aimed at one must not arm the
other and autoselect on one must not answer device ids out of the other. **The firmware's own
verdict changed, which is the proof:** `partition 'OB'(0x4f42) validated` on bank 1, then
`[BGLOAD] No mirror needed.` where it used to say `Second runtime not here. Mirroring.` Zero
flash programs and zero erases either way, so nothing is written and the background download
task is now free rather than mirroring.

## What the SI we broadcast does, and what it does not

**It is received and parsed — proved by a name that could not have come from anywhere else.**
Broadcasting an SDT whose services are called `ZQXBBC One` and `ZQXBBC Two` puts those exact
bytes in DRAM at the SDT filter's own context pointer (`0x8019A2E4` on that boot, the value in
filter 23's record `+0x10`), 26 bytes in — precisely where the service name falls in the
section. Absent before the push, present after, and a second copy of the table overwrites the
same buffer rather than accumulating.

**And past that it changes nothing.** `__siCarousel()` repeats NIT, BAT, SDT and TDT at DVB's
own intervals, pushing each only to a filter the firmware has armed for its PID. Sixty-three
sections over a minute, none refused: **no debug line, no task-state change, no blit, no
pixel**, and `[0x80106220]` stays 0. Cold or warm, carousel on air before the boot or switched
on after it, the box ends in the same place. So the application is not waiting on standard
DVB SI in any way it will admit to, and the next thing to establish is what it IS waiting for
— which is now a question that can be asked of a machine that talks.

## The interpreter, instrumented — and what application 0x2 is actually waiting for

**It is waiting for messages on its own queue, it gets them, and it draws nothing.** That is
the answer, and it is not the answer the previous section's framing expected.

**How the blocking object was named.** The trace now records `$sp` alongside `$ra`, because a
trace of a shared RTOS primitive cannot say which task made the call — several tasks call the
same wrapper — while every Nucleus task has its own stack range in its control block at
`+0x24`/`+0x28`, so the stack pointer names the caller. With that, TASK30's suspension resolves
by name rather than by inference: status **5 (pipe)**, cleanup routine `0x800CF0A1`, and a
suspend structure on its own stack whose fields point at **`PIPE "EVQP0004"`** and
**`SEMA "EVQS0004"`** — the semaphore-and-pipe pair this firmware uses for a queue. The
interpreter is parked on the application's message queue.

**Reading the stack for return addresses would have got this wrong, and nearly did.** The
scan the project recommends — walk from the saved stack pointer to the stack end taking odd
words in the code range — produced a plausible twelve-deep chain for TASK30, and two of its
entries (`0x800508F0`, `0x80099244`'s neighbours) disassemble as nonsense because they are pool
words and data, not return addresses. A decoder run over data does not fail; it produces a
listing. Take the suspend structure instead, which the RTOS maintains on purpose.

**And it is fed.** Eight presses, spaced, on a settled box: **every one delivers two key events**
(press and release) at `0x8006EA04`, and every one wakes TASK30 — 4 to 175 scheduler activations
depending on the key, so the application is discriminating between them. Over the same ten
presses `__blitLog()` stays at **8** and the OSD surface hash stays at **`0x3b3dfdc5`**, byte for
byte, all 103,680 of them. The application receives its input, runs o-code for it, and has
nothing to put on the screen.

**Hash the surface, not VRAM.** Every earlier "the framebuffer never changes" here hashed
`window.__vram()` — the video decoder's memory behind the port at `0xB0002000` — while the
application's menus are drawn into DRAM surfaces the OSD display list points at. Those are
different memories and a claim about one says nothing about the other. Hashing the right one
also caught the zero-value trap on the way: the first version seeded its hash with 0, so an
all-zero surface hashed to 0 and could not be told from a read that returned nothing.

**Where the o-code dispatch is NOT.** The 51-entry table at `0x80081F54` is the only run of
consecutive code pointers in the image and every entry lands in `0x80083000`-`0x80086000`. It is
not the dispatch: registering all 51 with the tracer and pressing a key gives **zero hits**, with
a known-executed address as the control. Nor is the region: measured against an idle control in
alternating windows, the key-attributable work in `0x80080000`-`0x80095000` is about **1,700
instructions**, while the work a press provokes across the whole image is **millions** and lands
in `0x80099000`-`0x8009A000` and `0x80068000`-`0x8006C000`. The earlier note that a press runs
tens of thousands of instructions "in `0x80083000`-`0x80090000`" does not survive a two-sided
control and should not be built on. What a press does reach, through a thunk at `0x80081B40`, is
`0x80099244`, and through it a routine at `0x800992A0` that builds a 256-entry byte remap from a
four-entry two-bit selector — a pixel-format table, called about 26 times a press.

**A key press is dropped while the box is busy, and that is ours.** Mapping the second flash
chip gave the resident content manager real work: TASK20 validates the partition in bank 1 by
CRC-ing **1.5 MB** of it (the inner loop is at `0x800B1AF8`, table-driven CRC-32 with its table
at `0x80101A34`, reading `0xBF82xxxx` — bank 1). A real box does that in about a quarter of a
second; this emulator takes **twenty to twenty-five**, and while it runs the machine is
saturated and key frames on the CSI link are dropped. Measured both ways: three presses inside
that window produced **one** key event between them, eight presses outside it produced **two
each, every time**. Nothing is wrong with the firmware or with the input path — the emulator is
simply too slow for a background task the real hardware could afford.
**`./ctl.sh digibox` now waits for BGLOAD to stop being scheduled before it tests a key**, and
says so in its output, because a key check that fires as soon as 42 tasks exist was a coin toss
from the moment the second chip was mapped. A run that never settles reports that rather than
quietly testing a saturated box.

**What this leaves.** The application is alive, fed, MAXIMIZED, and empty. There are **no EPG
strings anywhere in DRAM** — fourteen candidates, from "Services" and "TV Guide" to "Favourites"
and "Box Office", all absent — which is what an application with no content looks like from the
outside. Together with `[BGLOAD] Searching for IEPG` never finding one and neither flash chip
holding an EPG partition, the reading is that **the resident application is a shell and the
screens themselves are broadcast**. That is not a small fix away.

## The signature check — what it is, and why it is not what is stopping us

**Two answers, and the second matters more than the first.**

### It is public-key verification, and a carousel we generate cannot be signed

The chain, read off the firmware from BGLOAD's own call site: `0x8003A124` (BGLOAD's wrapper)
→ `0x800E8ED0` (thin) → `0x800E40EC` (type map) → `0x800DCD58` (parse) → `0x800DCD90` (policy)
→ `0x800E46F0` (the verify). BGLOAD calls the first with
`(0, kind, buffer, len+40, sigPtr, 0)` where **kind is 1 for an application directory and 2 for
a runtime directory**, and treats a non-zero return as "signature checked".

**The signature blob is `[flags][algorithm id][?][length][length bytes]`.** `0x800DCD58` reads
those four bytes and hands the rest on.

**Three gates, all read off flash rather than guessed:**

1. **A per-kind allow-list** at `0x9FCC39C0` onwards — `[count][minimum length][ids…]`. Runtime
   directories accept ids `{0x10,0x11,0x12,0x13}` with a 12-byte floor; application directories
   accept `{0,1,8,9}` with an 8-byte floor; two more kinds accept `{2,3,0x0A,0x0B}` and
   `{4,5,6,7,0x0C..0x0F}`.
2. **A per-security-level permission bitmap** at `0x9FCC3414`, six bytes a row, indexed by the
   byte at `[0x80106AFF]`. Row 0 permits **all twenty** algorithms; row 1 permits four; rows 4
   and 6 permit one and two; the rest permit none in that range.
3. **A 48-byte descriptor per algorithm** at `0x9FCC4DE0`, twenty of them (the count `0x0014`
   is at `+0x0C` of the parameter struct at `0x9FCC49FC`, and the code refuses any id above it).
   Each carries an operand size at `+0x10` — **`0x20`, `0x30` or `0x40`, i.e. 256, 384 and
   512 bits** — a per-algorithm parameter block at `+0x14`, and a pointer at `+0x1C` to a
   record shared by each group of four whose leading word is **`0x00010001` = 65537**, the
   standard RSA public exponent, beside a pointer into a table of **primes**
   (31, 163, 313, 353, 397, 547, … all of them prime, verified).

**That is asymmetric verification with the public keys embedded in the flash, and it settles
the question the project needed settling: a carousel we generate cannot be signed.** Not
because it would be hard — because the private keys are not in the box and never were. Any
plan that depends on the box accepting a directory or module we produced through BGLOAD's
download path is closed by cryptography rather than by effort.

**What is NOT established, and should not be repeated as though it were:** which field of the
descriptor is the modulus. Of four parameter blocks sampled, only algorithm 0's has the shape a
modulus has (top bit set, odd); algorithm 8's is even-ended. So "RSA" here is the reading that
the exponent 65537 and the 256/384/512-bit operand sizes support, not a verified parameter set.
And **security level 0 permitting everything is measured on a box whose CA module never
initialises**, so it may be a default rather than what a real box runs at. Do not build on it.

### And none of it runs, which is the finding that matters

**Across a full boot and thirty-five seconds past it — the whole start-up sequence, through
`[BGLOAD] Ready to process messages.` — every one of those functions is called ZERO times.**
`0x8003A124`, `0x800DCD58`, `0x800E40EC`, `0x800E46F0`: nothing. Measured with the call tracer,
so it is the absence of a call rather than the absence of a log line.

**And the interpreter never asks for a code module.** `0x8008E7C8` is the call it makes to
BGLOAD to obtain one — the call whose result it decodes into `#INTPRT security: failed` and
`#INTPRT autoload: no code module` — and it is called **zero** times too.

**The module handle is NULL.** `[0x80106520]` reads `0x00000000` while `[0x80106508]` reads 3
(RUNNING) and `[0x80106524]` holds app id 2 in its high halfword. So the interpreter reports
itself running application 2 **with no code module at all**. That is the same fact the string
search found from the other side — no EPG text anywhere in DRAM — and it explains the blue
screen exactly: there is nothing to run, so nothing is drawn.

**So the signature is not the obstacle today.** The obstacle is that no module ever reaches the
interpreter, by any path, signed or not. Three routes follow, and the first is the one the
evidence points at:

- **Find the path that delivers a module without BGLOAD.** `#CONTROL` distinguishes
  `Starting app in flash` from the plain `starting application=0x%x`, and our box prints only
  the latter — so the app manager has a non-flash case, and it started application 2 through it
  without any verification. What that case reads, and whether it can be pointed at content we
  supply, is the open question.
- **Answer the verify in the model.** The emulator is ours and the platform is dead; making the
  verifier return success is a two-line change. It is also a deliberate decision to bypass a
  code-signing check rather than an implementation detail, so it belongs in the record as a
  decision somebody took, not in a commit that quietly makes a green light appear.
- **Accept that the resident box cannot run a guide**, and drive the UI another way.

## The registry load path — real, drivable, and NOT the one this box uses

**Everything in this section is about the autoload path, app id −1, which this box never takes.
It is worth keeping because it can be driven and it proved the loader has no cryptographic
gate — but it is not why the guide does or does not appear.** The box's own path is app id 2,
which gets its module from a fixed descriptor in the flash; see *The EPG is in the flash*.

**Application 2 was started as a NAME, not as code.** That is the answer, and it closes the
question the signature work left open.

**What `#CONTROL` actually does when it starts an application.** The routine is `0x80036D0C`,
called from the interpreter (`0x80051466`, `0x80051654`, `0x80051746`, `0x8005177C`) as
`f(id, descriptorBuffer, …)`. It switches on the id:

- **ids −40 … −10** take the `Starting app in flash` arm. Our box never prints that line, so it
  never takes it.
- **id 2** → `0x80036EC4`, **id 3** → `0x80036E3C`, ids 0/1 → return 0, id ≥ 4 → other arms.

Both the id-2 and id-3 arms do the same load-bearing thing: call **`0x80036B54`**, which fills a
four-word descriptor with `{provider, name, flags, number}` from a static table —
**1 → OpenTV/STARTUP/22, 2 → NDS XSG/IEPG/16, 3 → OpenTV/GUIDE/21, 4 → NDS XGS**, default
BUILT-IN. Traced live: `appDescriptor(0x00000002, 0x801D6A90, 0)` from `ra=0x80036ECF`, on
TASK30's own stack. **That is the whole of it.** The app manager names an application. It does
not fetch one.

**Where the code would come from, and why it never does.** The interpreter asks for a module
through `0x8008E7C8` → `0x8008D0F4` → `0x8008C8A8(appId)` → **`0x8007BB3C`**, a **256-bucket
hash registry** at `[0x80106F54]`, keyed `(id − 1) mod 256` with the chain at `+4` of the
bucket. A record carries **a four-character SOURCE TAG at `+8`**; the code tests for
**`"CRSL"`** — the carousel — and redirects those through `0x800511D0`, while a record with any
other tag is returned as it stands. So a non-carousel source is possible in principle; the
registry is simply the seam through which any source is declared.

**The registry is empty, and this is controlled rather than asserted.** The table is real heap —
a `0x408`-byte block at `0x80314C14`, which is exactly 256 words plus a header — and of the 256
bucket words **only index 0 is non-zero, holding `0x00010000`, which is not a pointer**.
`"CRSL"` occurs **exactly once in the entire machine**, as the comparison literal at
`0x8008CB8C`. And nothing ever registers anything: traced over 25 seconds, the insert
(`0x8007BCC0`) is called **zero** times, `0x8008C8A8` **zero**, `0x8008E7C8` **zero**, and the
lookup **once**, with id 0, from an unrelated site.

**So the chain is complete and every link is measured.** Nothing registers an application →
the registry is empty → the module lookup has nothing to find → the interpreter never asks →
the module handle stays NULL → the interpreter reports RUNNING with no code → no o-code
executes → nothing is drawn → and no signature is ever checked, because there is nothing to
check. Every earlier observation falls out of this one fact.

**The seam, for whoever picks it up.** `0x8007BCC0(id, record)` hashes `(id − 1) mod 256` and
links the record into its bucket — that is the entire insert. The record's shape, from its
consumers: `+0` chain pointer, `+4` id, `+8` source tag, `+0x2C` a field `0x8008E7C8` reads
before calling `0x8008DB28`. **If a record can be registered with a source that is not `"CRSL"`,
the module comes from wherever that record points — no carousel, and no signature, because the
signature lives in BGLOAD's download path, which this bypasses entirely.** That is a different
proposition from forging a signature and it is the one worth testing next.

**A method note, because it cost time.** Driving `0x80036D0C` directly with `__call` to ask the
box to start the GUIDE looked like a shortcut and answered nothing: the id-2 arm writes its
descriptor to **`$s1`**, which `__call` does not set, so the descriptor went somewhere
unrelated, the buffer stayed zero and nothing printed. A firmware routine called outside its
register context is not the routine. Trace it where it runs instead.

## Registering an application record — the mechanism, driven and proved

**The interpreter has an explicit autoload path and it can be driven.** `0x8005141C` takes no
register arguments: its entire input is the module handle at `[0x80106520]`, a packed
`{appId << 16 | moduleId}`. Set that, call it, and the firmware walks its own chain —
`preCheck 0x80050770` → `0x8008E5EC` → `moduleLookup 0x8008D0F4` → `appById 0x8008C8A8` →
`registryLookup 0x8007BB3C` → `recordAccept 0x8008CDA4` — and reports the outcome on its own
debug channel. **That is how the interpreter is meant to be given an application**, and the
reason it never happens on this box is that `#CONTROL` only ever asks for NAMED applications
(id 2), which take the plain start arm; **id −1 is the autoload arm**, and nothing asks for it.

**The record's fields, each one established by driving the firmware rather than by reading:**

| offset | meaning | how it was learned |
|---|---|---|
| `+0x00` | chain pointer to the next record in the bucket | the walk's `lw $s0, 0($s0)` |
| `+0x04` | **id, a HALFWORD** | written as a word first, so `lhu` read the zero half and the record was never found |
| `+0x08` | four-character source tag | **must BE `"CRSL"`** — see below |
| `+0x1C` | flags — accepted iff `(f & 5) != 0` and `(f & 0x180) == 0` | `0x8008CDA4` |
| `+0x2C` | **a module table**; `0x8008DC2C` returns its length, and the requested module id must be less than that | `0x8008D11C` onwards |

**The `"CRSL"` test is the opposite way round from the obvious reading, and only driving it
showed that.** `0x8008C8A8` branches PAST the redirect when the tag matches, so **`"CRSL"`
means "use this record as it stands"** and any OTHER tag is sent through `0x800511D0` to be
re-resolved — which returned 0 for our id, so a record tagged `"MINE"` was found and then
thrown away. Read statically it looks exactly like the reverse.

**Where it stands: the application record is solved and the module table is not.** With the id
as a halfword and the tag as `"CRSL"`, a record poked straight into the registry bucket
(`table + ((id−1) mod 256)*4 + 4`, no firmware call needed — the insert is only a link) is
**found by the lookup and passed to the acceptor**. The chain then stops on `+0x2C`: our record
has none, `0x8008DC2C` reports a length of zero, and module id 1 is not less than zero, so the
box says `#INTPRT[RUNNING] autoload: no code module.` — which is the correct answer to the
record we gave it.

**Note what this means for the signature.** Nothing in that chain verified anything. The
registry is reached, the record is accepted, and the failure is a length comparison. The
signature lives in BGLOAD's *download* path; this path does not touch it.

### The instrument was blind for the first two attempts, and that is the lesson

`__call` runs `runCall`, which has **its own execution loop carrying none of `burst()`'s
profiling hook**. So `__pcHits` and `__traceCalls` saw nothing a `__call` did — the firmware's
own printf included. The first two runs of this experiment returned `v0 = 0` with an **empty
trace and no debug output**, and "the function refused" was indistinguishable from "I was not
watching". Both readings were wrong: it had run the whole way and printed.

The hook is now in that loop too (breakpoints deliberately excluded — stalling inside a
synchronous call leaves the machine mid-function with no way to resume). **Before drawing any
conclusion from a quiet instrument, check that the instrument covers the path being measured.**

## The module table, built — and the load path driven to its last gate

**The application load path now runs end to end on demand, and the firmware's last word on it
is `#INTPRT[RUNNING] security: failed.` rather than `no code module`.** Getting there mapped
the whole structure, and every offset below was read from Ghidra's decompilation and then
confirmed by the machine executing it.

**The module table, which a registry record points at from its `+0x2C`:**

| offset | meaning | read by |
|---|---|---|
| `+0x00` | 16-bit offset to sub-structure **A**, self-relative (`t + [t+0]`) | `0x8008DB88` |
| `+0x02` | 16-bit offset to sub-structure **B** (`t + [t+2]`) | `0x8008DB94` |
| `+0x04` | big-endian dword | `0x8008DBA0` |
| `+0x08` | big-endian dword — a limit, intersected by `min` with A's | `0x8008DBBC` |
| `+0x0C` | big-endian dword — a permission mask, intersected by `&` with A's | `0x8008DBD8` |
| `+0x10` | big-endian dword — a limit, intersected by `min` | `0x8008DBF4` |
| `+0x18` | **module count** | `0x8008DC2C` |
| `+0x28 + (id−1)*0x10` | module entry, 16 bytes | `0x8008DB28` |

**Every field is byte-wise big-endian and unaligned, and the sub-structures are reached by
16-bit SELF-RELATIVE offsets** — which is what a directory parsed out of broadcast sections
looks like, and is corroboration that this is the carousel's own format rather than an
internal one.

**The index bias is a trap worth naming.** `0x8008DB28` computes `(moduleId + 0x0FFFFFFF) * 0x10`,
which reads as an enormous offset and is not: `0x0FFFFFFF * 0x10` is `-16` modulo 2³², so the
expression is `(moduleId − 1) * 0x10`. Read as written it sends you looking for a table 256 MB
away.

**The gates, in the order the firmware applies them:**

1. `0x8008D0F4` requires `count >= moduleId` — `sltu` then `btnez` to the failure arm, so
   `count < moduleId` fails and equality passes.
2. `preCheck 0x80050770` asks whether the module is ALREADY LOADED: it walks five-word entries
   from `[0x801064F8] + 8` while `e[2] != 0`, matching `e[0]` against
   **`appId << 16 | moduleId`** (built by `0x8008D0D8`, whose app half is `lhu [record+4]` —
   the record's own id). **`[0x801064F8] is NULL on this box**, because no module has ever been
   loaded, so the walk cannot even start and the answer is 3.
3. **`e[4]` is the status the caller reads, and at this site ANY non-zero is a failure** —
   `0x8005142C` is `bnez v0` to the error arm. A planted `0x0F` was found correctly and read as
   "error 15", which is why the first run with a valid entry still said `no code module`.
4. With `e[4] = 0`, the path proceeds: `getCodeModule 0x8008E7C8` reads the module entry, and
   then **`0x8008ED54` is the security gate**.

**And the security gate is a PERMISSION INTERSECTION, not a signature.** `0x8008ED54(appId)`
re-finds the record, takes the module table, and computes: `min` of the table's `+0x08` against
A's, a bitwise `&` of the table's `+0x0C` against A's, `min` of `+0x10` against A's, plus three
more fields — then hands the six-word result to `0x8007AFA4`. With a table of zeros every
minimum is zero and the mask is empty, so the capability set is empty and it returns 0, which
the interpreter reports as `security: failed`. **There is no cryptography anywhere in this
path.** The signature described earlier lives in BGLOAD's download path, and this does not
touch it.

**So what an application must supply is now fully named:** a registry record (id halfword, tag
`"CRSL"`, flags, `+0x2C`), a module table with a count and at least one entry, an entry in the
loaded-module list with a zero status, and the permission and limit fields the security gate
intersects — and, of course, actual o-code, which nothing on this box has.

### What Ghidra changed, measured

The registry side of this (sky-eluc.6) took hours of hand-disassembly. This side — a deeper and
wider structure — took one session, and three specific things came back in a single command
each: the module count being an **unaligned big-endian dword at `+0x18`**; the `(id−1)*0x10`
bias hiding inside a `+0x0FFFFFFF`; and the whole of `0x8008ED54`'s permission computation,
which is thirty lines of C and would have been a very long hand-read.

**It does not replace the machine, and the reason is on the record.** Ghidra loses MIPS16 return
sequences — `lw $a0,n($sp); jr $a0` reads to it as an unrecoverable jumptable, so return values
are routinely missing from the decompilation, and both `0x8008D0F4` and `0x8008E7C8` came back
with their results dropped. Worse, the two facts that were *backwards* this session — the
`"CRSL"` branch that skips on match, and a record id that is a halfword among words — were both
plausible on the page and only corrected by running them. **Read here to find the shape fast;
prove the shape by driving it.**

## The permission gate, passed — the box will start an application we supply

**`#CONTROL[running] starting application=0xffffffff.` followed by
`user memory allocated (Kbytes)=0x180000.`** That is the firmware starting an application we
declared, through its own load path, having accepted our names and our permissions, with **no
cryptographic check anywhere in it**.

**The capability set is seven words, and two of them are STRINGS.** `0x8008ED54` builds it on
its stack and hands `&set` to `0x8007AFA4`:

| word | value | from |
|---|---|---|
| `[0]` | **a name** — `A + 0x14` | `0x8008DCD4` |
| `[1]` | **a name** — `t + [t+2]`, i.e. sub-structure B | `0x8008DB94` |
| `[2]` | `A[+4]` — must satisfy `0x8007B170` | `0x8008DC64` |
| `[3]` | `t[+4]` | `0x8008DBA0` |
| `[4]` | `min(A[+8], t[+8])` | a limit, intersected |
| `[5]` | `t[+0xC] & A[+0xC]` | a permission mask, intersected |
| `[6]` | `min(t[+0x10], A[+0x10])` | a limit, intersected |

**That `[0]` and `[1]` are names is not an inference from their shape — it is what the code does
with them.** `0x8007AFA4` passes both to **`0x8007E2C4`**, which is the same string setter this
image uses for `"resident"` and the modem's APN. Driven, the trace reads
`setName(0x801683D2, 0x803F1054)` and `setName(0x801683E2, 0x803F1070)` — our `"GUIDE"` at
`A+0x14` and our `"OpenTV"` at B. **The module table carries the application's provider and
name, and they are the same pair the app-id table hardcodes for the resident ids.**

**Sub-structure A mirrors the table's own shape**: dwords at `+0x04`, `+0x08`, `+0x0C`, `+0x10`,
then a payload at `+0x14`. Both A and B are reached by 16-bit **self-relative** offsets from the
table's `+0x00` and `+0x02`, so the whole thing relocates as one blob — which is what a
broadcast module directory has to do.

**What satisfied the gate**, and each value is there for a reason rather than to be non-zero:
`A[+4] = 2`, because `0x8007B170` does nothing for `0` and for `1` specifically; the table's
limits set to `0xFFFFFFFF` so every `min` picks A's real value and every `&` picks A's real
mask; A's own limits set to something finite; and the two name strings in place. The result was
`useCapabilities` → `f_8007B170(2)` → both `setName`s → **`appStart(0xFFFFFFFF, 0)`**, which is
the autoload arm's id −1.

**It stops there, and the reason is the one that has been true all along.** The application is
started and 1.5 MB of user memory is allocated, and then nothing: no `MAXIMIZED`, no
`#INTPRT running`, no blit, surface unchanged. **There is no o-code.** The module entry at
`t + 0x28` and the three words `preCheck` copies out of the loaded-module list
(`{e[1], e[2], e[3]}`, planted here as a pointer, a size and a pointer) are where bytes would
have to be, and everything we put there was zeros.

**So the load path is open and the supply problem is the whole of what remains.** Every gate
from the application registry to the permission intersection has been driven and satisfied by
hand, and not one of them asked for a signature. What the box does not have, and what no amount
of structure will conjure, is an application's compiled o-code.

## The EPG is in the flash, and it is running — the correction that matters

**The guide is resident, it is loaded, and the interpreter is executing its bytecode right
now.** Several conclusions earlier in this document said otherwise and were wrong; this section
is what replaced them, and the evidence is below rather than asserted.

**The flash names its own modules.** SCCS `@(#)` banners give the directory:

    0x24408  CA API Glue: 0x0341 Jul 02 2002      0xC1EC0  POPUP: 3.0.11 Jun 20 2002
    0x2456C  MDM GLUE: 1.2                        0xC58A0  ICAM: Mar 06 2002
    0xB3A24  CA API Glue: 0x0341 in EPG           0xC594C  Verifier: 3.11.11 (5)
    0xBB430  EPG: 3.0.11* Jun 20 2002 16:09:56    0xCBAD8  OpenTV 1.2S4Bj

**And a second, independent artefact confirms they are standard.** The 2003 over-the-air
firmware `20030114_PACE_9F03_3E` — a different file, a different build, not derived from our
dump — carries the same module set one version on: `EPG: 3.1a.5`, `POPUP: 3.1a.5`, ICAM,
Verifier, CA API Glue, MDM GLUE, `OpenTV 1.2S4Bq`. The EPG is a firmware component, not
something our box happened to download. (Our dump additionally carries text the OTA does not —
`CHOOSE TIME SCREEN`, `PROGRAMME TIME CLASH`, `IPPV PURCHASE` — so the two are not equal.)

**The module container is a FourCC chunk chain**, `tag` then a big-endian length that INCLUDES
the eight-byte header, and each module is followed by a `{start, length}` descriptor:

| chunk | EPG | POPUP |
|---|---|---|
| `INFO` | `0x04A3F0`, 16 | `0x0BC754`, 16 |
| `CODE` | `0x04A400`, **353,188** | `0x0BC764`, 22,240 |
| `DATA` | `0x0A07A4`, 113,532 | `0x0C1E44`, 4,464 |
| `SWAP` | `0x0BC320`, 1,048 | `0x0C2FB4`, 92 |
| `GDBO` | `0x0BC738`, 12 → `"sky"` | `0x0C3010`, 16 |
| `LAST` | `0x0BC744`, 8 | — |

The accounting closes: `0x4A3F0 + 0x7235C = 0xBC74C`, and the word AT `0xBC74C` is
`0x9FC4A3F0` — the module's own start — with its length beside it. `INFO` carries two sizes
(EPG `0x1A748` and `0x2000`; POPUP `0x1C44` and `0x1000`). **The 764-entry resource manifest at
`0xA07AC`, already decoded by `bitmap.html`, is eight bytes into the EPG's `DATA` chunk** — so
the bitmaps, fonts and 537 UI strings that page renders are the EPG's own resources.

**The app manager hands the module out by id.** `0x80036D0C`'s id-2 arm ends by loading the
word at `0x9FCBC74C` and returning it, so **`appStart(2)` returns `0x9FC4A3F0`, the EPG**; id 4
returns `0x9FCBC754`, the POPUP. (That return value was sitting in an earlier experiment's
output, unrecognised.) The interpreter's caller at `0x8005177C` saves it, branches to failure
only if it is zero, and proceeds.

**And the interpreter is executing it.** TASK30's stack — bounds read from its control block,
not remembered — holds **45 words pointing into the EPG's `CODE` chunk**, and they MOVE: 17
changed across an idle window and 19 more after one key press. A static leftover does not
change; an o-code call stack does. Pointers into the `DATA` chunk: zero, which is what makes
the code hits a signal rather than a scatter.

**So why did this take so long to see, and what were the errors?** Three, and each is the same
shape — a true measurement read as a wider claim than it supported.

- **"No EPG strings in DRAM"** was true and misleading. The strings live in the flash `DATA`
  chunk and are read in place, exactly as every other flash string in this image is. Searching
  DRAM for them could only ever have found nothing.
- **"Nothing registers an application; the registry is empty"** was true and irrelevant. That
  registry serves the AUTOLOAD path, app id −1. This box uses id 2, which never touches it.
- **"One partition, therefore one thing"** was the root error. The partition table was read
  correctly and then over-read: a partition holding a runtime can also hold an application.

The useful residue is real: the loader can be driven end to end, and nothing in it checks a
signature. But the guide did not need loading. **It was running the whole time, and the
question is the one this document started with — what it is waiting for.**

## The compositing gap, chased and closed — the drawing path is healthy and unused

**The emulator's graphics are not the problem.** The box paints its screen with the blitter,
correctly, and `__blitLog()` carries the whole of it with parameters:

    fill cell16 0x80575798 @0,0   360x288 pitch 360 value 0x00001010
    fill cell16 0x805A81A8 @0,0   360x144 pitch 360 value 0x00008080   (both twice)
    fill 8bpp  0x80584048 @0,0    720x144 pitch 720 fields value 0x000000DC
    fill 8bpp  0x80584048 @0,144  720x144 …
    fill 8bpp  0x80584048 @0,288  720x144 …
    fill 8bpp  0x80584048 @0,432  720x144 …

Four strips of 144 rows covering all 576, into the field buffer the display list points at.
**That is the blue screen, and it is a real render through the real path.** The blitter, the
queue, TASK6 and the display list all work.

**And on a key press none of it moves.** TASK6 — the only drainer of the `BlitReq` queue, whose
control block sits at `0x80184250` — is scheduled **zero** times, so nothing is queued. The
visible field buffer takes **zero** writes across three key presses. The application runs three
to four thousand bytecode operations and asks for nothing to be drawn.

**The write watch that proves it needed fixing first, and the fix is the lesson.** The
single-address watch is keyed on the address as written, and DRAM is reachable through BOTH
aliases — `0x80000000` cached and `0xA0000000` uncached. The firmware quotes its field buffers
in the **uncached** alias, so a watch set on the cached address sees nothing and reports
silence. An earlier run of this measurement did exactly that and returned zero **with its
control also reading zero** — the one result that cannot be interpreted at all, and it was
nearly reported as a finding. `__writeWatch(lo, hi)` now compares PHYSICAL offsets and catches
either alias, and every use of it since has carried a control that fires.

**So the elimination is clean: the pipeline is healthy, the application declines to use it.**
Nothing further will be learned by instrumenting the emulator's graphics. What remains is the
branch the o-code takes on a key press, and reading that needs a disassembler for this runtime's
bytecode — built from the VM at `0x80069298`, which is authoritative for OpenTV 1.2, rather
than from the 3.2 SDK's `ocodedef.h`, which is indicative at best.

## The o-code, disassembled — and the blue screen is the EPG's own work

**The bytecode now reads.** `scripts/ocode-disasm.py` disassembles the runtime the EPG is
written in, and nothing in it is guessed: the operand table is MEASURED off the running machine
by `scripts/digibox-probes/bytecode-trace.js`. The interpreter's main fetch site `0x80069298`
reads opcode bytes and every other site reads operands, so the bytes consumed between two
main-site reads are that opcode's operands, and a sample is only trusted when the next opcode
lands at `op + 1 + operands`. One key press yielded **45 opcodes with zero disagreements** —
every opcode seen was seen with exactly one operand length.

**The control-flow forms were each fitted to EVERY observed occurrence**, and a form that fitted
some and not all was rejected:

| Opcodes | Form |
|---|---|
| `0x15` | call, 4-byte signed offset from the next opcode |
| `0xBB` | call, 1-byte offset |
| `0x17`, `0x41` | unconditional jump, 1-byte offset |
| `0x42` | unconditional jump, 2-byte offset |
| `0x29`, `0x32`, `0x33`, `0x34`, `0x36` | conditional branch, 1 byte — **and the interpreter does not read the operand when it does not branch**, which is why these looked ambiguous |
| `0xB4`, `0xB5`, `0xB6`, `0xB8` | return, four variants |

The returns are the strongest evidence in the set: **their targets are exactly the return
addresses of the calls that preceded them**, which no coincidence produces.

**THE VALIDATION WAS VACUOUS ON ITS FIRST WRITING, and that is why `check()` is shaped as it
is.** It required every decoded boundary to be either fetched or reachable from a decoded
instruction — and stayed GREEN when an operand length was deliberately corrupted, because
`reachable` was computed from the drifted walk itself and each bad boundary whitewashed the
next. Almost every opcode falls through, so almost every address was reachable and the check
could not fail. **The sound test runs the other way**: a wrong operand length makes the walk
STEP OVER an address the machine really fetched, and the trace is independent evidence that no
self-consistency can hide. Falsified three ways (`0x9f` 1→2, `0xa2` 0→1, `0x6b` 0→1) and each
goes red. Across the whole trace: **359 addresses fetched in 36 runs, 358 boundaries decoded, 0
stepped over.**

The EPG's key handler at `0x9FC4A538` then reads as what it is — a dispatcher comparing the key
against `0x0900`, `0x0100`, `0x0080`, `0x20` and `0x07` before calling.

### The native API — 800 functions, and how to name a call

**`0xC8`–`0xCE` are the call-a-native family** (seven adjacent handlers at `0x80069F1C`…
`0x8006A24E`). `0xCA` carries two operand bytes, *module* and *function*; `0xC8` carries one.
The handler at `0x8006A1FC` saves the VM state into the frame at `[$sp+52]` and calls the
dispatcher held at `0x8006A2E8` → `0x8006E6A8`, which bounds the module against **64** and
indexes a table of `{function array, count}` pairs at the word in `0x8006E71C`.

**That table is built at boot in DRAM beyond the static image (`0x80120050`), so it can only be
read from a live box.** Twelve modules are populated and they hold **800 native functions**;
module 1 alone has 236. Each array entry is an 8-byte `{implementation, argument descriptor}`
pair in flash, and the implementation goes through a 16-byte shim before reaching the real
function — so resolving `(1, 0xE4)` runs `0x9FC29F04[0xE4]` → `0x9FC2A9D8` → `0x80081C58` →
`[0x80081F60]` → `0x800837A0`.

**Reading a native call's operand needs `[$sp+44]`.** The VM keeps its bytecode pointer there,
and it dispatches to a handler with `jr` rather than a call, so the handler shares the fetch
loop's frame: at a handler's first instruction `[$sp+44]` is the address of that instruction's
operands. `__traceCalls` takes `deref: {off, bytes}` for exactly this, because **the operand
never reaches a register a trace can see**. It is checked against the disassembly — the capture
reads `ca 01 26 9f ee` at the address where `ocode-disasm.py` prints `ca 01 26` followed by
`9f ee`.

### The blue screen is the EPG's, and it is the only thing the EPG ever draws

Every blit carries an `icount` and so does every native call, so each blit can be attributed to
the call that preceded it. The four 720×144 fills into `0x80584048` — the field buffer the
display list points at — follow **`ca 01 e4`, native (module 1, function 0xE4)**, at 1,965
instructions. The two `cell16` fills before them precede the first native call in the boot and
are not the application's.

**So the application CAN draw, DOES draw, and draws into the right buffer.** That retires the
last version of the compositing question. What it does instead is the finding:

- Across a whole boot the EPG makes **361 native calls and exactly ONE of them draws.** It opens
  a window, paints the background, and has nothing to put in it.
- `(1, 0x26)` is the **event wait**: 1,018 CPU instructions when an event is already queued,
  **414,171** when it blocks. That single call is 90.7% of the CPU a key press costs.
- A key press produces **six** native calls and no drawing: `c8 08` ×2, `(1,0x26)` ×2,
  `(1,0x36)`, `(1,0x56)`, `(1,0x43)`.
- Its steady state is `c8 08 0a` ×2 → `(1,0xbc)` → `(1,0x26)`, once every ~22M instructions.

**And it is not blocked on a stream it asked for.** On a booted box filters 22, 23 and 24 carry
PIDs `0x14`, `0x11` and `0x10` — the TDT, SDT and NIT the firmware's own SI layer requested.
Filters 25–31 are armed and carry contexts but **have no PID programmed at all**, and filters
29, 30 and 31 share one context (`0x80172F73`). Nothing in the box has ever asked for an OpenTV
carousel PID, so delivering one would be delivering it to no filter.

## The whole Sky interface is in the flash, and the EPG is waiting for a channel list

**Every menu the product wants is already in the box.** The EPG module's DATA chunk
(`0x9FCA07A4`–`0x9FCBC320`) holds its entire user interface as plain text:

    TV Listings · Other Channels · ALL PROGRAMMES A-Z · MOVIES A-Z · SPORTS & EVENTS
    NEWS & DOCUMENTARIES A-Z · KIDS A-Z · ENTERTAINMENT A-Z · MUSIC & RADIO A-Z
    PARENTAL CONTROL · CHANGE PIN · SET NEW PIN · INCORRECT PIN · Series Link
    SKY+ COPY · SKY+ PLAYBACK · CUSTOMER SERVICES · SUBSCRIPTION UPGRADE · Start Time

1,786 printable runs in all, 776 of them English. Nothing needs to be drawn by us.

**And the application carries its own explanation for why none of it appears.** Among those
strings are its diagnostics, and they name one thing:

    SVL is not ready after O_SKY_get_event_handle
    get_event_info: event handle not ready after O_SKY_event_get_info
    # TLML: SVL is not ready after O_SKY_service_get_info
    # TLML: SVL entry was not found

**SVL is the service list.** The platform API it is built against appears in the same chunk —
`O_SKY_service_get_info`, `O_SKY_get_event_handle`, `O_SKY_event_get_by_time`, `O_svl_search`,
`SERV_svl_search`, and the DVB object status codes `O_DVB_OBJECT_NOT_FOUND`,
`O_DVB_OBJECT_NON_AVAILABLE`, `O_DVB_ACTUAL_NIT_CHANGED`, `O_DVB_ACTUAL_SDT_CHANGED`. The
runtime holds three service-list builders by name — **`ASTRA_SDT_SVL`, `OTV_NIT_SVL` and
`SKY_BAT_SVL`** — and immediately before the last of them sits the message **`OTV: Failed
matching descriptor tag`**. The build itself is named too: `/nfs/release/Cougar/124Bj/otv`.

**The list is read from NVRAM, not from the stream directly.** The registry paths are in the
same chunk: `/eeprom/svl`, `/eeprom/svl/ochn`, `/eeprom/otherts`, `/eeprom/tsl`,
`/eeprom/favchn`, `/eeprom/pin`, `/eeprom/book`, `/eeprom/bid_sbid`, `/eeprom/teleepr`. So the
chain is **SI → the runtime's SVL builder → `/eeprom/svl` → the EPG**. Our NVRAM is virgin:
**486 of 16,384 bytes are non-blank**, and while idle the box reads `/eeprom/otherts` 62 times
and the string `demodulator` 262 times — a box trying to acquire a line-up rather than one
using one.

**It is waiting, not working slowly.** Watched for three minutes with both channels
instrumented: the blit count stays at **8** from the 30-second mark to the end while native
calls climb to **638**. Forty different remote keys, every one the handset can send, produce
**zero blits** — though not zero response: `0x1B` and `0x83` call `(1,0xBA)`, `0x03` calls
`(1,0xBD)`, `0x39` a different set again, and eleven keys never reach the application at all.

### The instrument that found it, and why it had to be built

The application never prints any of this. Its messages exist as strings and nothing routes them
to the firmware printf — three minutes of `__debugPrintf` captured **ten** lines, all of them
BGLOAD's. So the EPG was read a different way: **a string that is about to be used is first
READ, and reads are watchable.** Watching the resource region and mapping every read back to
the string it lands in gives a trace of what the application is doing in its own vocabulary,
with no cooperation from it at all.

**Two zeros had to be disbelieved before it worked, and both were the instrument.** Watching
the DATA chunk in FLASH returned zero reads for eight windows — and that one is real, confirmed
against a positive control on the CODE chunk in the same window on the same key, which fired
2,155 reads to the DATA chunk's 0. The application works from a RAM copy. Finding that copy
then failed too, because `__find` was given `0x80000000`–`0x80400000` while **this box has 32MB
of DRAM and the resources are at `0x80466000`–`0x80478000`** — a search that examined an eighth
of the address space and reported on all of it. Neither zero looked thin.

## What actually happens to a section we broadcast — it is parsed to its header and dropped

**"The box parses our tables" was too generous, and this is the measurement that corrects it.**
The delivery path is sound: the demux accepts the section, the LISR fires, the section task
runs, and the firmware copies the section whole out of the ring into a fixed parse buffer at
`0x80199AD4` — the destination read off the copy loop's own registers at `0x800FA958` rather
than searched for, after a DRAM search found only the *previous* section's block.

Then the SI layer parses it with a bitstream reader (`get_bits` at `0x800A92F1`, whose byte
fetches at `0x800A92D8`/`0x800A9306`/`0x800A937E` are what makes this measurable). The NIT
parser at `0x800AF866` pulls the header out field by field —

    get_bits(8) table_id · skip(4) · get_bits(12) section_length · get_bits(16) network_id
    skip(2) · get_bits(5) version · get_bits(1) current_next · get_bits(8) section_number
    get_bits(8) last_section_number

— and then does this:

    800af928  lw    $v1, [0x800afc68]      ; find_subtable, 0x800AB1B0
    800af92a  jalr  $v1                    ;   lhu $a0, 4(header) = network_id;  $a1 = type 4
    800af92c  move  $a0, $s0
    800af92e  move  $s0, $v0
    800af930  beqz  $s0, 0x800afa9c        ; NULL -> abandon the section

**It returns NULL, so the descriptor loop is never entered.** Measured: a pushed NIT produces
exactly **9** bit-fetches over a 7-byte spread and stops; an SDT produces 11 and stops. A NIT
carrying one descriptor of every private tag `0x80`–`0xFF` had **not one descriptor body read**
— controlled against an empty window, which reads zero, and against the code chunk, which
reads 2,155.

**And it is not the network id.** Sweeping `network_id` across 0, 1, 2 … 0x10, 0x20, 0x40,
0x60, 0xA0, 0xFF, 0xFFF, 0x1000, 0x2000 gives a **flat 9 fetches for every one of them**. Flat
across every id is a scan and cannot be a match — the same forensic move that settled the tape
id question. **Nothing in the box is subscribed to the NIT for any network**, which is what the
boot log meant by `Subtable: allocated 32 bytes for 2 clients`.

**So more SI, or better SI, changes nothing on its own.** A subscriber has to exist first, and
what would create one is a service acquisition — which is also what the box looks like it is
trying to start, reading `demodulator` 262 times and `/eeprom/otherts` 62 times while idle.
That is the next thing to find: **what registers an SI client, and what has to happen before it
does.**

**The first version of the descriptor experiment learned nothing and looked like it had.** It
watched the section ring, where the only reader is the memcpy — 1,307 reads from one PC — so
every tag scored identically and the answer was "all of them", which is the same shape as "none
of them". A parser reads specific offsets from several PCs; a copy reads everything from one.
**When every subject in a sweep scores the same, the instrument is measuring something other
than the thing that discriminates.**

## What registers an SI client — traced to the instruction, and it is not a missing chip

**Every link in the chain works, and that is the finding.** Nothing here is unimplemented, no
instruction is missing, no register is unmapped. The box runs correctly and declines.

**The service list registers itself, successfully, once.** `create_svl_from_sdt` at `0x800A6454`
is called from exactly one site in the whole image — `0x80050B9C`, inside a long unconditional
chain of subsystem initialisers — and, profiled across the boot:

    0x80050B9C  1     the only caller (positive control: its neighbours store live pointers)
    0x800A6454  1     create_svl_from_sdt entry
    0x800A647A  1     it registers a descriptor named ASTRA_SDT_SVL
    0x800A6484  0     the FAILURE branch never fires
    0x800A648E  1     the SUCCESS branch does
    0x800AD928  0     register_subtable_on_filter NEVER RUNS
    0x800ADC40  0     and neither does the only create=1 call site in the image

It comes away with a real SI client handle — **`0xF003`** — and entry 2 of the SI filter table
at `0x802D77E8` holds its own callback `0x800A6E55`. **And then that module executes 33
instructions in the entire boot and never runs again**: `0x800A6454`–`0x800A6498`, start to
finish, once. It registered four callbacks and not one of them is ever invoked. The module is
armed and waiting to be told to start.

**Nothing ever attaches a subtable to a filter, so every section is dropped.** All four subtable
slots (2 for type 4, 2 for type 6) stay free for the life of the box, which is what the boot log
meant by `Subtable: allocated 32 bytes for 2 clients`. The SI parser's `find_subtable` at
`0x800AB1B0` is a **lookup only** — its `create` argument is hard-wired to zero
(`move $a2, $zero`), and `find_or_create` at `0x800AAECC` returns NULL without one:

    800aaf2c  beqz  $v1, 0x800aaf6e      ; create flag == 0 -> NULL, no matter what arrives

**Driving the start path proves the machinery downstream of the trigger is healthy.** Calling
`0x800A68D8` — the function that would attach a subtable — directly: it reaches
`register_subtable_on_filter`, passes the handle check (`(0xF003-1)>>12 == 15`, index 2, and
the state test accepts any state that is not 1), switches on request type **7**, takes the
branch at `0x800ADA44`, and **runs through to `0x800ADA76` storing a new handle. It succeeds.**
`0x800ADC40` stays at zero because the create=1 site lives on the type 3/4/5/6 branches, not on
type 7 — so the type-4 (NIT) table was never going to be populated by that call. Measured before
and after, with all three tables pushed each time: NIT **9** bit-fetches both times, SDT **11**
both times, blits **8** both times. Nothing moved.

**And the BAT gets ZERO bit-fetches**, before and after — it never reaches a parser at all,
because `__siBAT` pushes to PID `0x0011` where the only armed filter is the SDT's, and no filter
in the box is asking for table `0x4A`.

**So the box is not missing a device. It is a Digibox that has never been installed.** Blank
NVRAM (486 of 16,384 bytes), no transport list at `/eeprom/tsl`, nothing at `/eeprom/otherts`,
no home transponder — and while idle it reads the string `demodulator` 262 times and
`/eeprom/otherts` 62 times, which is a box looking for a line-up it does not have. What is
missing is whatever drives **first-time installation**, and that is the next thing to find.

### The harness could not see the boot, and two readings paid for it

`__pcHits` accumulates only while profiling is on, and a probe body runs AFTER the boot is
asserted — so every boot-time routine read **0**, meaning "I was not watching" in exactly the
shape of "it did not run". `scripts/digibox-probe.mjs` now takes **`--profile-boot`**, which
arms the profiler in the same pre-boot window where `--trace-boot` already arms the printf.
Every count in the table above depends on it, which is why `0x80050B9C` is quoted alongside
them: it is the control that proves the profiler was armed at all.

## The box wants a config store that does not exist, and the installer never runs

**At startup the EPG looks up 38 four-character keys under `/eeprom/.stbconfig`**, read
straight off its own resources in the order it touches them:

    PSPF PSLB PSSC PSVO PSCO PSBT   SSAO SSVO SSOO SSBM SSBP   LSFL LSST
    LNFL LNFH LNPS LN22             <- the LNB: low and high oscillator, power, 22kHz
    DTFR DTPO DTSR DTFE             <- a transponder: frequency, polarisation, symbol rate, FEC
    DTON DTTS                       <- its original-network and transport-stream ids
    TSDT RFCH DRFSO  INST           <- and whether the box is INSTALLED
    MNFR MNPL MNSR MNFC SPRS SPPR RSCC CSDM CSDN PSPS CSCP

then `/eeprom` and `.stbconfig` themselves. **That is a home transponder and an installed
flag** — exactly what a box needs before it can tune anything, and exactly what a blank box
lacks.

**`.stbconfig` is not in NVRAM.** The 16 KB store holds five records and none of them is it:
`blacklist`, `ladm`, `OTVx`, `.cpprotconfig`, `batch`. So all 38 lookups miss. `.cpprotconfig`
existing is the control that matters — the create mechanism works and another module has used
it, so this is not "the registry is broken".

**The registry API is module 2, and the natives bracket the reads exactly**: `(2,0x15)` at
icount 162,934,478 against the `/eeprom` read at 162,934,944, and `(2,0x22)` at 162,940,259
against `.stbconfig` at 162,940,794. The firmware registry functions this project already knew
(`0x8007BB3C` and friends) are called **once** in the whole window — the application does not
use them.

**The EPG is itself the installer, and it never reaches any of it.** Its resources hold the
whole flow: `LNB SETUP`, `Enter an LNB oscillator frequency`, `Enter a default transponder
frequency`, `Now Scanning, please wait`, `Testing System, please wait`, `Ensure that the
satellite dish is correctly aligned`, `to cancel installation`, `Installation complete`, `No
signal found`, `No satellite signal is being received`, `Searching for listings`, and the codes
`LNB_LO_FREQ_LN` / `LNB_HI_FREQ_LN`. Watched at their DRAM addresses from before the
application starts and across eleven different remote keys: **zero reads of any of them**, with
both controls passing — the strings are inside the watched window, and the instrument fired 377
reads on a wider range in the same run.

**So the box is not showing installation and not showing a guide. It paints a background and
waits**, and the blit count independently confirms it: 8, from thirty seconds after boot to the
end of a three-minute watch.

### A warm boot and a cold boot are not the same machine

Worth stating because every measurement above was taken warm. A **warm** boot emits no
`#CONTROL` lines at all, going straight from `#INTPRT[READY] init` to `#INTPRT[RUNNING]
running:0x2`. The **cold** boot on record runs a longer path — `#CONTROL[init] ready to
start=0x0`, `#CONTROL[running] starting=0x2`, `starting application=0x2`, `user memory
allocated (Kbytes)=0x180000`, `unknown control error=0x19`, `app status : MAXIMIZED`. The
error is the default arm of a switch in the CONTROL task, reached through the pool word at
`0x800373F8`: a message CONTROL has no case for, at the moment the application starts.

## Nothing drives first-time installation, and the o-code says so line by line

**The EPG has two routines for its config store and only ever runs the wrong one.**

    0x9FC9A27C   load  .stbconfig    1 call site (0x9FC96BBB)     RUNS, and FAILS
    0x9FC9A3C7   create .stbconfig   7 call sites                 NONE of them runs

Call sites are found exactly rather than heuristically: opcode `0x15` is a call with a 4-byte
signed offset from the next instruction, so a call to a given address is a fixed 5-byte pattern
and an accidental match is about 2^-32 per position — roughly 0.00008 expected false hits across
the whole 353,188-byte CODE chunk.

**The load routine's failure is readable instruction by instruction**, and the trace confirms
every branch:

    9fc9a282  09 00 01 b4 30   push "/eeprom"
    9fc9a287  ca 02 15         native (2,0x15) -- open
    9fc9a28c  34 05  br        TAKEN      -> /eeprom opened fine
    9fc9a297  09 00 01 b4 38   push ".stbconfig"
    9fc9a29c  ca 02 22         native (2,0x22)
    9fc9a2a9  ca 02 1e         native (2,0x1e)
    9fc9a2b3  2c 05  br        NOT taken
    9fc9a2b7  42 01 0b jump 0x9fc9a3c5  ->  a2 / b8 ret      the routine just RETURNS

and the caller then takes the failure exit:

    9fc96bbb  15 00 00 36 bc   call 0x9fc9a27c      -- load_config()
    9fc96bc2  36 03  br        NOT taken            -- the success path
    9fc96bc4  42 01 83 jump 0x9fc96d4a              -- the config-missing exit

`0x9FC96D4A` is a fallback loop that looks the 38 keys up **one at a time** — `op_72` pushes a
four-character literal, so `72 4d 4e 53 52` is literally `"MNSR"` — every one misses, each miss
lands on the same handler at `0x9FC96E0A`, and the box carries on with zeros. **It never calls
create and it never shows installation.**

**The create routine sitting unused directly after the load routine even shows how it is done:**

    9fc9a3c7  push "/eeprom" ; ca 02 15    open the directory
    9fc9a3e4  push ".stbconfig" ; ca 02 1f  native (2,0x1f) -- CREATE, a different native
    9fc9a3f5  ca 02 1e                      then open it

**And the NVRAM is not the problem, which had to be checked rather than assumed.** A cold boot
from a genuinely blank store writes **486 bytes** — the firmware creates `blacklist`, `ladm`,
`OTVx`, `.cpprotconfig` and `batch` for itself. The write path works end to end. `.stbconfig` is
absent because the application never asks for it, not because the device cannot save.

**A cold box behaves exactly like a warm one.** 161 seconds, 446M instructions, blits reach 8
and stop, and the screen is a uniform dark blue — looked at, not inferred. The two boots differ
in their commentary (the warm one emits no `#CONTROL` lines at all) and not in their outcome.

**So the honest answer to "what drives first-time installation" is: on this box, nothing does.**
The seven create call sites are almost certainly System Setup menu handlers — user-driven — and
the UI cannot be reached because nothing draws. Whether a real box has an automatic trigger we
have not modelled, or whether the chain is genuinely entered from a screen we are failing to
render for some earlier reason, is the open question.

## The box created its own config store, and it was not enough

**The synthetic call fails for a reason worth knowing: the registry is PER-TASK.**
`store_open(name, 0, create, -1)` at `0x800655EC` is how the firmware creates
`.cpprotconfig`, and driving it through `__call` returns `-1` — for `.cpprotconfig` itself,
which exists. Profiling inside it shows why: it runs `0x800655EC`–`0x80065680` and turns back.
That stretch calls `0x8001E505` (the current task id), then walks a **7-entry, 188-byte-per-entry
table at `0x8016204C`** looking for an entry whose first word matches, and bails when none does.
`__call` runs on whichever task happens to be current, and that task has no registry context.

**Three controls caught this before any conclusion was drawn**, and they are the reason the
first `-1` was not read as "the store does not exist": open an existing store read-only, open a
second existing store, and only then the one in question. All three returned `-1`, which is a
statement about the caller, not about the stores. (A fourth, earlier, caught a plainer mistake:
the pool word holds `0x800655ED` and bit 0 is the **ISA tag**, not part of the address —
passing the odd value decodes a byte out of phase and dies on a bogus `sd is MIPS64-only`.)

**So the application was made to do it itself, with one byte.** Its config-load failure is a
jump to `0x9FC9A3C5`, a bare `ret`, and its unused **create** routine begins two bytes later:

    0x9FC9A2B7  42 01 0b   jump 0x9FC9A3C5     next = 0x9FC9A2BA, +0x10B -> the ret
    0x9FC9A2B9     ^^ 0b -> 0d                 +0x10D -> the create routine at 0x9FC9A3C7

`__flashPoke(addr, bytes)` writes the flash array directly, bypassing the command state machine
a plain `store()` would interpret (which is why `__poke` cannot change a byte there). It is in
memory only — the chip arrays refill from the `.bin` on every load, so the rollback is a reload
— and it returns the bytes it replaced so a caller can restore them and prove causation.

**It worked.** `.stbconfig` appeared in NVRAM at `0x1FB6` and the store count went from five to
six, 486 to 506 non-blank bytes. Because NVRAM persists to localStorage, the **next warm boot
had the store from its first instruction with no patch at all** — verified: the byte read back
as the original `42 01 0b`.

**And it was not enough, which is the finding.** The store is EMPTY — created, but with no keys
— so all 38 lookups still miss and the box still has no LNB, no transponder and no `INST`.
What did change is measurable and real: the same instruction window that previously ran the
config-reading code now runs completely different code (`0x9FC4A4BD`–`0x9FCA0647` instead of
`0x9FC96B61`–`0x9FC9A935`) and calls module-1 UI natives — `(1,0xd5)` ×4, `(1,0x57)` ×4,
`(1,0x75)` ×4, `(1,0xd7)`, `(1,0xe3)`, `(1,0xdc)` — **none of which had ever been called
before**. The screen is still uniform blue and blits still stop at 8, looked at rather than
inferred.

**So creating the store moves the application on and does not make it draw.** Watched for two
minutes with the store present: **no installation text read at any point** (controls passing —
the strings are inside the window and the instrument fires 377 reads on a wider range in the
same run) and the blit count **flat at 8 across all twelve samples**. What it needs is the
store's CONTENTS: a home transponder and an LNB. Those come from the installation UI on a real
box, and the seven create call sites look like System Setup menu handlers.

**Forcing the OTHER end of the chain does not work either, and the attempt is worth recording
because of where it stopped.** The SI parser calls `find_subtable` with its create flag
hard-wired to zero in a delay slot, at two sites — `0x800AB1C0` (type 4, NIT) and `0x800AB1E4`
(type 6, BAT), both `0x67C0` `move $a2, $zero`. Patching both to `0x6E01` `li $a2, 1` (encoding
verified by round-trip through the project's own MIPS16 disassembler, and both in DRAM so a
plain `__poke` does it) **did** make `find_or_create` take its create path — measured, one hit
at `0x800AAF2E`, and a slot came back stamped with our network id `0x1` where both had been
zero. And the parse depth did not move: NIT **9** bit-fetches before and after, SDT **11**,
BAT **0**. `find_subtable` does not return what `find_or_create` returns — it passes the result
to `0x800AB021` and returns THAT, and that second call still answers NULL. So a subtable
existing is necessary and not sufficient, and the remaining step is whatever `0x800AB021` wants.

**A note on the baseline, because it has changed.** The probe harness's warm profile now carries
`.stbconfig` in its NVRAM, so probe runs after this point are of a box with a config store and
earlier measurements were not. The rollback is `rm -rf /tmp/digibox-profile`, and `--cold` takes
a fresh one regardless. `./ctl.sh digibox` is unaffected — the gate runs its own browser with no
persistent profile, which is why it still measures a genuinely cold machine.

## The gate is one word: entry[+24], and forcing it makes sections parse

**`find_subtable` at `0x800AB1B0` does two things and returns the SECOND**, which is why forcing
the first changed nothing:

    find_or_create(id_from_header, type, create)   0x800AAECC
    then  0x800AB020(entry, header)                <- and it returns THIS

**`0x800AB020` was profiled during a real NIT push and the path is exact:**

    800ab020-800ab032   prologue; the entry is non-NULL
    800ab03a-800ab046   the entry[+28] tests (find_or_create sets it to 1 on creation)
    800ab054-800ab074   calls 0x800AB350(entry+32, header), tests bits 4 and 0 of the result
    800ab07e-800ab08c   a bit was set, so: lw $v1, 24($s0)
    800ab0a4            BAIL
    800ab08a  lw   $v1, 24($s0)
    800ab08c  beqz $v1, 0x800ab0a4      <- entry[+24] is ZERO, so every section is dropped

**`find_or_create`'s create path sets `+28`, `+30`, `+76`, `+80` and `+84` — and never `+24`.**
A sweep of every word store to offset 24 across the whole SI and DVB layers
(`0x800A0000`–`0x800B2000`, 122 candidates, 32 of them through a non-stack register) finds
exactly one that matters:

    800adcb2  beqz $s0, ...          ; a client handle must exist
    800adcb6  lw   $v1, 24($v1)      ; entry[+24]
    800adcb8  bnez $v1, ...          ; already set -> skip
    800adcbe  sw   $v1, 24($v0)      ; entry[+24] = 4

**That is inside `register_subtable_on_filter`, immediately after the only `create = 1` call
site in the image — and that function never runs.** So the chain closes: no registration, no
`+24`, no section ever kept, no service list, no menu.

**And writing that one word proves everything downstream of it is healthy.** With
`find_or_create` forced to create and `entry[+24]` poked to 4:

    baseline                     NIT = 9 bit-fetches   (the header, dropped)
    after forcing create         NIT = 9
    after writing entry[+24]=4   NIT = 37
    and again                    NIT = 37

The firmware then **moved the field itself**, 4 → 2, so its own state machine took over; and
`0x800AB0C2` and `0x800AB116` — arms of the switch on `+24` that had never executed — began to
run. Nothing on screen changed (blits still 8), but sections are being parsed for the first
time.

### The descriptor sweep finally ran, and its control killed the finding

With the gate open, a NIT carrying one descriptor of every private tag `0x80`–`0xFF` is parsed,
and counting only the bit reader's own fetches (the bulk copy at `0x800F99FE` reads all 1,283
bytes and must be excluded — that confound had already wasted one run against the section ring)
gives a uniform **2 reads per descriptor**, the tag and length as the parser skips, with two
exceptions: **`0x84` at 4 and `0x85` at 9**, `0x85` uniquely using the third bit-reader site.

**Reversing the descriptor order puts nothing above the median.** So the exceptions were
POSITION, not tag — a parser that reads the front of a 1,285-byte descriptor loop more deeply
than the back — and the reversed control is the only reason that is not written down here as a
discovery. The honest state is that **no private descriptor tag is yet established as one this
firmware wants.**

A follow-up offering four tags at a time in rotated groups returned **zero reads for all 128**,
which is an instrument that measured nothing rather than a result: its section versions cycled
through `& 0x1F` and repeated, so the later pushes were almost certainly refused as duplicates.
No control fired, so nothing is concluded from it. A rerun needs a version that never repeats
and a positive control that a section was accepted at all.

### And the last step is a CLIENT, not a subtable

Profiled in slices, before opening the gate and while broadcasting into it:

    SVL module        0x800A6400     0  ->     0  ->      0
    subtable manager  0x800A6000     0  ->     0  ->      0
    SI layer          0x800A8000     0  ->  2322  ->  10677
    SI registration   0x800AD000     0  ->     0  ->      0
    DVB object layer  0x800A2000     0  ->     0  ->      8

**The SI layer goes from dead to 10,677 instructions — sections really are being parsed.** And
the service-list module still executes **nothing**. Writing `entry[+24]` is only half of what
`register_subtable_on_filter` does: at `0x800ADCC0` it immediately calls `[0x800ADF9C](entry, 1)`,
and that is what attaches the client whose callbacks the SVL registered. A subtable with no
client parses sections into nothing, which is exactly what the counts show.

So the remaining gap is one call, not a mystery: **whatever `0x800ADF9C` does to bind a client
to a subtable.**

## The registration path ran, and a section was parsed end to end

**Attaching a client means PROGRAMMING A SECTION FILTER.** `0x800B0C70(entry, 1)` switches on
the subtable's type, and for type 4 it builds `{0xFE40, id}` — a mask/value pair matching
table_id `0x40` under mask `0xFE`, i.e. NIT actual and other — and programs it, then sets
`entry[+28] = 2`. That is why filters 25–31 carry contexts and no PID: nothing had ever asked.

**The whole registration was made to run by correcting ONE HALFWORD.** The service list's start
path at `0x800A68D8` builds a request of type **7**, which takes the branch at `0x800ADA44` and
succeeds *without* reaching either the create or the client attach — both of which live on the
type 3/4/5/6 branches. So:

    0x800A68DC   0x6B07  li $v1, 7   ->   0x6B04  li $v1, 4      (round-trip verified; DRAM)

Driving `0x800A68D8` with that in place, every step of the chain executed for the first time:

    0x800AD928  register_subtable_on_filter   1
    0x800ADC40  the create = 1 call           1      <- never before
    0x800ADCBE  entry[+24] = 4                1      <- never before
    0x800B0C70  the client attach             1      <- never before

and the slot came back with `+24 = 4`, `+28 = 2` **set by the firmware rather than by hand**.
The SVL module executed **39** instructions and the SI registration layer **116**, both of which
had been zero in every boot ever measured.

**And then a section parsed end to end — but only with the right network id.** The subtable is
registered for the id the service list passes, which comes from `[0x800A6B28]`/`[0x800A6B2C]`
and is **0**; every NIT this project has ever broadcast said network_id **1**. Pushed in the
same run, on the same machine, alternating:

    network_id 1   ->   9 bit-fetches   (the header, dropped)
    network_id 0   ->  37               (PARSED)

The repeats that follow read 0 because the subtable moves to `+28 = 3` once its version is
complete and stops accepting — which is what a working SI layer does, not a fault.

**So the chain is coherent all the way down, and it closes on the config.** `DTON` and `DTTS`
— original-network-id and transport-stream-id — are two of the 38 keys in `/eeprom/.stbconfig`,
the store the application never creates. No config, so those ids are zero, so the box subscribes
to network 0 while a real Sky multiplex announces something else. **Config → ids → subtable
registration → SI parsed → service list → menus**, and every link in it has now been observed
working except the first.

Still 8 blits and a blue screen: one NIT is not a service list. But nothing between the stream
and the service list is unexplained any more.

## The ids in our SI are not ours to choose

**A section is kept only if `find_subtable(id_from_its_header, type)` matches a REGISTERED
subtable**, and the id it matches on is the halfword at `+30` of a subtable entry. So the
network and bouquet ids we broadcast have to be the ones the box has asked for — and it asks
for whatever its service list passes, which comes from two words that are **zero** until
`/eeprom/.stbconfig` supplies `DTON` and `DTTS`.

**Every NIT this project broadcast said network_id 1, so none of them could ever have been
kept.** `__siNIT`, `__siSDT` and `__siBAT` now read the answer off the machine instead —
`__siIds()` reports the registered subtables first, because `+30` is literally what the match
is made against, and falls back to the service list's own variables for before anything is
registered. Hardcoding 0 would have been wrong for the same reason 1 was: it is only right
while the config stays empty.

    __siIds() before registration : "the service list's own variables", networkId 0
    __siIds() after  registration : "a registered subtable", subscribedNetworks [0]

**Verified with two controls that must fail, in the order that makes them mean anything:**

    CONTROL  explicit networkId 1   =  9 bit-fetches   (the header, dropped)
    CONTROL  explicit networkId 9   =  9
    default NIT, following the box  = 37               (PARSED)

**The order is not cosmetic and the first attempt got it wrong.** Once a subtable completes a
version it moves to `+28 = 3` and stops accepting, so every push after the first successful one
reads **0** — not "dropped for the wrong id" but "never reached the parser at all". Controls
placed after the success read 0 and proved nothing; moved in front of it, while the subtable is
still open, their 9 means what it says.

## Nothing calls the start path, and nothing calls anything in that layer

**No call site in the image asks `register_subtable_on_filter` for type 4 or 6.** Eleven sites
call it; five ask for type **7**, two for type **11**, and the rest could not be resolved from a
`li` → `sw $sp` pair. So the type-4 branch used to force a registration was a hack that happened
to reach the create — **type 11 is the real path**, and it passes `create = 1` too
(`li $a2, 1` at `0x800ADA9C`) through a different helper.

**And it never runs.** Profiled across a whole boot with `--profile-boot`, with
`0x80050B9C` — the single call site of `create_svl_from_sdt`, in an init chain known to run —
as the control:

    0x80050B9C   1     CONTROL: the profiler was armed
    0x800A3280   0     the type-11 registration function
    0x800B05E4   0     its callers' functions
    0x800B0714   0        (this one has 62 call sites in the image)
    0x800B0860   0
    0x800A2414   0     a DVB function called from the interpreter
    0x80051A3E   0     that interpreter call site
    0x800A68D8   0     the service list's start path
    0x800AD928   0     register_subtable_on_filter
    0x800ADA94   0     its type-11 branch
    0x800ADC30   0     its type 4/6 branch
    0x800ADC40   0     the create = 1 call

**The entire DVB monitoring stack executes zero instructions in a whole boot.** Not "the start
path is called with the wrong arguments", not "a branch declines" — nothing in that layer runs
at all. The service list registers itself and its four callbacks during the init chain and then
the whole subsystem sits idle for ever.

**It cannot be driven from below either, and that is a limit worth stating rather than working
around.** `0x800A3280` takes a live DVB object pointer — it dereferences `+16` of its argument
and then `+84` and `+88` of that — so calling it synthetically would mean fabricating a
structure, which is inventing data rather than measuring it. The objects are made by the DVB
object layer, which the application drives through module 7's natives; the EPG calls exactly one
of that module's 61 functions.

**So the trigger is above all of this, and it is the configuration.** The box has no LNB and no
transponder, so it never begins service acquisition, so it never opens a DVB object, so nothing
ever registers a subtable, so every section is dropped. That sends the work back to
`/eeprom/.stbconfig` with a much sharper target than before: **the store needs real VALUES —
`LNFL`/`LNFH`/`LNPS`/`LN22`, `DTFR`/`DTPO`/`DTSR`/`DTFE`, `DTON`/`DTTS`, `INST` — not merely
to exist.** Creating an empty one has already been tried and moves the application on without
making it draw.

## The flash has the defaults, and they are real numbers

**Nothing needs inventing and no EEPROM dump is needed.** The EPG's data chunk carries a
defaults table at **`0x9FCBB128`** — 38 entries of `{FourCC, 32-bit big-endian value}`, one per
config key, found by scanning the whole image for clusters of the key names and noticing that
one cluster sits in DATA on an 8-byte stride rather than in code:

    LNFL  0x0094C5F0   9,750,000      the LNB low local oscillator, in kHz  -> 9750 MHz
    LNFH  0x00A1BE40  10,600,000      the LNB high local oscillator         -> 10600 MHz
    DTFR  0x00B3B7D0  11,778,000      the default transponder               -> 11778 MHz
    MNFR  0x00B3B7D0  11,778,000      the manual-tune frequency, the same
    DTPO  1 · DTSR 1 · DTFE 1         polarisation, symbol rate, FEC -- INDICES, not values
    LNPS  1 · LN22 1                  LNB power on, 22kHz on
    RFCH  0x44        68              the RF modulator's UHF output channel
    INST  0                           not installed
    DTON  0xFFFFFFFF · DTTS 0xFFFFFFFF    original-network and transport-stream ids, UNSET
    SPRS  0xFFFFFFFF · SPPR 0xFFFFFFFF

**Three of those are independently checkable against the real world and all three match
exactly**: 9750 and 10600 MHz are the universal LNB's two local oscillators, 11778 MHz is Sky's
home transponder on Astra 28.2°E, and UHF 68 is the channel a Digibox modulates onto. That
agreement is the evidence the table is what it looks like — it was found by pattern, and three
numbers landing on published values by chance is not a thing that happens.

**And `DTON`/`DTTS` being `-1` is why the box subscribes to network 0.** The two ids the service
list passes into a subtable registration come from those keys; unset, they resolve to zero, and
a NIT announcing any other network is dropped at its header. Everything is consistent.

Two more key names turn up in the code that the 38 startup lookups do not use — `INLN`, `CSCS`,
`BIDL`, `CBLN` — and `RFSO` appears in the table where `DRFSO` appears in the read loop.

**The three all-keys runs in the CODE are dispatchers, not writers.** `0x9FC92498`–`0x9FC92709`,
`0x9FC94F00`–`0x9FC950B0` and `0x9FC99B6D`–`0x9FC99D5F` each name every key once or twice in the
shape `push key; compare; jump to that key's handler`, and the handlers live in `0x9FC9A0xx` —
the same module as the config load at `0x9FC9A27C` and the create at `0x9FC9A3C7`.

## CORRECTION: the box is not short of an LNB or a transponder, and the config is not the gate

**The previous section concluded "no LNB and no transponder, so no service acquisition". That
is wrong.** The defaults table in the flash is not merely present — it is **loaded**, and the
application runs on it. Five copies live in DRAM as `{FourCC, u32}` pairs on an 8-byte stride,
two of them complete at 38 entries:

    PSPF=0 PSLB=0 PSSC=1 PSVO=0 PSCO=1 PSBT=1 SSAO=1 SSVO=3 SSOO=0 SSBM=1 SSBP=1 LSFL=0 LSST=0
    LNFL=9750000  LNFH=10600000  LNPS=1  LN22=1  DTFR=11778000  DTPO=1  DTSR=1  DTFE=1
    TSDT=0  RFCH=68  RFSO=0  INST=0  MNFR=11778000  MNPL=1  MNSR=1  MNFC=1
    SPRS=-1  SPPR=-1  DTON=-1  DTTS=-1  RSCC=1  CSDM=0  CSDN=0  PSPS=1  CSCP=24

So the box knows its LNB and its home transponder from the first instruction. The empty
`/eeprom/.stbconfig` never deprived it of either — it only means nothing has been *overridden*.

**THE TABLE MOVES BETWEEN BOOTS** — `0x804306D0` one run, `0x80430620` another — so it must be
found by pattern (the literal `LNFL` followed by 9,750,000) rather than addressed. A hardcoded
address here is how a poke writes somewhere harmless and reports success.

**`INST` is not the gate either, and that was tested properly rather than assumed.** Writing
`INST = 1` into all four RAM copies after startup changed nothing, which is what one would
expect of a decision already taken — so it was written where the decision is *read from*, the
flash default at `0x9FCBB1EC`, before the application loads it:

    0x9FCBB1E8  "INST"  00 00 00 00   ->   00 00 00 01

The box then booted believing it was installed — **verified in all four RAM copies it built,
each reading 1** — and the blit count went 4 → 8 and stayed there, exactly as always.

**So the configuration hypothesis is disproven.** A box with a valid LNB, a valid home
transponder, `INST = 1` and an always-locked demodulator still never starts SI monitoring, and
the whole DVB stack still executes nothing. What remains wrong in the config is only
`DTON`/`DTTS` = −1, and those are ids a box is supposed to *learn* from the NIT it cannot yet
parse — which is a chicken and egg, so a real box must have a scan mode that accepts any id.

**The strongest remaining lead is the tuner, and it has been in plain sight since sky-eluc.14.**
While idle the box reads the string `demodulator` 262 times and polls registers `2/sel 2`,
`10` and `28/sel 3`, `29/sel 7` — **none of which our model answers**; it returns five named
registers and `0x00` for everything else. Every transaction now carries the PC and `$ra` that
started it. A driver waiting for a bit that can never set would look exactly like this.

## The tuner was the trigger: demodulator register 11, bits 0-5

**One register answer starts service acquisition.** The model answered five named demodulator
registers and `0x00` for everything else. Register **11** now answers `0x3F`, and on a plain
boot — no override, no flash patch, no poked request type, no hand-written `entry[+24]` — the
box does the whole thing itself:

    SI registration layer        0  ->   1,931     (it was ZERO in every boot ever taken)
    SI layer                     0  -> 130,792
    DVB object layer            24  ->  22,914
    register_subtable_on_filter  0  ->       3
    the create = 1 call          0  ->       2
    entry[+24] = 4               0  ->       1
    the client attach            0  ->       1
    NIT parse depth              9  ->      37     (9 is a header, dropped)
    demux PIDs      0x14 0x11 0x10  ->  0x52 0x14 0x11 0x10

It registers a subtable for network `0x20`, programs a **new section filter on PID `0x52`**, and
our broadcast follows the id automatically because `__siIds()` reads it off the box.

**The value was narrowed by falsification, not chosen.** Eleven separate boots, one value per
boot, with the signal being whether `register_subtable_on_filter` runs at all:

    0x00  0x0F  0x1C  0x3E  0x7B  0x80  0xF0   ->  nothing
    0x3F  0x7F  0xBF  0xFF                     ->  the box acquires

`(v & 0x3F) == 0x3F` — **bits 0..5 set, bits 6 and 7 don't-care** — explains all eleven. The
driver reads registers 10 and 11 as a pair (a two-byte read at port 3 whose index
auto-increments), which is why no single register tested alone reproduced it and why the
"written" register 11 turned out to be the one being read.

**THE FIRST RULE WAS WRONG AND THAT IS THE POINT.** `(v & 0x7F) == 0x7F` fitted the first seven
values perfectly. `0xBF` was run specifically to break it, and passed. The rule here is the
second one, and `0x3F` and `0x3E` were then predicted in advance and both came out as predicted.
A rule fitted to the data it was derived from is not evidence; a rule that survives a value
chosen to kill it is.

**And the intermediate reasoning was wrong too, which is worth recording.** Partway through this
the evidence looked like the opposite: the outermost routine behind every transaction is an I2C
RETRY loop, the wrapper receiving the byte normalises it to success/failure without testing its
value, and 1,119 registers had been written — all of which said "the driver is monitoring, not
stalling", and the premise was about to be written off. It was only kept alive by running the
falsification rather than concluding from the reading.

**What this does NOT establish is what register 11 MEANS** — a lock indicator, a quality
threshold, a status word. Only what the driver insists on. Said plainly because the next reader
will want to know which part is measured.

## The box states what it wants, in its own section filters

**The demux's section-filter programming is the box telling us what to broadcast, and it was
being recorded and never decoded.** A value goes to `+0x148` and then a command to `+0x144` of
the form `0xC0 | (byteIndex << 4) | filter`; the low half of the value is `{match, mask}`, byte
0 is the `table_id` and bytes 1-2 the `table_id_extension`. `__siMatches()` decodes it, and once
service acquisition starts this box asks for:

    table 0x40  mask 0xFE  extension 0x0020     the NIT, network 32
    table 0x42  mask 0xFB  extension 0x0000     the SDT, transport 0
    table 0x4A  mask 0xFF  extension 0x1000     the BAT, bouquet 4096
    table 0x73  mask 0xFF                       the TOT

`__siIds()` now takes its ids from there in preference to anything else, because a hardware
filter is the most direct statement of intent there is. **The bouquet id was wrong: we were
sending `0x20` and the box wants `0x1000`** — 4096, which nobody would have guessed.

**And the box asks for a table we had never sent.** Table `0x73` is the **Time Offset Table**:
the TDT carries the time, the TOT carries the time *and* the local offset, and a box needs the
second to put a programme in a viewer's timezone. `__siTOT()` builds it and `__siBroadcast()`
puts it on the air. It differs from the TDT in two ways that matter — it has a descriptor loop,
and it HAS a CRC despite a `section_syntax_indicator` of 0, which is unusual enough in DVB to be
worth stating. January 1998 is GMT, so the offset is zero with no change pending, which is the
honest answer for this box's era rather than a placeholder.

**A name collision made the new instrument silently return another function's data**, and it is
the reason `__siIds()` reported every extension as null while looking perfectly healthy:
`__siFilters` already existed, reporting each filter's PID and ring, and a second definition
shadowed the first. Renamed to `__siMatches()`. **Two functions, one name, and the one that
loses is the one defined first** — nothing warns, and the caller gets a well-formed object of
the wrong shape.

**PID 0x52 is transient and is NOT where the BAT goes.** It opens in response to our NIT and
closes again, and the same BAT pushed to both PIDs in one run gives **24 bit-fetches on 0x11 and
0 on 0x52**, twice. The hypothesis that the NIT was pointing the box at a bouquet stream is dead.

All three tables now parse where two used to be dropped: NIT 37, SDT up to 77, BAT 24. The
service-list module runs and grows. The screen is still 8 blits of blue.

## The service list rejects nothing — and the application is now working

**The premise of this question was wrong, and finding that out relocated the search.** The SVL
module's entire activity is a single `jalr $v1` at `0x800A6414` inside a dispatcher — when
`[sp+4]` is 4 it calls the handler at `[sp+8]` — so the three addresses that kept showing up
were not a callback returning early. They were a **dispatch**, and the work happens in a module
nobody had profiled: `0x800CA555`, `0x800CA6B9`, `0x800CA725` and `0x800C4261`, next door to one
of `register_subtable_on_filter`'s call sites.

**And that module consumes everything it is given.** Instructions added per broadcast round:

    SI consumers   0x800C0000    57,400  121,285   43,211   59,211   58,839   52,781
    SVL dispatcher 0x800A6400         3        9        3        0        0        0
    SI layer       0x800A8000    11,788   98,075   14,037    1,569    1,264    1,569
    EPG bytecode   0x80069000   238,956  339,980  317,089   32,036   27,234   32,036

Substantial work every round, and everything settles after the third — which is a box that has
finished processing what it was given, with repeat versions correctly ignored. **Nothing is
being refused.**

### The application is a different program now

Across a whole boot before any of this worked, the EPG made 361 native calls, dominated by the
event wait, with exactly **one** call into module 7 — the DVB API whose 61 functions were
sitting unused. Fed for twenty-five seconds it makes **408**, and they are these:

    108x (2,0x30)  107x (2,0x1e)  53x (2,0x28)  53x (2,0x2c)      the registry, hammered
      7x (7,0x0d)    2x (7,0x10)   2x (7,0x1c)   1x (7,0x06)
      1x (7,0x07)    1x (7,0x05)   1x (7,0x0b)   1x (7,0x0e)
      1x (7,0x2b)    1x (7,0x2c)                                  TEN different DVB natives
      1x (8,0x01)    2x (8,0x02)   1x (12,0x20)

**It is reading services through the DVB API and writing them to its registry** — work it did
none of before. And `(1,0xE4)`, the only native that has ever drawn anything here, is called
**zero** times; the blit count stays at 8.

So the chain from the tuner to the application is complete and working, and the application is
busy. What it is not doing is drawing, and that is now the whole of the remaining question.

## Why it still does not draw — NOT answered, and here is what was eliminated

**This question is open.** Recording it that way because the eliminations are worth having and
the temptation is to dress up progress as an answer.

**A fed box still reads none of its own UI text.** Watched over an idle window and eight
different keys, with both controls passing — the strings are located live (they move between
boots) and a wider range fired 22 reads in the same run:

    TV Listings  Other Channels  ALL PROGRAMMES A-Z  Searching for listings
    LNB SETUP    Now Scanning    No satellite signal  PARENTAL CONTROL     -> ZERO reads

and `(1,0xE4)`, the only native that has ever drawn here, is called zero times. The blit count
stays at 8. So the application is not preparing a screen, fed or not.

**`power_control` is a device name, not evidence of standby.** It appears in the box's device
inventory — `scart, video_encoder, audio_encoder, demodulator, power_control, modem,
rf_modulator, box` in the EPG's resources, and `scart, ScartGate, panel_leds, power_control,
beep, BARBGate, rf_modulator` in the runtime. That also explains, finally, why the box reads
the string `demodulator` 262 times while idle: it is a device lookup by name, not a poll.

**The registry work is in memory, not NVRAM.** Fed, the EPG makes 321 calls into module 2, but
NVRAM stays flat at 574 non-blank bytes across a forty-second feeding-and-settling window.

**Though it HAS persisted something it never had: `/eeprom/favchn`.** The store list went from
eight to nine and 506 to 574 bytes across the fed runs — a favourite-channels store, created by
the box, which had never appeared before any of this worked. So it is storing channel-related
state; it is simply not drawing.

**What remains untried, in the order worth trying:**

1. The o-code. `scripts/ocode-disasm.py` covers 78 of 209 opcodes and grows with each trace; the
   EPG's main loop at `0x9FC4D948` and its key handler at `0x9FC4A538` disassemble cleanly. The
   application's own logic is the one thing that has never been read end to end.
2. Whether a screen needs an event class we never send. `(1,0x26)` is the event wait and costs
   414,171 instructions when it blocks — so the EPG is genuinely waiting on something, and what
   arrives on that queue has not been captured.
3. ~~Whether the box considers itself to have a CURRENT SERVICE.~~ **Tried, and it is not that.**
   Our services are `0x64`-`0x67`, so the channel numbers are 100 to 103 — and "1" on its own,
   which the first attempt pressed, is a channel this box does not have. Typing **1-0-0** as a
   viewer would, on a fed box, produces 33 native calls including `(7,0x1c)` into the DVB layer;
   1-0-1 produces 27; the menu keys `0x1b` and `0x03` produce six each including a module-12
   native, `(12,0x24)`, that had never been seen. **Every one of them draws nothing: 0 blits,
   the drawing native never called.** The box reacts to a channel selection and still shows
   nothing.

**And the asymmetry that is left is the sharpest thing to chase.** This application drew
exactly ONCE, during its startup burst, and the blue screen is that draw — it opened a window
and filled it. So it HAS a screen, and has simply never put anything in it or opened another.
Whatever ended that burst without a second draw is the question, and the burst itself is on
record: `(1,0x45) (1,0x44) (1,0x2f) (1,0x56) (1,0x5c) (3,0x08) (3,0x40) (1,0x33) (1,0x56)
(1,0x52) (1,0x36) (1,0x35) (1,0x56) (1,0x56) (1,0xd2) (1,0x4f) (1,0x56) (1,0xbb) (1,0xbc)
(1,0xe4)` then `(2,0x1e)`. Disassembling the o-code around that last call is the untried step.

### The o-code around the draw, and the two planes

**Done, and it changes the question.** The draw was found statically first: `CA 01 E4` is a
literal byte sequence and it occurs at exactly THREE addresses in the 353,188-byte CODE chunk --
`0x9FC6D107`, `0x9FC6D163` and `0x9FC72A7F` -- and nowhere else. Which one runs is then a
runtime fact the trace already carried: the `0xCA` handler shares the fetch loop's frame, so
`[$sp+44]` at its first instruction IS the address of the operand bytes, and the one draw of the
boot reports `at = 0x9FC72A87`. **The third site.**

**THE APPLICATION HAS TWO DISPLAY PLANES AND HAS ONLY EVER USED ONE OF THEM.** The arms are all
of one shape -- `aa 09 <u32 global> 20 ca <module> <function>` -- and a scan for that shape finds
381 of them, of which 127 name one of two globals, **`0x02E118` and `0x02E11C`**. They appear in
matched pairs throughout the image, the same native applied to each a few bytes apart
(`D8`/`D8` at `0x9FC5EF3F`/`0x9FC5EF52`, `D7`/`D7`, `D5`/`D5`, `D2`/`D2`), which is what makes
them two instances of one thing rather than two unrelated variables. The three draw sites split
across them: `0x9FC6D107` and `0x9FC6D163` draw plane **`0x02E118`** and have never run;
`0x9FC72A7F` draws plane **`0x02E11C`** and is the one that painted the blue screen.

**`0x9FC72A7F` IS A FUNCTION ENTRY AND IT IS THE SEVENTH MOST-CALLED FUNCTION IN THE WHOLE
IMAGE -- 59 call sites -- AND IT RUNS ONCE PER BOOT.** That is a much sharper statement than
"the box does not draw", and it rests on a control rather than on a pattern match, because a
blind scan for a 5-byte call encoding over 353 KB of bytes is exactly the instrument this
project distrusts. The control: of 3,395 `0x15` bytes in the chunk, **89.4% resolve to a target
inside the code chunk**, where random 32-bit offsets would land there 0.008% of the time, and
the 3,034 in-range calls share only **509 distinct targets**. A scan over data does not produce
a 509-entry function table. `0x9FC72A7F` sits 7th in it, behind `0x9FC4A420` (338 calls) and
`0x9FC725AB` (179).

**AND THE CONTENT CALL HAS NEVER BEEN MADE AT ALL.** `(1,0xE8)` is the workhorse -- about fifty
sites, all but two of them on plane `0x02E118` -- and a census of every native call across a
whole boot, a feed and thirteen key presses counts it **zero times**:

    boot   250 natives, 53 distinct, 31 of them module 1 -- including (1,0xC7) x100, (1,0x2D) x49
    fed    381 natives, 28 distinct,  7 of them module 1
    keys    48 natives,  7 distinct,  5 of them module 1
    (1,0xE8) content call: 0        (1,0xE4) draw: 1        blits 8

The instrument plainly fires -- thirty-one distinct graphics natives at boot, two of them a
hundred and forty-nine calls between them -- so the zero is a zero. Plane `0x02E118` IS
configured: `(1,0xD0)` at `0x9FC72A53` runs, and so do `D5`, `D7`, `D2`, `E3`. **It is set up,
never written into, and never presented.**

**The main loop is healthy, which rules out the obvious explanation.** `0x9FC4A538` is the event
loop and the decoder was validated over it -- 42 boundaries, every one of the 25 addresses the
machine fetched landed on, zero drift. It calls `(1,0x26)` to wait, then walks a comparison
chain (`0xEE`, `0xEF`, then constants 7, `0x0900`, `0x0080`, `0x20`, `0x0100`) and dispatches.
Idle, it processes **eight events per tick**. It is not stuck; it has nothing to repaint.

**A KEY PRESS ON A FED BOX REACHES NONE OF THE 59 CALL SITES.** Typing channel 100 executes 780
o-code instructions, 155 of them in code idle never touches, in new regions at `0x9FC7320C`,
`0x9FC7B190`, `0x9FC90DEA`, `0x9FC9DE22` and `0x9FCA05BC` -- and enters neither the repaint
function nor any of its callers. The gate is visible at `0x9FC4A825`: the box compares an event
code against `0x09`, `0x08`, `0x0B` and `0x1E`, and only the `0x1E` arm falls through to
`(1,0xE8)` on plane `0x02E118`. Measured over four passes, the first comparison branched away
every time.

**What is NOT established, because two of the instruments here are heuristics.** The call graph
was walked upward with "the enclosing function is the nearest call target at or below", which is
only correct if every function is a call target -- event handlers and callbacks are not, so the
chain it drew from the plane-`0x02E118` draws up to `0x9FC4A4F0` is **not evidence**. What IS
exact is the single call site: `0x9FC6CC55` (which holds both of those draws) is called from
`0x9FC6AC15` and from nowhere else, and `0x9FC6AAEF` from `0x9FC4A882` and nowhere else.
Separately, every trace held here together covers **0.8% of the code chunk**, so "no caller was
ever executed" is a statement about a very small window. And the decoder stops at two opcodes,
`0x72` and `0x3E`, whose operand lengths no trace has yet shown.

**One measurement nearly became a finding and was caught by its own control.** A key press
capture read ZERO o-code fetches, which is impossible for a running interpreter, and the obvious
reading was that the previous action had wedged the box. Re-running with an idle control between
every action killed it: idle read **exactly 1031** three times running, and a fourth idle with
nothing before it read 0. The application runs in a periodic burst of exactly 1031 fetches and a
2.8-second window sometimes falls between ticks. The zero was sampling, not state.

### What raises the content call: NOT a key, and the call graph cannot answer it

**The application dispatches through VTABLES, and that retires a whole line of reasoning.**
Opcode `0x14` is an indirect call with no operand at all -- the target comes off the stack -- and
it occurs **944 times** against 3,034 direct calls. It cannot be measured the way the rest of the
operand table is, because the interpreter never reads an operand for it, so a trace shows only a
transfer and that is indistinguishable from a jump. **The return address settles it**: at
`0x9FC4DCE3` the machine fetched the opcode, transferred to `0x9FC4DF3C`, and later resumed at
`0x9FC4DCE4`. One byte on, so the instruction is one byte; a call rather than a jump because
control came back at all. Falsified rather than asserted -- with one operand byte the walk drifts
off a real boundary, with two it is rejected outright, with zero it lands on all 23 addresses the
machine fetched in that range and none it did not.

So **a function with no `0x15` caller is not thereby dead**, and "none of the 59 call sites is
ever reached" is a statement about direct calls only. It also corrected a reading made an hour
earlier: the 601-byte transfer out of `0x9FC4D9E8` is not a branch skipping five content calls,
it is a method call that returns.

**The chain at `0x9FC4A825` is real but its constants are not key codes.** It switches on a value
against `9`, `8`, `0x0B` and `0x1E`, and only the `0x1E` arm falls through to `(1,0xE8)` on plane
`0x02E118`. `0x1E` is also the logical code for CHANNEL UP, and that looked decisive for a while
because of a genuine second defect: **every channel-up this project had ever pressed was on the
wrong device.** The decoder at `0x8002A811` promotes device 4 to device 5 for raws inside six
ranges, and `0xD1..0xF2` is one of them -- so raw `0xD3` and `0xD1`, used in every probe, are the
Sky keyboard and not the handset. On the handset, channel up is raw **`0x70`** and channel down
**`0x6F`** (read live off the range table rather than carried).

**Pressed and held on the correct device, it changes nothing.** Channel up produces the same four
natives as a digit -- `(1,0x26)`, `(1,0x36)`, `(1,0x43)`, `(1,0x56)` -- with no content call, no
draw and no blit, and so do channel down and the other three constants. The device was a real bug
and was not this one.

**A SWEEP OF ALL 141 LOGICAL KEYS WAS THROWN AWAY, AND ITS OWN SHAPE IS WHY.** It reported zero
natives for every key, pressed and held -- including keys measured minutes earlier producing 48
natives on the same rig. Two emulator probes had been left running concurrently and starved each
other, so every key was "measured" in a window where the machine barely advanced. An instrument
that reads zero over everything has measured nothing. The re-run carries a positive control
INSIDE it -- a channel digit, which must produce natives -- and returns `VOID` rather than a
table if that control is silent. Run emulator probes one at a time; wall-clock settles are not
emulated time.

**What would answer it, and has still not been done:** read the event record the wait returns.
`(1,0x26)` at `0x9FC4A53E` hands back `{u16 class, u16 kind, u16 key, u16 device}`, the loop tests
class against 2 and kind against 7, and the classes it switches on are bit flags -- `2`, `0x08`,
`0x20`, `0x0080`, `0x0100`, `0x8000`. Nothing has ever captured what actually arrives on that
queue, and every reading of those constants so far has been inference from an unexecuted branch.

### The event stream, read off the machine at last

**The poster's arguments can simply be read, and now have been.** `0x8006EA04(a0, class, code,
a3)` fires TWICE per key -- a press from `ra=0x8002A903` and a release from `ra=0x8002AA3B` --
with `class = 2` on every one and `a3` a watchdog count on the press, `0x80105CCC` on the
release:

    a0=1 a1=2 a2=0x31 a3=0x14   '1'          a0=1 a1=2 a2=0x1E a3=0x14   channel up
    a0=1 a1=2 a2=0x30 a3=0x14   '0'          a0=1 a1=2 a2=0x1F a3=0x0E   channel down
    a0=1 a1=2 a2=0x30 a3=0x13   '0'          a0=0 a1=2 a2=...  a3=0x80105CCC  release

**THE DOCUMENTED ADDRESS IS ISA-TAGGED AND THE FIRST RUN WATCHED A PC THAT DOES NOT EXIST.**
`0x8006EA05` is written that way throughout this file and bit 0 is the MIPS16 tag, not part of
the address -- the PC to watch is `0x8006EA04`. Every working trace in this project uses an even
address and nobody had noticed the poster was quoted odd. It cost nothing only because the probe
carried a guard that returned `VOID` when the traced PC was never reached, instead of a table of
zeroes.

**The key slot is `0x8011E2E4`, and it is not the record anyone assumed.** It was found by
following the code through the writes rather than by searching for a layout: the poster builds
a u16 on its own stack at `0x801775DC`, and `0x8006EA50` places it at `0x8011E2E4`. Watching a
band around it through five keys gives ten writes and no others --

    pc 0x8006EA50   0x8011E2E4 <- 0x0031     pc 0x8006EAF8   0x8011E2E4 <- 0x0000
    pc 0x8006EA50   0x8011E2E4 <- 0x0030     pc 0x8006EAF8   0x8011E2E4 <- 0x0000
    pc 0x8006EA50   0x8011E2E4 <- 0x001E     pc 0x8006EAF8   0x8011E2E4 <- 0x0000

-- so it is a **single u16 "current key" slot, set on press and cleared on release** about
440,000 instructions later, not a queue of `{class, kind, code, device}` records. That layout was
put to the machine as a prediction and **falsified**: the words around `0x8011E2E0` read class
`0x1C2`, kind `0x96`, device `0x2` where a key press must give 2, 7 and 4.

**Twenty-five seconds of idle wrote to that slot ZERO times**, on an instrument that had just
fired ten times for five keys. So keys genuinely reach the application, and **the classes the
main loop switches on -- `0x08`, `0x20`, `0x0080`, `0x0100`, `0x8000` -- do not travel through
this slot.** Where they do travel is still unknown, and that is the open end of this thread
rather than a conclusion.

**A THIRD WRONG ASSUMPTION ABOUT KEYS, AND IT INVALIDATES EVERY "CHANNEL 100" MEASUREMENT IN
THIS FILE.** The live map at `0x800FCEC0` says **raw `0x00`-`0x09` are the digits `'0'`-`'9'`**
(logical `0x30`-`0x39`). Every probe that "typed channel 100" sent `[0x03, 0x01, 0x01]`, which is
**`'3'`, `'1'`, `'1'` -- channel 311**, a channel this box does not have either. Read the digits
off the map; the comment in the first probe that asserted `0x03` is `'1'` was wrong and was
copied forward. Three separate key assumptions have now been falsified in one session -- the
device, the channel-up raw, and the digits -- and all three were inherited as prose rather than
measured.

> **SUPERSEDED — read *THE KEY PATH IS OPEN END TO END* below before using anything in this
> section.** `0x9FC4D4F6` is the TIMER REGISTRATION function, the table at `DS+0x0252F8` is 48
> records of 40 bytes describing timers, and `0x025310` is a timer's `userData` field which all 53
> registration sites pass as zero. The gate below is a timer-expiry check, not a drawing decision,
> and the reasoning built on it does not hold.

### The drawing gate, found: a class-8 timer already drives the repaint, and the app declines

**The content call does not need raising. It is raised every idle tick and refused.** The chain is
now measured end to end and every step of it is in a validated decode:

1. the main loop blocks on `(1,0x26)`;
2. an event of **class 8** arrives -- read off the branch counts in the idle traces, not guessed:
   at `0x9FC4A5FC` the "not equal to 8" branch is NOT taken, so the class IS 8, and control falls
   through to `jump 0x9FC4A8D1`;
3. `0x9FC4A8D1` calls **`0x9FC4D9E8`** -- the function holding six `(1,0xE8)` content calls;
4. `0x9FC4D9E8` runs **274 opcodes**, makes indirect method calls, and takes none of its draws.

**The refusal is one branch, and it is the same shape twice:**

    0x9FC4DB3F   aa 09 00 02 53 10  02 20  36 03  42 01 29    the drawing gate
    0x9FC4DC86   aa 09 00 02 53 18  02 20  36 03  42 00 f9    the same shape
    0x9FC4DCD9   aa 09 00 02 53 18  02 20  6c a3  14          a calli that DOES run, x5

At `0x9FC4DB47` the conditional branch to `0x9FC4DB4C` -- the arm that reaches
`aa 09 00 02 E1 18 20 ca 01 E8`, the content call on the main plane -- **is not taken in any
trace held**, and the 297-byte jump past it is taken every time.

**`0x025310` is the gate and it has only SIX references in the whole image**, in three functions
(`0x9FC4D4F6`, `0x9FC4D71B`, `0x9FC4D9E8`), one of which is among the most-called in the
application. Its siblings `0x025314` (5) and `0x025318` (6) are accessed identically. They are
plainly a small set of application state objects, and they are distinguishable from the display
planes by their access opcode: `0x02E114`/`0x02E118`/`0x02E11C` are followed by `0x20` in 114 of
115 references, while every one of the `0x0253xx` references is followed by `0x02`.

**And one sibling is alive while the gate's is not.** `0x025318` has both the gate shape and a
`calli` at `0x9FC4DCD9` that executes on every tick. So the application is not inert and the
objects are not all empty -- **it is `0x025310` specifically that answers in the way that skips
the drawing.**

**`aa` PUSHES THE BASE AND `09` THE OFFSET, so these are not addresses** -- `0x00025310` is not
valid on this machine. Breaking at the interpreter's handler for opcode `0x09` (resolved live from
the dispatch table: `0x80069AB8`) and reading all 32 registers found **no** register holding a
plausible base, tested rather than eyeballed -- a base `B` would make `B+0x02E118` read as a
pointer, and none did. The base comes from `aa`, which is where to look next.

**WHAT IS NOT ESTABLISHED HERE:** that `0x025310` is null or empty. What is measured is that its
branch is never taken and the draw is always skipped; *why* it answers that way is the open
question, and reading its value needs the globals base first. There is also a **third display
plane**, `0x02E114`, seen at `0x9FC4A8E5` with `(1,0x56)` -- the two-plane description in the
section above is incomplete.

### Reading the gate's value: NOT solved, and two instruments failed on the way

**`0x025310`'s value has not been read.** `aa` pushes the application's globals base and `09` the
offset, so the base must be found before the object can be. Two approaches were tried and both
failed; the failures are recorded because each is a shape that would otherwise be repeated.

**A break at the interpreter handler is the wrong instrument here, twice over.** First the handler
address came out as `0x-7FF927CE` -- **`(x >>> 0) & ~1` converts back to a signed int32**, so any
handler above `0x7FFFFFFF` goes negative, and the run took zero stops while looking like a
measurement. The mask must precede the unsigned coercion. Fixed, the handler resolves to
`0x8006D832` and the run STILL took zero stops, because **a stall is not reliably detectable by
polling `__regs().pc`** -- an earlier probe reported `brokeThere: false` while the machine was
demonstrably stalled at the address. A run that takes zero stops is indistinguishable from a run
where nothing happened.

**THE READ-CORRELATION APPROACH RETURNED A CONFIDENT ANSWER THAT WAS VACUOUS BY CONSTRUCTION.**
The idea was sound -- the base is whatever `B` makes several of `B+0x02E114`, `B+0x02E118`,
`B+0x02E11C`, `B+0x025310`, `B+0x025314`, `B+0x025318` appear in the set of addresses the machine
read -- and it reported `base 0x800A54F8, matched 3/6`. **It is nonsense.** The three plane
offsets are four bytes apart and so are the three state offsets, so ANY three consecutive words
the machine reads satisfy "3 of 6" at three shifted bases. That is exactly what came back:
`0x800A54F0/F4/F8` for one group and `0x800AE2F4/F8/FC` for the other. A threshold of three was
the number coincidence produces, so the test could not fail.

The only non-trivial constraint is a match **spanning both groups**, which are `0x8E08` apart and
therefore unreachable by a consecutive run. Rescored that way: **no candidate qualifies, and the
answer is unsolved.**

**And the reason it is unsolved is itself informative.** The gate FAILS, so the content function
never reaches `aa 09 00 02 E1 18 20` on that path -- the plane offset is never read there. The two
groups cannot co-occur in a window through this code at all, so a cross-group correlation cannot
succeed while the box is in this state. Either the base must come from somewhere else, or `aa`
pushes a per-object base and there is no single globals base to find.

**Widening the offset set was tried and did not finish the job either.** A static scan of
`aa 09 <u32>` finds **1,742 distinct offsets over 8,136 sites, spanning `0x0` to `0x362B4`** -- a
~222 KB globals area, which is the most useful thing to come out of this attempt. Scored against
the 24 most-used of those, the best base reached **6 of 24 with a span of `0x6C10`** across three
independent clusters (`0x29D2x`, `0x3092x`, `0x2DDDC`) -- genuinely non-trivial, unlike the
previous 3-of-6 -- with two bases tying `0x20` apart.

**And all six candidates read ZERO when watched directly.** Not just the gate word: `B+0x02E118`,
the most-referenced offset in the whole image at 115 sites, read zero at every candidate, and
their gate values were plainly garbage (`0x2B03F3E0`, `0xDBC0DBC1`). So the correlation was still
manufacturing false positives. That confirmation probe carried only a NEGATIVE control, so
strictly it shows the candidates are unconfirmed rather than wrong -- a gap worth naming, since
it is the same shape as everything else in this section.

**THE REASON EVERY CORRELATION FAILED IS THE READ WATCH'S 4,000-ENTRY CAP.** A full-DRAM watch
fills from the hottest code within a few milliseconds of emulated time and never contains the
o-code tick, which runs periodically. Measured: a census of read sites across the whole runtime
(`0x80000000`-`0x80120000`) finds **1,155 sites**, and the widest of them touches **22** distinct
addresses in a `0x1A4` span -- Nucleus task structures at `0x8017B3xx` and `0x80111Bxx`. Nothing
reads the wide scatter a globals fetch would produce, because the sampling never reaches it. A
narrow watch would work but needs the answer in advance.

**One hypothesis was raised, tested and did NOT explain it, which is worth recording so nobody
re-runs it.** `__readWatch` compares RAW VIRTUAL addresses (`rwLo = lo>>>0`), while
`__writeWatch` maps to physical and says in its own comment that it does so to catch a write
"through either alias" -- so read watches really are blind to the uncached DRAM mirror at
`0xA0000000` in a way write watches are not. That asymmetry is real and should be fixed. It was
not the cause here: a census through the alias found the same zero interpreter read sites, with
its own positive control reading 21,546.

**So the gate's value is still unread, and the next step was a TOOL change rather than another
guess. IT IS BUILT.** `__readWatch(lo, hi[, {fromPc: [lo, hi]}])` now records only reads made
from a chosen code range, and a DRAM range is matched on the PHYSICAL offset so it catches either
alias -- the asymmetry with `__writeWatch` is gone. `__readWatchLog()` also reports `capped`,
because a full log is a sample of the busiest few milliseconds and every wrong conclusion this
watch has produced came from reading one as though it were a complete record.

**It passes a self-test that can fail, which is the only kind worth having:**

    A  the CODE chunk, no filter            1803 reads
    B  the CODE chunk, fromPc the fetch     1130 reads, and EVERY pc is 0x80069298
    C  the CODE chunk, fromPc a dead PC        0 reads   <- the filter can refuse
    D  a DRAM range                         matching "physical -- either DRAM alias"

**C is the one that matters** -- a filter that cannot come back empty is not filtering. And the
self-test had to be rewritten once before it passed: its first version read zero everywhere,
because the EPG executes o-code in periodic bursts of about 1031 fetches and a quiet 1.5-second
window legitimately reads nothing. Each window now DRIVES the machine with a key press instead of
waiting for it, which is the same lesson as the 1031-fetch tick two sections above, met again.

**First measurement with it: 364 distinct DRAM addresses read from inside the interpreter** -- and
they are the interpreter's own dispatch table and code region (`0x800692C0`, `0x800692F2`, around
the dispatch at `0x800692E0`), not application globals. All twenty windows still capped, because
the interpreter reads its dispatch table on every opcode. Solved against the 1,742 offsets the
best base explains only 9%, which is not an answer.

**Filtering to the reads that land OUTSIDE the runtime image then worked**, and produced the
first solid new fact in this thread: **60,659 such reads from 551 sites, clustering tightly at
`0x80494D24`-`0x80494DC2`.** That is the o-code VM's working area, and it is corroborated by an
instrument built for something else entirely -- the register dump taken at the opcode-`0x09`
handler had **`s1 = 0x80494DA4`**, squarely inside it. Two measurements that were not designed to
check each other agreeing is worth more than either alone.

**The narrow read watch is now PROVEN to work, which settles an earlier doubt.** Watching
`0x80494DA4` for 2.5 seconds records **49 reads, from `0x8006C706`, `0x80069ADE` and `0x8006B498`
-- the same PCs the filtered capture had independently named.** So the four-byte watch fires
properly, and the earlier confirmation run that read zero at all six correlated bases was a REAL
NEGATIVE rather than a dead instrument. Those bases were simply wrong.

**And the VM context does not hold the globals base.** Of the 64 words at
`0x80494D00`-`0x80494E00`, 23 look like DRAM pointers; 14 were tested by watching
`base+0x025310`, the drawing gate's own word, on a box whose content function runs every tick.
**None of them is read.** Most also read `0x0` at that offset, which is not what a live object
looks like.

**Filtering to a single handler finally answered it, and the answer is that THE QUESTION WAS
WRONG: there is no single globals base to find.**

`0xAA` is the opcode that pushes the base, so its handler (`0x8006D5C8`, resolved live from the
dispatch table) was filtered to on its own. The measurement is the cleanest in this whole thread
-- **no window capped, both controls firing** (the positive control at `0x80494DA4`, and the
`0x09` handler watched separately so "0xAA read nothing" could be told from "no interpreter reads
are visible"). It reads **exactly four addresses**:

    0x80147270 = 0x80315288        0x801D6288 = 0x8045CB64
    0x80147274 = 0x00180000        0x801D6904 = 0x8045CB64

**`0x801D6xxx` IS THE O-CODE VM'S STACK** -- `$sp` was `0x801D6250` at the `0x09` handler, and an
earlier watch logged thousands of writes there from o-code PCs. So `0xAA` reads the **current
frame**, and what it pushes is a per-invocation OBJECT REFERENCE, not a fixed segment base. The
`{0x80315288, 0x180000}` pair it also consults has the shape of a heap descriptor, and `0x180000`
comfortably contains the `0x362B4` offset span -- which is exactly why that pair looked like the
answer and is not.

**Both candidates were tested to destruction.** Six-second windows (the positive control records
157 reads at that length, so the window is ample and a zero is a real negative), three attempts
each, direct and through one dereference: `base+0x025310` is **never read** and reads `0x0`, and
so does `base+0x02E118`.

**THIS RETIRES THE WHOLE LINE OF ATTACK AND EXPLAINS EVERY FAILURE IN IT.** The `0x0253xx` and
`0x02E1xx` numbers are FIELD OFFSETS WITHIN OBJECTS, not addresses in one global segment. No
single base can satisfy a cross-group constraint because no single base exists -- which is the
hypothesis raised three sections above ("either the base comes from somewhere else, or `aa`
pushes a per-object base") now settled by measurement rather than left open.

### THE GATE'S OBJECT IS `0x8045CB64`

**Caught, at the instant it runs, and corroborated.** Sampling was the wrong instrument for this
all along -- a read watch is a SAMPLE of a periodic event, which is how this thread produced eight
wrong answers and, at the end, a run that failed its own control (the `0xAA` handler read four
addresses once and zero the next time). `__traceCalls` is not a sample: it records EVERY execution
of a PC.

**The gate identifies itself.** Tracing the handler for opcode `0x09` -- which pushes the OFFSET --
with `deref: {off: 44}` reads the operand bytes, and they are `00 02 53 10` only at `0x9FC4DB40`:

    off [00 02 53 10]  at=0x9FC4DB41               the gate pushes offset 0x025310
    stk [80 45 cb 64 80 49 4d b8 9f c9 da eb]  at=0x801D6A60
    off [00 02 53 18]  at=0x9FC4DC88               the next sibling, further down

`[$sp+4]` and `[$sp+52]` both hold `0x801D6A60` -- the **o-code data stack pointer** -- and
`deref` follows a frame word to read what it points at, so that second line is the data stack
itself. **The object is `0x8045CB64`**, and the reading is reproducible: two separate gate
executions, at icount 444,556,995 and 467,137,391, give byte-identical stack contents.

**It is corroborated by an instrument built for something else.** The `0xAA` handler -- the opcode
that pushes the object -- was measured loading `0x8045CB64` from BOTH of its stack slots
(`0x801D6288` and `0x801D6904`) in an earlier, separately-designed run. Two independent
measurements naming the same value is worth more than either.

**AND THE EARLIER TRACE CONFIRMED THE DISASSEMBLY AT RUNTIME.** In the same capture the fetch
after the gate's `0x02` at `0x9FC4DB45` is at `0x9FC4DC7F` -- the 297-byte jump past the content
call, taken, exactly as the static listing said. The decode and the machine agree.

**What is NOT yet read is the field's VALUE.** `object + 0x025310` = `0x80481E74` reads `0x0` and
is never fetched as a word at that address, tested with six-second windows against a positive
control recording 157 reads. So the field access is not a plain byte offset from the object
pointer -- there is an indirection or a scaling still to establish, and `0x8045CB64` is a handle
rather than a struct base. That is the next step, and it is now a question about ONE object at ONE
known address rather than a search.

**One correction to method, since it cost a probe.** Peeking the frame AFTER the trace does not
work and the data says so: `[$sp+44]` read `0x9FC4A541`, the main loop, because the frame had
moved on. `$sp` being stable at `0x801D6250` across executions makes the stale frame look current,
which is exactly what makes it convincing. Capture at the instant or not at all.

### THE "OFFSETS" ARE RESOURCE IDS. There was never a globals base.

**Dumping the object settled it in one line.** `0x8045CB64` begins:

    +0  0x52535243  = "RSRC"          +4  0x02FC0000
    +8  0x804643E8 / 0x20FA1     +16  0x80464400 / 0x20FA2     +24  ... / 0x20FA3

A magic word, then `{pointer, id}` pairs on an 8-byte stride whose payload pointers step by
`0x18`. **`0x8045CB64` is a RESOURCE TABLE**, so `aa` pushes the table and `09 <u32>` pushes a
**resource ID** -- not a byte offset into a data segment. That is why eight attempts to find a
globals base failed: **there is no base, and there never was.** It also explains, retrospectively,
why every addressing hypothesis was rejected -- `obj+id`, `[obj+k]+id`, word-scaling and the rest
all assume arithmetic where the machine does a lookup.

**The numbers corroborate it.** The 1,742 distinct `09` values span `0x0`-`0x362B4`, and this
table's ids start at `0x20FA1` -- the same numeric range, which a byte-offset interpretation gives
no reason to expect.

**Walking the table, the gate's id is ABSENT** -- and so are its siblings and the display planes:

    CONTROL  0x20FA1, 0x20FA2, 0x20FA3     found (the ids read out of the dump)
    CONTROL  0xDEADBE                      ABSENT (so the walk can report absence correctly)
    gate     0x025310, 0x025314, 0x025318  ABSENT
    planes   0x02E114, 0x02E118, 0x02E11C  ABSENT

**AND THAT LAST ROW FALSIFIED THE EASY CONCLUSION**, which sent the next measurement after the
right question: if `0x02E11C` were simply a resource this box lacks, the one draw that DID happen
-- `(1,0xE4)` on that plane -- could not have happened.

### Which table the gate consults: there is only ONE, and it is this one

**Measured, not inferred.** Tracing the `0xAA` handler with `deref: {off: 56}` -- the frame slot
the earlier dump showed holding the table pointer -- captures both the pointer and the bytes at
it, so the deref validates itself: a real table reads `52 53 52 43`. Across 318 trace entries,
**exactly one table is ever pushed: `0x8045CB64`**, magic confirmed, and it is the one live at the
gate, captured at the instant twice (icount 448,804,771 and 471,375,796).

**So "there is more than one resource table", written one section above, is WRONG and is corrected
here.** One table, and the gate's id is absent from it -- with both controls passing: the six ids
read out of the table's own head are all found, and a fabricated `0xDEADBE` is not.

**The entry count corroborates the walk.** The header word is `0x02FC0000`, and `0x2FC` is **764**
-- exactly the number of entries the walk collected before running off the end. The table is 764
entries, ids ascending from `0x20FA1`, so it spans roughly `0x20FA1`-`0x212C0`. `0x025310` is not
merely missing, it is **beyond the table's range entirely**.

**WHICH MEANS THE `09` VALUES ARE NOT ALL RESOURCE IDS, and the contradiction resolves a different
way.** Of the 1,742 distinct `09` values spanning `0x0`-`0x362B4`, only a narrow band could be ids
in this table. The discriminator was in the static scan all along and was noted before its
significance was: the `0x0253xx` group is followed by **`0x02`** in every one of its references,
while the plane group is followed by **`0x20`** in 114 of 115. **They are two different
operations**, not one addressing mode -- which is exactly why no single interpretation ever fitted
both, and why the planes work while the gate does not.

**What is solid now:** the gate executes and branches past the draw (deterministic trace and the
disassembly agree); `aa` pushes `0x8045CB64`, a 764-entry `RSRC` table; `09` pushes `0x00025310`;
that value is not an id in that table; and only one table is ever pushed.

**What is NOT established:** what `0x02` actually computes from (table, `0x025310`), and therefore
whether "absent" is the reason the branch fails. That is the next question and it is a single
opcode's semantics -- the `0x02` handler is at `0x80069654`, it was measured reading `0x80147270`,
`0x80147274` and `0x80494D6x`, and it is the only opcode left between the id and the decision.

### The SDK: the vendor's own opcode table, and what it corrected

**OpenTV's GNU SDK ships the o-code specification, and it had been sitting unopened in this
session's scratchpad for hours while the interpreter was reverse-engineered by hand.**
`binut-2.51/include/opcode/oplist.h` defines the opcodes IN ORDER with `HALT` first, so the index
IS the opcode byte -- and it defines exactly **209**, the same count derived independently from
the dispatch table at `0x800692E0`. `ocodedef.h` gives the operand forms. `scripts/ocode-disasm.py`
is now built on it and every checkable range still validates against the traces.

**It agrees with the machine on 95 of the 102 opcodes measured here**, and the seven exceptions are
a naive suffix rule failing to size single-letter parameters (`_N_M`, `_M_IND_FP_N`), not conflicts.
So the SDK is the floor and measurement the ceiling: an opcode this machine has been watched
executing overrides the SDK, because the SDK is 3.2 and this runtime is 1.2S4Bj. Where they agree,
each is a check on the other.

**THREE CONTROL-FLOW FORMS WERE WRONG, and every listing this tool had produced was wrong with
them.** `0x16` and `0x17` are `CALLR_NNNN` and `CALLR_NN` -- **calls, not jumps** -- and `0xBB` is
`RET_4_NN`, a **return** carrying a byte, not a one-byte call. They had been fitted to traces,
which cannot tell a call from a jump: both transfer control, and only the later return
distinguishes them. `0x14 = CALL` was confirmed correct, exactly as the return-address argument
had established.

**AND THE "RSRC RESOURCE TABLE" WAS WRONG TOO.** `0xAA` is `PUSH_DS` -- push data segment. So
`aa 09 <u32> 20` is *push DS, add constant, GET*: an ordinary global fetch, and `0x8045CB64` is
the DATA SEGMENT BASE. `"RSRC"` is simply the first four bytes stored in it. The offsets were
offsets all along; the thing that was missing was the name of one opcode.

### The drawing gate, in plain code at last

    9fc4db3f  aa              push_ds                  DS = 0x8045CB64
    9fc4db40  09 00 02 53 10  add_nnnnnnnn 0x025310
    9fc4db45  02              add                      + index
    9fc4db46  20              get                      load DS[0x025310 + i]
    9fc4db47  36 03           jnz_nn  0x9fc4db4c       non-zero -> DRAW
    9fc4db49  42 01 29        jmpr_nnnn 0x9fc4dc75     zero     -> skip

and the arm it skips loads `DS[0x025314 + i]`, then `DS[0x02E118]` (the plane), then
`scall (1,0xE8)`. The index is built with `shift_2`, so the stride is 4.

**WHAT WRITES IT: four sites, and NONE of them has ever executed.** Every reference to the
location is now classifiable by its following opcode -- `0xB1` is `PUT`, `0x20` is `GET`:

    0x9FC4D545  put   <- push_0     WRITE (clears it)
    0x9FC4D5E8  put   <- a local    WRITE (populates, then scall (1,0x3D))
    0x9FC4D76A  put   <- push_0     WRITE (clears it)
    0x9FC4D80D  put   <- a local    WRITE (populates, then scall (1,0x3D))
    0x9FC4DB3F  get                 READ  -- the gate
    0x9FC4DBC1  get                 READ  -- passes it to (1,0x56)

Across **9,159 distinct addresses** in every trace held, all four writers read **False** and the
gate reads **True**. The readers run; the writers never have.

**The containing function IS entered** -- `0x9FC4D4F6`, 53 direct callers -- and the "never gets
past its first call" reading of that WAS WRONG, which is why it was recorded as suggestive rather
than proven. Two things were missing from it.

**First, the populate path is BOOT-TIME ONLY.** A nine-second driven window in steady state records
the gate executing twelve times and `0x9FC4D4F6` **zero** times. Its earlier count of four came
from boot windows, so no amount of steady-state probing could ever have tested it -- the watch has
to be armed before the boot and left there.

**Armed from t=0 over a whole boot, with the PC filter set to the opcode fetch, it is clean and
uncapped -- 988 reads -- and the return point executes:**

    0x9FC4D4F6  entry (the control)                    5
    0x9FC4D505  callr_nnnn 0x9FC4D941                  5
    0x9FC4D508  THE RETURN POINT                       5     <- the call returns fine
    0x9FC4D50C  loop test                             13
    0x9FC4D52D  get DS[0x025304 + i], then jz         13
    0x9FC4D530  divert -- that slot is empty           8
    0x9FC4D545  WRITER 1, which stores push_0          5
    0x9FC4D5D3  push_fp_nn ; jz_nn 0x9FC4D612          5
    0x9FC4D5E8  WRITER 2, which POPULATES              0

**So the loop runs, examines 13 slots, diverts on 8 of them because the slot is empty, and on the
remaining 5 reaches the writer that stores ZERO -- then diverts around the writer that would
populate, five times out of five, at `0x9FC4D5D3`.** The local it tests there is initialised in the
prologue from another frame slot, so the value that decides this arrives from the caller.

**That is the shape of the whole thing: the table is being actively CLEARED at boot and never
filled.** It is not that the drawing code is unreachable, and not that a resource is missing -- the
code runs, five times, and writes zero each time because what it was given to store is zero.

**`0x9FC4D941` is a 48-slot search**: `push_nn 0x2F` bounds `i` at 47, it tests
`DS[0x025304 + i*4]` and skips zero entries -- counted 289 times with only 15 non-zero.

> **SUPERSEDED TWICE.** The original "six parallel arrays" reading was wrong, the "shifted views
> of one contiguous array" correction below was also wrong, and the answer is 48 records of 40
> bytes with ten word columns — see *THE KEY PATH IS OPEN END TO END* below, where the index
> arithmetic `((i<<2)+i)<<3` is read off the bytecode.

### What the table actually holds -- and a correction

Dumping `DS + 0x025300` for 48 slots shows the six "parallel arrays" are **shifted views of ONE
contiguous array**, not separate columns: `0x025304` is the same data one slot on, `0x025310` four
slots on. That reading is corrected here.

Its contents are recognisable as a **timer/callback table**:

    ... 0, ffffffff, 9fc4dec1, 1, 755a, c350 ...      0xC350 = 50000
    ... 0, ffffffff, 9fc85c1b, 0, 7541, 2710 ...      0x2710 = 10000
    ... 0, ffffffff, 9fc4eb6d, 0, 753f, 4e20 ...      0x4E20 = 20000

Flash bytecode addresses paired with 10000, 20000 and 50000 -- millisecond periods, which fits the
class-8 timer events that drive the whole main loop. So `0x025310` is a FIELD within a record of
this table rather than a standalone draw-list, and "the draw list is empty" is too simple a
reading of it.

> **SUPERSEDED.** Crossing it fires `(1,0xE8)` because that is the timer CALLBACK INVOKE. The
> measurement is sound and the interpretation is not; see *THE KEY PATH IS OPEN END TO END* below.

### THE GATE IS THE BLOCKER, AND CROSSING IT FIRES THE CONTENT CALL

**`(1,0xE8)` has now been called. It had never been called once in the history of this project.**

The gate at `0x9FC4DB3F` is `push_ds; add 0x025310; add; get; jnz_nn -> DRAW; jmpr -> skip`, and
the word it tests reads zero. Poking that column non-zero from outside and watching the NATIVE
DISPATCHER rather than the blit counter:

    (1,0xE8) content call, before the poke      0      <- never, in any run ever taken
    (1,0xE8) after the poke                     3
    (1,0xE8) after a key press                  4
    gate executed / took the DRAW arm / skipped 1 / 1 / 0

**The first forcing attempt got this wrong by measuring the wrong thing.** It counted blits, found
none, and concluded the gate was not the blocker. A blit is several steps downstream of `(1,0xE8)`,
so "no blits" cannot separate *the gate still fails* from *the gate passed and what it drew was
empty*. The native trace separates them, because a single `(1,0xE8)` is unambiguous on a box where
the lifetime count was zero.

**What this establishes and what it does not.** It establishes that the gate is genuinely what
stands between this application and its content calls, and that everything downstream of the gate
-- the arm, the plane fetch, the native -- is wired and willing. It does NOT establish a picture:
**blits are still 8 and `(1,0xE4)` is still zero**, so there is a second stage after the content
call that has not been reached. And the poke is an EXPERIMENT, not a fix: writing application
memory from outside proves what the branch does, and changes nothing about why the branch is false
on a box nobody has poked.

**Planting a payload as well did NOT produce a blit, and the reason is instructive.** The arm
passes two things -- `DS[0x025314 + i]` and `DS[0x02E118]` -- and reading the application's most-used
globals at the verified DS shows what they hold:

    0x02E114 = 0x80430A14   a DRAM pointer -- a real object
    0x02E118 = 0x00000001   a small integer
    0x02E11C = 0x00000001   a small integer
    0x029D20 = 0x8046F7A9   a DRAM pointer
    0x02DDDC, 0x01AD70, 0x029FB4  DRAM pointers
    0x030928, 0x030930, 0x030F88, 0x03130C, 0x025C00, 0x02E200, 0x034054 ... all ZERO

**The small integers are not the problem: `(1,0xE4)` drew the blue screen through `0x02E11C`, which
holds 1, so a small window id is acceptable to the graphics module.** What was wrong was the
payload -- the value planted into `0x025314` was copied from a live slot of the same column, and
that column belongs to the TIMER TABLE, so the content call was handed a timer field and drew
nothing. Five calls, no blit, and correctly so.

**THAT IS THE BOUNDARY THIS LINE OF WORK REACHES.** The gate is real and crossing it is real -- the
content call fires for the first time in the project's history -- but what the application would
draw does not exist to be drawn. It cannot be synthesised by poking, because the thing missing is
the application's own content, and the long row of ZERO globals above is what that absence looks
like from outside.

### The box has no network identity -- and post-boot config writes cannot test it

**The flash defaults table says so plainly.** `INST = 0` (not installed), and both
`DTON = 0xFFFFFFFF` and `DTTS = 0xFFFFFFFF` -- original network id and transport stream id, unset.
A receiver that does not know which network it is on has nothing to match an incoming NIT or SDT
against, so the firmware's own `[xsi]: == trash handles & memory- free subtables` when we broadcast
is correct behaviour rather than a fault.

**The keys are in DRAM and writable**, four copies of each, found by pattern (they move between
boots, so they are never addressed from memory of a previous run):

    DTON  0x80430694, 0x8043086C, 0x804775D8, 0x80492818    all 0xFFFFFFFF
    DTTS  0x8043069C, 0x80430874, 0x804775E0, 0x80492820    all 0xFFFFFFFF
    INST  0x8043068C, 0x80430864                            both 0

Writing `DTON = 2`, `DTTS = 0x07D4` and `INST = 1` into every copy takes -- the read-back confirms
all three -- and **changes nothing**: the content call stays at zero, blits stay at 8, and all ten
sampled application globals are still zero after six more broadcasts.

**THAT RESULT DOES NOT MEAN THE IDENTITY IS IRRELEVANT, AND IT IS IMPORTANT NOT TO FILE IT THAT
WAY.** The configuration is read during BOOT, and this probe -- like every poke experiment in this
section -- writes at 45 seconds, long after the SI layer has initialised from it. The measurement
supports exactly one claim: *writing the identity after boot changes nothing*. Whether a box that
BOOTS with an identity behaves differently is untested.

**And that is the general shape of the limit this whole line of work has reached.** Every
poke-based experiment here acts after the application has established its state, while the state
that matters is built during boot.

**NVRAM was the obvious way out and it is NOT where the configuration lives.** Searched directly:
none of `DTON`, `DTTS`, `INST`, `DTFR`, `RFCH` or `PSPF` appears anywhere in the 16 KB store, and
the DRAM copies read `0xFFFFFFFF` again on the next boot -- so the configuration is rebuilt from
the flash defaults table every time and last run's writes did not survive, as expected. What NVRAM
DOES hold is a set of named stores:

    .stbconfig   .cpprotconfig   batch   favchn   blacklist   ladm   pin   OTVx   GBR

574 non-blank bytes of 16,384.

### What this all points at: the box has never been INSTALLED

`INST = 0`. `DTON` and `DTTS` unset. No services, no line-up, and a table of application globals
that is almost entirely zero. Those are not separate faults -- **they are what a Digibox looks like
before its first-time installation has run**, and the interface for that is sitting in the flash
alongside the EPG's: `LNB SETUP`, `Now Scanning`, `No satellite signal`, `Searching for listings`.

A real box gets `DTON` and `DTTS` from the NIT it receives DURING installation, and writes `INST`
when the user completes it. This box has never done that, so it has no identity; with no identity
the SI layer has nothing to match arriving sections against, and `[xsi]: == trash handles &
memory- free subtables` follows correctly; with no services the application has nothing to put in
its globals; and with empty globals every screen it could draw is empty. **One cause, and the whole
chain of symptoms hangs off it.**

**So the question that matters is no longer "why does it not draw" -- that is answered -- but "why
does the installation flow not run", which is `sky-eluc.14` and already has an issue.** The box
should be showing an installation wizard rather than a blue screen, and it is not.

> **WITHDRAWN.** "Why does it not draw" was NOT answered, and this whole chain -- no identity, no
> services, therefore empty globals, therefore nothing to draw -- is an inference rather than a
> measurement. Measured later: a key press runs 9.6M firmware instructions and 673 natives, selects
> a screen, opens its object and calls the paint native, and still writes no pixel anywhere in 32 MB.
> The application has plenty to show. See *THE KEY PATH IS OPEN END TO END* below.

### sky-eluc.14 ANSWERED: the box marks itself INSTALLED during boot, from `0x800F9A02`

**The application does not skip installation because it fails to reach it. It skips installation
because it believes it is already installed.**

`DS[0x0001ACB0]` resolves to `0x80477814`, inside the config block in DRAM -- the same structure
holding the `DTON` and `DTTS` copies at `0x804775D8` and `0x804775E0`. On a running box it reads
**1**, while the flash defaults table says `INST = 0`. So something sets it, and a four-byte write
watch armed from t=0 across a whole boot names exactly one PC:

    pc 0x800F9A02   0x80477814 <- 0x00   icount 149,210,673
    pc 0x800F9A02   0x80477815 <- 0x00   icount 149,210,680
    pc 0x800F9A02   0x80477816 <- 0x00   icount 149,210,687
    pc 0x800F9A02   0x80477817 <- 0x01   icount 149,210,694

Four one-byte stores, seven instructions apart, from a single PC -- an unaligned big-endian word
write of **1**. That is the moment this box decides it has been set up.

**Clearing the flag afterwards does nothing**, and that is consistent rather than disappointing:
flag `1` -> `0`, read-back `0`, and across eighteen seconds of idle and six key presses the content
call stayed at zero and blits stayed at 8. The screen was chosen at boot, which is the same
post-boot limitation every poke experiment in this section has run into.

**So the chain inverts.** The box is not stuck BEFORE installation waiting to be set up -- it is
stuck AFTER it, believing setup is done while holding none of the state setup would have produced:
no network identity (`DTON`/`DTTS` unset), no services, and application globals that are almost
entirely zero. Every symptom follows from that one write.

**THE OBVIOUS SUSPECT WAS OUR OWN NVRAM IMAGE, AND A COLD BOOT FALSIFIES IT.** The theory was
that the shipped 574 non-blank bytes -- including a `.stbconfig` store -- were saved from a box
that HAD been installed, and that the box was reading its predecessor's state. `--cold` gives a
fresh browser profile and therefore a genuinely blank NVRAM, and the box still comes up with
`INST = 1`:

    cold: true, 92.3 s, 447,149,298 instructions, 42 tasks
    NVRAM non-blank after the boot   486 bytes   (it was blank at the start -- the box wrote these)
    INST at 0x80477814               0x1         <- unchanged
    DTON / DTTS                      0xFFFFFFFF / 0xFFFFFFFF
    (1,0xE8) content call            0
    blits                            8

**So the box marks itself installed regardless of NVRAM, and the source of that `1` is still
unidentified.** The flash defaults table says `INST = 0`, so `0x800F9A02` -- which looks like a
generic byte-at-a-time unaligned config copy -- is reading its `1` from somewhere other than that
table. That is the open end.

**The cold boot also bought three lines of commentary a warm one never prints**, and one of them
settles what the running application is:

    #INTPRT[READY] init.
    Cannot program connect to com_0.passive.
    [BGLOAD] Searching for IEPG.
    [BGLOAD] analizing bank 1.
    #INTPRT[RUNNING] running:0x2.

**`Searching for IEPG` immediately before `running:0x2` is the loader naming what it went to find.**
Application `0x2` IS the IEPG. There is no second application to launch and no installation app to
start: the interactive EPG is what is running, and it is running in its installed state.

### Where the 1 comes from: a static initialiser in the flash DATA chunk -- and clearing it changes nothing

**Caught in the act.** `__breakWhen(0x80477817, 0x01)` stalls the machine inside the store itself
rather than relying on a polled breakpoint, and the register file at that instant names everything:

    v1 = 0x80477817    the destination byte
    v0 = 0x9FCBB460    FLASH, and the bytes there are 00 00 00 01
    a1 = 0x9FCA07AC    FLASH, and the bytes there are "RSRC" -- the DATA chunk
    a0 = 0x8045CB64    DS, the destination base

**`0x800F9A02` is the runtime copying the application's initialised data segment from flash into
RAM.** The `1` is not computed, not read from config, and not in NVRAM: it is a **static
initialiser in the application image**, and it checks out statically -- flash `0x9FCBB45C` holds
`0x00000001`, which is `DS + 0x0001ACB0` given the real chunk base of `0x9FCA07AC` that register
`a1` supplied. (An earlier hand-computed base of `0x9FCA07A4` was eight bytes out and read
`0x36000000` there, which is exactly the sort of carried-number error this file keeps warning
about; the live register settled it.)

**So the firmware was dumped from a box that had been set up, and that state ships inside the
application's data.**

**AND CLEARING IT DOES NOT PRODUCE AN INSTALLATION FLOW.** `__flashPoke` writes the flash array in
memory, before the data-segment copy runs, so the application can be booted believing it has never
been installed. It works exactly as intended and the result is negative:

    flash 0x9FCBB45C   00 00 00 01  ->  00 00 00 00
    INST after boot    0x0                     <- the application agrees it is not installed
    (1,0xE8)           0
    blits              8
    boot commentary    identical to an installed boot

**That breaks the last link of the inference chain that led here**, and the break is worth more
than the chain was. `DS+0x0001ACB0` is a flag the application holds and a flag the config system
names `INST`, but it is NOT the switch between the EPG and the first-time-installation flow --
because a box that reads it as zero behaves identically. Whatever selects that flow is something
else.

**What the detour bought, since it was not nothing:** the exact mechanism and address of the
application's data-segment initialisation (`0x800F9A02`, flash `0x9FCA07AC` -> DS `0x8045CB64`), a
verified DATA chunk base, and the ability to change any of the application's initialised data
before it boots -- which is a far better lever than poking RAM after the fact, and is the one tool
that escapes the post-boot limitation everything else in this section ran into.

### What selects the installation flow: NOT found, and a whole class of search is ruled out

**The installation screens are real, resident, and identifiable.** With the DATA chunk base now
verified (`0x9FCA07AC` from register `a1`, not the hand-computed `0x9FCA07A4`), every string
resolves byte-exact in RAM and four of them match a resource-table pointer exactly:

    Searching for listings   RAM 0x8046765F   resource id 0x51398
    Now Scanning             RAM 0x80468088   resource id 0x514C7
    LNB SETUP                RAM 0x8046780C   resource id 0x51550
    No satellite signal      RAM 0x80468CF7   resource id 0x51B66

**And the application's code never names any of them.** Each id occurs exactly ONCE in the entire
2 MB image -- in the resource table itself -- and a search of every `push_nnnnnnnn` constant in the
code chunk (248 distinct values over 859 sites) finds **no group-5 constant at all**, and only two
group-2 ones. The constants the code does push are overwhelmingly config FourCCs: their high halves
are `PS`, `SD`, `DT`, `LN`, `SS`, `SI`.

**So resources are reached by COMPUTED id, never by a literal**, and that rules out the entire
approach of finding a screen by searching for its id. It is a negative result with real value: any
future attempt to locate a particular screen's code by hunting its resource id will find nothing,
and would otherwise look like evidence the screen is dead.

**What that leaves.** The installation UI exists, is loaded, and is addressable -- but which screen
the application shows is decided by data it computes rather than by named references a static
search can follow. Finding the selector therefore means following the application's own startup
o-code, which is now readable: the disassembler is built on OpenTV's opcode table, validates
against the traces, and the boot path can be captured with a PC-filtered read watch armed from
t = 0. That is a piece of work rather than a probe, and it is the honest next step.

**And one lever is now in hand that was not before:** `__flashPoke` on the DATA chunk changes any
of the application's INITIALISED DATA before it boots, which is the only way found in this whole
investigation to influence state the application establishes at startup. Everything else acts too
late.

### WHAT SELECTS THE SCREEN: `DS[0x00025A80]`, and it is now under control

**Found by following the startup o-code, which is what the corrected disassembler made possible.**
Tracing the three call handlers with `deref` yields every o-code call of a boot with its site and
target -- 368 calls, deterministic, no sampling -- and the startup chain reads straight off it:

    9fc4a416 -> 9fc4a4f0     the application entry (a real call, not an inferred one)
    ...
    9fc73235 -> 9fc4e00e     THE SCREEN BUILDER
       9fc4e031 -> 9fc4e159
       9fc4e1b4 -> 9fc50799
       9fc4e208 -> 9fc4df3c
          9fc4df50 -> 9fc4d4f5     the populate function
             9fc4d505 -> 9fc4d941  the 48-slot search
    9fc728d4 -> 9fc72a7f     THE DRAW -- once, ever

**The dispatcher at `0x9FC7320C` is the application's message pump**, and it reads as source now:

    pushs_ind_fp_nn ; stoiu ; pop_fp_minus16      the message CLASS
    push_2 ; jeq_nn 0x9FC7323E                    class 2  -> a 9-case switch (keys)
    push_nnnn 0x0100 ; jeq_nn                     class 0x0100
       pushs_m_ind_fp_n 0x1C                        the code, from [msg+0x1C]
       push_nnnn 0x0FA3 ; jne_nn                    code 4003?
       push_0 ; callr 0x9FC4E00F                    -> BUILD THE SCREEN

The class-2 switch table decodes cleanly and self-consistently -- `C7 <u16 count> <count x u16
offset>`, count **9**, offsets relative to the table, and case 0 lands on the byte immediately
after the table's last -- so there are nine key-driven screens.

**And the builder stores its argument as the screen number:**

    9fc4e00f  push_fp_8                            the argument
    9fc4e012  push_ds ; add 0x00025A80 ; put       DS[0x025A80] = it      <- THE SELECTOR
    9fc4e019  push_0  ; add 0x00025A84 ; put       three siblings zeroed
              push_0  ; add 0x00025A88 ; put
              push_0  ; add 0x00025A8C ; put
    9fc4e031  callr 0x9FC4E159                      build it

**The caller hardcodes `push_0`. One byte of flash decides which screen this box shows.**

**PROVEN, not inferred.** `__flashPoke` changed `0x9FC73234` from `0x75` (`push_0`) to `0x76`
(`push_1`) before the boot:

    patched byte      0x75 -> 0x76         (read back both ways)
    DS[0x00025A80]    0x1                  <- the application built screen 1
    siblings          0, 0, 0              <- zeroed by the builder, as the code says
    new native        (1,0x3D)             <- never seen in any previous run on this box

`(1,0x3D)` is the native the populate path calls, so screen 1 runs genuinely different code. Blits
stayed at 8, so screen 1 does not draw either -- **but that is now a content question rather than a
control question, and those are very different problems.** The application's screen selection is
identified, addressable and demonstrably steerable from outside.

**SWEPT ALL FIVE, one boot each, screenshot each.** `0x75`-`0x79` are `PUSH_0`..`PUSH_4`, all one
byte, so every substitution keeps the instruction stream valid:

    screen  patch        selector  boot natives  (1,0xE4) draws  blits  picture
    0       0x75 (none)  0x0       53            1               8      blue
    1       0x76         0x1       69            2               8      blue
    2       0x77         0x2       69            2               8      blue
    3       0x78         0x3       69            2               8      blue
    4       0x79         0x4       69            2               8      blue

**The selector steers the application and the screens are all the same picture.** Screens 1-4 do
measurably more than screen 0 -- sixteen extra natives and a SECOND `(1,0xE4)`, including
`(1,0x3D)` and `(1,0x45)` which never fire on screen 0 -- so the branch is real and the code
differs. And all five screenshots are **byte-identical**, same md5, 2940 bytes, the size a
solid-colour PNG compresses to.

**Screens 1 to 4 are also identical to each other**, which says the selector value beyond zero does
not pick different content on this box -- every one of them reaches the same "nothing to show"
path. A second window is opened and filled, and what goes into it is empty.

**So the sweep confirms the diagnosis rather than escaping it.** Control over which screen the
application builds is not the missing piece; the missing piece is that no screen has anything to
put in itself. That is the same wall the forced gate hit, reached from the opposite direction, and
two independent routes arriving at it is worth more than either alone.

### THE KEY PATH IS OPEN END TO END, AND THREE EARLIER READINGS OF THIS FILE ARE WRONG

**The question that produced this section was the right one: a box with no satellite signal should
still show its menus.** It does not, and the reason is not what this file said.

**First, the application is not idle on a key press and is not short of things to do.** The read
watch with the PC filter pinned to the interpreter's opcode fetch, pointed at the whole application
code chunk, turns the log into an execution trace of the o-code. Idle runs 554 fetches in three
seconds over 528 addresses. One key press runs far more, over addresses the idle loop never
touches:

    key   logical   o-code fetches in 3 s   addresses idle never visits
    0x7D   0x700     4000 (capped)           1398
    0x0C   0x100     4000 (capped)           1102
    0x3C   back up   3706                     946
    0xF5   0x802     4000 (capped)            894
    0x83   0x00E     4000 (capped)            694
    0x5C   select    2001                     363

**And it is firmware code, not just bytecode.** Profiled across one press: **9.6 million firmware
instructions** and **20,142 distinct PCs against 6,854 while idle** -- 13,288 addresses of MIPS code
that idle never executes. Nothing is bailing out in the native dispatcher.

**Second, the key reaches the application's real handler.** The event loop is `0x9FC4A538`: it calls
`scall (1,0x26)` to wait for an event, reads the class from the event record and dispatches. Class
**2 is a key**, and the handler is `0x9FC4A6A0`:

    9fc4a6a0  pushs_fp_nn -17 ; push_nn 7 ; jeq 0x9FC4A6AC     only sub-type 7 is a key press
    9fc4a6a9  jmpr 0x9FC4A825                                  anything else leaves
    9fc4a6ac  push_fp_nn -8 ; push_ds + 0x01BB74 ; put         remember the code
    9fc4a6b5  callr 0x9FC72D40 ; jz 0x9FC4A6FC                 the enable test
    9fc4a6c1  ... a six-code whitelist ...
    9fc4a6f6  push_0 ; jmpr 0x9FC4A8FD                         swallow the key
    9fc4a6fc  callr 0x9FC913C7 ; callr 0x9FC4D9E8              THE REAL PATH
    9fc4a715  ... switch the key code to a screen ...

`0x9FC72D40` is three instructions -- `push_ds ; add 0x018868 ; getc ; ctoiu ; not ; ret` -- so it
returns `!DS[0x018868]`, a single byte that is the box's "the remote is live" flag. The message pump
clears it at `0x9FC7329C` and sets it at `0x9FC732DF`; three more writers exist at `0x9FC9ABDF`,
`0x9FC9ACC7` and `0x9FC9ADD5`.

**It was a plausible suspect and it is NOT the blocker**, which is worth recording because the
disassembly makes it look like one. Measured over five presses on a live box, the byte reads 1 when
it matters and the key takes the real path:

    press    flag   handler entered   swallowed   real work 0x9FC4A6FC   content 0x9FC4D9E8   natives   blits
    0x7D      1           2               0              1                     1               669       0
    0x0C      1           2               0              1                     1                26       0
    0x83      0           2               0              1                     1               153       0

From there the chain runs to the end without stopping: `0x9FC725AB` (go to screen N, range-checked
1..202) -> the screen table at `DS[0x014080 + n*8]`, which is **non-zero for the screen the key
asks for** -> `0x9FC7287A`, which opens the screen object through `0x9FCA0634`, posts itself a
message of class `0x0100` code `0x0FA3` with `scall (1,0x56)`, and calls `0x9FC72A7F`:

    9fc72a7f  push_ds + 0x02E11C ; get ; scall (1,0xE4)     THE PAINT
    9fc72a89  push_0 ; callr 0x9FC72ABE                     refresh the palette if it is dirty

**Third, `0x9FC4D4F6` is the TIMER REGISTRATION function and `0x025310` is not a draw list.** This
file previously called it "the populate function" and read the table at `DS+0x0252F8` as a draw
list whose emptiness was why nothing appeared. Both readings are withdrawn. The index arithmetic is
`((i<<2) + i) << 3` = **i * 40**, so the table is 48 records of 40 bytes with ten word columns at
`0x0252F8, 0x0252FC, 0x025300, 0x025304, 0x025308, 0x02530C, 0x025310, 0x025314, 0x025318,
0x02531C` -- genuinely parallel fields of one record, which also retires the "shifted views of one
contiguous array" correction that replaced the original mistake.

The signature is `register(timerId, periodMs, repeat, userData, callback)`, and all **53 call sites
decode**:

    9fc4df50   push 30017 ; push 10000 ; push 1 ; push 0 ; pushea_pc 0x9FC4EB6D
    9fc85c5d   push 30042 ; push 50000 ; push 0 ; push 0 ; pushea_pc ...
    9fc8589b   push 30008 ; push 14400000 ; push 0 ; push 0 ; pushea_pc 0x9FC8594C

Ids 30001-30101 and 60051, periods of 150 ms to four hours. **The argument the so-called drawing
gate at `0x9FC4DB3F` tests is `userData`, and every one of the 53 registrations passes `push_0` for
it.** So the gate is a timer-expiry check asking "does this timer carry a user pointer", the answer
is correctly no, and forcing it fired `(1,0xE8)` because that is the callback invoke -- not because
a drawing decision had been crossed. The whole "the draw list is empty, the application has nothing
to show" line of reasoning rests on this table and does not survive it.

### WHAT IS ACTUALLY WRONG: THE BOX LAYS OUT A SCREEN AND NEVER RASTERISES IT

**The visible plane is untouched, and this was read in full rather than sampled.** The display list
has one descriptor -- 720x576, 8 bpp, fields at `0x80584048` and `0x805B6A58`, CLUT at
`0xA05E9468`. All 207,360 bytes of each field read **palette index `0xDC` and nothing else**, before
a key press and after it. The screen is 100% background.

**And no drawing is ever submitted to the hardware.** A blit here is a 40-byte DMA descriptor at
`0x80108A60 + 40*12` copied by DMA channel 12 into the blitter's command registers. Across a key
press: `blitCount` unchanged, `dmaLog` unchanged, and a full-page signature diff of all 32 MB of
DRAM shows page `0x80108000` **not among the 27 pages that changed** -- the descriptor is not even
written.

**The 27 that do change are the application's own state, and one of them is interesting**:
`0x800FD000`, `0x80105000-0x80107000`, `0x8011B000`, `0x80129000`, `0x80162000`, `0x80173000`,
`0x80176000-0x8017C000`, `0x8018A000`, `0x80190000`, `0x801CC000`, `0x801D5000-0x801D7000`,
`0x80215000`, `0x802AF000`, `0x80312000`, `0x8042F000-0x80431000`, `0x8046B000`, `0x80475000`,
`0x80481000`, `0x80491000`, `0x80494000`, and **`0x805E9000` -- the CLUT's own page**. The box
prepares a palette for a screen it then does not draw.

**The decisive diff is between the one time this box DOES blit and a key press.** Four 720x144 fills
of index `0xDC` at icounts 196.3M-197.7M are the blue screen; they are the only blits in the
machine's life. Tracing every native from t = 0 and censusing the window before the first fill
against the census of a key press:

    called before the blit, never on a key press
      85x (1,0xC7)   49x (1,0x2D)   (1,0x13) (1,0x17) (1,0xBA) (1,0xD0) (1,0xD2)
      (1,0xD7) (1,0xDB) (1,0xDC) (1,0xE3) and module 2, 3, 8, 9, 0x2A, 0x37, 0x48 calls

    called on a key press, never before the blit
      62x (1,0x57)  44x (1,0x5D)  40x (1,0x62)  40x (1,0x7D)  36x (1,0x75) (1,0x76)
      26x (1,0xCB)  21x (1,0x6E)  18x (1,0x58) (1,0x63) (1,0x7A) (1,0x81) (1,0x82) (1,0xAB)

**Two disjoint families.** The key press runs what looks like measurement and layout; the blit
window runs what looks like rasterising. `(1,0xE4)` -- the paint -- is called on both.

**`(1,0xC7)` has exactly TWO call sites in the whole 353 KB code chunk**, both inside `0x9FC72ABE`,
which is the palette loader and opens with a dirty check:

    9fc72abe  arg -> fp-20 ; if arg != 0 : DS[0x0146F0] = 1      force
    9fc72acd  if DS[0x0146F0] == 0 -> return                     NOT DIRTY, nothing to do
    9fc72ad8  DS[0x0146F0] = 0
    9fc72ae0  n = DS[0x0146EC] ; ptr = DS + 0x0146F4 + n*4
    9fc72af7  push [ptr] ; push 6 ; scall (0,0x0A)               fetch the palette resource
    9fc72b12  push 0xFF ; push 1 ; push 1 ; [ptr]+4 ; DS[0x02E118] ; scall (1,0xD5)
                                                                 ... then the (1,0xC7) loop

So the 85 calls are a palette upload behind a dirty flag, and on a key press the flag is clear.

**WHAT IS ESTABLISHED:** the key reaches the application, the application selects a screen, opens
its object, posts itself the build message and calls the paint native `(1,0xE4)`, running 9.6M
firmware instructions and 673 natives on the way -- and no pixel is written anywhere in 32 MB and no
blit is submitted.

**WHAT IS NOT:** which native in the pre-blit family is the missing step, and what state gates it.
`(1,0x2D)` is the better lead of the two -- 50 sites across 47 functions, so it is a general
primitive rather than the palette special case, and it runs 49 times before the blit and never on a
key press. The honest next move is to identify what `(1,0x2D)` and `(1,0xC7)` are by following the
native dispatcher's resolution of module 1 into firmware, rather than by inferring from call
counts.

> **DONE — see *THE NATIVES HAVE NAMES NOW* below, and the lead moved.** `(1,0x2D)` is a registry
> insert and not a raster op at all, so it was the wrong one of the two to chase; `(1,0xC7)` is the
> palette upload and is confirmed not key-driven. The native that matters is `(1,0xD2)`, which sets
> a plane's background colour — it is the producer the boot path calls and the key path does not,
> and `(1,0xE4)` then drains what it produced. Everything else in this section stands, including
> `(1,0xE4)` being called on both.

### Two instrument failures from this session, both of the shape this file keeps warning about

**A 4 MB scan of a 32 MB machine reported on the population.** The page-signature diff that first
answered "where do the pixels go" swept `0x80000000-0x80400000`. The framebuffer is at
`0x80584048`. It returned "nothing changed" over a region that did not include the thing being
asked about, and the number it printed -- 8 changed pages -- was real, correct, and about somewhere
else.

**And the replacement had a hole its own control caught.** The next version hashed the first 512
bytes of each 4 KB page; its falsification poke landed at page offset `0xAF0` and the scan did not
see it. `scanCanSeeAChange: false` is what a scan that can only miss looks like from the inside, and
without the poke the run would have reported "nothing changed" again, twice as convincingly. The
version that stands hashes every 4th byte of the full page and reports the control's page.

### THE NATIVES HAVE NAMES NOW, AND THE PAINT RUNS ON A KEY PRESS OVER AN EMPTY QUEUE

*Answers `sky-eluc.26`. The census above stands; what it was read to mean does not.*

**Module 1's native table resolves statically, so any native can be decompiled and NAMED.** The
array at `0x9FC29F04` is 236 **four-byte pointers**, each to an 8-byte `{implementation, argument
descriptor}` record — it is *not* an array of 8-byte records. That misreading is self-consistent
enough to survive a review: the records sit immediately after the array, so entry *n*'s
"implementation" reads as the pointer to record *n* and its "descriptor" as the pointer to record
*n+1*. **Every descriptor then comes out as implementation + 8, and that is the tell.**

The implementation is a MIPS16 address (bit 0 set) of a 16-byte thunk — `lw rx,off(pc) ; jalr rx` —
so the real function is the word at `((lw address) & ~3) + off`. 210 of the 236 have that shape; the
other 26 do the work inline. The descriptor is a byte string `[return type][arg type]…[0x00]`, which
makes the **argument count exact**: `(1,0xD5)` declares five and its one known call site pushes five.

**Checked against the running machine, because walking the flash and reporting on the box would be
the two-record trap.** The dispatcher indexes a table of `{function array, count}` pairs at the word
in `0x8006E71C`, built at boot in DRAM; `scripts/digibox-probes/native-table.js` reads it live and it
holds the same record pointers as the flash copy. `scripts/opentv-natives.py` is the resolver, and
`./ctl.sh decompile <fn>` then reads the function.

    (1,0x57)  0x80082A6C  new widget(class)      class <= 6, vtable table at 0x9FC27AE0
    (1,0x62)  0x80082738  set anchor obj+0x0D    then re-layout
    (1,0x76)  0x80080FD0  set pair obj+0x24/26   then re-layout
    (1,0x2D)  0x800855AC  registry insert        {u16,int,int} into 16-slot blocks -- NOT raster
    (1,0xD2)  0x80084388  set a plane's background colour, replicated to its bit depth
    (1,0xE4)  0x800837A0  drain a plane's command ring: while (produce != consume) executeOne()

**`0x80082BCC` is the layout engine** the two setters share: it measures the widget through
vtable+`0x04`, resolves one of sixteen anchor cases from `obj+0x0D` (the byte `(1,0x62)` writes —
`>> 1` for the centred cases, subtract the full extent for the trailing ones), writes the resolved
position to `obj+0x12`/`+0x14` and applies the bounds through `0x80082D50`. The class table holds
seven entries; **two of them (`0x801017B4`, `0x801017CC`) point into DRAM**, so those classes are
registered at runtime.

#### The window record, read off a live box

The plane array is at `*0x80105E9C` with `*0x80106F24` records of **100 bytes**; `*0x80106F20` is a
gate the validator tests. `(1,0xD2)` and `(1,0xE4)` open with the identical check, which is what
identifies the family — and the family can be found structurally rather than by guessing from names:
scan the RAM image for MIPS16 `lw rx,off(pc)` instructions that RESOLVE to a literal pool word
holding `0x80105E9C`, then take the enclosing native. (The base is the instruction word-aligned,
except in a jump delay slot where it is the jump's address.) **Attributing pool words to the nearest
preceding function instead is a proximity guess and it was wrong** — it put `(1,0x3A)` in the family,
which is a list iterator taking a callback in `$a1`. The 34 real members:

`(1,0x0E) (1,0x13) (1,0x1F) (1,0x20) (1,0x27) (1,0x29) (1,0x2A) (1,0x9A) (1,0x9E) (1,0xC3)
(1,0xC4) (1,0xCB) (1,0xD0) (1,0xD1) (1,0xD2) (1,0xD3) (1,0xD4) (1,0xD7) (1,0xD8) (1,0xDA)
(1,0xDC) (1,0xDD) (1,0xDE) (1,0xDF) (1,0xE0) (1,0xE1) (1,0xE2) (1,0xE3) (1,0xE4) (1,0xE5)
(1,0xE6) (1,0xE7) (1,0xE9) (1,0xEA)`

    +0x00  kind (1 = live)      +0x40  bit depth (2, 4 or 8)
    +0x50  PRODUCE index        +0x54  CONSUME index
    +0x58  background set flag  +0x5C  background colour, replicated across the word

**Window 1 is the visible plane and reads `0xDCDCDCDC`** — exactly the palette index the four
720×144 fills wrote. Window 0 is a trap: the validator errors when the id is 0 while the gate reads
0, and the error handler does not return, so a hand-call burns its whole instruction budget and
leaves the machine wedged, which then reads as "the drain did nothing". That cost one run.

#### THE FINDING: the paint IS called on a key press, and the queue it drains is empty

Natives and blits interleaved by instruction count (`scripts/digibox-probes/who-drew-the-blue.js`),
which is what settles attribution — a two-second bucket cannot:

    196111684  (1,0xD2)(window=1, colour=0xDCDCDCDC)     the producer
    196316013  (1,0xE4)(window=1)                        the drain, 204k instructions later
    196328206  BLIT fill 8bpp 0x80584048 @0,0   720x144 value 0x000000DC
    196775667  BLIT fill 8bpp 0x80584048 @0,144 720x144
    197229037  BLIT fill 8bpp 0x80584048 @0,288 720x144
    197686683  BLIT fill 8bpp 0x80584048 @0,432 720x144

So the blue screen is **produce-then-drain**, and both halves work. And on a key press the drain runs
**twice** — `(1,0xE4)` ×2 in the census bucket for the press — with **no background-colour producer
anywhere in the press**. Of the whole plane family the press reaches exactly one other member,
`(1,0xCB)` ×26; the window that draws reaches `(1,0xD2)`, and the periodic burst reaches
`(1,0x13) (1,0xD0) (1,0xD7) (1,0xDC) (1,0xE3)`.

**Driving the producer by hand confirms the ring is real.** `(1,0xD2)` on window 1 with colour `0x2A`
returns 1, sets `+0x5C` to `0x2A2A2A2A` and advances **produce 1 → 2**; the consume index stays at 1,
nothing blits, and the screen does not change until something drains it. (`__call` cannot complete
`(1,0xE4)` itself — the function tail-jumps through a jump table, losing the harness's sentinel
return address with `PC left mapped memory at 0x00000000`.)

**So the previous section's reading was right about the paint and wrong about nothing else that
matters.** The families are not as disjoint as they looked — `(1,0x2F) (1,0x33) (1,0x37) (1,0x52)
(1,0x56) (1,0x57) (1,0x75) (1,0xBB) (1,0xBC) (1,0xD5)` all appear in both windows — but the
*pre-blit-only* set is real, and its load-bearing member is `(1,0xD2)`. `(1,0xC7)` is confirmed
**not** key-driven: 100 calls in the first six seconds of an untouched box, none on a press.

**WHAT IS ESTABLISHED.** A key press builds a screen — 62 widgets created, 36 geometry sets, 40
anchor sets, each triggering a measure-and-position pass — and then calls the paint twice over a
queue nothing has written to. Every layer below the application works: producer, ring, drain,
blitter, DMA, plane.

**WHAT IS NOT.** Which producer the widget tree is supposed to reach, and why the layout pass does
not reach it. **`(1,0xCB)` at `0x80083528` is where to start** — the one plane-family native a key
press reaches besides the drain, 26 times. It sets an indexed attribute on an object and applies it
via `0x80082604`, but only `if (classOf(obj) == 1)` and only if the value CHANGED, so either
condition can swallow all 26 calls silently. `0x80082D50`, where the layout pass ends by applying
bounds, is the other thread. Filed as `sky-eluc.27`.

#### The instrument failure that produced a confident wrong answer for half an hour

**`__pcHits` keys its result with the page's `hex32`, which UPPERCASES the hex digits.** A census
that builds the lookup key with a plain `toString(16)` therefore misses **every address containing a
hex letter** and returns a perfectly plausible **zero** for it, while addresses that happen to be all
digits count correctly. `(1,0x2D)`'s shim is `0x80082040` and counted; `(1,0xE4)`'s is `0x80081C58`
and `(1,0xC7)`'s is `0x80081B40`, and both read as never called.

**The output looked nothing like a broken instrument.** It was a 236-entry census returning specific,
varied, reproducible counts, and it supported a tidy story — that the application lays out a screen
and never asks for the paint. It survived two runs. What broke it was an **independent instrument
disagreeing**: the icount trace put `(1,0xE4)` 204k instructions before the fills, on a box where the
census said it had never run at all.

The fix is mechanical rather than attitudinal: the probes now build the key the page's way and a
**missing key throws** instead of returning zero. A census that cannot find its own subject must fail
as a harness error, never report a count.


### THE WIDGET TREE CALLS THE RIGHT THING; ONE WORD AT `ctx + 0x0C` DISCARDS IT

*`sky-eluc.27`. The key press is not missing a producer — it reaches the damage function's doorstep
490 times and is turned away.*

**The chain, decompiled and then counted on one press:**

    0x80082604  apply(obj)    x490   if (obj[2] != 0 && gate(obj) != 0) {
                                         win = ctxOf(obj[0])[0x28];
                                         damage(obj+0x16, win);
                                         if (rec[win][4] == 2 && rec[win][0x38] != -1)
                                             damage(obj+0x16, rec[win][0x38]);
                                     }
    0x800825F4  gate(obj)      x48    -> 0x80085884(obj[0])
    0x80083830  damage(rect,win) x0   <- NEVER REACHED

`&&` short-circuits, so the counts decompose it with no ambiguity: **442 of 490 fail `obj[2] != 0`,
and all 48 that reach the gate get zero back.** The gate is four instructions and leaves no room for
interpretation:

    80085888  bnez a0, +          ; a0 == 0 ?
    8008588a  move v0,zero        ;   -> return 0
    8008588e  lw v1,0x370(pc)     ; else ctx = resolve(a0)          (0x80085004)
    80085890  jalr v1
    80085894  lw v0,0xc(v0)       ; RETURN *(ctx + 0x0C)

**AND THE CONTRAST IS THE WHOLE PRODUCT SYMPTOM.** `(1,0xD2)`, the background-colour setter, calls
**the same `0x80083830`** — its pool word `0x80084820` holds `0x80083831` — but it calls it
DIRECTLY, with the window's own clip rect at `rec+0x24` and no object gate in front. The fill path
was never subject to the flag; the widget path is. That is why this box draws a blue screen and
cannot draw a menu, and it is one branch rather than two subsystems.

#### What the press does right, so these are not the answer

Measured on a settled box (`scripts/digibox-probes/key-press-anatomy.js`):

- **62 widgets created** — class 1 ×40, class 0 ×18, class 3 ×4. So `(1,0xCB)`'s `classOf(obj) == 1`
  requirement is satisfiable and a class mismatch is not the blocker.
- **The drain targets WINDOW 1**, twice — the live 8 bpp plane holding `0xDCDCDCDC`, not some other
  window. Window 0 exists but is 4 bpp with an empty ring. Not a routing fault.
- **The apply path is alive**: 490 calls, 336 `classOf` calls, 26 `(1,0xCB)` attribute sets.
- And window 1's `produce` index never moves off 1.

#### The context record, read live, and who writes it

Captured by breaking on `0x80085894` — the instruction that READS the field, so `$v0` still holds
the context, which is the only way to learn an address that comes out of a resolver rather than a
global. `ctx = 0x8043073C`:

    +0x00 0x80430C64   +0x04 0x00AD0000   +0x08 0x00000001   +0x0C 0x00000000   <- the gate
    +0x10 0x00000000   +0x14 0x80430A14   +0x18..+0x24 0     +0x28 0xFFFFFFFF
    +0x2C 0x00000000   +0x30 0x00000034   +0x34 0x80430BFC   +0x38 0x0A8C0000

A write watch over the whole record across a key press catches **30 writes and not one of them to
`+0x0C`**: fifteen byte-wise stores from `0x80000E28` (the OS `memset`, zeroing the record on
allocation), then field stores from four natives the press runs —

    0x800857AA/B4/BE  inside 0x80085768  (1,0x2F)   x7 on a press
    0x80085512        inside 0x80085500  (1,0x33)   x8
    0x80085920        inside 0x8008589C  (1,0x43)   x4
    0x80085D32        inside 0x80085D10  (1,0x4F)   x8

So the application allocates the record, zeroes it, populates it through four natives, and **the one
word the drawing gate reads keeps its `memset` zero**.

#### Forcing the gate open: it IS the gate, and the objects are bound to no window

Breaking on `0x80085894` and writing `1` into `[$v0 + 0x0C]` before the read executes, for every gate
read of a key press — 320 of them across 20 distinct contexts:

    DAMAGE calls   0  ->  204        the gate is real, and forcing it opens the path
    produce        1  ->  1          and NOTHING is enqueued
    blits          8  ->  8          surface hash unchanged

**The damage arguments say why, and it is a second unset field in the same record.** `apply()` passes
the window as `ctxOf(obj[0])[0x28]`, and it comes out as:

    damage(0x804309D2, 0xFFFFFFFF)     the "no window" sentinel -- the same value apply itself
    damage(0x80430982, 0xFFFFFFFF)     tests rec[win][0x38] against
    damage(0x804306DA, 0x00000000)     window 0 -- the 4 bpp window, NOT the visible plane
    damage(0x8043056E, 0x00000000)

and **never window 1.** The captured record's `+0x28` is `0xFFFFFFFF`, which is exactly that
sentinel. So `+0x0C` and `+0x28` are both at their post-`memset` non-values: **these objects are not
attached to a plane at all.** The gate is not a drawing permission that somebody forgot to set — it
and the window id are two halves of a binding that never happened.

**WHAT IS NOT ESTABLISHED, and the next question is narrow:** which operation binds a context to a
window — setting `+0x28` to a window id and `+0x0C` to non-zero — and why the application never
performs it. Start by decompiling the resolver `0x80085004` and the four natives that populate the
record, `(1,0x2F)` `(1,0x33)` `(1,0x43)` `(1,0x4F)`. The plane family holds obvious candidates for an
attach: `(1,0x1F) (1,0x20) (1,0x27) (1,0x29) (1,0x2A)` are the low-numbered members and none of them
runs on a key press.

**A caution for whoever forces this next.** Window 1's clip rect at `rec+0x24` reads `0x00000000`, so
even a correctly routed damage may clip away to nothing — check the clip before concluding that a
binding fix failed. And driving 4000 breakpoint stalls from a JS loop shares a thread with the
emulator: that probe ran fifteen minutes for twenty seconds of arithmetic, with 3680 of 4000
iterations spinning on no stall.


### `sky-eluc.27` ANSWERED: the binding is `(1,0xE3)`, and THE APPLICATION UNDOES IT

*2026-09-14. Two questions were open — what attaches a widget context to a plane, and why the
application never does it. The first has one answer. The second had a false premise.*

#### The operation, read and then proved

**`0x80085128`, the native `(1,0xE3)`, is the binding, and it is the ONLY thing that writes both of
the fields the drawing path needs.**

    setWindowRoot(windowId, obj)
        rec = windowArray[windowId]                    // *0x80105E9C, 100-byte records
        if (obj != rec[+0x60]) {                       // +0x60 IS THE WINDOW'S ROOT OBJECT
            applySubtree      (rec[+0x60])
            setGateRecursive  (rec[+0x60], 0)          // detach: ctx+0x0C = 0 over the subtree
            setWindowRecursive(rec[+0x60], 0xFFFFFFFF) //         ctx+0x28 = the no-window sentinel
            rec[+0x60] = obj
            setGateRecursive  (obj, 1)                 // attach: ctx+0x0C = 1 over the subtree
            setWindowRecursive(obj, windowId)          //         ctx+0x28 = the window id
            applySubtree      (obj)
        }

    0x800858B0  setGateRecursive(root, v)      walks the subtree writing ctx+0x0C
    0x800858F4  setWindowRecursive(root, v)    walks the subtree writing ctx+0x28
    0x80085D10  (1,0x4F) addChild(parent,child)  links, then INHERITS the parent's gate and window
    0x80085D84           addChild variant, same inheritance

That closes the loop on the two unset fields exactly: they are set together, by one call, over a
whole subtree — and a child added afterwards inherits both from its parent. So a tree whose root is
not a window's root has both at their memset zero and sentinel, which is the state measured here.

**The xref is sound rather than by proximity.** `scripts/mips16-xrefs.py` decodes every MIPS16
`lw rx,off(pc)` in the image and computes its pool target, under both the normal and the
delay-slot PC-base rules; only four instructions in the whole image resolve to a pool word holding
`0x800858F5`, and two of them are inside `(1,0x4F)`, which is the known inheritance path and
therefore this scan's positive control. The tool exists because the *nearest preceding function*
heuristic is what put `(1,0x3A)` in the plane family and was wrong.

**The resolver is subtraction.** `0x80085004` is `ctx = obj - 0x2C`, bounds-checked against the heap
descriptor at `0x80147270`. Proved by two independent captures agreeing: the gate break recorded
`ctx = 0x8043073C` and an `addChild` trace recorded `obj = 0x80430768`.

**Performing those writes by hand draws.** `scripts/digibox-probes/bind-the-tree.js` takes the key
press's 9-node tree, pokes `rec[1]+0x60`, `ctx+0x0C = 1` and `ctx+0x28 = 1` over it, and presses
again: **DAMAGE goes from 0 to 48 calls, every one carrying window 1**, the plane's ring advances
produce 1 → 16, and **five real fills reach the framebuffer**. The binding was the whole of what was
missing from the routing.

#### THE APPLICATION DOES BIND IT. THEN IT UNBINDS IT, 2727 INSTRUCTIONS LATER

`scripts/digibox-probes/setwindowroot-args.js` traced the call with its arguments and `$ra`:

    164838860  setWindowRoot(1, 0x80430A14)   ra 0x80081D3B  -- the (1,0xE3) o-code shim
    164841592  setWindowRoot(1, 0x00000000)   ra 0x80081D3B  -- the same shim, 2727 later

and the firmware's own internals confirm both halves, in order: `setGateRecursive(0x80430A14, 1)`
and `setWindowRecursive(0x80430A14, 1)` on the way in, then `(…, 0)` and `(…, 0xFFFFFFFF)` on the
way out. Both calls come from the application's own o-code, on the same stack.

**So "the application never performs the binding" was false, and it was false in the direction that
matters.** The key press builds its 62 widgets at icount ~320M — **155 million instructions after**
the root was detached — which is why every one of them carries gate 0 and window `0xFFFFFFFF`. The
press is not failing to bind; it is drawing into a tree that was unbound long before it ran.

#### What is in the gap: NOTHING, and that is the finding

`scripts/digibox-probes/the-unbind.js` stops the machine at the first call, arms a census over all
236 module-1 shims, and runs to the second. **Between them the application calls no native at all**
— the only entries in the window are the two `setWindowRoot` calls themselves — and the box's own
printf says nothing across the same span.

That rules out the reading the timing invites. The application did not consult the tuner, the card,
the service list or anything else and then give up; it has no conversation with the firmware in
that window. The decision is made in o-code from state it already held, and **the next instrument is
the o-code itself**, not another native trace: `scripts/ocode-disasm.py` over the bytecode either
side of the `(1,0xE3)` operand, reached the way `scripts/digibox-probes/bytecode-trace.js` reaches
it. Whatever the application is waiting for, it decided before it put the screen up.

#### Two corrections to the previous session's notes

- **Window 1's clip is NOT empty**, and the caution to check it before believing a binding fix had
  failed rested on a misread. `rec+0x24` reads `0x00000000` and `rec+0x28` reads `0x02D00240`;
  these are PACKED HALFWORD PAIRS, `(0,0)` and `(720,576)`. The clip is the whole screen.
- **There are TEN windows, not two.** `*0x80106F24` is 10; windows 0 and 1 have `kind = 1` and the
  other eight are `kind = 0`. Reporting over "both windows" was reporting over a subset that
  happened to be the right one.

#### An instrument failure, and the shape it belongs to

The first run of the binding probe pressed the key 38 s after boot and measured **five** widgets and
no `addChild` at all, against the 62 and 490 this file records. The box had not settled — window 1's
background still read `0x00000000` and reached `0xDCDCDCDC` afterwards — and the session that
measured 62 had waited 80 s before pressing. **A press before the box settles measures a different
machine**, and the count it returns is a real count of something nobody asked about.

The same run then reported a widget-tree root of `0x02020000`. `(1,0x57)` newWidget takes a CLASS —
an integer ≤ 6 — and its ARGUMENT had been fed into a set of objects and resolved, so the probe read
memory at `(class - 0x2C)` and climbed a parent chain out of whatever was there. **An argument is
not a return value.** The fix was to derive the forest from `addChild`, whose two arguments are both
genuine handles, and to report the roots twice over — once by walking `ctx+0x14` in memory and once
by taking the nodes that never appear as a child in the trace — so that a disagreement between the
two is visible rather than silent. The probe now also prints newWidget's arguments, which come back
`{0, 1, 3}` and are the control that proves the diagnosis.


### REFUSING THE UNBIND PAINTS THE SCREEN — the application draws a menu

*2026-09-14, `scripts/digibox-probes/refuse-the-unbind.js`. The decisive experiment for
`sky-eluc.28`, and the first time this box has rendered anything the application laid out.*

Binding the tree by hand AFTER the key press proved the routing and produced nothing to look at —
five fills, all in the background colour, the surface hash unmoved, the screenshot plain blue. That
left the real question open, because **a tree bound after the fact is not the same machine as a tree
that was bound the whole time**: whatever a widget emits when its geometry is set, it emitted while
detached.

So this probe binds nothing. It lets the application do its own bind — which it already does,
correctly — and stops it undoing it. `setWindowRoot`'s entire body sits behind
`if (obj != rec[+0x60])`, so poking `rec[+0x60]` to 0 at a breakpoint on the unbind makes the guard
`0 != 0`, the function returns having touched nothing, and the field is put straight back. Two
pokes, no register writes, no patched firmware.

**The result, with the control first.** The root's `ctx+0x0C` and `ctx+0x28` read **1 and 1** after
the refusal, so the tree really did stay bound. Then one key press:

    newWidget  x62      apply x534      DAMAGE x48, every one carrying window 1
    ring produce/consume  1,1  ->  10,10
    surface hash  0x9825B318 -> 0xB5E7D358      -- IT MOVED
    dominant colour  0xDC (414,704 px)  ->  0x88 (400,368 px)

and the blits are not a repaint of the background. They are a **layout**:

    fill @128,0   592x112     value 0x0055C288
    fill @0,112   720x144
    fill @0,256   720x144
    fill @0,400   720x104
    fill @0,504   720x72
    fill @120,148 480x32      <- six rows, x=120, width 480, pitch 32
    fill @120,180 480x32
    fill @120,212 480x32
    fill @120,244 480x32
    fill @120,276 480x32
    fill @120,308 480x32

Six equal rows, 480 px wide, 32 px apart, inset at x=120. **That is a menu.** The box has gone from
"nothing rasterises" to "the whole screen and its six menu rows rasterise", and it did so without
one peripheral being modelled, one register answered or one chip identified — which settles a
question that was live all afternoon: **the OSD and blitter were never the blocker, and neither was
the video hardware.**

**What is NOT yet there, stated plainly so nobody reads this as finished.** Every fill carries the
same value, `0x0055C288`, so the rows are the same colour as the field they sit on and the
screenshot is a flat light-blue page with one dark rectangle where the old background survives
(`@128,0` leaves `0..128 x 0..112` untouched). There is no text and no row highlight. So the
remaining work is not routing and not geometry — both now demonstrably work — it is **what a widget
draws with**: why every fill resolves to one colour, and where glyphs would come from. That is the
next question, and it is a different one from the one this section answers.

**And the refusal is a PROBE, not a fix.** Nothing in the page or the firmware has been changed; the
unbind still happens on every ordinary boot. What the experiment establishes is that the unbind is
the whole of what stops the application painting — so the fix is to find the o-code branch that
chooses to clear the root, which is `sky-eluc.28`, now with a known-good end state to test against.


### `sky-eluc.28`: THE BRANCH, FOUND — one native returns -1 and the box clears its own screen

*2026-09-14. The o-code that decides, the native it tests, the value it got, and the state that
produced it. Everything in this section was read off the running machine.*

#### The o-code

`scripts/digibox-probes/ocode-at-the-unbind.js` arms `__readWatch` over the EPG module's CODE chunk
from inside a breakpoint on the bind and reads it at the unbind — 48 entries, uncapped — and
`scripts/ocode-disasm.py` turns the gap into this:

    9fc72fb8  ca 03 15        scall (3,0x15)              call it
    9fc72fbb  6b              pop_fp_minus16
    9fc72fbd  67 f8           pop_fp_nn   (fp-8)          store the result
    9fc72fbf  9e f8           push_fp_nn  (fp-8)          load it back
    9fc72fc1  75              push_0
    9fc72fc2  29 3a           jgt_nn 0x9fc72ffe           TAKEN
      ---- 58 bytes skipped: THE HAPPY PATH ----
      9fc72fcd  72 dc dc dc dc  push 0xDCDCDCDC
      9fc72fd9  ca 01 d2        scall (1,0xD2)            set the background colour
      9fc72fdc  callr 0x9fc72a7f
      9fc72fec  jz_nn 0x9fc72ffc
      9fc72ff7  callr 0x9fc4a420
      9fc72ffc  jmpr 0x9fc73031                           and skip the clear
    9fc72ffe  75              push_0                      the object: ZERO
    9fc73000  09 00 02 e1 18  add DS+0x2E118              the window id
    9fc73006  ca 01 e3        scall (1,0xE3)              setWindowRoot(win, 0) -- the clear

#### The native, and the value it returned

`(3,0x15)` resolves through the LIVE module table (`*0x8006E71C`, ten modules, module 3 holding 99
functions at `0x9FC2B2B4`) to `0x80054DDC`, which is four lines:

    int f(void) {
        int r = -1;                                                  // pool word 0x80054F84
        if (*(char*)0x80161D3C != 0 && *(int*)0x80161D40 != *(int*)0x8010161C)
            r = *(int*)0x80161D40;
        return r;
    }

**Measured at the `jr ra`, with the three locations read at the same instant:**

    v0 = 0xFFFFFFFF  (-1)          the default: the `if` did not fire
    *(char*)0x80161D3C = 0         the flag is clear
    *(int*)0x80161D40  = -1        the value was never written
    *(int*)0x8010161C  = -1        the reference

**AND THE BRANCH IS THE OPPOSITE WAY ROUND FROM ITS MNEMONIC.** `push X; push 0; jgt_nn` is
satisfied by **X = -1**, so it means *jump if 0 > X*, not *jump if X > 0*. The two readings put the
box on opposite branches and would have produced opposite investigations — on the wrong one, you go
hunting for whatever wrote a positive value, and nothing ever did. The disassembler's mnemonic is
not evidence about operand order; the returned word is, which is why it was read rather than
inferred. Third time the static reading of this firmware has been backwards.

**So the box clears its own screen because a byte at `0x80161D3C` is zero.** Not because it failed
to bind, not because of the plane, and not because of any hardware: the happy path — which sets the
background and calls two further routines — is skipped because one state flag was never set.

#### What the flag belongs to, and what is NOT yet proven

`scripts/mips16-xrefs.py` finds 30 instructions resolving to `0x80161D3C`/`0x80161D40`, all inside
`0x80054000`–`0x80056000`, so the flag belongs to one subsystem. `FUN_80054570` writes that region
and builds descriptors against three flash strings — **`"video"`, `"image"` and `"av"`** — and
`FUN_80055D84` in the same block dispatches on `*(byte*)(param+0x21)` against `0xF4/0xF5/0xF6`,
which are broadcast table ids. So the block reads as **AV/decoder state fed by SI sections**.

**That is indicated, not established**, and the distinction matters because it is the whole of the
next step: nothing here proves which write sets `0x80161D3C`, nor what precondition gates it.
Successor issue filed.

**And it does NOT mean the video hardware is the blocker.** Refusing the unbind makes this box lay
out and rasterise its menu with no MPEG decoder, no identified ASIC and no answered register — so
whatever this flag guards, the interface does not need it to draw. The application is declining, not
failing.


### `sky-eluc.29`: the flag IS set — 592,817 instructions too late, and it would not be enough

*2026-09-14. What writes `0x80161D3C`, when, and why opening the gate is necessary but not
sufficient.*

#### Finding the stores, and the tool that found them

`scripts/mips16-xrefs.py --stores` tracks the register an `lwpc` wrote forward through the
MIPS16 memory ops, so "who stores to this" is answered rather than "who mentions this". Its
decoder is validated on every run against a known case — the `lb +0` and `lw +4` inside
`(3,0x15)` itself — because a decoder that finds nothing and a firmware that stores nothing look
identical.

**Nine instructions write the byte, and it is not a boolean — it is a state machine taking 0, 1,
2 and 3.** Two functions drive it: `FUN_80055D84`, a message handler dispatching on
`*(byte*)(param+0x21)` against `0xF4/0xF5/0xF6` to three handlers, and `FUN_8005803C`, an "if
idle, begin" that sets it to 2 when it is 0.

**The tool's blind spot, stated because it produced a wrong-looking answer once already.** It sees
only MIPS16 PC-relative addressing, not MIPS32 `lui`/`addiu` pairs, and it matches a literal
address rather than a base-plus-offset — so scanning for stores to `0x8010161C` found none, when
that word is simply `+12` into a struct based at `0x80101610`. An empty result from it means "not
found by this method", never "not written".

#### It is a race, and the box asks before the answer exists

    164838860   setWindowRoot(1, 0x80430A14)     the bind
    164841592   setWindowRoot(1, 0)              the clear -- the flag is 0 here
    165434409   FUN_80055D84 runs, flag -> 1     592,817 instructions LATER
    165434445   FUN_8005803C runs

So nothing is missing in the sense of never happening. The state the application consults is
established, correctly, just over half a million instructions after it consulted it.

**And raising the flag would still not be enough.** `(3,0x15)` also requires
`*0x80161D40 != *0x8010161C`, and after the handler runs both settle at **0**. Measured:

    at probe start    flag 0   value -1   ref -1
    after 80 s idle   flag 1   value  0   ref  0

so the conditional stays false and the native returns its default for the life of the machine.

#### Opening the gate at its source: necessary, NOT sufficient

The default return is a single word in the native's own literal pool at `0x80054F84`.
`scripts/digibox-probes/open-the-gate.js` pokes it from `0xFFFFFFFF` to `0` and then changes
nothing else — no patched branch, no forced bind, no skipped unbind. The application runs its own
code and makes its own decision with one answer changed from "no" to "yes". The poke is read back
before anything is concluded, because a poke that did not land and a gate that did not matter
produce the same blue screen.

**It defeats the first unbind.** Across the whole settle, `setWindowRoot` is called **once** — the
bind — and `rec[1]+0x60` still holds `0x80430A14`. The clear at 164841592 does not happen.

**And a second decision undoes it on the key press.** The press produces 62 widgets, 494 applies,
**two more `setWindowRoot` calls**, and `rec[1]+0x60` ends at `0x00000000` — with DAMAGE at zero
and no new blits. So this gate is one of at least two, and the second is on the key path rather
than the startup path. Its arguments were not captured in this run; that is the next measurement,
and it is a narrow one.

**What this does and does not establish.** The chain from o-code branch to native to state to the
race is complete and each link is measured. What is not established is that this gate is the only
one — and the evidence now says it is not. The known-good end state remains
`refuse-the-unbind.js`, which paints six 480x32 menu rows by neutering the clear itself rather
than by answering the question that leads to it.


### `sky-eluc.30`: the second gate is on the key path, and it tests the ROOT OBJECT itself

*2026-09-14. With the startup gate open, the first thing a key press does is clear the root again.
This is where, and what it reads.*

#### Where

With `0x80054F84` poked to 0 the settle makes exactly **one** `setWindowRoot` call — the bind — and
`rec[1]+0x60` keeps `0x80430A14`. The key press then calls `setWindowRoot(1, 0)` **immediately**;
it is the first call on that path, not the second. The o-code, captured with a sliding
`__readWatch` window and disassembled:

    9fc730c5  aa             push_ds
    9fc730c6  09 00 02 e1 14 add DS+0x2E114
    9fc730cb  20             get                      load DS[0x2E114]
    9fc730cc  67 f8          pop_fp_nn  (fp-8)
    ...
    9fc730e3  9e f8          push_fp_nn (fp-8)
    9fc730e5  6b             pop_fp_minus16
    9fc730e6  91 5c          push_m_ind_fp_n          read a member through it
    9fc730e8  36 0b          jnz_nn 0x9fc730f5        NOT TAKEN -- the member is ZERO
    9fc730ea  75             push_0
    9fc730eb  aa             push_ds
    9fc730ec  09 00 02 e1 18 add DS+0x2E118           the window id
    9fc730f2  ca 01 e3       scall (1,0xE3)           setWindowRoot(win, 0)

**This is not the `(3,0x15)` gate again**, and the listing will mislead a reader who assumes it is:
the same function carries further copies of the first gate, at `0x9FC73105` — another
`scall (3,0x15)` with the same `push_0` / `jgt_nn` shape and the `0xDCDCDCDC` background path
behind it. Two different tests, a few dozen bytes apart.

#### What it tests, and the part that matters

`DS` resolves to `0x8045CB64` — arithmetic on the recorded `DS[0x0001ACB0] -> 0x80477814`, checked
in the probe rather than trusted. And then:

    DS[0x2E114] = 0x80430A14      THE WIDGET TREE ROOT ITSELF
    DS[0x2E118] = 1               the window id
    DS[0x2E11C] = 1

**So the application tears down its screen after inspecting the very object it just bound.** Read
at the instant of the clear, that object is:

    +0x00 0x804309BC   +0x04 0x8043096C   +0x08 0x8043091C   +0x0C 0x804308CC
    +0x10 0           +0x14 0            +0x18 0            +0x1C 1   (0 during the settle)
    +0x20 0xC8        +0x24 0x80430B50   +0x28 0x03F00000   +0x2C 0x438
    +0x30 0x9FC6C4C8  +0x34 .. +0x70 ALL ZERO

Its live fields stop at `+0x34`. Every candidate reading of the `0x5c` operand lands in the zero
region, so the branch not being taken is consistent with all of them — which is why the probe dumps
the object rather than naming one word as "the field". **Which member the operand addresses is NOT
established**, and that is the next question rather than a detail: it decides what to go looking for
the writer of.

#### What this changes

The two gates are different in kind, and only one of them is about the outside world. The startup
gate asks a native whether some AV-shaped state is ready; this one asks whether a field of the
application's own screen object has been filled in. Nothing here points at a peripheral, a stream or
a chip — it points at the application not having finished populating its own object.

That fits what the drawing already shows. `refuse-the-unbind.js` makes the box lay out and rasterise
six 480x32 menu rows, and **every fill carries the same colour**, `0x0055C288`, with no text. An
object whose fields past `+0x34` are all zero and a menu drawn entirely in one colour are plausibly
the same fact seen from two ends.


### `sky-eluc.31`: the member is `+0x14`, and the code zeroes it and then asks whether it is set

*2026-09-14. Which word the key-path gate reads, how that was established without decoding the
operand, and what writes it.*

#### Which member — measured, not decoded

The gate reads through `PUSH_M_IND_FP_N` with operand `0x5C`, and the operand's meaning was
unknown. Rather than pick a reading, the probe pointed `__readWatch` at the OBJECT and reported
every reader's PC, so the interpreter computes the address and the log records it:

    pc 0x8006C0EE   at 0x80430A28 = obj + 0x14   size 4   value 0x00000000

**Three things agree that this is the gate's read**: it is the last read of the object before the
clear, 153k instructions after the previous one; its PC lies inside the handler that
`scripts/ocode-disasm.py --handlers` resolves for opcode `0x91` through the interpreter's own
209-entry dispatch at `0x800692E0`; and its value is zero, which is what makes the `jnz` fall
through.

**The first attempt filtered reads to `0x8006C050`–`0x8006C090` and logged nothing.** That range is
where the handler reads its OPERAND BYTE out of the o-code chunk — which the earlier trace had
already shown — not where it reads the member. The filter was the error, and an empty log from a
too-narrow filter is indistinguishable from a handler that never ran, which is why it was asserted
rather than interpreted.

#### The operand mapping, from two measured points

A second PC turned up on the write side: across a whole key press the object head is written
**exactly once**, `+0x1C = 1`, by `0x8006D23C` — inside the `POP_M_IND_FP_N` handler, executing the
`pop_m_ind_fp_n 0x7C` at o-code `0x9FC730D2`. Two operands with two measured offsets:

    operand 0x7C -> offset 0x1C            operand 0x5C -> offset 0x14
    offset = (operand - 12) / 4            operand = offset * 4 + 12

Fitted to two points, so a hypothesis — but one that **predicts** a write to `+0x14` is encoded
`5b 5c`, and that prediction is checkable against the bytecode.

#### Who fills it: nothing, on this path — it is deliberately zeroed and then asked about

Scanning the EPG code chunk for `5b 5c` finds 37 candidate sites, and one of them is `0x9FC72FA2`
— thirteen bytes before the startup bind. Disassembling there lands exactly on the executed
boundary at `0x9FC72FAF`, which validates the decode, and the sequence is unambiguous:

    9fc72f95  push_fp_minus20 ; pop_fp_minus16 ; push_0 ; 5b 6c   ->  obj+0x18 = 0
    9fc72f9a  push_fp_minus20 ; pop_fp_minus16 ; push_0 ; 5b 7c   ->  obj+0x1C = 0
    9fc72f9f  push_fp_minus20 ; pop_fp_minus16 ; push_0 ; 5b 5c   ->  obj+0x14 = 0
    9fc72fa4  push_fp_minus20 ; push_ds ; add DS+0x2E118 ; get ; scall (1,0xE3)    THE BIND

**The application zeroes `+0x18`, `+0x1C` and `+0x14`, binds the screen, and the key path then
refuses to keep it because `+0x14` is zero.** That is not a contradiction — it is a "has anything
filled this in yet?" flag, reset at bind time on purpose. Nothing fills it in before the question
is asked.

Of the 37 sites, **13 are preceded by `push_0`** (zeroing, like this one) and **24 push something
else** — those are the writers that would fill the member. None of them runs on the path this box
takes. Which one belongs to this object, and what would reach it, is the next question.

**A caution carried into that question.** A byte-pair scan over bytecode is the shape that produces
plausible nonsense: many `5b 5c` pairs will be the operands of other instructions rather than
instruction boundaries. `0x9FC72FA2` is confirmed because its listing lands on an executed
boundary; the other 36 are candidates until each is confirmed the same way.


### `sky-eluc.32`: BOTH GATES ANSWERED — the box keeps and paints its own screen

*2026-09-14. The writer of `obj+0x14`, the race it loses, and the end-to-end demonstration that
these two values are the whole of what stopped this machine drawing its interface.*

#### The end-to-end result, first, because it is the point

`scripts/digibox-probes/fill-plus-14.js` changes **two words** and nothing else — the `(3,0x15)`
pool constant at `0x80054F84` from `-1` to `0`, and `obj+0x14` from `0` to `1`. No patched branch,
no forced bind, no neutered clear, no touched widget. The application runs its own code and answers
its own two questions:

    setWindowRoot calls during the key press   ZERO   -- it no longer clears its own screen
    rec[1]+0x60 at the end                     0x80430A14   -- the root STAYS BOUND
    newWidget 62   apply 534   DAMAGE 48, every one carrying window 1
    ring produce/consume  1,1 -> 10,10         11 new blits
    surface hash  0x9825B318 -> 0xB5E7D358     dominant colour 0xDC -> 0x88

    fill @128,0 592x112   fill @0,112 720x144   fill @0,256 720x144
    fill @0,400 720x104   fill @0,504 720x72
    fill @120,148 480x32  fill @120,180 480x32  fill @120,212 480x32
    fill @120,244 480x32  fill @120,276 480x32  fill @120,308 480x32

**Byte-identical in outcome to `refuse-the-unbind.js`, but reached the honest way.** That probe
neutered the clear; this one lets the clear run and gives it the answers it asks for. Two
independent routes to the same end state is what makes the chain a finding rather than a story.

#### The writer

A write watch on the four bytes themselves, across eight different handset keys, catches **exactly
one** write:

    pc 0x8006D23C   at 0x80430A28 = obj+0x14   size 4   val 0x00000001   icount 378027961

`0x8006D23C` is the **POP_M_IND_FP_N handler** — the same PC that writes `+0x1C`. So the
application's own o-code does set this member, to 1, through the ordinary member-store opcode.

**It is late, and that is the second instance of a shape this box has already produced once.** In
`sky-eluc.29` the AV flag at `0x80161D3C` is raised 592,817 instructions after the decision that
reads it. Here the member is written after the screen has already been torn down by the first key
press. Both times the application answers the question after asking it, and both times the answer
would have been the right one.

#### What the static side got, and where it stopped

A write to `+0x14` encodes as `5b 5c` (from `offset = (operand - 12) / 4`). The EPG code chunk holds
37 such byte pairs: 13 preceded by `push_0` — zeroing, like the one three instructions before the
bind — and 24 pushing a real value. Disassembling around the six nearest confirms four as genuine
instruction boundaries, and two of those read as tiny **setter functions**:

    9fc704f8  push_fp_nn 0xf6 ; 5b 5c ; jmpr ; ret_0_4
    9fc705d6  5b 5c ; push_1 ; pop_fp_minus16 ; ret_4_4

So the writer is a setter, called from somewhere, and **which somewhere is a question about a
CALLER that a byte scan cannot answer** — which is why the watch was used instead. Two of the six
could not be confirmed at all: the decoder stalls on an opcode whose operand length the traces have
never observed, and that is reported as unknown rather than guessed.

#### What is left, and what is now closed

**Closed:** why the interface never appeared. Two gates, both found, both measured, both answered,
and the box then keeps its screen and rasterises its menu with no peripheral modelled, no register
answered and no chip identified. The OSD and blitter were never the blocker and neither was the
video hardware.

**Open, and a different question:** every one of those eleven fills carries the same value,
`0x0055C288`. The six menu rows are the colour of the field behind them and there is no text. The
box is drawing its layout correctly and drawing it in one colour.


### `sky-eluc.33`: the menu is one colour because NOTHING DRAWS TEXT — and a blitter branch is now load-bearing and untested

*2026-09-14. Two separable findings: what the application is actually doing, and a place where our
own model is guessing.*

#### The application really does fill eleven rectangles with one value

Across all eleven fills the blitter's fifteen command words were compared word by word. **Only two
vary:** word 12, the destination `(y<<16)|x`, and word 13, `(h-1)<<16|(w-1)`. Every other word —
including every candidate for a colour — is byte-identical from the first fill to the last. So the
single on-screen colour is not an artefact of our decode flattening different values: the
application sends one value eleven times.

#### And every blit is a FILL. Not one copy

    blits 11    kinds { fill: 11 }    anyCopy: false

Text on this hardware is a **copy** — glyphs blitted from a source surface — so *"no text"* and
*"no copies"* are the same observation. The plane family's two five-argument members, `(1,0xE9)`
and `(1,0xEA)`, which are the shape of a blit, **do not run at all** on the press.

A full module-1 census of the press against a same-length idle baseline shows a busy, coherent
widget build and no drawing beyond the fills: `(1,0x57)` newWidget x62, `(1,0x5D)` x44,
`(1,0x62)` set-anchor x40, `(1,0x7D)` x40, `(1,0x75)`/`(1,0x76)` x36 each, `(1,0xCB)` x26, down to
`(1,0xE4)` the drain x2. The box builds and lays out a menu in full and then paints only its
backgrounds.

**That fits the unfilled object.** The widget root's fields past `+0x34` are all zero
(`sky-eluc.31`), and a menu drawn with no content is the same gap seen from the other end. They
should be worked as one question, not two.

#### THE PART OF THIS THAT IS OUR BUG, and it is the kind this file exists for

The emulator's fill decode is:

    if (bpp === 16)            v = r[14] >>> 16;
    else if (w0 & 0x1000000)   v = r[14] & 0xFF;      // "pattern from word 14"
    else                       v = r[1] & 0xFFFFFF;   // "else the value is word 1"

**Every fill this machine had ever executed took the middle arm.** The boot's 8 bpp fills are
opcode `0x01AC0000` — bit 24 set — and read their `0xDC` out of word 14. The menu fills are
`0x00AC0000`, bit 24 **clear**, and are the first commands in the life of this emulator to reach
the `else`. It is an inference that has never once been exercised, and it is now the only thing
deciding what colour the interface is.

**And the value it produces is address-shaped.** Word 1 is the SOURCE surface's field-0 address in
the command layout, and on the boot fills it holds `0x00584048` — the framebuffer — while being
entirely unused. On the menu fills it holds `0x0055C288`, and the decode reports
`value 0x0055C288` for an 8 bpp fill that wants a palette index, with `blitPut` keeping the low
byte `0x88`. A 24-bit number standing in for an index is exactly the shape of a misread register.

So `0x88` may be the application's colour or may be our guess. **The one-colour finding does not
depend on it** — nothing varies across the eleven fills whichever word is read — but the actual
colour of the interface does, and so does anything built on it. The source-side words on these
commands (`w2`–`w6`) hold code and heap pointers rather than a plausible surface, so a pattern-copy
reading does not fit either; what bit 24 clear really selects is **not established**.


### `sky-eluc.34` ANSWERED: there IS no fill colour — the menu commands are COPIES we execute as fills

*2026-09-14. Why the interface paints blank, and it is our defect rather than the firmware's.*

#### What was eliminated first

**The command is not truncated.** It is DMA'd on channel 12 with `len = 60`, so all fifteen words
reach the register file and nothing is left over from a previous blit. The stale-register reading
of words 2–6 is dead.

**The command is not garbage.** A write watch over the DMA buffer catches 165 writes, every one
from a single PC, `0x800D01A0` — the word-copy loop inside `FUN_800D00BC`, a generic **ring-buffer
enqueue** taking a word array and a count that knows nothing about blits. The command is assembled
on the caller's stack (`0x801D6114`, found by looking for the register whose target bytes ARE the
command rather than the register that merely looked like a pointer) and pushed from there. So word
1 is genuinely what the application built.

*Read that stack at the moment of the copy, not afterwards: sampled a few instructions later it
holds completely different values, because the frame is immediately reused.*

#### The answer

    boot fills   opcode 0x01AC0000   bit 24 SET     word 1 = 0x00584048 (the framebuffer, unused)
                                                    word 14 = the colour, 0xDC
    menu fills   opcode 0x00AC0000   bit 24 CLEAR   word 1 = 0x0055C288
                                                    word 14 = 0x801D6160 -- a STACK POINTER

Word 14 holding a stack pointer is already fatal to reading it as a colour on the menu commands.
And `0x8055C288` sits among this machine's other plane buffers — `0x80575798`, `0x805A81A8`,
`0x805B6A58`, the last of which is *this very command's destination field-1*.

**So look at the memory rather than decode further.** `0x8055C288` holds:

    6d 6d 6d 6d 6d 6d 6d 6d 6d 6d 6d 6d 6d 6d 6d 6d ...
    three distinct byte values over 1920 bytes -- 0x6D x972, 0x59 x816, 0x55 x132 -- 100% non-zero

Three palette indices in runs. **That is a picture.** Not empty, not random, and not where a colour
constant would accidentally point.

**Word 1 is a SOURCE SURFACE, and the menu commands are COPIES.** Our decode treats bit 23 as the
fill bit, so it classifies them as fills and paints each one as a flat rectangle of
`word1 & 0xFF` = `0x88`. The correct reading is that **bit 24 is the fill bit**: the boot's
`0x01AC0000` is a genuine fill taking its `0xDC` from word 14, and the menu's `0x00AC0000` is a
copy from word 1.

**That is why the interface has no text.** The application is compositing its menu from prepared
surfaces — which is exactly how glyphs and graphics reach a screen on this hardware — and the
emulator overwrites each one with a solid colour. The firmware has been drawing the menu correctly
the whole time.

#### What is NOT established, and why the fix is not in this commit

The source *geometry* does not fit a straight copy under the current side layout: word 5 would be
the source pitch and reads `0x8042F354`, giving 62,293 pixels, and words 2–4 and 6 are code and
heap pointers. So `blitSide(r, 1)` is not the source descriptor for this opcode — the base address
is right and the rest of the mapping is not. Changing `blitExec` therefore needs the source layout
worked out first, and **a hardware model does not fail with a stack trace**: a wrong source pitch
will read plausible rubbish out of DRAM and paint a convincing screen of noise. The change belongs
with `./ctl.sh digibox` and a screenshot, not at the end of a long session.


### `sky-eluc.35`: THE INTERFACE IS ON SCREEN — bit 24 is the fill bit and the source is packed

*2026-09-14. The fix, the measurement behind it, and the picture.*

#### The change

    -  if(w0 & 0x800000){            // bit 23 = fill
    -    if (bpp === 16) v = r[14] >>> 16; else if (w0 & 0x1000000) v = r[14] & 0xFF;
    -    else v = r[1] & 0xFFFFFF;
    +  if(w0 & 0x1000000){           // bit 24 = fill
    +    v = (bpp === 16) ? (r[14] >>> 16) : (r[14] & 0xFF);
    +  } else if(!blitSideLooksReal(r, 1)){
    +    // copy from a tightly packed source: base = word 1, stride = the blit width

Both observed opcodes carry bit 23, so it cannot be the discriminator: the boot's `0x01AC0000` is a
real fill whose colour is word 14 (`0xDC`), and the menu's `0x00AC0000` is a **copy** whose source
is word 1. The old reading called both fills and, for the menu, took a colour from word 1 — an
ADDRESS — keeping its low byte `0x88` and painting a flat rectangle over every glyph the
application had composited.

#### The pitch was measured, not chosen

`blitSide(r,1)` is not the source descriptor for this opcode: word 5 would be the source pitch and
reads `0x8042F354`, 62,293 pixels, while words 2–4 and 6 hold code and heap pointers — the builder
assembles the command on its caller's stack and simply does not fill in what its opcode does not
need. So the source is a base plus a stride, and **a wrong stride does not throw; it reads
plausible rubbish out of DRAM and paints a convincing screen of noise.**

So the stride was scored, not guessed. A 24 KB dump of a live source surface, every candidate from
8 to 1600 scored on row-to-row byte agreement — a bitmap's rows resemble their neighbours and a
wrong stride destroys that, which gives a sharp maximum instead of a judgement call:

    stride 480   0.8792     <- the blit width
    stride 481   0.8466
    stride 479   0.8464
    stride 720   0.6171     <- the DESTINATION's pitch, and clearly not the source's

480 wins outright over 1,593 candidates and beats its own neighbours by three points, which is the
signature of a true stride. The source is packed at the blit width, progressive, origin 0,0, while
the destination stays field-paired.

`blitSideLooksReal()` decides which layout to read, and it is a **structural** test rather than a
score: a surface base must be a mapped DRAM address and a pitch a plausible raster width. An
unused source side fails it because it holds stack.

#### The result

`./ctl.sh digibox` green on a cold boot: 42 tasks, 446.7M instructions, every row passing. Then
`fill-plus-14.js` — two pokes, no patched firmware — and the framebuffer goes from 12 distinct
colours to **37**:

    blit 8bpp 0x8055C288 packed/592 -> 0x80584048 @128,0  592x112 fields
    blit 8bpp 0x8055C288 packed/720 -> 0x80584048 @0,112  720x144 fields
    ... and six at packed/480 -> @120,148 .. @120,308, 480x32

**`scripts/shots/01-plus14-forced.png` is the Sky interface.** The tab bar — TV GUIDE, BOX OFFICE,
SERVICES, INTERACTIVE, each with its icon — and a six-item Box Office menu: MOVIES BY START TIME
highlighted in yellow, then MOVIES A–Z, NEW MOVIES, SPORTS & EVENTS, SPECIALIST, FREE PREVIEWS.
The firmware had been drawing this the whole time.

#### THE PAGE NOW SHOWS THE INTERFACE, and what it took to get there

**Load the page, let it boot, and press the handset button marked `sky` (raw `7D`).** The menu
appears with no probe and no pokes -- `scripts/digibox-probes/plain-key-press.js` is the version
of the run that does nothing but boot, press and look, and it produces the same 37-colour surface
as the instrumented one. Verified on `https://retrotv.demosrv.uk/digibox-boot.html` as well as
locally.

**The gate answers are a DECLARED FICTION and they wait for the boot.** `skyGatesTick()` patches
`(3,0x15)`'s default return and the `push_0` that zeroes `obj+0x14`, once the task list reaches 42.
`__skyGates(false)` turns it off.

**Waiting for the boot is the whole lesson, and it cost two failed attempts.** Answering either
gate BEFORE the RTOS is up stops the boot dead at **28 tasks instead of 42** -- first by patching
the literal pool word, then by patching the two o-code bytes at image load. Same failure, same
place, caught both times by `./ctl.sh digibox`. Answered early the application believes its AV path
is ready, takes it, and needs something this emulator does not model. That is a real finding about
the gates rather than a bad patch: they are not arbitrary, and **papering over them early breaks
more than it fixes.**

**A SINGLE-FILE BIND MOUNT PINS AN INODE, and `git checkout` is a rename.** `docker-compose.prod.yml`
mounts `digibox-boot.html` file by file, so an in-place rewrite is served immediately but anything
that REPLACES the file -- `git checkout`, `mv`, an editor saving by rename -- leaves the container
serving an inode that no longer exists on disk. That is exactly what happened here: the file was
correct, the page was stale, and nothing looked wrong. `./ctl.sh restart:prod frontend` (added for
this) re-resolves it. And note the demo host is `retrotv-prod`, a different compose project from
the development stack -- restarting the wrong one changes nothing and looks identical.

#### What still is not there

**Nothing here changes what a visitor to `https://retrotv.demosrv.uk/digibox-boot.html` sees.** The
blitter fix is live on the demo host -- verified by fetching the page and finding
`blitSideLooksReal` in it -- but the menu appears only when a probe pokes the two gate values
first. Left alone, the firmware still clears its own screen and the box is blue exactly as before.
The fix is what makes the interface VISIBLE once the gates are open; it does not open them.

**And the pokes only work on a WARM boot.** Driving the demo host cold failed the probe's own
control -- the window root read 0 while the global still named the tree -- because a cold boot runs
446M instructions and the bind/clear pair happens at ~164.8M, so by the time the 42-task assert
fires and the probe pokes anything, the decision is 280 million instructions in the past. A warm
boot is 139.5M and the poke lands in time. Any future attempt to make this permanent has to
intervene BEFORE that point, not after the boot completes.

#### Two things to be honest about

**Every one of the eleven blits reads the same source address**, `0x8055C288`, with a different
width each time. That is consistent with a scratch surface the application renders into and blits
from, and the emulator copies at DMA time so it sees each render — but it has not been proved, and
if it is wrong the picture is right by luck. It is also why the per-blit stride is taken as the
blit width rather than a fixed surface pitch: that fits every observed command and would not fit a
fixed-size scratch buffer.

**The top-left 128x112 is not painted** — the first blit starts at x=128 and that corner keeps the
old background. It may be where the video window belongs, or it may be a blit nothing has issued
yet. Not established.


### `sky-02me.2`: the handset map, and two gaps it found

*2026-09-15. Every button on the page's handset, pressed on a settled box.
`scripts/digibox-probes/keymap.js`. The button set is read out of the DOM rather than typed in, so
it cannot silently stop covering one.*

**33 buttons. Five reach a new screen:**

    raw   what it reached                                widgets  blits  screen moved
    7D    the Box Office menu (the known-good control)        62     11   yes
    7E    the Box Office menu again                           62     13   yes
    80    THE TV GUIDE                                       249      2   yes
    83    the TV Guide                                       249      7   yes
    F5    the INTERACTIVE tab, highlighted in the tab bar     28     17   yes
    CC    standby                                             98     22   yes
    0C    back to a 12-colour screen                           0      5   yes

`scripts/shots/04-btn-80-0x10880.png` is **the TV Guide**: *FOR YOUR INFORMATION / Searching for
listings / Please wait*, with *Further schedule information is not available* and a
*Search Channel / Search Favourite* footer. That is the screen `sky-02me.5` is about, and it
confirms the dependency on a real line-up rather than assuming it. `10-btn-F5` shows the tab bar
with INTERACTIVE highlighted, so tab navigation works.

#### A WRONG FINDING, AND THE MISTAKE THAT PRODUCED IT

**The first write-up of this walk said the arrow keys redraw and never change the screen "on the
Box Office menu". That is false, and the arrows work perfectly.** Pressed from a state that was
actually CHECKED to be the menu (`scripts/digibox-probes/arrows-on-the-menu.js`):

    down    2 blits -- exactly @120,148 and @120,180, the old row and the new one -- screen MOVED
    down    moved again
    up      moved, and the hash returns to the value after the first down
    right   62 widgets, 18 blits -- a whole new tab
    left    62 widgets, 18 blits, and the hash returns to the ORIGINAL menu hash exactly

`scripts/shots/02-arrows-01-down.png` shows the highlight on **MOVIES A-Z** instead of MOVIES BY
START TIME. Up and down repaint precisely two 480x32 rows -- the one losing the highlight and the
one gaining it -- which is the minimal correct redraw. Left and right change tab and come back to a
byte-identical screen.

**THE MEASUREMENT WAS REAL; THE SENTENCE ATTACHED TO IT DESCRIBED A SCREEN NOBODY HAD CHECKED.**
The walk is a sequence, and by the time it reached the arrows it had already pressed `CC` (standby)
and `80` (the TV Guide) -- so the arrows landed on an EMPTY guide, where *Searching for listings /
Further schedule information is not available* has nothing to move a highlight between and
redrawing the same nothing is correct behaviour. Every number in that row was accurate. The claim
about which screen it was on was invented.

This is the project's own oldest shape wearing new clothes: the instrument worked, and the scope of
the claim was wider than the scope of the evidence. The probe's own caveat even said the rows were
state-dependent -- and the write-up then named a state anyway. **A caveat that the analysis
contradicts is worse than no caveat**, because it reads as though the question was considered.

The mechanical fix is in `keymap.js`: every row now carries the screen it was pressed ON, taken
before the press, so a row cannot be attributed to a screen nobody verified. The same correction
applies to the number keys below.

#### The number keys: they work, and the first finding was wrong in BOTH directions

Pressed from a state verified to be the menu, and restored to it between every subject
(`scripts/digibox-probes/quiet-buttons-on-the-menu.js`):

    59  down (CONTROL)   2 blits, moved -- so the instrument registers a real change
    01  digit 1          22 widgets, 353 applies, 10 blits, MOVED
    02  digit 2          25 widgets, 375 applies, 16 blits, MOVED
    03  digit 3          25 widgets, 375 applies, 16 blits, MOVED
    06  digit 6          23 widgets, 349 applies, 16 blits, MOVED
    00  digit 0          silent -- and correctly so, there is no row 0

`scripts/shots/04-quiet-02.png` is **MOVIES A-Z**: the title in the banner and *Searching for
listings / Please wait* beneath it. The digits select menu rows exactly as they should.

**Genuinely silent ON THE MENU**, which is now a claim about a screen somebody checked: `0x500`,
`0x501`, `0x502`, raw `6D`-`70`, and `0x604`. Those may well be correct -- they are candidates for
channel up/down and other functions this screen has no use for -- and that is a separate question
from whether they do anything anywhere.

**THE RESTORE CONDITION HAD TO BE LOOSENED, and the reason is worth keeping.** The first run
demanded a byte-identical menu between subjects and stopped dead after one, because the control
press legitimately moves the highlight to row 2 and `0x7D` does not reset it. The state that
mattered was "the Box Office menu", not "the menu with row 1 highlighted". The check is now the
menu's measured colour signature -- 37, against the guide's 34 and 30 and the post-`0x0C` screen's
12 -- and every row records the hash it was actually pressed on, so the looser condition is still
attributable rather than vague.

#### What this all adds up to

**The interface is fully navigable.** Arrows move the highlight, left and right change tab, digits
select rows, and every screen reached draws correctly. What is missing everywhere is *data*: the
guide, MOVIES A-Z and the Interactive screen all show *Searching for listings / Further schedule
information is not available*. That is `sky-02me.3` and `sky-eluc.12`, not a UI problem.



The same walk found raw `0x01`-`0x09` and `0x00` producing zero of everything -- but they were
pressed after `F5`, on the Interactive screen, **not on the numbered menu**, so "the number keys do
nothing on a menu whose rows are numbered 1 to 6" is exactly the same error and is not established.
It stays open as `sky-02me.10`, re-scoped to test them from a proven menu state first.

**THE MAP IS A SEQUENCE, NOT A DICTIONARY.** Rebooting between presses would cost ~40 s each, so
the walk accumulates state and each row says what that button did *on the screen the previous
presses left behind*. That is why `83` and `80` show the same screen. A second pass that returns
to a known state between presses is what turns this into a per-button meaning.


### `sky-02me.3`: the tuner mock works, the box acquires, and the guide finishes searching

*2026-09-15. What a broadcast actually gets this box, and what is left.*

#### The box acquires and accepts everything it is sent

Started AFTER the boot (`scripts/digibox-probes/guide-with-a-broadcast.js`), the SI carousel gets
straight through:

    carousel      79 sections sent, 0 REFUSED
    demux PIDs    0x52 (ch 21)  0x14  0x11  0x10   -- 0x52 is the box's OWN subtable registration
    ids           network 0x20, bouquet 0x1000, read off the hardware section filters
    si log        table 0x40 (NIT), 0x42 (SDT), 0x4A (BAT) all accepted, repeatedly

So the demodulator mock -- register 11 answering `0x3F` -- does its job end to end: the box
acquires, registers a subtable for network `0x20`, **programs its own filter on PID `0x52`**, and
takes every table the carousel offers.

#### And the guide changes: it stops searching

Before a broadcast the TV Guide shows *FOR YOUR INFORMATION / Searching for listings / Please wait*
above *Further schedule information is not available*. With the carousel running,
`scripts/shots/03-bcast-03-guide-settled.png` shows the **searching box gone** -- only *Further
schedule information is not available* remains, and the screen does not change across a further 25
seconds of broadcast.

**The box concluded its search rather than waiting for ever.** That is the difference between "no
data has arrived" and "data arrived and contained no schedule", and it moves the question from
plumbing to content.

#### What is left

The guide wants **schedule events**, and NIT, SDT and BAT do not carry them. The box has told us
where it expects them: it armed **PID `0x52`** itself. That is the next thing to feed, and it sits
with `sky-eluc.12` (a real line-up) and the OpenTV carousel on PIDs `0xA0`-`0xB1` whose parsers are
already located (`0x800C95D0` / `0x800C9CA0`, Huffman `0x800BECF0`).

#### One observation, stated on its own conditions

With the runner's `--carousel` flag **alone**, the box stalled at **22 tasks** and burned **2.0
billion instructions** without finishing, against 42 tasks and 140M warm with no broadcast.

**That does not overturn the recorded claim above that the box reaches 42 tasks with the carousel
on air**, because that measurement was taken with `--ack-all` as well, where every peripheral
command is answered. Two different machines. Whether `--carousel` needs `--ack-all` to boot is a
real question and it is filed rather than assumed -- which is the discipline this file exists for,
and the same one that produced two wrong findings earlier today when it was skipped.


### Reading what PID `0x52` wants: the hardware filters it at all, and two instrument gaps fixed

*2026-09-15. `sky-02me.5` and `sky-02me.12`. The route there mattered as much as the answer.*

#### The answer

**The match table holds 16 filters; the PID channels number 32. A channel above 15 cannot have a
match, so nothing filters it.** The box arms channel **21** for PID `0x52` after acquisition, and 21
is above 15 -- so **every section delivered on `0x52` reaches the firmware** and the SI layer
decides. A feed for it does not need a table id guessed past the hardware.

**Measured, not preferred between readings.** The command at `+0x144` packs `{byteIndex, filter}`
into one byte and the split was open: four bits each, or five for the filter and three for the byte
index. The first discriminator tried -- "every programmed filter should be a channel the box armed"
-- fails under BOTH readings, because the firmware sweeps every filter at init, so the set of
indices says nothing. **The byte index does**: `__siMatches()` keeps a ten-byte match array, and
across 524 commands from a live acquisition the byte index reaches **9**, which three bits cannot
hold. Filters 0..15, byte indexes 0..9. The existing decoder was right.

That also narrows a comment in the page that was too strong. *"THE CHANNEL INDEX IS THE FILTER
INDEX; there is no second mapping to discover"* holds for the PID and enable registers and **not**
for the match table, and it has been corrected where it sits rather than only here.

#### `__dmxLog()` kept the OLDEST writes, and the boot filled it

`dispWrite` pushed only `if(dmxLog.length < 4000)`, and acquisition happens after the boot -- so the
match-programming commands, which are the box telling us what to broadcast, were never in the log.
A probe asking for them got *"the box programmed none"*: the cap talking, reading exactly like the
firmware having done nothing.

**It disagreed with its own sibling and only one of them said so.** `dmxMatch` is populated
regardless of the cap, so `__siMatches()` returned four filters while `__dmxLog()` looked empty.

Now a ring that keeps the NEWEST, dropping half a cap at a time so the cost stays amortised constant
on a per-write path, and `__dmxLog()` returns `{dropped, kept, entries, raw}` -- **the count of
writes thrown away travels with the ones kept**, because a truncated log and a short one must not
look alike. With it: 0 dropped, 3,635 kept, 524 match commands recovered.

#### And the join that produced a confident artefact

`__siMatches()` is indexed by match unit (0..15); `__siFilters()` and `__dispState().pids` by PID
channel (0..31). A probe paired them by index and concluded *"nothing feeds PID 0x52"*. The
conclusion happens to be true and **the reasoning was worthless** -- it came from joining two
different spaces, not from the filter count. Tracked as `sky-02me.13`.



*2026-09-15. `sky-02me.5`. The answer is not here; three things that would have made it look like it
was are.*

**`__siMatches()` and `__siFilters()` ARE DIFFERENT INDEX SPACES, and joining them by index is
wrong.** `__siMatches()` reports filters 1, 2, 3 and 5; `__siFilters()` and `__dispState().pids`
report channels 21, 22, 23 and 24 carrying PIDs `0x52`, `0x14`, `0x11`, `0x10`. A probe paired them
by index and concluded *"nothing feeds PID 0x52"* -- which is an **artefact of the join**, not a
finding. Whether the demux has one namespace or two (match units attached to PID channels by a
third register) is itself unestablished, and the answer decides whether the row reading
*"filter 5 wants table 0x73, no extension"* is about PID `0x52` or about something else entirely.

**The match decoder takes four bits of filter index for a demux with 32 filters.** `dispWrite()`
decodes `+0x144` as `0xC0` in the high byte and `(byteIndex << 4) | filter` in the low byte, so
`fi = val & 0x0F`. Every filter from 16 up would be recorded 16 lower. That is a *suspicion*, not a
conclusion: it is equally possible the two namespaces are genuinely separate and four bits is
correct. It was not settled because of the next one.

**`__dmxLog()` IS CAPPED AT 4000 ENTRIES AND THE BOOT FILLS IT.** `dispWrite` keeps the FIRST 4000
writes, and acquisition happens after the boot -- so the match-programming commands are not in the
log at all, and a probe asking for them gets *"the box programmed none"*. It is the cap talking, and
it reads exactly like the firmware having done nothing. `dmxMatch` is populated regardless, which is
why `__siMatches()` works while `__dmxLog()` appears empty -- two accessors over the same writes
disagreeing, with only one of them saying so.

**So what PID `0x52` wants is open.** The route that does not depend on any of the above is to make
the demux log keep the LAST writes rather than the first, or to clear it after the boot, and then
read the raw `+0x144`/`+0x148` pairs at acquisition. The field width and the namespace question both
fall out of that, and neither needs to be guessed.


### Feeding PID `0x52`: the delivery works, the guide does not fill, and the reason is measured

*2026-09-15. `sky-02me.5`. The guide did NOT fill. What that cost to establish honestly is the
useful part.*

#### The feed works

`__siPush(0x52, ...)` is accepted for **every** table id from `0xA0` to `0xB1` and for `0x4E` and
`0x42` as controls -- 0 refusals -- which is what the 16-match-units-for-32-channels finding
predicted: channel 21 is above 15, so nothing filters that PID and the hardware takes anything.

#### The parser addresses in our own notes are COLD

The first sweep watched `0x800C95D0`, `0x800C9CA0` and `0x800BECF0` -- the OpenTV carousel parsers
this project's notes name -- and reported that no table id woke any of them. **That result meant
nothing**, because the sweep never established those addresses execute. They do not:

    0x800C95D0  0 hits      region 0x800C9000-0x800CA000   total 0
    0x800C9CA0  0 hits      region 0x800BE000-0x800BF000   total 0
    0x800BECF0  0 hits      region 0x800B0000-0x800D0000   total 42,883,848

The surrounding firmware is among the hottest on the box and those two neighbourhoods are stone
cold, against a positive control (the o-code interpreter at `0x80069000`, 2.5M hits) proving the
profiler was recording. So the sweep was watching code that is not on this path.

#### So the firmware was asked to name its own consumer

Arm a read watch over filter 21's ring -- `0xA07DF000`, and `__siFilters()` and the
`0xA07A0000 + f*0x3000` layout agree on that address, which is what made it safe to watch -- then
push one section. Four PCs read the bytes:

    0x80004506  x2    the section_length bytes at ring+1 and +2
    0x80004508  x2
    0x800047F0  x1    ring+0x10
    0x800A857A  x1    the SI-layer consumer

**No address was guessed.** `0x800A857A` is `lbu v1,0(s1)` -- it reads the **table_id** -- and then
`cmpi v0,0x40`.

#### And that dispatcher understands three tables

`cmpi v0,X` encodes as `0x7240` for `0x40`, so the full opcode-and-register field is known from a
disassembled instance rather than guessed at two bytes. Sweeping `0x800A8000`-`0x800A9800` for it:

    0x800A8594  cmpi 0x40     NIT actual
    0x800A85D4  cmpi 0x42     SDT actual
    0x800A8630  cmpi 0x70     TDT

**Nothing else.** Sections arriving on `0x52` reach a dispatcher that understands NIT, SDT and TDT
-- which is exactly why feeding `0xA0`-`0xB1` changed nothing and why the guide still reports no
schedule. `0x4A` (BAT) is demonstrably accepted by this box and is **not** in that routine, so it is
handled elsewhere and there are sibling dispatchers this scan did not cover.

#### What is open

Where EPG **event** data is parsed. The recorded OpenTV carousel addresses are cold, so that note
does not describe this box's live path, and the question is now "which code consumes schedule
events" rather than "which table id do we send". The method that worked here transfers directly:
watch the ring, let the firmware name its consumer, and read the comparison chain.


### `sky-02me.14`: the page is a Digibox again, and the "settled" signal was measuring nothing

The demo page now leads with the framebuffer and a Sky remote, with every instrument that was
beside them moved into a modal one click away and nothing deleted. That part was layout. The
part worth recording is what building it found.

**"BGLOAD stopped being scheduled" was satisfied by BGLOAD NOT HAVING STARTED.** Both the boot
gate (`docs/reference/oracle-boot-gate.md`) and, briefly, the page's own readiness readout waited for
`TASK20`'s schedule count to stand still for two consecutive one-second samples, on the reasoning
that TASK20 *is* the task CRC-ing bank 1 and its stillness therefore means the work is done.
Sampled properly, every two seconds from the moment the 42nd task appears on a cold boot:

        t(s)   icount     TASK20.runs
           0    448M            1
           2    457M            1
           4    466M          372
           6    473M          720
          10    489M         1559
          20    520M         2589
          22 .. 120                     2589, never moving again

There are **two** plateaux and the test cannot tell them apart. The first is the two seconds
before BGLOAD is first dispatched; the second is the real one. At one-second sampling the first
plateau is three samples wide, so the condition fires at t≈2 with eighteen seconds of flash
checking still to come. It is not intermittent so much as a coin toss decided by where the
sampling phase lands: this session saw the gate print *"BGLOAD stopped being scheduled after 2s"*
and, with the fix, *"BGLOAD ran 2588 times and then stopped, after 25s"* on the same machine
minutes apart.

**The gate stayed green through the false reading**, and that is the instructive half. Its key
check only asks whether a press reaches the input layer, which it does even while the box is
saturated — so the weaker claim passed and nothing contradicted the stronger one. What caught it
was driving the page the way a person would: the failing run clicked `sky` on the page's own
handset the instant the readout said Ready, got **no menu**, and a following press of the pad
painted the screen flat green. That is the same shape as everything else in this file — a
ready-condition satisfied by the very state it was written to exclude — arriving this time in
the code written to keep other people out of that state.

The fix in both places is to track the PEAK and only count stillness once the peak has risen
above where it started, with a narrow fallback (120 s, and only while the count has never moved
at all) so a box whose BGLOAD never runs cannot wait for ever. `./ctl.sh digibox` now reports how
many times BGLOAD ran as well as when it stopped, because a settle time with no run count behind
it is exactly the number that was wrong.

**Two things were found and NOT fixed, and both are filed rather than left in a comment:**
`FLASH_U202.bin` is served publicly from the demo host (HTTP 200, 2,097,152 bytes) while the
page's own head comment says the firmware is never served from there — the note is the thing
that is wrong, but which way to resolve it is the owner's call. And in the window between the
handoff and the application programming its OSD, the panel falls back to reading video RAM at
whatever bit depth the format selector happens to hold, which renders the decoder's black field
as vertical stripes — memory dressed as a picture, which this page's own code comments forbid
two functions further down.


### `sky-eluc.12`: the acquisition ladder, and the channel-list descriptor is `0xB1` behind specifier 2

Four probe stages, each with its control, and every number below is from a run whose control held
in the same run. Probes: `scripts/digibox-probes/si-what-is-armed.js`, `which-descriptors.js`,
`descriptors-after-the-copy.js`, `which-tag-the-bat-walk-wants.js`, `private-specifier-unlocks.js`.

#### The ladder: one NIT is what makes the box ask for a bouquet

On a settled plain boot the hardware match table carries **`0x40` (NIT, network `0x20`) and `0x73`
(TOT) and nothing else**, and stays that way for 220M instructions -- sampled on a curve rather
than read once, because a single reading cannot tell "never" from "not yet". No SDT, no BAT. Push
**one** NIT and the table becomes `0x40/ext=0x20`, **`0x4A/ext=0x1000`**, `0x73`, with
`__siIds()`'s bouquet id moving from `0x0` to `0x1000`. Reproduced in three separate runs.

So `__siBAT()`'s note that "nothing changes until acquisition arms the masked filter" is right, and
the thing that arms it is a NIT. **`0x42` (SDT) never appeared**, in any run -- which differs from
the older note in `__siIds()` claiming acquisition brings up `0x42 ext 0x0020` as well. That note
describes a state this session could not reproduce.

#### Delivery is by PID; the match table is what the box has SUBSCRIBED to

`siPush()` writes into the filter's ring and sets the status bit. The hardware table-id match is
not in that path, so a section reaches the firmware whenever its **PID** is armed. The first
version of `which-descriptors.js` refused to run on "no match unit for `0x4A`", which was stricter
than the machine and would have turned a subscription finding into a delivery excuse.

#### The section is parsed on a COPY, and watching the ring answers about memcpy

A ring watch over a BAT whose transport loop carried one descriptor of every private tag
`0x80..0xFF` reported **every one of the 128 bodies read** -- 786 bytes, 100% coverage, from a
single PC. That PC is `0x800FA958`, and decompiling it settles it:

    undefined1 * FUN_800fa93c(undefined1 *dst, undefined1 *src, int n)
    { while (n = n + -1, -1 < n) { *dst = *src; dst++; src++; } return dst0; }

the byte loop inside **memcpy**. Without the per-PC coverage grouping that run would have shipped
"128 private descriptors, every one consumed" -- a finding of exactly the shape this file is
organised against, and one that looks like a jackpot rather than an error.

**And following the copy has its own trap, which caught the next version.** Push twice and the
copies land at `0x8019A49A` then `0x8019A7C7`, `0x32D` apart -- exactly the 813-byte section, so
that family is a RING. The address present in BOTH runs is the one the first push wrote and
nothing has overwritten: **stable because it is dead**. Watching it read back a clean zero. The fix
is to stop predicting an address: watch the few-KB REGION, push, and locate the section afterwards
by finding the marker. A wrong prediction returns zero; a right region cannot.

#### The walk is a generic iterator, and the callback is what reads a body

    FUN_800ad590(loop, off, wantedTag, userData)          // callback in $t0
      tag = *desc; len = desc[1];
      while (tag != 0) {
        if (tag == wantedTag || wantedTag == 0) callback(desc, i, userData);
        desc += len + 2; tag = *desc; len = desc[1];
      }

It reads **only** the tag and length byte. `0x800AD5B4/B6` take the first descriptor's pair and
`0x800AD5F2/F4` the rest, so on a 130-descriptor loop the counts are 1 + 129 -- every descriptor
walked. Tag `0x00` terminates it, which is why no sweep can probe tag 0.

#### The sweep: `0x5F` is the only tag consumed, and it explains the negative

One run, two passes, the second being the first's control:

    tags 0x01..0x7F   128 walked, a callback fired at 0x800CAE88.., body read: 0x5F only
    tags 0x80..0xFF   130 walked, only the iterator touched the loop -- nothing consumed

Decompiling that callback: it tests `tag - 0x5F`, assembles bytes 2..5 into a 32-bit value, and
compares it against the constants **2** and **5** -- the DVB `private_data_specifier_descriptor`,
with this firmware carving the specifier into `0..1`, `2..4` and `>=5`. **Which explains the
negative rather than sitting beside it:** a private descriptor has no meaning until a specifier has
declared whose namespace applies, so the box skipped all 128 because none of them was in any
declared namespace. The parser was conformant; the feed was missing the descriptor that gives the
others meaning.

#### Declaring specifier 2 unlocks tag `0xB1`

Same sweep with `[0x5F, 4, specifier]` at the head of the loop, three passes:

    specifier 2   0x5F read, 261 descriptors walked, private tag 0xB1 CONSUMED (0x800BF7DA..)
    specifier 0   0x5F read, 131 walked, no private tag consumed
    specifier 9   0x5F read, 131 walked, no private tag consumed

**The walk count is the corroboration and it was not designed for:** 261 = 131 + 130, so the box
walks the loop a SECOND time once the namespace is declared. Both controls behaved identically to
each other and differently from specifier 2, so it is the VALUE that unlocks it, not the presence
of a `0x5F`.

#### What `0xB1` is, read off its consumer

    800bf7da  lbu  v0,1(a0)       ; descriptor_length
    800bf7dc  li   a3,9
    800bf7de  addiu v0,-2         ; length - 2
    800bf7e0  div  v0,a3          ; entry count = (length - 2) / 9
    800bf7e2  lbu  v0,2(a0)       ; body[0]
    800bf7e4  lbu  a3,3(a0)       ; body[1]
    800bf7e6  sll  v0,v0,8
    800bf7e8  or   v0,a3          ; a 16-bit field
    800bf7ea  cmpi v0,0xffff      ; tested against the 0xFFFF sentinel
    800bf7ee  addiu v1,a0,2       ; v1 = &body[0]
    ...
    800bf80c  addiu v1,0x2        ; step past that field -- entries begin at body[2]
    800bf812  li   a1,0x12
    800bf814  mult a2,a1          ; 18 bytes per OUTPUT record
    800bf81c  lhu  a3,0xa(s0)     ; two halfwords out of the transport context
    800bf820  sh   a3,0x0(v0)     ;   seed each record
    800bf824  sh   a3,0x2(v0)

So the descriptor is **`[0xB1][len][16-bit field, 0xFFFF = sentinel][N x 9-byte entries]`** with
`N = (len - 2) / 9`, and each entry becomes an 18-byte record seeded from the transport it arrived
on. That is the shape of Sky's channel list.

**The probe gave `0xB1` a 4-byte body, so `(4-2)/9 = 0` entries and none was decoded.** The field
breakdown inside the 9 bytes is therefore still OPEN, and that is the next experiment: send a
`0xB1` with `len = 2 + 9N` and a distinctive entry, watch which of the nine offsets are read and
where each lands in the 18-byte record. Do not guess the layout from a public table -- the
firmware has been the specification at every step here, and it is cheaper than being wrong.

**`__siBAT()`'s own comment is still correct and should stay:** it does not emit a private
descriptor, and the layout is not fully established. It is now established enough to say that the
descriptor is `0xB1`, that it needs `0x5F` specifier 2 ahead of it in the same loop, and that its
entries are nine bytes each.


### `sky-eluc.38`: the nine bytes of a `0xB1` entry, and the gate that decides whether any are read

`scripts/digibox-probes/b1-entry-layout.js`. Four passes in one run, three of them controls, and
both controls held. Every byte of the probe identifies itself -- entry *i* byte *j* is
`0xC0 + i*16 + j`, so `0xC0..0xC8`, `0xD0..0xD8`, `0xE0..0xE8`, unique across the loop and never
zero (zero terminates the descriptor walk).

#### The read order IS the layout

With the 16-bit field at body`[0..1]` set to `0xFFFF` and three entries, every one of the 27 entry
bytes was read, by nine PCs repeating once per entry -- so the field boundaries are visible without
guessing a single width:

    0x800BF826 +0   0x800BF828 +1      a 16-bit field
    0x800BF830 +2                      an 8-bit field
    0x800BF836 +3   0x800BF838 +4      a 16-bit field
    0x800BF840 +5   0x800BF842 +6      a 16-bit field
    0x800BF84A +7   0x800BF84C +8      a 16-bit field

and `800bf84e addiu v1,0x9` closes it: nine bytes per entry, confirmed by the stride rather than by
the `/9` in the prologue alone.

#### Where each field goes, read off the stores

    800bf81c  lhu a3,0xa(s0);  sh a3,0x0(v0)    record[0..1]   <- transport context +0x0A
    800bf822  lhu a3,0x8(s0);  sh a3,0x2(v0)    record[2..3]   <- transport context +0x08
    800bf826  entry[0..1]      sh a3,0x4(v0)    record[4..5]
    800bf830  entry[2]         sb a3,0xc(v0)    record[12]
    800bf836  entry[3..4]      sh a3,0x6(v0)    record[6..7]
    800bf840  entry[5..6]      sh a3,0x8(v0)    record[8..9]
    800bf84a  entry[7..8]      srl a1,a3,0x4;  sh a1,0xa(v0)   record[10..11] <- the top 12 bits
    800bf860  (a3 & 8) >> 3    sb a1,0xd(v0)    record[13]
    800bf86c  (a3 & 4) >> 2    sb a1,0xe(v0)    record[14]
    800bf878  (a3 & 2) >> 1    sb a1,0xf(v0)    record[15]
    800bf896  (a3 & 1)         sb v0,0x10(a1)   record[16]
    800bf89e  sltu a0,s1; btnez 0x800bf812      loop while i < entryCount
    800bf8a2  sw a2,0x4(s0)                     the running total, saved back to the context

So the **last 16-bit field is packed: twelve bits of value plus four flag bits**, and each flag is
unpacked to its own byte of the record. `a2` is a running index saved back into the context, so
several `0xB1` descriptors across several transports append into one array rather than each
starting over.

#### The measured layout

    0xB1  [tag][length]  [u16 gate]  [N x 9-byte entries]        N = (length - 2) / 9

      entry +0..1   u16          -> record[4..5]
            +2      u8           -> record[12]
            +3..4   u16          -> record[6..7]
            +5..6   u16          -> record[8..9]
            +7..8   u16 packed   -> record[10..11] = value >> 4, and bits 3,2,1,0
                                    unpacked to record[13],[14],[15],[16]

    record  18 bytes (0x12); [0..1] and [2..3] come from the transport the descriptor arrived on

#### The `u16` at body`[0..1]` is a GATE, and that is what the second pass was for

    header 0xFFFF   all 27 entry bytes read, 30 bytes of the descriptor touched
    header 0x1234   ONLY the length and the two header bytes read -- no entry byte at all

`800bf7ea cmpi v0,0xffff` / `800bf7fa bteqz 0x800bf80c` takes the entry loop **only when the field
is `0xFFFF`**; the other branch re-reads the same two bytes at `0x800BF7FC/FE` and never touches an
entry. So a `0xB1` whose first two body bytes are anything else decodes nothing, silently -- a
well-formed descriptor that the box accepts and ignores, which is precisely the failure this whole
line of work keeps meeting. **Send `0xFFFF`.**

#### The controls

    N = 0 (length 2)    only length and the two header bytes read -- the entry loop did not run,
                        so (length - 2) / 9 is the count and not an approximation of it
    specifier 0         NO non-walker PC read inside the descriptor at all -- 0xB1 stays locked,
                        reproducing sky-eluc.12's result in this run rather than across runs

#### What is still not named

The **semantics**. This is widths and destinations, measured; it is not "service id, type, channel
number". The honest way to name them is the next task rather than a public BAT table: feed a real
line-up in `sky-eluc.12` with distinguishable values per field and read which number appears where
on screen. A twelve-bit value with four flag bits in the last field is the obvious candidate for a
logical channel number, and "obvious candidate" is exactly the phrase this file exists to distrust.

The 18-byte record itself was **not** located in DRAM: the probe searched for a four-byte run of the
first entry's bytes and found only copies of the section, because the record stores derived values
rather than the raw ones. That is a gap in the probe, not a fact about the box -- the read order and
the stores above are the measurement.


### `gort-qbn.2`: two corrections to the gate and the specifier, from the Go port

`internal/broadcast/lineup_firmware_test.go`, 2026-09-20. Building the BAT in Go put both of
`sky-eluc.38`'s conclusions under a sweep rather than a single control, and **both were wider than
the record said**. Neither changes what to broadcast — the builder still sends the `0xFFFF` gate
with the specifier first — but a boundary asserted from one negative control is a boundary nobody
has actually found.

#### The gate admits `0x0000` as well as `0xFFFF`

`sky-eluc.38` compared `0xFFFF` against `0x1234` and concluded *"a `0xB1` whose first two body
bytes are anything else decodes nothing"*. Sweeping nine values, with the entry-read PC
`0x800BF826` counted once per decoded entry:

    0xFFFF  2 entries      0x0001  0        0x8000  0
    0x0000  2 entries      0x00FF  0        0x7FFF  0
                           0xFF00  0        0xFFFE  0
                           0x1234  0

So **two values admit entries and seven do not** — not a range, not a mask, and `0xFFFE` and
`0x0001` sitting either side of an admitted value rule out an off-by-one in the comparison.

Tracing the PCs between the gate read and the entry loop shows they arrive by **different paths**:

    0xFFFF   ...7ea 7ee 7f0 7f2 7f6 7fa -> 80c            bteqz jumps straight to the loop
    0x0000   ...7ea 7ee 7f0 7f2 7f6 7fa    7fc..80a -> 80c falls through, then CONTINUES into it
    0x1234   ...7ea 7ee 7f0 7f2 7f6 7fa    7fc..80a        falls through and stops

`sky-eluc.38` described the fall-through block as one that *"re-reads the same two bytes and never
touches an entry"*, which is true of `0x1234` and false of `0x0000`: the block ends in a second
test that admits zero. Whether that is a deliberate "no restriction" case or an accident of a
zero-check meant for something else is **not established** — the branch at `0x800BF7FC..0x800BF80A`
has not been read.

#### The specifier's VALUE gates the private tag; its POSITION does not

`sky-eluc.12` and `__siBAT()`'s comment both say the `0x5F` must be *ahead* of the `0xB1` in the
same loop, which is what DVB defines — a private_data_specifier scopes the descriptors that follow
it. Moving it **after** the `0xB1`, so the loop reads `[0xB1 …][0x5F 4 … 2][0x41 …]`, still decoded
every entry. Changing only its VALUE from 2 to 9, position untouched, decoded none.

    0x5F before 0xB1, specifier 2     4 of 4 entries
    0x5F after  0xB1, specifier 2     4 of 4 entries
    0x5F before 0xB1, specifier 9     0

`sky-eluc.12`'s own descriptor-walk counts are the likely explanation and were in the record all
along: **specifier 2 walked 261 descriptors, specifier 0 and 9 walked 131** — about double. A
second pass that consumes private tags once the namespace is known would make position irrelevant,
and that is a hypothesis with a number behind it rather than a reading of the code.

**What to emit is unchanged.** The Go builder declares the namespace first and sends `0xFFFF`,
because being accidentally right on one box is not a reason to broadcast a loop that a stricter
parser would reject. What changed is what the record may CLAIM: this box is more permissive than
`sky-eluc.38` and `sky-eluc.12` concluded from one control each.


### `sky-eluc.12`: the line-up goes in, and the box immediately asks for table `0xA1`

`scripts/digibox-probes/feed-a-lineup.js`. NIT, SDT and a BAT carrying
`[0x5F, 4, specifier 2]` then a `0xB1` whose gate halfword is `0xFFFF` and whose entries are the
nine bytes from `sky-eluc.38`, broadcast four times with rising versions. Four services, each field
carrying a value from its own decade so that any number seen anywhere names the field it came from.

#### Four of four services built a record, and the flag unpacking is confirmed on live data

    0x802B5340: 00 20 00 00 00 64 0b b8 17 70 0a bc 01 00 01 00 01 00

      rec[0..1]   0x0020   from the transport context (+0x0A) -- the network id
      rec[2..3]   0x0000   from the transport context (+0x08)
      rec[4..5]   0x0064   entry +0..1      the service id, as fed
      rec[6..7]   0x0BB8   entry +3..4
      rec[8..9]   0x1770   entry +5..6
      rec[10..11] 0x0ABC   entry +7..8 >> 4
      rec[12]     0x01     entry +2
      rec[13..16] 0,1,0,1  the four flag bits, exactly the 0b0101 that was sent
      rec[17]     0x00

The flags were sent as `0b0101` and read back as `0,1,0,1`, so the bit-by-bit unpacking measured in
the disassembly is now confirmed against live data rather than only in a listing. The records are
contiguous at 18 bytes apart (`0x802B5340`, `…52`, `…64`, `…76`), which is the array the running
index in the context is counting.

#### And then the subscription widened by itself

    before the line-up   0x40/ext=0x20  0x73
    after the NIT        0x40/ext=0x20  0x4a/ext=0x1000  0x73
    after the line-up    0x40/ext=0x20  0x42/ext=0x0  0x4a/ext=0x1000  0x73  0xa1/ext=0xbbb

**`0xBBB` is not a number anyone chose.** It is `0x0BB8 + 3` — the value fed in field `+3..4` of
the **last** `0xB1` entry. So that field is the identifier the box turns into a **table-id
extension**, and **table `0xA1`** is what it asks for with it. `0xA1` is in the OpenTV carousel
range, which is where this project has suspected the listings live since the beginning and has
never before been able to show.

That names a second field by observation rather than by a public table: **`entry+3..4` is the
per-service reference the box uses to request its listings.** `0x42` (SDT) appeared at the same
time, having never appeared in any previous run.

**The ladder is the instrument, and this is the rung that proves it.** Each feed has opened the
next subscription: TOT and NIT on a bare boot, the BAT after one NIT, and now `0xA1` and the SDT
after a line-up. Nothing had to be guessed about which table carries listings — the box was made
to ask.

#### What did NOT change, and it is worth saying

The screen is byte-identical (`0x9825B318`, 12 colours, before and after) and the NVRAM is
unchanged at 4,011 bytes written. So the line-up is in the box's memory and has not yet reached
either the display or `/eeprom/svl`. No menu with real text yet — `sky-eluc.12`'s *done when* is
still ahead, and the next rung is what `0xA1` carries.


### Table `0xA1` on PID `0x33`: the listings path, and the signature that gates it

`scripts/digibox-probes/what-is-on-0xa1.js`. Three controls, all of which went silent.

#### The match unit is the specification, and reading it is what unstuck this

The first run pushed a 0xA1 section and **nothing read the payload**. The answer was already in the
match unit the box had programmed, printed by `__siMatches()` and nearly walked past:

    filter 7   tableId 0xA1/mask 0xFE   extension 0x0BBB/mask 0xFFFC
    bytes: ["a1/fe", "b/ff", "bb/fc", "0/0", "0/0", "0/0", "9e/ff", "8b/ff", "0/0", "0/70"]

The array indexes the FILTERABLE bytes, skipping the two length bytes, and that mapping is derived
rather than assumed: index 0 holds `0xA1`, which is section byte 0, and indices 1 and 2 hold
`0x0BBB`, the extension at section bytes 3 and 4. So index 6 is section byte 8 -- the first payload
byte -- and the unit demands **`payload[0] == 0x9E`, `payload[1] == 0x8B`** and
**`(payload[3] & 0x70) == 0`**. A payload beginning `0x40 0x41` is not a 0xA1 section as far as
this box is concerned.

Also in that line and worth keeping: `tableIdMask 0xFE` accepts **`0xA0` and `0xA1`**, and
`extensionMask 0xFFFC` accepts **`0x0BB8..0x0BBB`** -- all four services in a single unit.

#### Signed, it parses; and every control went silent

    PID 0x33, ext 0xBBB, payload signed 9E 8B     a parser read it: 0x800C64F2/FA, 0x800C651E
    PID 0x33, same ext, payload NOT signed        0 reads
    PID 0x33, signed, extension 0xBAD             0 reads
    PID 0x52, signed                              0 reads

So the listings PID is **`0x33`** and not `0x52` -- `0x52` is the one this project already knew and
whose consumer understands only `0x40`/`0x42`/`0x70`. Both appear as new armed filters after a
line-up, which is why the probe refused to pick between them and pushed to each.

#### The payload is another descriptor loop, and the walk decodes it exactly

    int FUN_800c64cc(base, index, wantedTag, userData)        // callback in $t0
      rec = base + (index & 0xFF) * 8;
      end = *(int *)(rec + 0x1C);
      for (p = *(char **)(rec + 0x14); p < end; p += p[1] + 2)
          if (*p == wantedTag || wantedTag == -1)
              callback(p, i++, userData);

A sibling of `FUN_800ad590` -- same shape, but bounded by an END POINTER instead of terminating on
a zero tag. The reads it made against the probe payload close the arithmetic with nothing left over:

    +2..+5   one `lw` -- the loop bound, read out of the record that precedes the payload
    +6       tag    = 0x46
    +7       length = 0x47 = 71      ->  next at 6 + 71 + 2 = 79
    +79      tag    = 0x8F
    +80      length = 0x90 = 144     ->  next at 79 + 144 + 2 = 225, past the 96-byte payload, stop

**So a 0xA1 section is a descriptor loop beginning at payload offset 6** (section byte 14), walked
by tag and length. The callback never fired because neither `0x46` nor `0x8F` was the tag it wanted
-- which is the identical situation the BAT was in before the tag sweep, one layer down.

#### The shape so far

    sec[0]      0xA1                     (0xA0 also accepted -- tableIdMask 0xFE)
    sec[1..2]   section_length
    sec[3..4]   extension = the per-service id from 0xB1 entry field +3..4, masked 0xFFFC
    sec[5]      version / current_next
    sec[6..7]   section_number / last_section_number
    sec[8..9]   0x9E 0x8B               the signature the hardware match unit demands
    sec[10..13] payload[2..5]           read as one word -- the loop bound
    sec[14..]   payload[6..]            a tag/length descriptor loop

#### What is open, and the method is already proven

Which descriptor tag the listings callback wants. The sweep that answered this for the BAT -- fill
the loop with one descriptor of every tag, watch the copy, see which body a non-iterator PC reads,
and run the known-negative half as the same run's own control -- transfers directly. Tag `0x00`
cannot be probed there because that loop is bounded by a pointer rather than by a zero tag, so
unlike the BAT sweep the whole space `0x00..0xFF` is reachable.


### A running broadcast stops the box drawing, and acquisition is fine (`sky-02me.18`)

The demo page was to put a multiplex on the air by default -- a Digibox tuned to a dead transponder
searches for listings for ever and says so, which is correct behaviour that reads to a visitor as a
broken emulator. It was built, measured, and **reverted to opt-in (`?si=1`) because it costs the
whole interface.** A/B in one run, same walk on both, pressed from a settled box:

                             press sky (0x7D)                then tv guide (0x80)
      no broadcast    9825b318/12 colours -> f3634409/37    -> 27538f44/34, stable to 30s
      broadcast on    9825b318/12 colours -> 9825b318/12    -> 9825b318/12, stable to 30s

Byte-identical across both presses, for thirty seconds. Not a slower draw and not a different
screen -- nothing at all.

**And the broadcast itself is healthy, which is what makes this a lead rather than a dead end.**
124 sections, **0 refused**, and the box walks the entire ladder by itself to
`0x40/ext=0x20 0x42/ext=0x0 0x4a/ext=0x1000 0x73 0xa1/ext=0xbbb` -- it builds its service records
and subscribes to its own listings with no probe involved. Acquisition works and the interface
stops, and those two facts sitting together are the finding.

**Why it matters beyond the demo:** every screen this project has reached was reached on a box
nobody was transmitting to. If a real broadcast takes the application somewhere it draws nothing,
the interface work and the SI work have been proceeding on two different machines -- and the
listings cannot arrive on the box that draws. The first suspect is `skyGatesTick()`, the declared
fiction that answers two screen gates once the task list reaches 42, because the races behind it
(`sky-eluc.29`, `sky-eluc.32`) are real, still open, and about TIMING -- which is exactly what a
busy acquiring box changes. That is a suspect, not a diagnosis; `sky-02me.18` carries the method
and its controls.

**`__siBAT()` gained `opts.lineup` in the same work and keeps it.** It emits
`[0x5F, 4, specifier 2]` then a `0xB1` with the `0xFFFF` gate and nine-byte entries -- everything
measured in `sky-eluc.12` and `sky-eluc.38`. Without the option its output is unchanged, because
every probe written before today measured a box being fed a plain BAT and changing the shared
builder's default would have moved all of their baselines silently.


### `sky-02me.18`: with a broadcast the application builds NO SCREEN, and a warm box will not boot at all

`scripts/digibox-probes/where-drawing-stops.js`, run as a matched pair. The probe reports which
state it is actually in rather than trusting the flag it was given, and it refuses to conclude when
its own known-good control fails.

**It did not go straight to the suspect, and that mattered.** `skyGatesTick()` was the obvious
candidate, and obvious candidates are what this file exists to distrust; jumping there would have
been testing a theory rather than locating a fault. The question answered first was WHICH LAYER goes
quiet, because the counts separate them without any interpretation:

    the key never arrives          -> an input-path problem, nothing to do with drawing
    no widgets built               -> the application is not building a screen at all
    widgets but no damage/blits    -> it builds one and declines to rasterise it
    widgets and blits, no surface  -> the content or the destination is wrong

#### The pair, pressing sky on a settled box

                     keyEvents  dispatcher  widgets  applies  damage  blits  surface
    silent               2          17         62      534      48     11    12 -> 37 colours
    broadcasting         2          12          0        0       0      0    unchanged

**The key arrives identically and the application then builds nothing.** Not a slower draw, not a
rasterisation fault, not a blitter fault -- zero widgets. The same holds for tv guide (silent: 249
widgets, 7 blits, screen moves; broadcasting: 0, 0, unchanged). The silent half is the control and
it reproduces `sky-02me.2`'s recorded figures exactly, so the instrument is sound.

**So the suspect is now the INDICATED next step rather than a guess.** Building no screen at all is
precisely what the two application screen gates decide (`sky-eluc.28`, `.30`, `.31`, `.32`), and
`skyGatesTick()` is the declared fiction that answers them once the task list reaches 42. A box busy
acquiring is a box with different timing, and the fiction fires once. **Do not answer the gates
earlier** -- tried twice, reverted twice, the boot stops at 28 tasks instead of 42.

#### And a WARM box with a broadcast never boots at all

Measured twice, `--carousel` and `?si=1` alike, so it is not about who starts the carousel:

    warm + broadcast   22 tasks, 1.92G and 1.99G instructions, 509 s and 514 s -- never reaches 42
    cold + broadcast   42 tasks, Ready at 533M, acquires cleanly (138 sections, 0 refused)

A healthy **cold** boot passes 22 tasks at ~288M and finishes at ~447M, so the stall sits exactly on
the 22-task milestone and then burns four times a whole boot going nowhere.

**The cold/warm split is why two harnesses disagreed, and the disagreement was the finding.**
`scripts/digibox-probe.mjs` uses `launchPersistentContext`, so every probe run is WARM; the gate and
the ad-hoc scripts use `chromium.launch()`, so those are COLD. Reading the two as the same
configuration would have made this look intermittent. It is not: it is two different faults, and
which one appears depends on whether the NVRAM has been written.

This is also a direct data point for `sky-02me.11` (*does `--carousel` need `--ack-all` to boot?*):
on a warm box, `--carousel` **alone** does not boot.


### `sky-02me.18`: the repetition is irrelevant — SIXTEEN hand-pushed sections stop the drawing

`scripts/digibox-probes/hand-fed-and-still-drawing.js`, `--cold`. Every earlier measurement of the
no-draw used the repeating carousel, so two very different worlds still fitted the evidence: either
the REPETITION keeps the SI manager permanently busy and a one-shot feed would be fine, or
acquisition itself is what stops it. That is worth separating before spending a session on gate
tracing, because the first world makes listings reachable today.

**It is the second world.**

    control: sky, BEFORE any feed        62 widgets, 11 blits, screen moved  -- the instrument is sound
    feed: 4 rounds of NIT + SDT + BAT + TDT by hand, 16 sections, carousel confirmed OFF throughout
          round 1 -> the table widens to 0x4a/ext=0x1000
          round 2 -> 0x42/ext=0x0 and 0xa1/ext=0xbbb, the full ladder
    tv guide  after the feed   keyEvents 2, widgets 0, applies 0, damage 0, blits 0
    back up   after the feed   keyEvents 2, widgets 0, applies 0, damage 0, blits 0
    sky again after the feed   keyEvents 2, widgets 0, applies 0, damage 0, blits 0

Tasks still 42 at the end, so nothing wedged; the keys keep arriving; and the surface stays on
`0xF3634409` — **the menu the control press drew**. The box is not blank and not broken. It is stuck
on the last screen it built and will not build another.

**So the fix is not a rate.** Sixteen sections, pushed by hand, with nothing repeating afterwards,
are enough to stop it — and the carousel's 124 sections were never the mechanism.

**And this is now a far cheaper reproduction than the carousel one**, which is the useful part for
whoever takes the gate tracing: boot cold, press sky and watch it draw, push a handful of sections,
press anything and watch it not. No two-minute broadcast, and a control that passes inside the same
run immediately before the failure.

**The obvious next narrowing is a bisect**: the ladder opens in stages (round 1 the BAT, round 2 the
SDT and `0xA1`), so feeding one table at a time and pressing after each would name WHICH table
costs the drawing — and that names the code path, which is most of the gate work done. Nothing here
rules out that it is simply "the first section of any kind".


### `sky-02me.18` NARROWED: a PLAIN BAT is what costs the drawing

`scripts/digibox-probes/which-table-stops-the-drawing.js`, `--cold`. One table at a time, two
presses after each, alternating sky and tv guide because those are transitions a healthy box always
redraws for -- pressing the same key twice would be the trap, since a box already on the menu has no
reason to rebuild it and "no widgets" would then mean "nothing changed".

    stage 0   nothing fed -- the control      sky 62 widgets · tv guide 249      draws
    stage 1   TDT only                        sky 62 · tv guide 249              draws
    stage 2   NIT                             sky 62 · tv guide 249              draws
    stage 3   SDT                             sky 62 · tv guide 249              draws
    stage 4   BAT, plain                      sky 0  · tv guide 0                DEAD

Keys arrived on every press including the dead ones (`keyEvents 2` throughout), and the task list was
still 42 at the end. The walk stops at the first quiet stage on purpose: after the box has stopped
drawing, every later stage is a reading about a box that was already broken.

**It is not the line-up.** Stage 4's BAT is `__siBAT()` with no options at all -- a bouquet_name and
a service_list and nothing else. No `0x5F`, no `0xB1`, no nine-byte entries, and no `0xA1`
subscription. Everything measured in `sky-eluc.12` and `sky-eluc.38` is exonerated.

**And it is not SUBSCRIBING to a BAT either.** Stage 2's NIT is what opens `0x4A/ext=0x1000`, and the
box kept drawing perfectly for two more stages after that. It is *parsing* one that does it.

So the subject is now **table `0x4A`'s parser** rather than "SI" -- a named code path with a named
builder (`SKY_BAT_SVL`) and a descriptor walk this project has already read
(`FUN_800ad590`). Note also that the BAT is what brings `0x42/ext=0x0` into the match table, so it
changes the box's subscriptions as well as its data.

#### A hypothesis, flagged as one

Building a service list is the moment the box first has an idea of *which service it is on*. An
application that then declines to draw is behaving like one waiting for something about that service
-- which is the shape of the two screen gates, and of `sky-eluc.29` (an AV flag raised 592,817
instructions after the decision reads it). That would make the no-draw the same family as the
original blank-screen work rather than a new fault, and it would explain why `skyGatesTick()` -- a
fiction that fires once, at 42 tasks -- does not cover it.

**That is a story, not a measurement.** The next run should trace the two gate decisions across the
stage-4 boundary, where there is now a control that passes seconds beforehand.


### `sky-02me.18`: the minimal repro is NIT→BAT, and the screen-gate hypothesis is REFUTED

`scripts/digibox-probes/does-the-bat-close-the-gate.js`, `--cold`, two runs.

#### "The stage at which it died" was not "the table that does it"

The bisect fed cumulatively -- TDT, NIT, SDT, then a plain BAT -- and the drawing died at the BAT.
The obvious reading was *a plain BAT costs the drawing*. **It does not.** Run one of this probe
pushed two plain BATs at a box that had been fed nothing else and it kept drawing perfectly: 62 and
249 widgets, gate word never written.

**The difference is the NIT, and it is a difference in whether the BAT is PARSED AT ALL.** A bare
box subscribes to `0x40` and `0x73` only; the NIT is what opens `0x4A/ext=0x1000`. Delivery here is
by PID, so a BAT pushed at an unsubscribed box arrives and is dropped -- indistinguishable, from
outside, from a BAT that was harmless. The probe caught its own contradiction and refused to read
the gate result as meaningful, which is the only reason this was not written up as a BAT fault.

#### The minimal pair, with the intermediate control

    control: sky / tv guide, nothing fed      62 / 249 widgets      draws
    NIT x2  -> table opens to 0x4a/ext=0x1000
    sky, after the NIT ONLY                   62 widgets            draws   <- the NIT is not it
    BAT x2  (plain, no options)
    sky / tv guide, after the BAT             0 / 0 widgets         DEAD

So the minimum that reproduces it is **NIT then BAT**. The TDT and the SDT are irrelevant, and the
NIT alone is harmless. It is a BAT that the box actually parses.

#### And the screen gate is NOT the mechanism

`skyGatesTick()` answers exactly two gates and fires once, at 42 tasks, so the natural story was
that the BAT parse shuts one of them again and nothing re-answers it. Tested rather than illustrated,
with a write watch (a word written and restored would read identical at both ends) and a repair
attempt (a hypothesis that can only be confirmed is not being tested):

    writes to 0x80054F84 across the BAT      0
    the word, before and after               0x00000000 -> 0x00000000
    re-poked to zero, then pressed again     0 widgets, still dead

**Nothing touched the gate word, and re-answering it changes nothing.** The service-list story --
that a box which has just learned which service it is on then waits for something about that service
-- is wrong **as stated**. That negative is worth as much as the positive would have been: it stops
the next session tracing two gates that are not involved.

#### What is left

The other gate is a FLASH byte in the EPG's o-code, and the application does not write flash, so it
cannot have closed on its own. That leaves the parse itself: a BAT that is parsed leaves the box in
a state where the widget builder declines, and neither of the two known gates is how. The next
subjects are the widget builder's own entry conditions and whatever the BAT parse writes -- and the
repro is now two tables and one press, with a control that passes seconds beforehand.


### `sky-02me.18` DIAGNOSED: it is not refusing to draw, it is appending to `favchn` for ever

Four probes, each one correcting the reading of the last. The title of the issue is now wrong: a
broadcast does not stop the box drawing. It puts it into a **non-terminating EEPROM list append**,
and the drawing never happens because the box never gets back to it.

#### "Refusing" was an assumption every previous measurement had baked in

Every press this project had measured waited **nine seconds** and read "no widgets" as "it refuses".
`is-it-refusing-or-busy.js` pressed and then watched for two minutes:

    t(s)    eeReads  eeWrites  saves      widgets
       5        161       167     18            0
      62       2592      2590    214            0
     124       6179      6175    440            0      still climbing, perfectly linear

Reads track writes to within half a percent at every sample; 440 whole-device saves; 299M
instructions burned. A healthy press does **zero** EEPROM work and draws in five seconds. So it is
neither refusing nor merely slow -- it is in a loop.

#### The loop, read off the wire

`__eeTx()` logs every transaction, **and it caps at 400, which the boot alone fills.** The first
attempt read it after the press and got `loggedDuringThisPress: 0` against a full log -- the boot's
first 400 transactions, presented as a record of what had just happened. It said so only because it
compared the length before and after. `__eeTxClear()` was added to the page for this and the window
is now explicit. With it, one cycle repeated ~18 times in 25 s:

    0x1FB6  n=10   66 61 76 63 68 6e 00 00 00 00        CONSTANT -- ASCII "favchn"
    0x1883  n=25   00 … 10 … 01 … 16 ff ff ff ff 03 33 91   CONSTANT
    0x3C46  n=4    00 00 00 1a                          CONSTANT  (26)
    0x3C44  n=2    00 88 -> 00 90 -> 00 98 -> 00 a0     ADVANCES BY 8
    0x1BD0  n=8    00 00 00 00 00 00 00 00              ADDRESS ADVANCES BY 8
                   0x1BD8, 0x1BE0, 0x1BE8 …

**A 2-byte offset at `0x3C44` is incremented by 8 and an 8-byte ZERO record is appended at
`0x1BD0 + offset`, with three constant records rewritten on every pass.** `favchn` is the header it
keeps rewriting. It is a list append whose terminating condition never fires -- not a read-back that
fails to verify, which is what the shape first suggested.

#### Read the data, not the probe's verdict

Twice in this sequence the probe's own one-line classifier was cruder than the numbers it printed,
and twice the numbers were right. It called the device attribution "BOTH went up" when the EEPROM
had gone from **exactly zero** to hundreds while a continuously-polled demodulator rose modestly;
and it called this loop "one address, one payload, a read-back that never matches" because it only
inspected the hottest address, when the log plainly shows a five-transaction cycle with two
advancing members. The summaries are convenient and the tables are the evidence -- a lesson worth
keeping, because an automatic verdict is exactly the kind of thing that gets quoted later.

#### Where to look next

Whatever writes EEPROM offset `0x3C44`. The count at `0x3C46` is a constant 26 while the offset
climbs past it without stopping, so the comparison that should end the loop either is not against
that count or is not reached. `favchn` is a favourites list, which is the box doing something with
the service list it has just built -- so this is downstream of the BAT parse rather than in it.

**And it is very likely OUR bug rather than the firmware's**, because a real box plainly does not do
this: the candidates are the 24C128 model's page wrap, its behaviour at a page boundary, or what it
returns for a record that has never been written (blank is `0xFF`, and the records being appended
are zeros).


### `sky-02me.18`: the path the dead press takes, named layer by layer

Continuing from the `favchn` loop. The question was who drives it, and two attempts to answer it from
the EEPROM transaction log FAILED in a way worth recording before the answer, because the failure is
reusable.

#### The caller is not reachable from an I2C transaction, twice over

The page's demodulator log captures a stack chain at each transaction, so the same was added to the
EEPROM log. It produced `ra = 0x00000000`, an empty chain and the same `pc` for every transaction --
`0x80006F30`, the I2C **completion interrupt**. Moving the capture to the arm-START, expecting task
context, gave the same shape at `0x80006C7E`: inside the driver's own write routine.

**The task that asked for the write is blocked on a semaphore and its frame is not on the stack at
either point.** That is not a bug in the capture, it is what an asynchronous driver looks like: the
transaction is begun by one context and completed by an interrupt, and neither is the caller. Two
attempts was the right number to stop at -- the third would have been iterating a technique that had
already been shown not to apply.

#### The histogram does not care who is on the stack

`what-the-dead-press-does-instead.js` diffs the PC histogram of a healthy press against two dead
ones. Grouped by 0x100 and with the known I2C driver excluded:

    ONLY IN DEAD -- the path taken instead        ONLY IN HEALTHY -- the drawing path
      0x80029200  11,895 hits   39 PCs              0x80000E00  486,998 hits   30 PCs
      0x800B4800  10,788        74                  0x80094100  236,845       116
      0x80030000   7,392        44                  0x80078200  137,246        55
      0x800B6400   3,564        81                  0x8006C700   83,972        56
      0x800B6500   3,388       106                  0x8006B500   71,371        66
      0x80011000   3,007        97                  0x8009BF00   65,732        51
      … 0x800B4600-0x800B6900 throughout            …

Noise floor 189 against 1,989, so the split is real.

**The dead press runs a whole subsystem at `0x800B4600`-`0x800B6900`** -- twenty-odd regions, up to
106 distinct PCs each -- plus the NVRAM layer at `0x80029xxx` and the I2C driver. `0x800BF7DA`, the
`0xB1` consumer found in `sky-eluc.12`, sits just above that band, so this is the service-list
subsystem doing something with the list it has just built.

#### The two routines named so far

`FUN_8002926c` at `0x8002926C` -- **the NVRAM access layer**, and the one PC that ran 359 times per
dead press and never once on a healthy one:

    FUN_8002926c(offset, *lenPtr, buf)
      if (offset + *lenPtr < 0x4001 && buf) {          // 16 KB + 1: the device size
          if (shadow[0x4000] == 0) *lenPtr = device_io(offset, *lenPtr, buf);
          else if (obtain(shadow[0x4004], 100) == 0) { // a semaphore, timeout 100
              memcpy(buf, shadow + offset, *lenPtr);
              release(shadow[0x4004]);
          }
      }

A 16 KB RAM shadow of the EEPROM with a flag byte and a semaphore just past it, and either the
device or the cache depending on that flag.

`FUN_800b4798` at `0x800B4798` -- the hottest dead-only routine, **a slot allocator**:

    n   = pool[4] / pool[8];                    // slots = size / entry size
    idx = (random() * n) >> 8 & 0xFF;           // a starting index
    for (i = 0; i < n; i++) {
        if (bitmap[idx >> 3] & (1 << (7 - (idx & 7)))) return idx;
        idx = (idx + 1) & 0xFF;                 // <-- masked to 256
        if (idx >= n) idx = 0;
    }
    return pool[0x20];                          // the fallback

**`idx` is masked to `& 0xFF` while the loop counter runs to `n`.** If a pool ever has more than 256
slots, the search rescans the first 256 for ever and can never reach the rest -- and a caller written
as "allocate until it succeeds" would spin. That is a HYPOTHESIS about a shape, not a finding: it is
the firmware's own code and presumably correct on a real box, so the interesting question is whether
`n` is bigger here than it would be there. **`pool[4]` and `pool[8]` are the two numbers to read.**

#### Where this leaves it

The chain is named end to end -- service-list subsystem → NVRAM layer → I2C driver → a `favchn`
append that never stops -- and the next step is arithmetic rather than archaeology: read `pool[4]`
and `pool[8]` at `FUN_800b4798`, get `n`, and see whether it exceeds 256. If it does, find what set
it, because that is very likely something our BAT or our EEPROM model made larger than a real box
would.


### `sky-02me.18`: the allocator is innocent, and the pool is DRAINING

`scripts/digibox-probes/how-many-slots.js`, `--cold`. Two hypotheses about `FUN_800b4798` put up and
both knocked down, which is what they were built for.

#### Both refuted, by arithmetic

    pool 0x80312A9C   pool[4] size 10,240   pool[8] entry 64   ->  n = 160 slots

**The `& 0xFF` mask is never reached.** `n` is 160 in every sample, so the index never wraps past
255 and the "a pool bigger than 256 can never be fully searched" shape is a red herring. It is kept
in the probe's own comment rather than deleted, because the next reader will see that mask too and
deserves to know it was checked.

**And the bitmap is not empty either.** The second idea was that the search never succeeds and falls
through all 160 iterations to the fallback every time. It has **121 of 160 bits set**, so the search
succeeds immediately most of the time. The allocator is doing its job.

#### What the same read found instead, which nobody predicted

    sample 0   121 of 160 bits set
    sample 1   120
    sample 2   119

**One bit lost per sample, seconds apart, and 39 slots already gone by the first reading.** The
allocator succeeds, the caller takes a 64-byte slot on every iteration, and **nothing ever gives one
back.** The pool is draining at roughly one slot per allocation and will be empty after about 121
more.

So the allocator is a *symptom*: it is called ten thousand times a press because something above it
is looping, and each turn of that loop costs a slot as well as an EEPROM append. Two consumables,
one loop.

**That is a gift for whoever takes this next**, because a draining counter is a clock. Watch the
bitmap to zero and see what the box does when the pool is exhausted -- it will either start failing
in a way that names the caller, or fall back to `pool[0x20]` and change behaviour visibly. Either is
more informative than the loop's steady state, and it arrives on its own within a couple of minutes.

#### Three hypotheses, three refutations, and why that is not waste

The screen gates, the `& 0xFF` mask, and the empty bitmap were each cheap to test because each was
stated as something that could FAIL -- a write watch with a repair attempt, an arithmetic read, a
twenty-byte peek. None survived. What survives is the map: the key arrives, the application declines
to build a screen, the service-list subsystem at `0x800B4600`-`0x800B6900` loops, the NVRAM layer
runs 359 times a press, `favchn` grows for ever, and a 160-slot pool drains one slot at a time. The
next subject is the CALLER of `FUN_800b4798` -- and the pool exhaustion is the cheapest way to make
it announce itself.


### `sky-02me.18`: eight hypotheses dead, and the bind stated precisely

A long sequence of cheap falsifications. None survived, so the value is the map and the eliminations
rather than a fix. Recorded in full because each one is a place the next person would otherwise go.

    1  the two screen gates            write watch + a repair attempt: nothing writes the DRAM gate
                                       across the BAT, and re-answering it restores nothing
    2  the carousel's repetition       16 hand-pushed sections do it; no rate is involved
    3  "it is refusing", not busy      it IS busy, for 113-137 s, and then stops and still refuses
    4  the allocator's & 0xFF mask     n = 160, within 256; never reached
    5  an empty allocator bitmap       121 of 160 bits set; the search succeeds
    6  pool exhaustion ends the loop   the loop stops with 56 slots still free, and no recovery in
                                       340 s of watching with a press every 14 s
    7  a blocked task                  NO task changed status between healthy and stuck; eight are
                                       still being scheduled; the RTOS is entirely healthy
    8  waiting for the 0xA1 listings   twelve well-formed, accepted sections across all four
                                       services it asked about: still zero widgets

#### What IS established

The loop **terminates**. It runs 113-137 s, writes ~6,900 EEPROM bytes across ~460 whole-device
saves, consumes ~104 of 160 pool slots, and stops with slots to spare. Afterwards the box is
completely quiescent: counters frozen for 227 s, all 42 tasks in their healthy states, keys arriving
at the input layer, and **not one widget built**. The surface stays parked on the menu the last
healthy press drew. It is an application-level refusal that outlives the work that triggered it.

**A reboot clears it** -- a warm boot after a feed reaches 42 tasks in 68 s and draws normally, 62
widgets on sky and 249 on tv guide.

#### And the bind, which is the thing to understand

**The line-up does not survive the reboot.** After the reload the match table is back to
`0x40/ext=0x20 0x73`, the NVRAM byte count is unchanged at 2,749, and the guide says *"Searching for
listings"*. So:

  * listings require a `0xA1` subscription
  * the subscription requires a parsed BAT carrying a line-up
  * parsing that BAT stops the box building screens
  * only a reboot clears it, and the reboot loses the line-up

**"Has a line-up" and "can draw" are currently mutually exclusive**, and that is why the guide cannot
show listings yet. It is one bug, not a missing feature.

#### A note on automated verdicts, for the third time

This run's probe printed *"IT DRAWS ON A WARM BOOT WITH THE LINE-UP STORED"*. It was wrong: the
`tables` line in the same output said `0x40/ext=0x20 0x73` and the screenshot said "Searching for
listings". The verdict had checked whether widgets were built and not whether the line-up was still
there. Three times now a convenient one-line summary has been cruder than the table beside it, and
three times the table was right. **Read the numbers.**

#### The instrument that has not been tried

Everything above diffs NATIVE code. The EPG is an OpenTV **bytecode** application, and "decline to
build a screen" is far more likely a decision in o-code than in the MIPS the histogram sees -- which
would explain why eight native-level hypotheses all missed. `__readWatch(lo, hi, {fromPc})` pointed
at the interpreter's opcode fetch is an o-code execution trace, and the CODE chunk's address is
known. That is the next technique, and it is a piece of work rather than another four-minute probe.


### `sky-02me.18` ANSWERED: the box is WAITING, and the wait is our clock rather than a fault

*2026-09-15. The o-code trace the previous section asked for, and it took three probes: one that
found the loop, one that refuted the obvious cause, and one that measured where the time goes.
Two claims recorded above are withdrawn and the issue's own title was wrong.*

#### The instrument the eight dead hypotheses were missing

`scripts/digibox-probes/ocode-healthy-vs-dead.js`, `--cold`. `__readWatch` over the EPG's CODE
chunk, PC-filtered to the interpreter's main fetch site `0x80069298`, so every logged entry is one
bytecode instruction. Feed: NIT ×2, 8 s, **assert `0x4A` is subscribed**, then BAT ×2 plain.

                        key        widgets  applies  blits  surface     o-code  distinct
    control sky         sky            62      798     13   moved       34,417        --
    control tv guide    tv guide      249     1212      7   moved       41,895     8,228
    after the NIT only  sky            62      798     13   moved       35,737        --
    fed tv guide        tv guide        0        0      0   UNCHANGED      258        43
    fed sky             sky             0        0      0   UNCHANGED      258        43
    after the work      tv guide      121      450      5   moved       23,627     4,480

**The dead press runs 258 bytecode instructions as six identical passes of one 43-instruction
sequence**, `0x9FC85F41`–`0x9FC85F92`, and never reaches the application's event loop at
`0x9FC4A538`. A healthy press covers 8,228 addresses. Independently reproduced by
`ocode-the-dead-loop.js`, which computed the repeat period from the stream itself: **period exactly
43, seven passes**. So the application is not declining to build a screen — **it is never asked**,
because the interpreter is somewhere else. That is precisely the class of thing eight native-level
hypotheses could not see, and it is why they all missed.

#### The loop, decoded and validated

`scripts/ocode-disasm.py --check` over `0x9FC85F41`–`0x9FC85F93`: 46 boundaries decoded, **43 of
them addresses the machine actually fetched**, the other three being the two never-taken arms.

    9fc85f41  push_ds ; add DS+0x00019A20 ; get     limit = DS[0x00019A20]
    9fc85f48  push_fp_nn 0xC2                       i = [fp-248]
    9fc85f4a  jltu_nn                               if (i <u limit) fall through
    9fc85f4c  jmpr 0x9fc85f94                       EXIT -- not taken in any observed pass
    9fc85f4f  push_fp_nn 0xF5 ; push_fp_nn 0xC2 ; mul     offset = stride * i
    9fc85f58  scall (2,0x28)                        an NVRAM read
    9fc85f72  scall (2,0x2D)                        an NVRAM write
    9fc85f7d  jne 0x9fc85f80                        NOT taken; 0x5f80 is `push_2 ; pop_fp_nn 0xF7`,
                                                    the error arm, never executed
    9fc85f83  push_fp ; add -0xF8 ; dup ; get ; add 1 ; swap ; put      [fp-248]++
    9fc85f92  jmpr 0x9fc85f41                       the back edge

**The back edge proves itself.** `41 ad` is `jmpr next + int8` = `0x9FC85F94 − 83` = `0x9FC85F41`,
the loop head, exactly — no coincidence produces that. And `push_fp_nn 0xC2` is a **word** index
(−62 × 4 = −248), which is the same slot `add -0xF8` increments at the bottom, so the counter tested
at the top and the counter incremented at the bottom are one variable. **It is a counted `for`
loop.** It cannot run for ever and it did not: it ended at 128 s and at 140 s in two separate runs.

#### The obvious cause, measured and refused

`scripts/digibox-probes/eeprom-mirror-is-the-cost.js`. `eeSave()` built a 16,384-character string,
base64-ed it and called `localStorage.setItem` **synchronously at every I2C STOP**, for an
eight-byte record. That is the suspect anyone would reach for, and the page's own note had already
guessed the fault was ours.

**It is 0.2% of the time.** Instrumented at the call: **289 ms across 383 whole-device saves, out of
128 seconds.** Batching the mirror was implemented, measured and **reverted** — a fix that changes
nothing means the diagnosis is wrong rather than too small, and leaving it in would have bought a
fifth of a percent at the price of a window in which NVRAM can be lost. The timing counter stays
(`__i2cState().eeprom.saveMs`) so that nobody re-derives the negative.

#### Where the time actually goes: nowhere. The box is idle.

`scripts/digibox-probes/where-the-128-seconds-go.js`. `__pcHist` has no clear function, so every
figure is a **difference** between two cumulative `__rangeHits` samples — ten-second windows across
the whole rebuild, against a ten-second idle baseline taken on the same box before anything was fed.

    idle, nothing fed     30,972,906 instructions / 10 s   hottest 0x800D35DC ×3,266,037
    during the rebuild    24.5M – 33.9M      / 10 s        hottest 0x800D35DC ×3,369,425

**The same PC, at the same rate.** `0x800D35DC`–`0x800D35F4` is a seven-instruction spin taking
~12% of samples *per address*, so about **85% of everything executed in those 140 seconds is that
one idle loop**. The "~44,000 instructions per EEPROM byte" is idle spinning, not driver work.

**So the 140 seconds is OUR CLOCK.** This emulator runs at ~3.1M instructions/s — measured, from the
idle baseline — against a real VR4111's ~81 MHz. The same ~345M instructions on the hardware is
about **four to five seconds**. A real Digibox rebuilding its service list into NVRAM after a bouquet
change takes a few seconds and shows a message while it does; ours takes over two minutes because it
is thirty times slower, and during it the interpreter is genuinely busy elsewhere.

#### What is withdrawn

- **"An application-level refusal that outlives the work that triggered it."** It is not a refusal.
  The key arrives, the interpreter never looks at it, and both facts are visible in the same trace.
- **"Only a reboot clears it," and with it the whole bind** — *listings require a subscription →
  which requires a parsed BAT → which stops the box drawing → which only a reboot clears → and the
  reboot loses the line-up.* The third link is false, so the chain does not close. **"Has a line-up"
  and "can draw" are not mutually exclusive**; `sky-eluc.12` is not blocked by this.
- The issue's title, *"appends to the favchn EEPROM list for ever, and never gets back to drawing"*,
  was wrong in both halves and has been changed.

#### A trap that caught two probes in one session, now mechanised

`eeprom-mirror-is-the-cost.js` pressed **tv guide while the box was already on the guide**, and
`where-the-128-seconds-go.js` pressed **sky while already on the menu**. A box with nothing to
rebuild reports zero widgets, which is byte-identical to the fault being measured. Both probes had
the screen they pressed *from* in their own output and neither looked — the same shape as the
three-wrong-findings warning in the next-session notes, arriving twice more in one afternoon.

A caveat did not fix this the first time, so it is not a caveat now: `assertRedrawable()` **throws**
on a repeated key, in all four o-code probes, and the guard was run against a repeat to confirm it
fires rather than assumed to.

### `sky-02me.17` ANSWERED: the `0xA1` descriptor loop consumes exactly one tag, `0xB5`

*2026-09-15. The sweep, its coverage failure — which turned out to BE the finding — and the
neighbourhood, which matters more than the single tag.*

#### The sweep

`scripts/digibox-probes/which-tag-the-listings-loop-wants.js`, then
`which-tag-the-listings-loop-closes-the-sweep.js`, both `--cold`. A `0xA1` section on PID `0x33`,
extension `0x0BBB`, payload signed `9E 8B`, descriptors from payload offset 6, one descriptor per
tag with a distinctive four-byte body. Watched on the **copy** at `0x802A761C`, located by finding
each pass's own marker afterwards, never on the ring.

    0x01..0x40          64/64 walked    silent
    0x41..0x80          64/64           silent
    0x81..0xC0          53/64           A CALLBACK FIRED on 0xB5 -- and only 53 BECAUSE of it
    0xC1..0xFF          63/63           silent
    0x00, 0x11, 0x22     3/3            silent -- and 0x11 BEHIND 0x00 was walked
    0xB5 alone           1/1            consumed
    0xB6..0xC0          11/11           silent     <- the eleven the first sweep never reached
    0x81..0xB4          52/52           silent     <- the covered half, as the second run's control

Both negative controls held in their own run: an unsigned payload and an extension outside the
unit's `0xFFFC` mask each read nothing at all. And **tag `0x00` is not a terminator here** — the
descriptor behind it was walked — unlike the BAT's loop, where it is. That was predicted from the
loop being pointer-bounded and is now measured rather than assumed.

#### The coverage failure WAS the finding

The `0x81..0xC0` pass walked 53 of 64 and stopped. `0x81 + 52` is `0xB5`, exactly. The callback
returns **1** on a match and `FUN_800c64cc`'s loop is `if (cb(...) != 0) return` — so **consuming a
descriptor ends the walk.** The probe's coverage guard fired for a real reason and the reason is the
answer; it refused to read the remaining silence as a negative and reported *"I could not check"*,
which is why the eleven unchecked tags were run separately instead of shipping as a clean sweep.

#### The consumer names its own constant

    800c6b42  lbu v0,0(a0)          tag = desc[0]
    800c6b44  addiu v0,-0xb5
    800c6b48  bnez v0 -> 800c6b62   not 0xB5: return 0, keep walking
    800c6b4a  desc[2..3] -> out[0..1]     a big-endian u16
    800c6b54  desc[4..5] -> out[2..3]     a big-endian u16
    800c6b5e  li v0,1               return 1: STOP the walk

Not an inference from a histogram — the instruction compares against `0xB5`.

#### The neighbourhood, which is the part to carry

Decoding every `addiu rX,-NN` in `0x800C6000`–`0x800C7A00` from the MIPS16 EXTEND encoding (both
known sites share EXTEND `0xF75F`; `f75f4a0b` is `addiu v0,-0xB5` and `f75f4b04` is
`addiu v1,-0xBC`) finds a **descriptor DISPATCHER** at `0x800C6930` switching on `tag - 0xB3`:

    0xB3 -> 0x800C6A5E    0xB4 -> 0x800C6B2C    0xB5 -> 0x800C695A
    0xB6 -> 0x800C6A92    0xB7 -> 0x800C6B16    0xB8 -> 0x800C6AE8    and more beyond

Its `0xB5` arm is **richer** than the standalone callback — `desc[2..3]`, `desc[4..5]` *and*
`desc[6]`, into a record whose `+8` it then zeroes. And separately `0x800C6C66` handles tag `0xBC`
and immediately computes `desc[1] / 9` — **nine-byte entries**, the same shape the BAT's `0xB1` uses
for its per-service records.

**So the honest statement is narrower than "0xB5 carries the listings".** `0xB5` is the only tag
consumed *by the callback that happens to be armed* when a `0xA1` section arrives on this path, and
it yields two 16-bit values. A descriptor that yields two u16s and stops the walk is a reference or
a window, not a programme list. **`0xBC`, with `len/9` entries, is the better candidate for the
programmes themselves**, and this feed never reaches its callback. Filed as `sky-02me.20`.

**The scan nearly missed both siblings.** Its first form required the `addiu` to be followed by
`BNEZ`, and `0x800C6930` and `0x800C6C66` both use the MIPS16 `move t8` / `bteqz` form instead. They
surfaced only because the scan **listed its near-misses** rather than dropping them — the
parser-does-not-fit-its-subject trap, caught by reporting what was not matched rather than by
getting the pattern right.

### `sky-02me.20`: the programmes are NOT in SI — three routes closed by measurement

*2026-09-15. The question was which descriptor carries the listings. The answer is none of them,
and the acquisition ladder that found every previous rung has stopped asking for anything.*

#### 1. Tag `0xBC` — walked, not consumed

`scripts/digibox-probes/does-0xbc-carry-the-programmes.js`, `--cold`. `0xBC` was the best remaining
candidate because its handler at `0x800C6C66` computes `desc[1] / 9` — nine-byte entries, the shape
`0xB1` uses. Fed as four nine-byte entries with `0xBC` **first** and a known-silent marker behind it:

    tagsWalked ["0xbc","0x3c"]   lastDescriptorWalked true   bcBodyOffsetsRead []   nonWalkerReaders []

Walked, marker reached, nothing consumed. So `0x800C6C66` exists but is **not the callback armed on
this path**. Reading it says it would not have helped regardless — it is a **search**, not a list
walk:

    N = desc[1] / 9 ;  p = desc + 2
    loop  a = u16(p[0..1]) ;  b = u16(p[4..5])
          if (a == caller[0] && b == caller[2]) {      <- both must match a pair it ALREADY HOLDS
              caller[4] = u16(p[2..3])
              if (p[6] & 0x80) caller[8] |= 1
              if (p[6] & 0x20) …
          }
          p += 9

A resolution table. So are the rest of the family, decoded from the MIPS16 EXTEND encoding across
`0x800C0000`–`0x800D0000`: `0xB2` at `0x800CB008`, `0xB4` at `0x800CB44E`, `0xBE` at `0x800C2872` —
all small bitfield extractors.

**And the same run turned an inference into a demonstration.** `0xB5` truncating the walk had only
ever been deduced from a coverage count. Pass C put `0xB5` in front of `0xBC` on purpose: only
`0xB5` walked, the marker was never reached, the consumer fired, and the watch logged **35 reads
against 981**.

#### 2. Table `0xA0` — the same table as `0xA1`, not a second one

`scripts/digibox-probes/what-is-table-0xa0.js`. The match unit's `tableIdMask` is `0xFE`, so it
accepts `0xA0` as well as `0xA1`, and in the life of this project `0xA0` had never been sent.

    0xA0  read by 0x800C64F2, 0x800C64FA, 0x800C651E   offsets 2,3,4,5,6,7,79,80
    0xA1  read by 0x800C64F2, 0x800C64FA, 0x800C651E   offsets 2,3,4,5,6,7,79,80

Identical. The mask is width, not a second table. Both controls — unsigned, and an extension outside
the `0xFFFC` mask — read **exactly 0**.

#### 3. A well-formed `0xB5` opens no new rung, and that is the finding

`scripts/digibox-probes/does-0xb5-open-the-next-rung.js`. Every rung of this project was found the
same way: feed one, read `__siMatches()`, see what the box asks for next. `0xBBB` was never a number
anyone chose — it was the value fed in `0xB1` entry field `+3..4`.

Four trials, three copies each with rising versions, field values a decade apart so any number seen
later would name the field it came from, including two trials placing a listings id the box already
holds in each field in turn. Subscriptions before and after, **every time**:

    0x40/ext=0x20   0x42/ext=0x0   0x4a/ext=0x1000   0x73   0xa1/ext=0xbbb

No new table, no new PID, nothing gone. **The ladder has stopped asking.**

#### What that leaves

The `0xB5` reference is resolved **internally**, against a carousel the box already believes it has.
The programmes are not in SI at all — which is consistent with two of the `0xA1` walker's 27 call
sites (`0x800C9BC4`, `0x800C9BF4`) sitting beside the OpenTV carousel parsers. Successor:
`sky-02me.21`, and the honest warning attached to it is that feeding Huffman-compressed carousel
modules means **generating** one the box will decompress, which is a much larger piece than a
section builder.

**Four trials was the right number to stop at.** A fifth would have been iterating a technique
already shown not to apply, which this project has paid for twice before.

#### A verdict bug, for the fourth time on this project

The `0xBC` probe printed `controlsHeld: false` over two controls whose own rows read *"only the
iterator touched the loop"* with nothing walked. The flag asked whether `__find` located a copy at
all — and it does, a **stale** one left by the previous pass, because the payloads differ by two
bytes. Corrected to require silence (nothing walked, nothing consumed), and the corrected form was
run against a consuming pass to confirm it still separates them. **Read the table, not the verdict.**

### `sky-02me.21`: the format is Sky's OpenTV EPG, and this box parses records we generate

*2026-09-16. Web research first, which should have come several sessions earlier, then five probe
runs — four of which measured the harness rather than the box.*

#### The format is published, and two of this file's own claims were wrong

Sky's EPG is read by at least three open-source projects — [`jcdutton/loadepg`], [`dave-p/openTVtoXML`],
[`vdr-projects/vdr-plugin-eepg`]. A TITLE section is:

    data[0]      table id -- titles 0xA0..0xA4 and 0xB0; summaries 0xA8..0xAB and 0xB1
    data[3..4]   channel id      <- the value we feed in BAT 0xB1 entry +3..4
    data[8..9]   two bytes the hardware unit filters on
    data[10..]   records, advancing by packet_length:
       r+0..1  event id     r+2..3  packet_length   r+4  0xB5   r+5  descriptor length
       r+6..7  start time, seconds-into-day = value * 2        r+8..9  duration, same
       r+10    genre        r+12  rating (low nibble)          r+13..  Huffman title

**`0xB5` is the programme record, not a reference.** This file said *"0xB5 yields two u16s and
stops — a reference, not a programme list"*. Those two u16s are **start time and duration**. The
sweep only saw a stub because it fed a four-byte body, truncating the record before its text.

**`9E 8B` is not "the signature the hardware match unit demands".** It is whatever identifier the
box holds for that field — `0x9E8B` is MJD 40587 (1 Jan 1970) and the reference readers call the
field the MJD, but **two live tests refuted the date reading on this box**: setting the clock after
the line-up did not move it, and a 1998 TDT on air from the first instruction (240 sections, 0
refused) did not either. The sibling SDT unit constrains the same two bytes to its
`original_network_id`. Stamp what the unit asks for and it stops mattering.

#### The writer, and why it is trustworthy

`scripts/skyepg/` — `huffman.py` (encoder *and* decoder from the published Sky UK dictionary, 446
entries) and `title_section.py`. The encoder is validated by round-tripping through a transcription
of the reference **decoder**: 9/9 including a leading space and multi-character phrases. The section
is then walked back with openTVtoXML's own arithmetic rather than my own.

**The bit packing is the trap.** The decoder starts the FIRST byte at mask `0x20` and every later
byte at `0x80`, so byte 0 carries six bits and its top two are skipped. An encoder that packs eight
bits into byte 0 produces plausible-looking Huffman that decodes to nonsense.

#### The box parses our records — measured

`real-title-section-minimal-diff.js` is `what-is-on-0xa1.js` with **only the payload changed**:

    0x800C64FA -> +6     the iterator reads the tag
    0x800C6B42 -> +6     the 0xB5 callback confirms it
    0x800C6B4A -> +8  \  START TIME and DURATION -- the four bytes we encoded
    0x800C6B4C -> +9   |
    0x800C6B54 -> +10  |
    0x800C6B56 -> +11 /
    0x800C64FA -> +42    the SECOND record's tag, exactly where our encoder put it

Both controls read 0. Payload `+6` is where our record places the tag; `+42` is record two's.

#### And the guide still does not render them

Twenty sections across all four channel ids: the guide is **byte-identical** before and after
(`0xB4E3CAD8`, 256 widgets, 26 colours) and still says *"Further schedule information is not
available"*. **The title text is never read** — offsets touched are 2,3,4,5,6,8,9,10,11,42,43 and
the text begins at +15. The box takes the times and nothing else.

Refuted on the way: *"nothing is on now"* — twelve records covering a full 24 hours back to back
from 00:00 changed nothing.

#### Two harness lessons, both of which this file already warned about

**A variant of a working probe went quiet for four runs, and the cause was the variant.** It
reported *"copied and NOT parsed"* for its own known-good control because it never located the parse
buffer at `0x802A76xx`, sitting instead on a first-stage copy at `0x801A85xx` that nothing re-reads.
Re-running the original verbatim parsed first time. **When a variant goes quiet, re-run the original
before adding hypotheses.**

**And a false positive caught only by opening the screenshot.** A run reported *"THE GUIDE SCREEN
CHANGED"*; its baseline presses had built zero widgets at 12 colours because the box was still in
the NVRAM rebuild, so the change was the rebuild ending. The baseline now waits it out and throws if
it did not draw. Fifth time here that a verdict string was cruder than the table beside it.

[`jcdutton/loadepg`]: https://github.com/jcdutton/loadepg
[`dave-p/openTVtoXML`]: https://github.com/dave-p/openTVtoXML
[`vdr-projects/vdr-plugin-eepg`]: https://github.com/vdr-projects/vdr-plugin-eepg

## What is not established

- **The stream interface is BUILT, the box parses what it is given, and standard DVB SI is not
  what it is waiting for.** That last part is now measured rather than assumed — see *What the
  SI we broadcast does, and what it does not* — and it retires the older form of this entry,
  which asked whether the o-code would draw a guide from SI alone. It will not, because the
  guide is an OpenTV application rather than C code with a data input.
- **WHAT THE RUNNING APPLICATION IS WAITING FOR IS THE OPEN QUESTION, and it is now a question
  that can be put to a machine that talks.** Application `0x2` (the IEPG) is in the foreground,
  MAXIMIZED, painting its background and nothing else, while every debug channel in the image
  stays silent. Three lines of enquiry, in the order they are cheap:
  **(1)** DONE, and it answered: the interpreter is parked on pipe `EVQP0004`, it wakes on every
  key press, and it draws nothing. See *The interpreter, instrumented* above. What is still open
  is the o-code dispatch itself — the 51-entry table at `0x80081F54` is not it.
  **(2)** `#CONTROL[running] unknown control error=0x19` arrives between the IEPG search failing
  and the application starting; the handler at `0x8003723E` is a switch whose default arm prints
  it, and nothing yet says what sends 25.
  **(3)** ANSWERED — see *The signature check* above. It is public-key verification with the
  keys in flash, so a carousel we generate cannot be signed; and it is called zero times on a
  normal boot, so it is not what is stopping us. What replaced it as the open question is how a
  code module reaches the interpreter at all, given its module handle is NULL and it never asks
  BGLOAD for one.
- **`__ackSet("all")` no longer stalls the boot, and the older note that it stalls at 19 tasks
  is superseded.** Measured twice on a cold boot: with every peripheral command answered the box
  reaches **42 tasks** both with the SI carousel on air and without it, so the carousel is not
  what freed it and something between then and now was. What acking everything DOES change is
  the display setup — the OSD descriptor comes up **8 bpp** rather than 4 — which is worth
  knowing before anyone treats the bit depth as fixed. It does not change the ending: the
  application is still MAXIMIZED with nothing drawn.
- **Moving video is a separate and much larger piece, and nothing here has started it.** The OSD
  and blitter are emulated and proved — they are what paints the screen you get today — but there
  is no MPEG-2 video decoder in the emulator at all. The reachable next visual milestone is the
  menus and the guide, not a picture behind them.
- **The bootloader handoff remains unproved.** Its image validator **SEARCHES for the image header**. The validator at
  `0xBFC122B6` reads a word through the unaligned big-endian reader at `0xBFC12A38`, compares it
  against the `JB` magic `0x4A42A007` (pool literal at `0xBFC12584`), and on a miss advances the
  pointer by a step held at **`0x800050BC`** and loops to a limit. `0xBFC13430` does the same for
  the `SIGN` terminator `0x5349474E`.
  `[0x800050BC]` is the flash descriptor's scan step. The earlier zero-step diagnosis was
  premature: the old 100-million-instruction run never reached the scanner. The first measured
  blocker was a missing board timer at `0xB000D000`; its `0x40` IRQ wakes BOOTMain. The next
  blocker was ROM's demux self-test, whose result is stored at `[0x800081F8]` by `0xBFC00500`
  and checked once at `0xBFC012C6`. This is a hardware-test result, **not** the copied pointer
  at `[0x800050D0]` as an earlier disassembly reading claimed. ROM sends six embedded transport
  packets through DMA channel 5, then tests the demux write pointers at indirect indices 0 and
  4 and compares the section RAM bytes. The old DMA channel-5 model signalled completion without
  moving those bytes, so the test returned one and BOOTMain entered a service loop. With the
  transfer, indirect match-word readback, section assembly and MPEG CRC filtering modelled,
  guest instructions wrote zero to `[0x800081F8]` and set `[0x800050BC]` to
  `0x10000`; no host mutation was involved in that diagnostic model. **This readback was later
  withdrawn:** the unmodified browser oracle answers zero at demux `+0x148`; Go answered `0xFF`
  there at retired instruction 3,209,923, creating its first checkpoint divergence. With the
  oracle's read contract restored, Go records `[0x800081F8]=1` at 3.4 million instructions.
  A second exact-state comparison found that the added DMA channel-5 transport path changed RAM
  at retired instruction 3,210,573, while the oracle only signals channel completion. The Go
  channel now follows that measured no-transfer behavior as well.
  The declared host handoff fires soon after this first idle interval, before either image scan.
  The earlier diagnostic branch then waited on `SMTAck` for CSI command
  `0x44`. A diagnostic peripheral reply using the oracle's synthetic all-buttons-released frame
  reaches scanner PC `0x9FC122B6` at instruction 19,193,163. Both JB images pass their header
  and payload CRC checks, with the second scan returning success at instruction 64,451,653.
  Validation stores payload address `0xBFC20024` in the selected descriptor's `+8` field. The
  nonzero field makes `0x9FC12846` take the call to `0x9FC036F8` at instruction 64,453,227.
  That routine sends a CSI `0x17` command and enters an unconditional sleep/service loop. The
  same command is sent from the demux-test-failure loop, and the oracle's normal startup sends
  it without resetting; it is not evidence for an external CPU reset. A temporary diagnostic
  that zeroed the selected field before the branch reached further loader setup and created a
  sixth task, but still did not enter the application by 200 million instructions. This host
  mutation is not acceptance.
  By 100 million instructions it reaches the browser oracle's declared fiction boundary:
  `ready=0x100`, `current=0`, still in the bootloader. That reply is not a measured
  physical-controller response, the guest never reaches the flash entry at `0x9FC2048C` or
  loader at `0x9FC20618`, and the application image at `0x800009F4` remains zero. The flash
  stub at offset `0x2002C` really does jump to `0xBFC2048C`; that is valid code, but no normal
  guest path to it has been established. The browser
  oracle then forces its own host-side handoff; that injection cannot establish that the firmware
  itself transferred control.
- **Whether the EPG runs without a viewing card.** 54 CA strings, `NDS XSG`, `CA API Glue`.
  Historically a Sky box showed its guide with no card and refused only to decrypt, and the EPG
  carousel was broadcast in the clear — but that is a reason to expect an answer, not evidence for
  one. Still unproven, and now sharper: the box does not merely want a card, it blocks its FIRST
  task on the card reader's semaphore before anything else runs. Whether a real no-card boot gets
  past this on a timeout, or whether the reader answers "no card" and the firmware proceeds, is
  the question — and the two have completely different implications for what we must model.
- **The I2C bus.** Past the card, TASK0 blocks immediately on semaphore `IICC0` at `0x80170288`
  (count 0, one waiter, suspend block `0x80172F84`). The name says I2C and the shape is expected to
  be the smartcard's again: an unmodelled peripheral, an ISR in the dispatch table, and a task
  holding the semaphore while it waits. The prime suspect is the third armed interrupt — mask
  `0x4`, handler `0x8000665D`, reading the block at `0xB200A000`-`0xB200A0A0` — which is the one
  device of the three the firmware arms that is still not modelled.
- The o-code VM's dispatch table, and whether the EPG application can be driven without it.

---

## `sky-02me.21`, 16 Sep 2026: the box STORES our programmes, and the guide draws none of them

*Seven probe runs. Every number below is from `scripts/digibox-probes/`, and the four claims that
retired earlier readings each retired them by measurement rather than by argument.*

### The guide's build path does not look at the title sections at all

A `tv guide` press is **44,935 o-code instructions**, and the interpreter's event-loop head at
`0x9FC4A538` splits it into **eight events** of 37,610 / 324 / 238 / 238 / 413 / 1,073 / 5,033 / 6.
The first is the guide build. On a box holding twenty parsed title sections that stream is
**byte-identical, instruction for instruction, to the same press on a box that has never seen
one** (`what-the-guide-reads.js`). So the question was never the section format.

**The fed press did run three MORE events, and that was NOT the listings.** One of the three was
byte-identical to an event the unfed press already had — a periodic timer that happens to land
inside one eleven-second window and not the other — and the run that produced it also painted
stripes over the guide's hint bar. Re-run with the same feed, the plain screen came back and the
extra events did not. **The o-code event COUNT between two windows means nothing without a
same-length control**, which is why every later run takes an idle window first.

That extra event, disassembled, is worth keeping: a timer dispatcher walking a **40-byte record
array at `DS+0x0252FC`** (the index is `((i<<2) + i) << 3`, so the stride is 40 — an earlier note
reading `shift_2` alone recorded it as 4), whose fired callback increments a retry counter at
`DS[0x0193F4]`, caps it at 99,999,998, and re-arms itself with a delay of 0xC350. A polling retry.

### Where the text lives, and why the flash watch reported nothing

`"Further schedule information is not available"` is at flash `0x9FCAB279`, and **the module's
whole resource region was read ZERO times during a 256-widget guide build** — a different answer
from "the watch saw nothing", and the probe says which. The text is in the data segment: at
`DS+0xAACD`, reached through a table of 8-byte entries at `DS+0x03C0`, each `{DS offset, id}` with
sequential ids. The message is string id `0x51397`; `"Searching for listings"` is `0x51398`.
**Neither the offset nor the id appears anywhere in the CODE chunk**, so the reference is computed
and a static search for it cannot work.

**The data segment is at `0x8045CB64` and it MOVES — locate it, never carry it.** The published
anchor (`DS[0x0001ACB0]` reading `DS + 0x1ACB0`, a word pointing at itself) is application STATE
that is `0x00000001` on a fresh boot, so a probe gating on it refuses to run on a healthy box. What
discriminates structurally is that `"RSRC"` — the segment's first four bytes — appears **twice** in
DRAM: once at `0x80078454`, inside the decompressed application image (which ends at `0x800FC66C`),
and once in the heap above it. Take the one above the image.

### `FUN_800c95d0` is the consumer, and the twelve "time lookup" call sites never run

Counted, not argued: the section parser ran **exactly 20 times for 20 sections pushed**, while
**all twelve** call sites holding the `0xB5` callback as a pool word ran **zero** times. The walk
is driven from `0x800C9BC4`, inside `FUN_800c95d0` itself. The static reading that put consumption
at `0x800C72D2`, "the time compare", named a site that is not on this path.

Its pool words resolve the whole chain, so it is a ladder rather than a mystery:

    0x800A3F7C  extensionLookup(ext)          must return 4, and an index < serviceCount
    0x800C70E0  dayKeyToSlot(mjd, tableId&3)  must return 0..31, else the section is dropped
    0x800C91F8  findBlock                     0x800CCECC alloc / 0x8007F77C alloc2
    0x8001DB18  memcpy                        one per record body
    0x800C1540  registerBlock(block, ext)
    0x800C1A00  dayKeyToDate(mjd, &out)       0x80074758 dateToBaseTime
    0x800C64CC  walkDescriptors(.., 0xB5)     0x800C6B38 the callback
    0x800C587C  PER-EVENT REGISTER            once per record, after the walk
    0x800C579C  notify(svc, key, tid, 0x3EC / 0x3EA)      0x800C5710 final notify

**THE FIRST CENSUS OVER THOSE ADDRESSES MEASURED ITS OWN HARNESS.** Every one came out of a pool
word, a MIPS16 function pointer carries the ISA bit, so they were all ODD — and `__pcHits`
normalises by counting `a` and `a|1`, which is **upward only**. Handed `0x800C64CD` it counts that
address twice and never looks at `0x800C64CC`, where the profiler recorded the hits. Eleven rungs
read zero while the two hand-typed EVEN addresses counted perfectly, and the probe printed a
confident "first rung that did not turn" naming the wrong function. The existing guard passed it
because the key was present; presence was never the question. **A ladder address is now rejected
by name if it is odd.**

### The record length was ours, and three references settle it against the fourth

Corrected, the ladder turned end to end — with the per-event register and the descriptor walk at
**2 per section, for sections carrying 12 records.**

The firmware's loop is `local_84 += local_8a + 4` with `memcpy(dst, section + local_84 + 4,
local_8a)`, so the 12-bit field at `r+2..3` counts the bytes **after** the four-byte header.
`scripts/skyepg` was written against **openTVtoXML, which advances by that field alone** — so our
records declared four bytes too many, the walk landed inside record 2, read the tag `0xB5` as a
length of 0x511, and ran off the end. Simulated against our own bytes it finds **2**; corrected it
finds **12**, and the box then registers **240 records over 20 sections, exactly twelve each**.

**tvheadend agrees with the box and openTVtoXML is the outlier**: `ev->eid = buf[0..1]; slen =
buf[2..3] & 0xfff; i = 4; while (i < slen+4)`. The writer's self-test now walks the firmware's
arithmetic and was mutated back to the reference's to confirm it goes red.

### `9E 8B` IS the MJD, and both earlier refutations were about re-subscription

This file has twice recorded the date reading as refuted. Both tests moved the clock **after** the
match unit was programmed and observed that the bytes did not follow — which is a fact about the
box not re-subscribing. Set the clock **first** and the entire request moves together:

| | table id | listings PID | `data[8..9]` | |
|---|---|---|---|---|
| no clock | `0xA1` | `0x33` | `0x9E 0x8B` | MJD 40587 = 1 Jan 1970, the Unix epoch |
| clock 1 Jan 1998 | `0xA0` | `0x37`, `0x36` | `0xC6 0x7F` | MJD 50815 = 2 Jan 1998 |

and the firmware's own converter agrees — `__call(0x800C1A00, 0xC67F, &buf)` returns a structure
carrying day **2** and year **0x62**. tvheadend reads the same two section bytes as
`(mjd - 40587) * 86400`, which is the third independent confirmation and also explains why an
unclocked box names the epoch.

**The table id and the PID are part of the same addressing and had both been hardcoded.** The
hardware mask is `0xFE`, so `0xA0` and `0xA1` both reach the parser — but the parser takes
`tableId & 3` as part of the day-slot key, so sending the wrong one of a pair is a silent
mis-filing rather than a refusal. Sky's title PIDs are `0x30`-`0x37` and summaries `0x40`-`0x47`,
which is exactly the range the box moved within. **Read all three off `__siMatches()` each round.**

### Where it actually stops

The box asks for the day **after** its clock, so the day it wants never contains "now". Feeding
that day and then moving the clock into it **does** change the guide — and the screenshot says the
change is the clock line, `7.00pm Thu 1` to `7.00pm Fri 2`, with the message untouched. (1 January
1998 was a Thursday, so the box's own calendar is right.)

So: 240 records stored, on the day the box asked for, with "now" inside it, through
`registerBlock`, the per-event register and **both** notifications — and
`Further schedule information is not available`. **Feeding 20 sections also produces ZERO novel
o-code events** against an idle control of the same length, so the running EPG application is never
woken by the `0x3EA` completion notify.

**The next thread is narrow: `0x800C587C`, the per-event register, and which index it writes** —
compared against what the guide build reads. The guide build's own DS profile is headed by
`DS+0x025304` (301 reads, the timer array), then `DS+0x01A1BC` (76), `DS+0x01A0A4` (70),
`DS+0x035BBC` (44) and `DS+0x013A2C`/`0x013A28`/`0x013958` (25 each); `DS+0x019A20` is the service
count the BAT loop counts against, so the `0x01A0xx`/`0x01A1xx` neighbourhood is the service table.

### And the guide is a SUBSCRIBER, not a reader — `sky-02me.21` continued, 16 Sep 2026

*Six more probe runs, after the store was proven full. The blocker moved two layers and the box
named the first of them itself.*

#### The guide asks its database exactly once, and it asks for a linkage

A `tv guide` press makes four module-12 calls and module 12 is the EPG database — `(12,0x23)`
resolves through the live module table at the word in `0x8006E71C` to **`0x800CB770`**, in the same
neighbourhood as everything `FUN_800c95d0` uses to fill the store. That function is lock / ask /
unlock around `0x800CB69C`, which presets the caller's out-parameter to `0xFFFFFFFF` and searches
the SI through `0x800AD608`. Its callback at `0x800CB660` is eight instructions:

    desc[0] != 0x4A  -> skip                  (linkage_descriptor)
    desc[2..3] -> out+4                       transport_stream_id
    desc[4..5] -> out+2                       original_network_id
    desc[6..7] -> out+6                       service_id
    desc[8]    -> out+9                       linkage_type

and the caller requires the search to return 3 or 4 **and** `out+9 == 0x91`. On success
`0x800A4040(ctx, service_id, tsid, onid)` walks the box's own `0x18`-byte service records; on
failure `0x800AC774(ctx)`. **Counted on a real press: the ask happens once and the NOT-ANSWERED arm
runs.** That is why the guide's 37,610-instruction build is byte-identical fed and unfed — it never
reaches the store, so filling the store harder could never have moved it.

#### Giving the BAT that linkage works, A/B, in one run

`[0x4A][7][tsid][onid][service_id][0x91]` in both of the BAT's descriptor loops, naming a service
the same BAT declares on the transport `__siIds()` reports. Pressed twice in one run with nothing
else changed — a second BAT version seven bytes longer — the ANSWERED arm replaces the not-answered
one, and the press costs **14,944 more o-code instructions and three more events** (43,836 → 58,780;
7 events → 10). The guide does substantially more work with the answer than without it.

**And the box then asked for a PID it had never asked for: `0x30`**, the first of Sky's eight title
PIDs, alongside the `0x36` and `0x37` it already had. The linkage unblocked acquisition.

#### What it draws instead, and the part that is NOT a hardware fact

    FOR YOUR INFORMATION
    No satellite signal is being received

`DS+0xC193`, string id `0x51B66`, in a family with "There is a technical fault with this channel"
and "Please try later". **It is drawn with ZERO demodulator reads and ZERO demodulator writes**
across every guide press, so it is a conclusion the application reached rather than a status it took
off the front end — and the SDT already declares those services `running_status` 4 and
`free_CA_mode` 0, so the obvious answer is not it either. It appears on the FIRST guide press with
the linkage, before any listings, so it is independent of the store.

#### The guide never asks for rows, and the reason is the architecture

Censusing **all 51** of module 12's functions across three guide presses on a box with the linkage,
the clock and 480 stored programmes gives the same four calls as a bare box —
`(12,0x6)` ×6, `(12,0x8)` ×2, `(12,0x23)` ×1, `(12,0x26)` ×6. Nothing new. The 14,944 extra
instructions are drawing, not querying.

**`(12,0x26)` is `0x800C6154`, and it is the NOTIFICATION COLLECTOR.** It reads a 44-byte slot and
dispatches only when the slot's `+4` word is **`0x3EC` or `0x3EA`** — the exact two ids
`FUN_800c95d0` sends — then writes 999 back. So the guide does not poll a store; it collects
notifications it is already subscribed to.

And `0x800C579C`, the notify our parser calls 40 times a feed, is the other half. It walks **two
tables of 44-byte slots at `0x80165048`** and fires a slot only when all of this holds:

    slot+0x00 == 1                                    the slot is active
    slot+0x04 == the id being notified                it is waiting for this one
    slot+0x1D != 2 ? slot+0x10 == service             which service, in one of two fields
                   : slot+0x12 == service
    slot+0x18 == dayKey   and   slot+0x1A == tableId & 3
    -- or slot+0x28 == 1, which matches regardless

**So a notification only reaches the guide if the guide is subscribed for that service, that day and
that table-id pair.** We feed all three read off the box and still measure **zero novel o-code
events** in a feed window against an idle control of the same length — the application is never
woken.

**The next measurement is narrow and the table is dumpable: `__peek` the slot tables at
`0x80165048` at the moment of a guide press and read what the guide actually subscribed for**, then
make the feed match it. Dispatch targets are `0x800C6079`, `0x800C9151` and `0x800C3BD5` by slot
kind.

#### Two harness rules this half paid for

- **A GENERATED CENSUS ENTRY THAT DISAGREES WITH A HAND-WRITTEN ONE AT THE SAME ADDRESS IS A HARNESS
  FAILURE, and it is the cheapest self-check available.** The module-12 census reported **zero for
  all 51 generated entries** while the hand-written entry for `0x800CB770` counted 1 — the walk had
  dropped a dereference, because the module array holds POINTERS TO the 8-byte
  `{implementation, descriptor}` records rather than the records. Without the cross-check it would
  have read as "the guide asks its database nothing", which is a conclusion, and a wrong one.
- **`& ~1` IN JAVASCRIPT YIELDS A SIGNED 32-BIT INT.** `0x800CB770 & ~1` is negative and compares
  unequal to the literal, so the self-check above failed on an address that was correct. `__pcHits`
  and `hex32` both apply `>>> 0`, so the census was never affected — only the guard was. Mask with
  `(x & ~1) >>> 0`.

### The demo page draws listings — `sky-02me.21` closed, 16 Sep 2026

*No probe, no pokes. Load the page, let the carousel run, press **sky** then **tv guide**:*

    0 Sky One                          12.10pm Thu 1
    NOW      AfternoonFilm
    2:00pm   ChildrensHour
      Search Time · Search Channel · Search Favourite

Afternoon Film runs 12:00–14:00 in the day the page broadcasts and the box's clock says 12:10, so
now and next are both right. "Search Time" only exists when there is a schedule to search.

#### Four things had to be true at once, and three of them had been hardcoded

1. **The record length.** The 12-bit field at `r+2..3` counts the bytes AFTER the four-byte header
   and the reader advances `length + 4`. `openTVtoXML` advances by the field alone; tvheadend and
   this box agree with each other against it. Following the wrong one makes twelve records arrive as
   two, silently.
2. **The clock goes first.** The entire listings request is day-addressed and the box programs it
   ONCE. With a clock it asks for table `0xA0`, PIDs `0x37`/`0x36` and the day's real MJD; without
   one it asks for `0xA1`, PID `0x33` and MJD 40587, the Unix epoch. The carousel now sends TDT and
   TOT every two seconds and holds the NIT, BAT and SDT until three pairs have gone.
3. **The BAT needs a `linkage_descriptor` of type `0x91`, in BOTH descriptor loops.** The guide asks
   its database exactly once and this is what it asks for. In the transport loop alone it changed
   nothing.
4. **THE DAY AND THE TABLE-ID LOW BITS ARE THE GUIDE'S TO CHOOSE, NOT THE MATCH UNIT'S.** This is the
   one that took longest. The acquisition subsystem asks for **tomorrow** on the table the match unit
   names; the guide registers a notification slot for **its own day** with its own `tableId & 3`, and
   `0x800C579C` fires a slot only when the service, the day key and those two bits all match. Send
   the match unit's pair and 1,728 records land in the store while the guide never hears about any of
   them.

   **And the low two bits are not a constant.** Two boxes measured minutes apart wanted **3** and then
   **2**. A carousel stamping a fixed table id is right by luck. `__siGuideSlot()` reads the slot and
   `__siTitles()` defaults to it — the same "let the box tell us" method that found the PID, the
   table and the date, applied to the last place it was still being guessed.

#### What the page does now

`__siTitles(pid, opts)` builds a title section carrying twelve pre-encoded two-hour programmes
(generated by `scripts/skyepg/title_section.py`, Huffman text against Sky's published dictionary),
stamped with whatever `__siGuideSlot()` reports. The carousel sends it to every PID in `0x30`–`0x37`
the firmware has armed for itself — `listings: true` rather than a fixed PID, because that choice
moves with the clock. Still opt-in behind `?si=1`, and `__siBAT`'s `linkage` option is off by
default so no earlier probe's baseline moves.

**The slot only exists once the guide has been opened**, so the first press registers it and the
next carousel wave fills it; the guide then updates in place. A visitor who opens the guide, waits a
few seconds and looks again sees rows.

#### Still open

`FOR YOUR INFORMATION / No satellite signal is being received` sits above the rows. It is drawn with
**zero demodulator traffic**, appears only once the linkage is present, and appears before any
listings — so it is the service-selection half of the linkage answer, not the schedule half, and now
provably not the gate on the rows. Filed separately.

### `sky-02me.5`: the ALL CHANNELS grid lists nothing, and what that is NOT

*16 Sep 2026. Six runs. The grid is reached `sky` → **left** to the TV GUIDE tab → select; the `tv
guide` KEY goes to the now/next banner, which is a different screen, and select there does nothing.
Three of this session's measurements were of the wrong screen for exactly that reason.*

**The screen draws a header and no rows:**

    2.36pm Wed 7
    ALL CHANNELS
          Today  2.30pm      3.00pm      3.30pm

#### Ruled out, each by measurement

- **Not a listings fault.** There are no channel ROWS, and listings cannot fill a list that has
  none.
- **Not a malformed line-up.** The box's 18-byte service records hold all twelve services, located
  at runtime by content: `00 20 00 00 | 00 64 0b b8 | 00 00 | 00 65 | 01 | 00 01 00 01 00` —
  transport, service id, listings reference, the unused `extra` field, **channel number 101**, kind,
  and the four flag bits unpacked as `0,1,0,1`.
- **Not the flag bits.** Setting them to `0x5` — the value every probe that ever drew used, against
  the page's long-standing `0` — changed nothing. Kept, as data rather than as a constant, because
  their meaning is still unestablished.
- **Not the notification model.** The grid registers **no slot at all** (`slots=[]` at every step),
  so nothing broadcast reaches it that way. The two slots the banner path registers always read
  `service 0, day 50814, tidLow 2` — never one per channel, which refuted the first hypothesis here.
- **Not that array.** A read watch over the 18-byte records during the grid open logged **zero**
  reads — and zero on the control press too, so that array is the acquisition-time structure and
  nothing reads it while drawing.

#### What the grid actually does

It runs **14,956 o-code instructions** against the working TV GUIDE menu's 34,796, builds 16
widgets, paints and returns to the event loop cleanly. It does not fail; it finds nothing to list.

Its own data-segment state, dumped before any screen, after the working menu and after the grid:

    DS+0x02E20C  = 12          THE CHANNEL COUNT, and it is RIGHT -- written by executed code at
                               0x9FC75F2F, read at 0x9FC75F7B / 0x9FC762FD / 0x9FC77D41
    DS+0x02E210..0x02E220      pointers into the DS string area, each the result of a
                               scall(0,0x0A) with a resource id -- the grid's chrome, loading fine
    DS+0x02E208                a BYTE buffer (GETC/PUTC, 21 sites), not a pointer. Reading it as a
                               word gives 0 because its first four bytes are zero, which briefly
                               looked like a null in a struct and is not.
    DS+0x01A270  = 12          twelve again, present from boot

**So the grid COUNTS TWELVE CHANNELS and draws none of them.** That is the finding, and it moves
the question from "does it know about our services" — it does — to why the row loop produces
nothing from a count it has.

#### Where to go next

`DS+0x02E20C` is read at three executed sites. `0x9FC77D41` compares it and selects a layout size
(19 rather than 16), so that one is chrome. The other two, `0x9FC75F7B` and `0x9FC762FD`, are the
places a row loop would start. Disassemble outward from those and find the loop whose body never
runs, the way the drawing gate at `0x9FC4DB3F` was found.

**And build a warm-boot path for the probes first.** Every run here pays a ~135 s cold boot plus a
~200 s channel-list rebuild before it can press anything, and the NVRAM persists in localStorage —
a box that has already absorbed the line-up skips the rebuild entirely. Six minutes a run, most of
it waiting for state that was already there, is what made this screen expensive to chase.

---

## The clock table is the TOT, and the listings PID is a day-of-eight rotation

*Measured on the Go port, 20 Sep 2026, against the post-acquisition snapshot fixture (a warm box,
1.1 billion instructions retired). Three findings, two of which correct entries above, and one
question left open rather than smoothed over. Every reading is the box's own match unit, read back
through `Demux.Match` — which is only trustworthy since it learned that units 8..15 carry their
rule in the HIGH half of the match word.*

### 1. The box's clock comes from the TOT. The TDT is not delivered to anything.

At rest the fixture's match units are:

    unit  1: 40/fe 00/ff 20/ff ...        NIT
    unit  2: 42/fb 00/ff 20/ff ...        SDT
    unit  3: 4a/ff 10/ff 00/ff ...        BAT, bouquet 0x1000
    unit  5: 73/ff 00/00 00/00 ...        TOT
    unit 10: c1/ff 01/fe ff/00 ...      TABLE 0xC1 -- see below, 2026-09-21

**Unit 10 was listed here unannotated for as long as this section has existed, and it is the one
filter nothing has ever answered.** A match unit skips `section_length`, so byte 0 is the table id
and bytes 1..2 are the extension -- which is exactly how unit 3 reads as "BAT, bouquet `0x1000`" and
unit 1 as "NIT, network `0x0020`". So unit 10 asks for a **long-form section with table id `0xC1`
and an extension whose high byte is `0x00` or `0x01`**. This transmitter has never sent one: the
builders emit `0x40`, `0x42`, `0x4A`, `0x70`, `0x73` and `0xA0..0xA3`, and nothing else.

**Answered once, and the box did a great deal with it.** `TestWhetherAnythingWantsTableC1` restores
two boxes from the same snapshot, lets both acquire the same block's listings, and runs both for
twenty million instructions -- giving only one of them a minimal long-form `0xC1` section, real
header and real CRC, payload deliberately all zeroes because the format is not known and guessing it
is what this phase forbids. The machine is deterministic, so every PC in the test run and not in the
control was caused by that section:

    control  16,888 distinct PCs
    test     17,432 distinct PCs        over 20,000,000 instructions each
    ONLY WITH THE SECTION: 544 PCs, led by 0x80004760 and a long contiguous run from 0x8007F0FC

**A CLAIM MADE HERE ON 2026-09-21 IS WITHDRAWN THE SAME DAY: "the box keeps what we send, so this
is acceptance".** It does keep it -- an eight-byte marker in the payload turns up twice in ordinary
heap, first at `0x801AF5D0`, absent from the control. But the control that mattered was not run
until afterwards, and it is fatal: **the box keeps the payload of table `0xA5`, `0x9E`, `0xC2` and
`0x42` too**, none of which it filters for and none of which this transmitter sends. Something
copies every section delivered on an armed PID out of the ring, so "the payload was stored" measures
that copy and says nothing whatever about `0xC1`. A marker in memory is not an acceptance signal
here. The same trap in its usual clothes: a positive result with no negative control.

**WHAT DOES DISCRIMINATE IS THE DIFFERENTIAL, and it is decisive.** Counting the guest PCs that run
only when a section is delivered, against one shared control of 16,888:

    table 0xC1 on PID 0x52        544 exclusive PCs
    table 0xA5 on PID 0x52          8 exclusive PCs   -- and all 8 are a subset of the 544

So `0xC1` is genuinely special; `0xA5` gets the generic eight-instruction "a section arrived"
handling and stops.

**PID: it is `0x52`, and nothing else is close.** The same `0xC1` section, extension `0x0100`, one
freshly acquired box per case:

    PID 0x10 (NIT)      8        PID 0x33 (titles)    8
    PID 0x11 (SDT/BAT)  8        PID 0x34 (titles)    8
    PID 0x14 (TDT/TOT)  8        PID 0x52           544
    PID 0x00            not delivered -- it is not armed, so the box never asks for a PAT

**EXTENSION: exactly two values are accepted, and they confirm unit 10's decode while adding a
constraint the hardware unit does not carry.**

    0x0000   492      0x0001    22      0x0200    22
    0x0100   544      0x01FF    22      0x8000    22
                                        0xFF00    22

The rule is **high byte `0x00` or `0x01`, AND low byte `0x00`**. The high-byte half is exactly what
unit 10 says (`01/fe` matches `0x00` and `0x01`); the low-byte half is the guest enforcing more than
its own filter asks for, since byte 2 of the unit is masked `00`, i.e. don't-care. The 22-PC floor
is a `0xC1` that got past the table check and failed the extension one -- further than the 8 an
unknown table reaches, which is the shape you would expect.

**The two accepted extensions do DIFFERENT amounts of work** -- 492 against 544 -- so `0x0000` and
`0x0100` are two distinct sub-cases rather than one rule with a spare value.

**Still not established:** the payload FORMAT, deliberately unguessed; what the consumer does with
what it stores; and what `0x0000` and `0x0100` mean to it. The 544 addresses are identified only by
behaviour -- no MIPS disassembler exists on this machine -- and are clustered in
`.artifacts/table-c1-exclusive-pcs.txt`, with `0x800CCECC` among them, which this file's listings
ladder already names as the allocator.

### The `0xC1` format, read off the parser and confirmed on the box

*2026-09-21. The first thing this project has been able to READ rather than infer, because until
now nothing here could disassemble MIPS16 — see `tools/disasm.sh`.*

The consumer is at **`0x800C4C34`**, a 96-byte frame taking the section pointer in `a1`. Its header
arithmetic is the format:

    lbu v1,1(s1) / lbu v0,2(s1) / sll v1,8 / addu v1,v0
    li  a0,4095  / and v1,a0            section_length & 0x0FFF
    addiu v1,-9                         payload = section_length - 9
    lbu v0,3(s1) / lbu a3,4(s1)         extension, 16 bits
    li  v0,9     / div zero,v1,v0       DIVIDED BY NINE
    mflo a0      / sh a0,0(sp+14)       ...which is the RECORD COUNT
    lhu a1,0(sp+14) / li v1,10 / mult a1,v1
    mflo a0      / addiu a0,12 / jalr   alloc(count * 10 + 12)

**So the payload is an array of NINE-BYTE records**, `count = (section_length - 9) / 9`, held in
memory as twelve bytes of header followed by **ten** bytes per record. That `jalr` is the allocator
at `0x800CCECC` this file already names, which is why it turned up in the 544.

The walk starts at **section + 8** — the ordinary long-section header — and per record:

    lbu a0,0(v1) / lbu v1,1(v1) / sll a0,8 / addu a0,v1 / sh a0,0(a3)
        rec[0..1] is a 16-bit id, stored first in the in-memory record
    lbu v0,0(a0) / li v1,240 / and v0,v1            rec[2] & 0xF0
    lbu a0,1(a0) / li v0,192 / and a0,v0 / sra a0,6 rec[3] & 0xC0 >> 6
    or  v0,a0    / sb v0,2(v1)                      the two packed into one byte
    sb  zero,3(v1)
    lbu a0,0(a0) / li v0,8 / and a0,v0              rec[2] & 0x08, a flag
    beqz a0,…    / lbu a0,3(v1) / addiu a0,1        which increments the byte at +3

Bit-packed, in the same manner as the `0xB1` line-up entry. The in-memory header holds
`section[6]` — the section number — at +0 and the record count at +2.

**CONFIRMED ON THE MACHINE, because a static reading of this firmware has been backwards twice.**
`TestTheTableC1PayloadIsNineByteRecords` sends four records carrying ids `0xBEEF`, `0xCAFE`,
`0xF00D` and `0xD00D` at the nine-byte stride the divisor implies, and finds all four in DRAM at a
**ten-byte stride**, at guest `0x80544260`. The spacing is the claim; presence alone would prove
nothing, since the box copies any section it is handed into the heap.

#### Every bit of rec[2] and rec[3], swept

*Seventeen single-bit cases plus three combinations, all as RECORDS IN ONE SECTION so the whole
sweep costs one acquisition rather than twenty.*

    baseline  rec[2]=00 rec[3]=00   ->  out = b0 00 | 00 | aa | 00 00 00 00 00 00

    rec[2] bit 4..7   ->  out[2] bits 4..7          the high nibble, as the disassembly said
    rec[3] bit 6..7   ->  out[2] bits 0..1          >> 6, as the disassembly said
    rec[3] bit 0..5   ->  NOTHING CHANGES           six wire bits this parser does not read
    rec[2] bit 0..3   ->  out[3], one 2-BIT FIELD EACH:
        bit 0  aa -> 6a   (out[3] bits 7..6)
        bit 1  aa -> 9a   (out[3] bits 5..4)
        bit 2  aa -> a6   (out[3] bits 3..2)
        bit 3  aa -> a9   (out[3] bits 1..0)

**`out[3]` IS FOUR TWO-BIT FIELDS, not a counter.** Each is **1** when its `rec[2]` bit is set and
**2** when clear, packed MSB-first in the order `rec[2]` bit 0, 1, 2, 3 — which is why an all-clear
record reads `0xAA` rather than zero. This CORRECTS the reading taken from the disassembly above,
where the walk was followed only as far as `sb zero,3(v1)` / `addiu a0,1` and reported as "a flag
that increments the byte at +3". It increments nothing; the code builds `(v<<2) | (flag ? 1 : 2)`
four times and only the first branch was read.

**Held as PREDICTIONS rather than a fit**, because a rule derived from single bits will fit almost
anything. `rec[2]=0x0F` must give `01 01 01 01` = `0x55`; `rec[2]=0x05` must give `01 10 01 10` =
`0x66`; `rec[2]=0xF0` with `rec[3]=0xC0` must give `out[2]=0xF3`. A rule that counted set bits or
OR-ed them gives none of those. All three measured exactly as predicted.

#### rec[4..8], swept — and the tenth byte the box adds

*Forty single-bit cases plus five whole-byte ones, again as records in one section. All
forty-five changed the stored record, so none of these five bytes is ignored.*

    rec[4] -> out[4]      rec[6] -> out[7]      rec[8] -> out[9]
    rec[5] -> out[6]      rec[7] -> out[8]

Every one is copied **verbatim**, bit for bit — `0xFF` in gives `0xFF` out, and each single bit
lands on its own. **But `rec[5]` skips `out[5]` and lands at `out[6]`**, and that gap is the whole
reason a nine-byte record becomes ten in memory:

    wire (9 bytes)                     memory (10 bytes)
    rec[0..1]  16-bit id           ->  out[0..1]
    rec[2] bits 7..4               ->  out[2] bits 7..4
    rec[3] bits 7..6               ->  out[2] bits 1..0
    rec[2] bits 3..0               ->  out[3], four 2-bit fields, 1 set / 2 clear
    rec[3] bits 5..0               ->  NOTHING
    rec[4]                         ->  out[4]
       (nothing from the record)   ->  out[5]
    rec[5]                         ->  out[6]
    rec[6]                         ->  out[7]
    rec[7]                         ->  out[8]
    rec[8]                         ->  out[9]

`out[5]` reads `0x00` in all forty-six records here. It is filled from somewhere that is not the
section, which makes it the box's own per-record state rather than broadcast data — a status or
sequence byte the array carries alongside what arrived.

**The twelve-byte header is confirmed as well**, read directly for the first time:

    00 00 00 2e 00 00 00 00 00 00 00 00
    +0  section_number, as the disassembly's `sb a2,0(a1)` said
    +2  0x002E = 46 = the record count we sent, as `sh v1,2(a1)` said
    +4..11  zero

#### NOTHING READS THE ARRAY — on any screen this port can reach

*The bridge question, and the answer is a negative worth as much as a positive: the box parses a
`0xC1` section, allocates for it and assembles the array, and then does not look at it again.*

A bus observer watching every data read inside the array and its header, over four windows:

    idle          4,000,000 instructions   NOTHING
    tv guide      8,000,000                NOTHING
    box office    8,000,000                NOTHING
    interactive   8,000,000                NOTHING
    services      8,000,000                NOTHING

**The observer is proved rather than assumed** — over 200,000 instructions it sees 38,234 data
reads, 37,930 of them in DRAM, and 16,668 writes. A hook that never fired would report these zeros
just as confidently, and this file's own rule is that an instrument which cannot find its subject is
a harness failure and not a count. It also watches EVERY ten-byte-stride copy in DRAM rather than
the first, because watching a dead copy while the live one is read elsewhere is the obvious way to
manufacture this result.

**Services was the best candidate and it is now ruled out.** `gort-p5x` was fixed to reach it --
`0x7E` is the Sky menu's Services tab and `handsetRaw` had never contained it -- and with the
product delivering the key, the screen reads nothing either. All five reachable screens are now
negative.

**So the consumer is not a top-level screen.** What remains: a completeness condition (every probe
so far has sent ONE section with `last_section_number` 0); a deeper navigation rather than merely
opening a menu; another table that references this one; or a consumer that runs during ACQUISITION
and has moved on by the time a restored box is probed -- which this file's own note that PID `0x52`
"opens in response to our NIT and closes again" makes the most interesting of the four.

What this does NOT establish: that the array is never read. It is not read in these four states
within these budgets. A completeness condition (this was one section with `last_section_number` 0),
a deeper menu, or another table referencing it would all look identical from here.

**Still unknown:** what `out[5]` is filled from; what the six-bit `out[2]` value, the four 2-bit
states and the five verbatim bytes MEAN; why `rec[3]` bits 0..5 exist at all if nothing reads them;
what the two accepted extensions (`0x0000` and `0x0100`) select; and what consumes the array. The
STRUCTURE is complete; the SEMANTICS are untouched.

**There is no unit matching `0x70`.** Feeding a TDT alone — correctly built, correct MJD, pushed to
PID `0x14` — moves nothing: not the requested day, not the PID, not the table id. Feeding a TOT for
the same instant moves the entire request together. Swept across five dates, the result is binary
and not marginal.

This invalidates the method of every earlier measurement in this project that set the clock with a
TDT and then read a day off the box. Those runs were reading the day the box already had. The
`__siTDT(…)` instrument name is part of why it went unnoticed for so long.

### 2. The box asks for its clock's OWN day, not the day after — **and in the evening, for both**

The entry above (*"Where it actually stops"*) reads *"The box asks for the day **after** its clock,
so the day it wants never contains 'now'."* Measured here with the TOT, the requested MJD equals the
TOT's MJD exactly:

| TOT | requested MJD | date |
|---|---|---|
| 1998-03-01 | 50873 | 1998-03-01 |
| 1998-03-03 | 50875 | 1998-03-03 |
| 1998-03-06 | 50878 | 1998-03-06 |
| 1998-06-15 | 50979 | 1998-06-15 |

The earlier +1 reading was almost certainly the same TDT-does-nothing artefact: a box left on its own
resting day while the instrument believed it had been moved.

**Amended 2026-09-20, and the amendment is the whole of the next section.** Every row above was
measured at midday. Swept again at 19:00 on the same dates, a box asks for **tomorrow** as readily as
for today — `0xA0` with the next MJD rather than `0xA3` with this one — and arms both days' PIDs.
So "its own day" is right about a daytime box and incomplete about an evening one, and neither
reading was ever about the calendar: *the time of day is the variable this whole area turned on.*

### 3. The listings PID is `0x30 | (MJD mod 8)`.

Sky's eight title PIDs are a day-of-eight rotation, and the box picks its own. Confirmed on eight
consecutive days (MJD 50873..50880 from a 1998-03-01 TOT):

    50873 -> 0x31    50874 -> 0x32    50875 -> 0x33    50876 -> 0x34
    50877 -> 0x35    50878 -> 0x36    50879 -> 0x37    50880 -> 0x30

and cross-checked on two unrelated days: 50814 -> `0x36`, 50979 -> `0x33`. The record previously had
only *"Sky's title PIDs are 0x30-0x37 … exactly the range the box moved within"*, which is the
observation this rule explains.

**The PID arms on every one of the eight days**, including the ones in the next paragraph. The
rotation is not conditional on anything.

### ANSWERED 2026-09-20: five days in eight arm the PID and then program no filter

On MJD ≡ 0, 2, 4, 5, 7 (mod 8) the box arms the correct listings PID and then programs **no title
match unit at all**, within 200 million instructions. On ≡ 1, 3, 6 it programs one within about
430,000. There is nothing in between, so it is not a timeout.

    1998-03-02 (50874, slot 2):  armed PIDs [0x32 0x52 0x14 0x11 0x10], units 1,2,3,5,10 only
    1998-03-03 (50875, slot 3):  armed PIDs [0x33 0x52 0x14 0x11 0x10], and
                                 unit 7: a3/fe 0b/ff b8/ff 00/00 00/00 00/00 c6/ff bb/ff

`0xc6bb` is 50875 — the unit carries the day it asked for. **A box with an armed PID and no filter
receives nothing**, so five days in eight cannot be fed *through the hardware filter* at all.

The finding above stands and is still worth having — but it was never why the guide was empty. It is
re-measured and put in its place two sections down: the box only ever filters for a day whose slot is
one of the three, and in the evening it will do so for TOMORROW, which is how the same box programs a
unit on six days in eight at 19:00 and three at noon. What the derived addressing then delivers is
registered on all eight. **The empty guide was a different fault entirely, and the day-of-eight rule
is what it was wearing.**

### The methodological note

The sweep's first reading was *"the box never asks on five days in eight"*, taken through a census
that required `table.Mask == 0xfe && table.Value&0xf0 == 0xa0`. OpenTV title tables are `0xA0`-`0xA4`
**and `0xB0`**, so that census could have been narrowing away a real subscription, and the reading
could not be believed until it had been checked without the narrowing.

Dumping all sixteen units unconditionally **confirmed** the finding rather than overturning it —
there is genuinely no title unit on those days — and in doing so produced the more useful half of
it, which the narrow census could not see at all: **the PID arms on every one of the eight days.**
"Never asks" was true about the filter and false about the request.

So the lesson is not the usual one. The census was too narrow, the answer it gave happened to be
right, and the cost of the narrowness was a missing observation rather than a wrong one. Widening it
was still the right move, and the unconditional dump is what made the day-of-eight rotation
provable. The census now accepts the whole OpenTV title family.

---

## A day of listings is FOUR SIX-HOUR BLOCKS, and the table id's low two bits say which

*20 Sep 2026, in the Go port, against the real firmware. This is the answer to TASK-6.13, and the
task's own title is wrong: nothing about it is a day-of-eight problem.*

**The guide registers its notification slot for the block its own clock is in.** Read off one date
(MJD 51171) at eleven times of day, with the transmitter and the schedule identical in every run:

| local time | slot's `tableIdLow` | | local time | slot's `tableIdLow` |
|---|---|---|---|---|
| 00:00, 05:00 | **0** | | 12:00, 15:00, 17:00 | **2** |
| 06:00, 09:00, 11:00 | **1** | | 18:00, 21:00, 23:00 | **3** |

The three edges are 06:00, 12:00 and 18:00, so the rule is `hour / 6`, and `0x800C579C` fires a slot
only when the arriving section's `tableId & 3` equals it. The day key is the clock's own day in all
eleven. A second slot registers moments later for the NEXT block — at 19:00 it reads day+1 with
`tableIdLow` 0, which is exactly the block after 18:00–24:00.

**So a transmitter that stamps every section `0xA3` broadcasts a whole day of television into the
evening block.** At 19:00 the guide draws it. At every other hour the box stores all 67 programmes,
raises nothing, and the banner says:

    101 Sky One                              12.00pm Thu 24
    Further schedule information is not available
          Search Channel · Search Favourite

— with *Search Time* absent, which is the tell: that option only exists when there is a schedule to
search. **This is what "the guide does not draw them" was, on every one of the five days.** The demo
pins 19:00, so the days that looked as though they worked were the days somebody looked at in the
evening; the correlation with MJD mod 8 was real but incidental, and it cost a week.

### What the box takes, and why 67 of 67 is no longer the right number

With the day cut into four blocks and all four broadcast, the box registers **only the blocks it is
listening to** — the one its guide is in, and the one its own match unit named:

| clock | registered | which blocks |
|---|---|---|
| 19:30 | 21 of 67 | block 3 alone (the unit names `0xA3`, which is also the guide's) |
| 12:00 | 45 of 67 | blocks 2 and 3 (the guide's, plus the `0xA3` the unit asks for by day) |

Both numbers are the schedule's own per-block counts to the programme. A firmware test that waits for
a whole day to register therefore waits for ever, which is how this change first presented: five
green tests turning red at once, each reporting a plausible fraction.

### The acquisition's block is not the guide's

The match unit asks for `0xA3` at every hour measured except the small ones, where it asks `0xA1` —
so the box's *acquisition* fetches the evening (and, overnight, the morning) block regardless of the
time, while the *guide* listens for the block it is in. The two are separate mechanisms with separate
policies, and reading either one as the other is the mistake this section exists to prevent.

### And the PID is a function of the day, not a choice off the box

An evening box arms TWO listings PIDs — today's and tomorrow's. They are adjacent on seven days in
eight and `0x37` with `0x30` on the eighth, so "the last armed PID" is today's on some days,
tomorrow's on others, and on MJD mod 8 == 7 the transmitter sent the entire schedule on tomorrow's
PID: 0 of 67 registered, no error anywhere. `0x30 | (MJD mod 8)` is the whole rule, now verified on
sixteen days across two months and two times of day, and the port computes it rather than picking one
off the armed list.

### The screens

Midday, after the fix, with the committed six-channel line-up:

    101 Sky One                              12.00pm Thu 24
    NOW      Dream Team
    1:00pm   Dream Team
      Search Time · Search Channel · Search Favourite

All four blocks draw when the schedule has programmes in them — the overnight block needed a schedule
with overnight television to prove it, because the demo line-up starts at 06:00 and an empty block is
honestly empty. All eight day-slots draw at midday, where before the fix none of them did.

### The instrument

`multiplex.GuideSlots` is the Go port of the oracle's `__siGuideSlot`: it walks the two tables at
`0x80165048` (0x10-byte headers, u16 count at +0, slot pointer at +8, slots of 0x2C) and dumps every
active one. Two things cost time and are worth carrying:

- **The active flag at +0x00 is a BYTE.** Read as a word it is `0x01xxxxxx`, which is not 1, so every
  slot in a fully populated table reads as inactive and the instrument reports a guide that
  subscribed to nothing. It reported exactly that, twice, on a box that had two live slots.
- **A key pressed the instant the last record registers does nothing at all** — no slot, and the
  screen never changes. The press has to follow a settle, and a measurement taken without one is a
  measurement of a box that never opened its guide.

---

## Addressing the listings: two masks, and two facts that came off the screen

*Measured 2026-09-20 while wiring the modelled multiplex into the running server. Everything here
was found by being wrong first, and each wrong reading presented as **"the box is not asking"** —
which is the same symptom as a box that has not acquired, a box on the wrong day, and a box whose
sections were dropped by the hardware. That symptom has now been produced by four distinct causes in
this project, so it should be treated as "something upstream is wrong" and never as a finding.*

### 1. The table-id match mask is not always `0xFE`

`a3/fe` and `a3/ff` have both been observed on the same firmware, for the same purpose. A census
written as `mask != 0xFE -> skip` reported that a box had programmed no listings filter while its
unit 7 read, one byte pair per match byte as `value/mask`:

    a3/ff 0b/ff b8/ff 00/00 00/00 00/00 c7/ff 23/ff 00/00 00/70

That is table `0xA3` matched exactly, extension `0x0BB8`, MJD `0xC723` = 50979 = 15 June 1998 —
the right extension and the right day, on a box being reported as not asking.

The only invariants worth testing are that the unit compares the table id at all (`mask != 0`) and
that the value is one the title parsers take (`0xA0`-`0xA4`, `0xB0`). Send back exactly the value
that was read and both masks are satisfied.

### 2. THE EXTENSION HAS A MASK, AND ONE REQUEST COVERS A SET OF CHANNELS

This is the important one. The box does not ask about one channel at a time. It builds a **set
filter**: the extension VALUE is the bitwise OR of the listings ids it wants, and the extension MASK
clears the bits that differ between them. Swept by loading N channels with ids `0x0BB8` upwards:

    channels loaded   1        2        3        4        5        6
    extension asked   0x0BB8   0x0BB9   0x0BBB   0x0BBB   0x0BBF   0x0BBF
                      = the running OR of 0x0BB8, 0x0BB9, 0x0BBA, 0x0BBB, 0x0BBC, 0x0BBD

and with six loaded the full unit reads `bf/f8` — value `0xBF`, mask `0xF8`, admitting `0x0BB8`
through `0x0BBF`.

**So the value on its own is frequently nobody's listings id.** A transmitter that reads it and
looks up the channel it names finds nothing, sends nothing, and reports nothing. Answering correctly
means asking each channel whether the filter wants it: `id & mask == value & mask`.

### 3. The guide's row number is the LISTINGS ID, not the line-up's channel number

With `listingsId` 3000 and `channel` 101 the guide drew **`3000 Sky One`**. Setting `listingsId` to
101 drew **`101 Sky One`**. Whatever the line-up's channel field is for, it is not what the now/next
banner prints, so a channel's listings id must BE the number a viewer expects to see.

### 4. The box applies its declared time offset to PROGRAMME times as well as to the clock

With a TOT declaring +60 (BST) and a programme sent at 19:00, the banner read `8.00pm` and named
that programme as NOW. Both the clock and the schedule shift together, so **the wire carries UTC and
a human-edited schedule is in local time**; the conversion belongs in the transmitter. Sending local
times directly reads correctly all winter and is an hour out all summer, which is a plausible
listing rather than a visible fault.

The demo therefore pins its in-world day to Christmas Eve 1998 — MJD 51171, which is in the
subscribing set of section *The clock table is the TOT* and is in GMT, so the demo does not rest on
the offset conversion being right.

### The eight-day rotation, re-measured

The finding that only MJD mod 8 in {1, 3, 6} programs a filter was first taken through the census
that assumed a `0xFE` table mask and no extension mask. Both assumptions were wrong, so it was
re-taken with a census that assumes neither. **It survived unchanged**, and is now pinned by
`TestOnlyThreeDaysInEightProgramAListingsFilter`, which is written to fail when the defect is fixed.

---

## The handset map, swept exhaustively

*Measured 2026-09-20. Every raw code 0x00-0xFF pressed on its own restored box, nine million
instructions to settle, framebuffer hashed. This replaces every partial key-map note above: it is
the complete set of codes this firmware reacts to from the idle picture.*

| code | screen |
|---|---|
| `0x0C`, `0x80` | **tv guide** — the now/next banner over the picture |
| `0x7D` | **box office** — the Sky menu, opened on the BOX OFFICE tab |
| `0x7E` | **services** — the Sky menu, opened on the SERVICES tab |
| `0xCC` | standby |
| `0xF5` | **interactive** |

**Every other code does nothing at all.** There is no code that opens the menu on TV GUIDE, and no
separate "sky" or "home" key that opens the menu: `0x7D` is what opens it. The TV GUIDE tab is
reached from inside the menu, one LEFT of BOX OFFICE, and the tab is REMEMBERED — press box office,
arrow to TV GUIDE, leave, and the next box office press reopens on TV GUIDE. That is why `0x7D` can
look like a home key on one box and a box-office key on another: it depends on where the last
session left it.

### The firmware names its own keys

The SERVICES menu's first item, *USING YOUR SKY DIGIBOX*, is a help screen that documents the
remote — which is a better authority than any external photograph, and settles what to call each
key:

    To find a programme press 'tv guide', then choose the category you need. Use the arrow keys
    to move around the listings and press 'i' for more information on a programme
    To see what's on Box Office and order a movie press the 'box office' key
    To set up Parental Control press 'services' and choose '3'
    Press 'Sky' any time to return immediately to TV viewing

So the box's own name for `0x7D` is **box office**, not sky. The 'Sky' key it describes is one that
RETURNS TO TV VIEWING rather than opening anything: swept from inside the menu, `0x65` and `0x83`
both exit to the picture and both do nothing from idle, which is exactly that behaviour. Neither is
named further here, because "returns to TV" does not distinguish them and guessing which is Sky is
the kind of plausible answer this file exists to stop.

### Keys that act only inside the menu

A code that does nothing from the idle picture may still do something once a menu is open, and the
first sweep of this map missed that entirely by pressing everything from idle. From inside the
menu: `0x60`, `0x61`, `0x62` and `0x81` move the highlight or open a sub-screen, `0x65`, `0x80` and
`0x83` exit to the picture, and `0xF7`-`0xFC` each draw something of their own.

---

## The Huffman dictionary is a DECODER'S table, and two readings of it were wrong

*Measured 2026-09-20, against the box's screen. Both defects had been in every title section this
project ever broadcast, and both survived a byte-for-byte reference-vector test — because the
vectors came from an encoder that had been validated against a decoder, and the two shared the
misreadings.*

### 1. A value's real code is the SHORTEST of its many codes

`skyuk.dict` has 512 lines and 447 codeable values. The gap is almost all SPACE, which appears
**sixty-five times**:

    3 bits    110
    7 bits    1110111
    17 bits   11100011011011101
    27 bits   x62

The long ones are the flattened tree's padding, not alternative spellings, and the box does not
decode them back to a space. A loader that keeps the LAST duplicate — which a plain map assignment
does — emitted a 27-bit filler for every space in every title, and the guide drew `DreamTeam`
where the schedule said `Dream Team`.

### 2. THE PADDING IS DECODED, and `s` is coded `0000`

The box reads a title record to the length the record declares, not to the terminator, so whatever
sits in the tail of the final byte is walked down the tree like any other bits. Zero-filling it is
the worst available choice in this table: `s=0000`. Every title gained a trailing `s` —
`Dream Teams`, `WalkerTexasRangers` — and a decoder that stops at the terminator, as ours did,
never sees it.

The fix pads with the TERMINATOR'S OWN BITS, truncated to what is left. Every proper prefix of a
code is by construction not a leaf, so a partial terminator walks part-way down the tree and runs
out of data without emitting anything. It needs no reserved code and stays correct if the
dictionary changes.

### 3. And the parse order recovers one more value

openTVtoXML's `huffman_read_dictionary()` tries a SINGLE CHARACTER pattern before a phrase pattern.
That order is what reads `==<bits>` as the code for `=` rather than as an empty value with
malformed bits. Cutting at the first `=` dropped it, so a title containing an equals sign could not
be encoded at all. The recorded entry count moves from 446 to 447.

### The method note, which is the point

**A round trip is not validation.** These three were invisible to a round-trip test, to a
byte-for-byte vector test, and to a decoder transcribed from the reference — because every one of
those instruments shared the encoder's reading of the dictionary. The only instrument that
disagreed was the box drawing the text on a screen, and it had been disagreeing in plain sight for
as long as there have been titles to draw.

---

## Reading the screen as an instrument

*Built 2026-09-20 for the encoder cross-check. Two mistakes were made getting it right, and both
are the same mistake in different clothes: hashing a frame at a chosen instruction.*

**A frame hashed at a guessed moment catches the box MID-REDRAW.** The now/next banner paints in
stages, and a capture landed on `NOW Dream Team` rendering over the *Further schedule information is
not available* line it was replacing — one picture containing both states, and a hash that says
nothing about either.

**A frame hashed once the screen has SETTLED catches the wrong screen entirely.** The banner is
transient: wait for the picture to stop changing and the answer is the plain blue it returns to when
the banner times out, which is identical whatever the title said. The first version of the
cross-check "passed" three different titles as one screen, and was only caught because it carried a
third assertion — that two titles differing in two ways must not draw alike — which is the guard
worth copying.

So a screen instrument needs both halves: **ignore what was already up, and wait for something new
to hold still.** `internal/multiplex/firmwaretests/rununtil_test.go` has `drawnScreen` doing exactly that, and
`screenNow` to capture the before.

### And it is the only independent check the codecs have

An encoder tested against a decoder written from the same reading of a format is one instrument
wearing two hats. The Huffman defects above prove it: a round trip, byte-for-byte vectors and a
transcribed reference decoder all agreed for as long as the guide was drawing `DreamTeams`. When a
codec's output is eventually DRAWN, the drawing is the check.

---

## The ALL CHANNELS grid, re-measured WITH listings on air

*2026-09-20. `sky-02me.5` ruled the empty grid "not a listings fault" on 16 Sep, and that ruling was
sound reasoning on the evidence available — but it was made before this project could deliver a
single programme, so it was an inference rather than a measurement. It has now been taken with
sixty-seven programmes in the box's store, counted on its own per-event register.*

**The ruling holds.** Route: box office (the menu) → LEFT to the TV GUIDE tab → select, which is
`0x5C`. The grid draws

    7.00pm Thu 24
    ALL CHANNELS
          Today  7.00pm      7.30pm      8.00pm

— our in-world date, our clock, and correctly spaced half-hour columns — **and no channel rows**.
Identical in substance to the pre-broadcast measurement. Whatever stops the row loop, it is not the
absence of programme data.

### One practical correction to the route

`0x5C` appears not to work if it is pressed while the menu is still painting. The TV GUIDE tab
takes a noticeable time to fill its rows when the box is also parsing a broadcast, and a select
delivered during that window is lost silently — three attempts here concluded "select does nothing"
before the settle was made long enough. Press, let the screen finish, then press again.

That is the same instrument failure as hashing a frame at a chosen instruction, wearing different
clothes: the box was not in the state the measurement assumed.
