# Spike 001 — Go interpreter throughput

**Verdict: PROVEN, with the SPEC target revised down.** A plain switch-dispatch interpreter in Go
sustains **44.2M instructions/s**. The browser emulator does **3.1M/s**. That is a **14× floor** on
the value of the port — and the SPEC's 50M/s target is not reachable this way and has been changed.

## The number, and what it is a number for

    executed   : 200,000,000 instructions
    elapsed    : 4.53 s
    throughput : 44.2M instructions/s
    working set: 18 distinct PCs — a real loop with real memory traffic

Representative mix: roughly one third ALU, one third load/store against a working set larger than
L1, one third branches — the shape of the interpreted code in this firmware.

**It is an UPPER BOUND.** No MIPS16 decode, no COP0 bookkeeping, no per-instruction interrupt
check, no peripheral dispatch on loads. A real core does all four, so expect meaningfully less.

## The first measurement was worthless, and how that was caught

The first version ran the real flash from the reset vector and reported **83.3M/s**. It was
measuring nothing. With no peripherals modelled the firmware leaves mapped memory within a few
thousand instructions; from then on the interpreter reads zeros from unmapped space and executes
them as `SLL $0,$0,0` — the cheapest instruction there is — forever.

The tell was in the PC histogram: **3,125,000 distinct PCs across 3,125,000 samples**, one per
sample, at addresses like `0xD10D59F0` and `0xC5D139F0` that do not exist on this board. A runaway
PC, not a program.

Had the histogram not been added, this spike would have reported a green light 88% above the truth.
**A throughput harness must prove it is executing a loop before its number means anything** — the
check is now in the code and prints a caveat when the distinct-PC count says otherwise.

## What this changes in the SPEC

- **NFR revised: `≥ 15M instructions/s`**, with 40M as a stretch. At 15M/s a 447M cold boot is ~30 s
  against 135 s today; at 44M/s it is 10 s.
- **And boot time is the wrong headline.** With snapshot/restore (FR-8) a full boot is paid *once*;
  the loop that matters is restore-and-press. The NFR emphasis moves accordingly.
- If sustained speed later proves limiting, the lever is threaded dispatch or a JIT — a
  substantially larger piece of work, and explicitly not v1.

## Reproduce

    cd .research/001-go-interpreter-throughput && go run .

Needs `firmware/FLASH_U202.bin`. Go 1.22.2, measured on the development host.
