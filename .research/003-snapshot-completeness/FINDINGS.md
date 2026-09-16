# Spike 003 — what a snapshot must capture

**Verdict: PROVEN tractable, and the state list is already written down — by the reference
emulator's own `reset()`.** That function has to return the machine to a known state, so it
enumerates everything that *is* state. Anything it clears and a snapshot omits is a silent
corruption of every later experiment.

## The inventory, read off `reference/digibox-boot.html`

**CPU** — `ram` (32 MB), `reg[32]`, `pc`, `hi`, `lo`, `cp0[32]`, `isa` (the MIPS16/MIPS32 mode bit),
`icount`.

**Interrupt and timer** — `irqCount`, `timerPending`, `timerCounter`, `timerTicks`, `timerArmed`,
`forceIP`, `eretCount`.

**Both flash chips** — `CHIP0`/`CHIP1` each with `mode`, `seq`, `program`, `erase`; plus
`flashAutoselects`, `flashPrograms`, `flashErases`. **A flash chip mid-command-sequence is real
state**: snapshot between the unlock write and the command and restore without it, and the next
write is interpreted as a fresh command.

**Peripherals**, each with its own reset and therefore its own state: `csiReset` (the handset link),
`i2cReset` (I²C and the EEPROM — *this is the NVRAM*), `vramReset`, `uartReset`, `sc1Reset` (the
smartcard link), `dmaReset`, `blitReset`, `dispReset` (the display/OSD registers).

**MMIO bookkeeping** — `mmioRegs`, and the `mmioReads`/`mmioWrites` maps.

**Boot sequencing** — `handoffDone`, `handoffAt`, `idleSpin`.

**Not state**: `cells`/`freshCells` (the memory-map visualisation), `busLines`, `phase`, `lastCount`,
`lastT`, `flashLog`, `unknownSites` — instrumentation and UI. Snapshotting them is harmless;
omitting them changes nothing.

## The trap this spike exists to name

**A partially-correct snapshot does not fail — it produces a plausible machine.** Restore without
the demux ring pointers and sections land at the wrong offset; without the I²C transaction state the
next EEPROM read returns the wrong byte; without `isa` the first instruction decodes in the wrong
mode and the divergence looks like a CPU bug. Every one of those reads as a firmware fault.

**So the acceptance test is not "it restores".** It is: snapshot at instruction *N*, restore, run to
*N+10,000,000*, and require the state hash to equal that of an uninterrupted run to the same point.
That reuses spike 002's checkpoint hash, and it is the only check that can actually fail.

## Consequence for the design

Snapshot/restore is not a convenience feature bolted on later — the acceptance test above depends on
the checkpoint-hash machinery from FR-6, so **the two are built together**. Peripherals must be
written with serialisable state from the start; retrofitting that across eight device models is the
expensive version.
