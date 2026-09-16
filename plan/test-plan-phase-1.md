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
  **Third look (gort-6ar.25 closed, commit 6e553e2) — the limits recorded on the second look:**
  (1) *composition unpinned* — CLOSED. `statehash/oracle_execution_test.go::TestTheOracleComputesTheSameHash`
  lifts the oracle's hash block out of the HTML and RUNS it under node against the same
  every-field-different machine Go hashes, comparing `wholeMachine`, `ramDigest` and the three
  primitives; it skips loudly if node is absent (node v24 is present here and it RAN, 0.22s).
  p1-core found the defect one level above mine — the literal test could not see an algorithm
  edit — and p1-qa proved the new one by overlay: a copy of the page with `hhi`/`hlo` swapped
  inside `cpHashOf` leaves `TestTheOracleAgreesWithThisHash` GREEN and turns
  `TestTheOracleComputesTheSameHash` RED naming `wholeMachine` (`0x03E0A005` vs `0x3D280665`).
  The literal test now also carries `wholeMachine` and `ramDigest_four_zero_pages`.
  (2) *oracle determinism by hand only* — NARROWED, not gone. A real cold boot is now a committed
  fixture, `statehash/testdata/oracle-cold-boot.stream` (4,629 checkpoints to 462.8M
  instructions; `oracle_stream_test.go::TestTheRecordedOracleStreamIsARealBoot` asserts it is
  complete, whole, contains ≥1-in-20 straddles — 477 — and pins the reset hash `0xF1F29240`), so
  the oracle's stream is regression-pinned and is what phase 2's port compares against. That the
  oracle produces the SAME stream on a second cold boot remains a by-hand fact (p1-qa: run2 a
  clean prefix of run1 over 4,613 checkpoints); a committed re-run needs a browser and belongs
  with the gate's phase-2 stage (`docs/reference/oracle-boot-gate.md`). Recorded, not tracked —
  it is a property of the instrument's platform, not an open defect.)

- [x] **TC-1.6: The comparison localises an injected divergence** (covers: TASK-1.10, TASK-1.12)
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
  **Marker correction:** commit da71a6e wrote this body and left the checkbox at `[?]` — the
  edit replaced the "(blocked …)" paragraph and never touched the marker line above it. The
  p1-qa report of that commit said `[x]`; the report was the intent, the marker was the error.
  Caught by the lead reading the file, which is what gate `gort-6ar.16` is for.
  **The straddle limit recorded on da71a6e — CLOSED as a tool defect (`gort-6ar.26`, commits
  0f2dc1f + 5c02f82):** the real oracle stream has 477 of 4,629 checkpoints off-boundary because
  its MIPS32 path retires a branch and its delay slot together; the first `Compare` stopped dead
  at the first one (checkpoint 4) so no live run could ever reach a state divergence. `Compare`
  now walks the WHOLE shared range: a window sampled at different counts is counted in `Cadence`
  and skipped (its hashes describe different instants, so it is neither agreement nor
  divergence), and `FirstState` carries the first window where equal counts had unequal hashes;
  the command points tier 2 at THAT window. `::TestAStraddledBoundaryDoesNotStopTheComparison`
  pins the bug, `::TestTwoAgreeingMachinesThatStraddleTheSameBoundariesCompareEqual` is the
  positive case (subject-guarded: asserts two checkpoints actually straddle), and
  `oracle_stream_test.go::TestTheComparisonWorksOnARealOracleStream` runs the localisation and
  the positive straddle case on the REAL fixture (`Cadence == 0` across 477 straddles against a
  copy of itself; one corrupted checkpoint at 4,500,000 localised with 4,628 agreed). p1-qa
  re-ran the built binary: fixture vs itself → exit 0 over 4,629; fixture vs a port-like stream
  with all 477 straddles moved onto boundaries and one planted divergence at 200,000,000 → exit 1,
  headline "cadence diverged at window 3", "states first differ at window 2000", 4,151 agreed, 477
  incomparable, tier 2 → `200000000..200099999`; the old identical-hash straddle → exit 1 with
  "the states agree everywhere both sampled alike, so this is a difference between the two
  EMITTERS". That last line is the design decision 5c02f82 records and this audit accepts: **the
  port conforms to the oracle** — the Go emitter must observe only where the oracle can, never
  between a branch and its delay slot (sampling mid-pair would hash a state this hash does not
  fully describe: the pending branch target and MIPS16 delay flag are not folded in). That is an
  obligation on phase 2's CPU/emitter integration (TASK-2.2, TASK-2.9), now enforced by
  `internal/cpu/checkpoint_test.go::TestCheckpointLoopSkipsReachableBranchSlot`; a per-instruction Go sampler will show as `cadence diverged`, exit 1, with the
  states still compared — a correct, readable report rather than a stopped instrument. The lead
  should carry it onto phase 2 as a constraint.)

