# Spike 002 — the shape of the oracle comparison

**Verdict: FR-6 was impractical as written, and is now a two-tier scheme.** The requirement said
"instruction-for-instruction across a full cold boot". Costed, that is 447,000,000 instructions:

| What you would store per instruction | Size for one boot |
|---|---|
| PC only, binary | **1.8 GB** |
| PC + 32 registers | **59.0 GB** |
| PC as hex text, which is what the browser emits today | **4.9 GB** |

Per run, on both sides, for every comparison. Unusable as a routine check.

## The scheme that works

**Tier 1 — rolling checkpoints.** A 32-bit hash of machine state (PC, registers, a digest of dirty
RAM pages) every **1,000 instructions**: **447,000 checkpoints, 3.6 MB**. Cheap enough to emit on
every run on both sides, and it answers the only question that matters continuously — *do these two
machines still agree?*

**Tier 2 — full trace of the disputed window.** When checkpoint *n* differs, re-run both sides with
full per-instruction tracing between checkpoints *n-1* and *n*: a bounded **1,000 instructions**,
trivially diffable, with the exact divergent instruction and both machine states.

Localisation is the whole point. A 1,000-instruction window is small enough to read.

## Why this is better than it sounds

- Storing less makes the check **runnable in CI**, and a verification nobody runs is decoration.
- The hash interval is a dial: 10,000 costs 0.4 MB and localises to 10,000; 1,000 costs 3.6 MB.
  Start at 1,000.
- Both sides already have the primitives — the browser has `__pcHits`/`__readWatch` and an
  instruction counter; the Go side controls its own loop.

## The limit, stated plainly

**Where both implementations are wrong in the same way, they will agree.** The oracle proves the
port matches the browser emulator, not that either matches a Pace 2500N. The independent evidence
is `docs/reference/digibox-emulation.md`, where each claim records how it was measured against the
real firmware's behaviour. The oracle catches porting regressions; it cannot catch inherited ones.
