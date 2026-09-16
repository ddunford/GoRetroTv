# Test Plan: Phase 1 — Foundation

## Prerequisites
- Go 1.22+, Docker, and `firmware/FLASH_U202.bin` present

## Audit record

Audited 2026-09-16 by p1-qa (a different model from the builders), by reading each named test's
body — not its name — and asking of every assertion "could this fail?". Where the answer was in
doubt the test was run against a deliberately degraded input via `go test -overlay` (no edit to the
tree) and the outcome recorded. Whole suite: `go test -count=1 ./internal/...` exit 0 (captured to
a file, not read through a pipe). The real firmware is on this machine, so
`TestTheRealFirmwareMatchesTheCommittedManifest` RAN rather than skipped.

## Test Cases

- [x] **TC-1.1: Logging carries emulator time** (covers: TASK-1.2, TASK-1.12)
  **Steps:** emit a log line from the core after N instructions.
  **Expected:** JSON with both `icount` and a wall timestamp; `icount` matches the machine.
  (proved: `internal/logging/logging_test.go::TestALogLineCarriesBothEmulatorTimeAndWallTime` —
  a real `clock.Clock` advanced 4,500,000 instructions, asserts the `icount` field is present,
  equals 4,500,000 and equals `machine.Now()`, and that `time` parses as RFC3339Nano.
  `TestICountTracksTheMachineRatherThanTheLoggersAge` closes the capture-at-build defect: one
  logger, three advances, three different counts. `TestWithoutACounterThereIsNoICountField` pins
  absent ≠ zero. Note: "the core" is `platform/clock` — there is no CPU until phase 2; the counter
  interface is the seam the CPU will implement.)

- [x] **TC-1.2: A device round-trips through snapshot** (covers: TASK-1.5, TASK-1.12)
  **Steps:** a stub device with internal state → mutate → `Snapshot` → mutate again → `Restore`.
  **Expected:** state equals the snapshot exactly. **And a device that forgets a field must fail
  this** — add one deliberately to prove the test can catch it.
  (proved: `internal/bus/bustest/bustest_test.go::TestSnapshotRoundTripsACompleteDevice` for the
  round trip and `::TestSnapshotCatchesAForgottenField` for the negative control — the `forgetful`
  device's `Snapshot` omits `seq`, a field no bus read can observe, and the check reports it by
  name. `::TestSnapshotCatchesARestoreThatDoesNotReplace` covers the mirror defect (a merging
  Restore). Comparison is whole-value including unexported fields via `bustest.difference`.
  **Finding, not a failure:** the negative control's assertion (`err != nil` and the message
  contains `"seq"`) is satisfied by TWO causes — the completeness failure it is meant to catch,
  AND the coverage harness-failure `"Mutate left scratchpad.seq at the value a fresh device has"`.
  Proved by overlay: degrading the shared `mutate` to three writes (so `seq` wraps to its fresh
  value) leaves `TestSnapshotCatchesAForgottenField` PASSING on the harness-failure text while
  `TestSnapshotRoundTripsACompleteDevice` goes red. The suite catches it only because the two
  tests share one mutator. Recommended to p1-core: also assert
  `!strings.Contains(err.Error(), "harness failure")` (or require `"incomplete"`) so the
  control cannot pass on a refusal to run.)

- [x] **TC-1.3: Memory aliases agree** (covers: TASK-1.6, TASK-1.12)
  **Steps:** write through `0x80000000`, read through `0xA0000000`, and vice versa.
  **Expected:** identical bytes; the flash region refuses writes.
  (proved: `internal/memory/memory_test.go::TestDramAliasesAgree` — byte, half and word widths,
  cached→uncached then the COMPLEMENT uncached→cached so a stale first value cannot satisfy the
  second read; includes `0x105C44`, an address the record quotes, and the top word of DRAM.
  `::TestFlashRefusesWritesThroughBothWindows` guards its own subject (`before != 0` or fatal),
  writes `0xFFFFFFFF` via the reset window and `0` via the cached mirror, asserts the bytes are
  unchanged AND that the part counted exactly 2 refusals — so a flash that silently swallowed the
  write would fail on the count. Bus-level decode of the same aliases:
  `internal/bus/bus_test.go::TestBothDramAliasesReachTheSameDeviceOffset`.)

