# Why this emulator is deterministic, and what that buys

Determinism here is not a nicety or a performance choice. It is the single assumption the project's
whole method of checking itself rests on, and four separate capabilities collapse without it. The
individual pieces each explain themselves in their own package doc comment; what follows is the part
that is nowhere else — **the chain, and why each link needs the one before it.**

## The chain

**1. The instruction counter is the only clock.** Every timer, device pump and carousel wave is
scheduled in instructions, and nothing consults the wall. → `internal/platform/clock`

**2. Nothing concurrent runs inside the instruction loop.** Socket I/O and the web layer sit outside
it. A `Clock` is deliberately not safe for concurrent use, because a mutex would imply it needed to
be.

**3. Therefore the same firmware produces the same run, every time** — on a loaded machine as on an
idle one. Even the tie-break between two events due at the same instruction is pinned to
registration order, because "whichever the map iterated first" is how a scheduler stops being
deterministic without anyone editing it.

**4. Therefore a single number can stand for "the machine is in exactly this state."** One hash,
used by every check that compares two machines. → `internal/platform/statehash`

**5. Therefore the port can be checked against something independent.** Run the oracle and the Go
port over the same flash image, hash both at the same instruction counts, and compare. Disagreement
localises to an instruction rather than to a vague sense that the picture looks wrong.

Break link 1 and every link after it goes. That is not hypothetical: the predecessor scheduled off
wall time, two runs of the same firmware produced different event counts, and a session was spent
hunting a device bug that did not exist.

<!-- anchor: internal/platform/clock/clock.go#^func \(c \*Clock\) Advance -->
<!-- anchor: internal/platform/statehash/statehash.go -->
<!-- fingerprint: sha256:424129e702c3947c8d7522afcdea9b224951b8c531ac1b6d6cf8e3cb5bdc2161 @ 2026-09-24 -->

## Why there is exactly one state hash

Three separate requirements accept against it — the oracle comparison, byte-identical replay, and
snapshot/restore. Had each grown its own hash, the three would verify against three different ideas
of "the same machine" and agree with one another only by accident. The algorithm is written out in
the package doc precisely because the oracle has to reproduce it in JavaScript, where a one-byte
ordering difference surfaces as a divergence at checkpoint zero that reads exactly like a CPU fault.

## Why every device serialises itself from its first line

A snapshot is the same determinism argument applied across time rather than across implementations:
restoring a machine has to produce a machine that behaves identically from that point. So `Snapshot`
and `Restore` sit in the device interface beside `Read` and `Write` rather than being methods a
device might grow later.

The reason is the failure mode, and it is this project's recurring one: **a partial snapshot does
not fail.** It produces a plausible machine whose faults read as firmware bugs — in a program whose
faults are supposed to be interesting. Restore without the demux ring pointers and sections land at
the wrong offset; without the I2C transaction state the next EEPROM read returns the wrong byte.

<!-- anchor: internal/bus/device.go#^type Device interface -->
<!-- fingerprint: sha256:b99f7a51837275455c18fa277cfa40400570adb07e009e5d8036f32872c407de @ 2026-09-22 -->

## What must stay true

- **No `time.Time`, no `time.Timer`, no goroutine in the instruction loop.** Concurrency lives at
  the edges.
- **One state hash, one definition**, matched byte for byte by the oracle's JavaScript emitter.
- **Whatever `Reset` clears, `Snapshot` must carry.** That equivalence is the only available
  cross-check on a device's own idea of what its state is.
- **An instrument that cannot find what it counts reports a harness failure, never zero.** A green
  that means nothing is worse than a red, because a red gets investigated.
  → `internal/platform/instrument`

<!-- anchor: internal/platform/instrument/instrument.go -->
<!-- fingerprint: sha256:af00007f115e309f9e87b5febf843a476a726e1d7ea2894325e16fd6230cdc03 @ 2026-09-22 -->

## What this does not buy

<!-- anchor: none - states the LIMIT of the oracle method; its subject is the measured record, not a file -->

Agreement with the oracle is not correctness. The oracle proves the Go port matches the browser
emulator; where both are wrong in the same way, they agree. Inherited errors are caught only by the
measured record in `docs/reference/digibox-emulation.md`, and where that record is silent the answer
is to measure rather than to reason.

This is also why editing the oracle to agree with the port is the one move that destroys its value —
and it will look reasonable at the time.