- [x] **TC-1.7: The boot gate can go red** (covers: TASK-1.11, TASK-1.12)
  **Steps:** run the gate against a deliberately broken build.
  **Expected:** non-zero exit and a readable reason. A gate nobody has seen fail is decoration.
  (proved: `tools/boot-gate.sh` via `./ctl.sh gate` (commits 945f57c, fixed in 23382d8 /
  `gort-6ar.27`). The gate is shell with no Go test of its own; the artefact is the gate run
  against broken inputs, which p1-qa did six ways on 2026-09-16, exit codes read from files:
  GREEN `./ctl.sh gate` → exit 0, five `ok` stages, `health names this build version=a8c6273-dirty
  commit=a8c6273 (matches HEAD)`. BROKEN: (a) a binary built with bare `go build` (no ldflags),
  `--binary` → exit 1, **exactly 1 of 5** — `FAIL health names this build reports
  commit='unknown', this tree is a8c6273 — those are the linker defaults, so the build was not
  stamped` — with the other four `ok`, which is what separates five stages from one check wearing
  five labels; (b) `--firmware <absent dir>` → exit 1, refuses before any stage ("A gate that
  cannot perform its check is a refusal, never a pass"), not a skip; (c) a copy of the firmware
  with one byte flipped in `FLASH_U203.bin` → exit 1, `FAIL firmware verified` and the gate prints
  the binary's own refusal, `SHA-256 is e2615f…` vs the manifest's `7832d6…`; (d) `--binary
  /bin/true` → exit 1, 5 of 5 with "the process exited (0) before listening"; (e) a stub that
  logs the two expected lines then ignores SIGTERM → exit 1, `FAIL shuts down gracefully still
  alive 10s after SIGTERM`, so stage 5 fails on its own path. **History that matters:** the first
  version's identity stage asserted only that version and commit were NON-EMPTY, which the
  linker defaults `dev`/`unknown` satisfy on every unstamped build; it had been watched going red
  only against a stub returning an EMPTY version, which the real build path cannot produce. The
  fix builds through `make build` (one definition of identity) and asserts commit EQUALS `git
  rev-parse --short HEAD`, a claim that can be false and was seen false in (a). The handler's
  unit test had the same defect one layer down — `TestHealthReportsOKAndIdentifiesTheBuild`
  asserted only non-emptiness, which the linker defaults satisfy — and was replaced in 1bdf713 by
  `handlers/health_test.go::TestHealthReportsTheBuildItWasLinkedWith`, which SETS
  `version.Version`/`version.Commit` to values nothing else produces and asserts equality (p1-qa
  read the body and ran it: PASS, exit 0). p1-platform's first repair compared against the
  unset globals and was still satisfied by a hardcoded `"dev"`; only mutation caught that. It
  still does not prove build identity — nothing under `go test` can — and says so in its own
  comment; identity is proved by the gate's stage 4 alone.
  **Limit, stated:** stages 2 and 3 trust the binary's own log lines (`"firmware verified"`,
  `"listening"`) — stub (e) passed both by echoing them. That is inherent to a black-box gate and
  is why stage 4's identity check exists: it is the one stage that ties the process to this tree.
  The gate cannot run in CI (firmware not redistributable) and refuses there rather than passing.)

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