- [x] **TC-1.4: A wrong ROM is refused** (covers: TASK-1.7, TASK-1.12)
  **Steps:** start with a flash image whose checksum does not match the manifest.
  **Expected:** refuses to start, naming the mismatch. Running a different ROM silently is the
  failure this prevents.
  (proved: `internal/firmware/firmware_test.go::TestAnImageWithTheWrongContentsIsRefused` — one
  byte flipped mid-image with the SIZE preserved, so only the digest can catch it; asserts the
  error names `FLASH_U202.bin` and `SHA-256`, and does NOT blame the untouched U203.
  `::TestAnImageOfTheWrongSizeIsRefused` pins the size check's own message so it cannot be
  deleted and pass on the digest mismatch alone. `::TestTheRealFirmwareMatchesTheCommittedManifest`
  RAN on this machine (not skipped) and additionally asserts the reset path
  `lui/addiu/jr $t0` at U202+0x08. "Refuses to start" at the process level is
  `cmd/goretrotv/main.go::run` calling `firmware.Load` before the listener opens and returning the
  error to `os.Exit(1)` — verified by reading, not by a test; the running-stack proof is gate
  `gort-6ar.17`.)

- [x] **TC-1.5: Checkpoints are deterministic and cover the ISA bit** (covers: TASK-1.8, TASK-1.9, TASK-1.12)
  **Steps:** run the same program twice; then run it with the ISA mode bit flipped at a known point.
  **Expected:** identical streams for the first pair; a differing checkpoint for the second.
  (proved — Go half: `internal/platform/statehash/statehash_test.go::TestTwoIdenticalRunsProduceIdenticalStreams`
  (byte-identical, with a ≥5-line guard against a vacuous empty stream) and
  `::TestFlippingTheIsaBitDivergesTheStream` (flip at instruction 2,500; asserts the FIRST
  differing line is the `3000` checkpoint, so it localises rather than merely differs).
  `::TestEveryFieldOfTheMachineChangesTheHash` has an explicit "the ISA mode bit" case;
  `vectors_test.go::TestKnownVectors` pins the algorithm and the `0x3D280665` whole-machine vector.
  Oracle half (TASK-1.9, commit 23487c1): `statehash/oracle_agreement_test.go::TestTheOracleAgreesWithThisHash`
  reads the oracle's own `__cpVectors()` expectations out of `reference/digibox-boot.html` and
  asserts they equal what the Go package computes, plus the same FNV constants, `Math.imul`, the
  `?cp=` flag guard, and the `GRTV-CHECKPOINTS`/`END` stream format; it harness-fails if an
  expectation cannot be found. Re-audited by overlay: pointing it at a copy of the page with ONE
  hex digit drifted turns it red naming the drifted digest. **First audit was `[?]` because the
  emitter was uncommitted and nothing committed checked the two implementations against each
  other; both are now true, so flipped.**
  **Limits, stated:** (1) the committed guard pins the PRIMITIVES (`mixWord`, two page digests),
  not the whole-machine composition — the oracle's self-test does not carry `0x3D280665`, so a
  field-order or ISA-encoding drift in `cpHash()` would pass it; by reading, `cpHash()` folds
  PC, ISA-as-octet, HI, LO, 32 GPR, 32 COP0, RAM digest in the same order as Go's `Hash`, and a
  drift there would surface at checkpoint 0 in TC-1.6's live comparison. Recommended to p1-core:
  add the whole-machine vector to `__cpVectors` and to this guard. (2) Oracle end-to-end
  determinism is proved by hand, not by a committed test: p1-qa independently checked the two
  cold-boot streams p1-core recorded — run2 is a clean PREFIX of run1 over 4,613 checkpoints /
  461M instructions (run1 was left running to 4,934), every line canonical, both `END` trailers
  self-consistent, 4,934 distinct hashes so the hash is not constant. Automating that needs a
  browser and belongs to the boot gate (TC-1.7). **Both limits are tracked as `gort-6ar.25`,
  which blocks gate `gort-6ar.16`; when it lands — a committed oracle `?cp=` stream compared to a
  Go stream, and the whole-machine vector pinned on both sides — TC-1.5 gets a third look and the
  limits come off.**)

- [?] **TC-1.6: The comparison localises an injected divergence** (covers: TASK-1.10, TASK-1.12)
  **Steps:** corrupt one register at instruction 4,500,000 in one stream.
  **Expected:** reports the divergence in the 4,500,000–4,501,000 window. **And over an empty or
  truncated stream it must report a harness failure, not success.**
  (proved, commit 8844aef: `internal/platform/statehash/compare_test.go::TestAnInjectedDivergenceIsLocalisedToItsWindow`
  — one bit of GPR7 from instruction 4,500,000 on; asserts `StateDiverged`, `Lo/Hi` exactly
  4,500,000..4,500,999, window 4500, and `Compared >= 4000` so it cannot have stopped at line one.
  Harness-failure clause: `::TestAnUnusableStreamIsAHarnessFailureAndNotAgreement` — truncated
  (END line removed, keyed off `Stream.Complete`, i.e. the trailer's ABSENCE, not line count →
  `ErrTruncatedStream`) and empty (header only → `ErrEmptyStream`), each tried in BOTH argument
  orders, each `IsHarnessFailure`. At the command: `cmd/oraclecmp/main_test.go::TestAnUnusableStreamExitsTwoAndSaysSo`
  — exit 2 (not 0, not 1) with "HARNESS FAILURE" in words; `::TestDivergingStreamsExitOneAndNameTheWindow`
  — exit 1 naming `2000..2999`. Empirically re-run by p1-qa on the built binary, exit codes
  captured to files: real oracle run1 vs run2 → exit 0 "agree over 4613 checkpoints"; run1 vs a
  2,000-line copy with no END → exit 2 harness failure; run1 vs itself with ONE hash zeroed at
  checkpoint 2000 → exit 1 "state diverged at window 2000 (200000000..200099999)". No hand-rolled
  hex in `compare.go`/`stream.go`/`main.go`; hashes print via `hexfmt.Word` and parse via
  `hexfmt.ParseAddr`.
  **Finding, recorded as a limit on this tick — the straddle case:** the real oracle stream has
  478 of 4,934 checkpoints off-boundary (`300001`, `1000001`, …) because its MIPS32 path retires a
  branch and its delay slot together. `Compare` pairs those by WINDOW (so no `WindowMissing`) but
  then demands equal instruction counts inside the window and reports `CadenceDiverged`, exit 1.
  p1-qa fed it a synthetic straddle with IDENTICAL hashes (`300000` vs `300001`) → exit 1 "cadence
  diverged at window 3". `compare.go`'s doc says demanding equal counts "would report a
  divergence at the first straddling branch between two machines that agree perfectly" — and the
  code then does exactly that, and `::TestADifferentInstructionCountInTheSameWindowIsItsOwnKindOfDivergence`
  pins it as intended. The classification is HONEST (a hash after 300,001 instructions is not
  comparable to one after 300,000), so the defect is upstream in the emitters, not in the
  comparator: on a live oracle-vs-Go run with per-instruction Go retirement the tool exits 1 at
  the 4th checkpoint and never reaches a state divergence. Fix belongs to whichever emitter
  changes — the oracle hashing at the exact boundary before retiring the delay slot, or Go
  adopting the oracle's retirement granularity — and must land before `gort-6ar.25`'s live
  comparison and phase 2's TASK-2.9 can mean anything. Issue to be filed by the lead.)

- [?] **TC-1.7: The boot gate can go red** (covers: TASK-1.11, TASK-1.12)
  **Steps:** run the gate against a deliberately broken build.
  **Expected:** non-zero exit and a readable reason. A gate nobody has seen fail is decoration.
  (blocked: no gate verb in `ctl.sh` yet — TASK-1.11 / `gort-6ar.11` is open; `ctl.sh health`
  exists but is a probe, not a gate. The health endpoint half has a unit test,
  `internal/httpx/handlers/health_test.go::TestHealthReportsOKAndIdentifiesTheBuild`, which
  asserts a non-empty version so a gate cannot pass against an unnameable binary. The
  "shown to go red" clause needs the gate itself.)

- [x] **TC-1.8: The platform primitives hold their contracts** (covers: TASK-1.14, TASK-1.12)
  **Steps:** format an address containing a hex letter through `hexfmt` and look it up by the same
  key; round-trip a struct through `snapcodec` at two format versions; ask `instrument` for a subject
  that is absent.
  **Expected:** the lookup hits (**a lower-cased key must fail this test** — the mismatch that
  reported zero for `0x80081C58` twice); the older version decodes or is refused by name, never
  mis-read; and the absent subject returns a **harness failure, not a count of zero**.
  (proved, clause by clause:
  hexfmt — `internal/platform/hexfmt/hexfmt_test.go::TestLookupByCanonicalKeyHitsAndByLowerCasedKeyMisses`:
  keys a map by `Addr(0x80081C58)`, hits; guards its own subject (`lowered == canonical` is a
  fatal "test is vacuous"); asserts the lower-cased key MISSES; then that `NormalizeAddr` makes it
  hit. If `Addr` ever emitted lower case, `TestCanonicalForms` and the vacuity guard both go red.
  snapcodec — `internal/platform/snapcodec/snapcodec_test.go::TestRoundTripAtTheCurrentVersion`
  and `::TestRoundTripAtTheOlderVersion` (both restore INTO a device carrying different stale
  state, so an unwritten field shows as the stale value rather than a coincidental zero; the v1
  path asserts the v2-only field is reset to nil), `::TestAFutureVersionIsRefusedByName` (names
  device and `v3`, and the device is untouched after refusal).
  instrument — `internal/platform/instrument/instrument_test.go::TestAnAbsentFindingIsAHarnessFailureNotAZero`
  (undeclared finding → error that `IsHarnessFailure` and wraps `ErrUnknownFinding`, and the
  count returned alongside is 0 not a plausible number) and
  `::TestACensusThatExaminedNothingRefusesToReport` (empty population → `ErrNothingExamined`).)

- [x] **TC-1.9: A missing required variable refuses the boot** (covers: TASK-1.15, TASK-1.12)
  **Steps:** start with a variable named in `.env.example` removed from the environment.
  **Expected:** refuses to start, naming the variable. And every variable the binary reads appears in
  `.env.example` — asserted by walking the config struct, not by eye.
  (proved: `internal/config/config_test.go::TestAMissingRequiredVariableRefusesTheBoot` — one
  subtest per required variable, the environment otherwise complete and everything else CLEARED via
  `t.Setenv` so a shell export cannot rescue it; asserts `Load` errors, returns nil config, and
  names the variable. `::TestEveryMissingRequiredVariableIsReportedAtOnce` is the guard that the
  required set is non-empty (with zero required variables it would fatal), so the per-variable
  loop cannot pass vacuously. Second clause:
  `::TestEveryVariableTheBinaryReadsIsDocumented` walks `config.Variables()` (reflect over `env`
  struct tags), first asserts the walk reached EVERY struct field (`len == NumField`) and found
  ≥5 via `instrument.MustFind`, then checks both directions against a parsed `.env.example` with
  a one-entry explicit allowlist for `GORETROTV_PORT` (read by ctl.sh/compose only).
  `grep os.Getenv|os.LookupEnv` over non-test Go finds only `config.go:134`, so the struct walk
  is the whole surface. Process-level refusal is `main.go::run` → `config.Load` error →
  `os.Exit(1)`, verified by reading.)
