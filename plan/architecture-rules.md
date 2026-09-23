# Architecture rules

This catalogue records which architecture decisions have executable checks. **Arming** says
whether a violation could be committed today; **state** says whether a checker and a proof that it
can fail have been installed. `pending` and `deferred` rules enforce nothing. A self-arming rule
must discover new subjects automatically and refuse an empty subject set when its first subject is
due to exist.

## Rules

| Rule | Invariant | Source decision | Class | Arming | State |
|---|---|---|---|---|---|
| `ARCH-DEV-1` | Developer surfaces bind only to loopback | Deployment and access | STATIC | ARMED | **enforced** |
| `ARCH-SNAP-1` | Every device state field is captured by Snapshot and Restore | The core patterns | STATIC | SELF-ARMING | **enforced** |
| `ARCH-DET-1` | CPU and devices use instruction time, with no wall clock or goroutine in the instruction loop | The core patterns | STATIC | SELF-ARMING | **enforced** |
| `ARCH-LAYER-1` | Core and device packages do not import outward transport packages | Application structure | STATIC | SELF-ARMING | **enforced** |
| `ARCH-FW-1` | Firmware bytes are absent from git and built images | Deployment and access | IMAGE | ARMED | **enforced** |
| `ARCH-PLATFORM-1` | The shared platform layer does not import domain packages | The shared / platform layer | STATIC | ARMED | **enforced** |
| `ARCH-MODULE-1` | The Go module declares only the approved WebSocket module and version | Phase 5 transport decision | STATIC | ARMED | **enforced** |
| `ARCH-PRESS-1` | Firmware probes press the handset through the one helper that lets the paint finish | Non-Obvious Domain Patterns | STATIC | ARMED | **enforced** |

## Rule definitions

### `ARCH-DEV-1`

The public demo must not expose developer endpoints. The current HTTP server can mount pprof; its
loader refuses a non-loopback address unless the container-only override is explicit, and Compose
must publish that container port on host loopback. Removing the refusal, setting the override
outside Compose, making it interpolated, or publishing the port publicly are separate violations.
The GDB listener is exercised directly: loopback binds succeed and empty or all-interface binds
are refused. A separate probe weakens that guard and must make the rule fail. Any future
instrument surface must use a validated bind path or extend this rule when it is built.

### `ARCH-SNAP-1`

Every device must capture its mutable state and restore it. A device with a state field omitted
from Snapshot is the deliberate violation. The checker must exercise that field, not merely
inspect whether Snapshot exists.

### `ARCH-DET-1`

CPU and device work advances on instruction time. A wall-clock call or a goroutine in the
instruction loop is a violation; separate probes must demonstrate each detector.

### `ARCH-LAYER-1`

Core and device packages must not import the outward HTTP or browser layer. Adding such an import
is the deliberate violation.

### `ARCH-FW-1`

Pace firmware must not enter the repository or a distributable image. Adding a flash-image file to
git, or copying it into a built image, is the deliberate violation. These are separate subjects and
need separate probes.

### `ARCH-PLATFORM-1`

The shared platform layer must remain independent of CPU, device, broadcast and transport domain
packages. Importing one of them from `internal/platform` is the deliberate violation.

### `ARCH-MODULE-1`

The phase 5 WebSocket transport requires exactly `github.com/coder/websocket@v1.8.15`, approved in
ADR 0001. A different module or version, a replacement, or removal of that requirement violates
the decision. The checker parses `go.mod` with Go's modfile editor; separate probes prove each
rejection and the missing-file refusal.

### `ARCH-PRESS-1`

Every firmware probe that reads a screen after a key press must go through
`pressAndLetItFinish`, and only `internal/multiplex/firmwaretests/rununtil_test.go` may contain a
settle loop. A probe that counts identical screen samples itself is the deliberate violation.

The settle detector calls a screen finished after four identical samples 65,536 instructions apart.
A menu painting under a busy carousel holds a half-drawn frame still for longer than that, so a
loop without the paint tail returns a screen that has not finished drawing; the next press lands in
a painting menu and is swallowed, and the probe reports that the box drew nothing. On 2026-09-23
broadcasting the `0xB2` guide-row descriptor gave every menu more to paint and thirty-one probes
stopped reaching the TV GUIDE tab in one run, all reporting `drew 00000000` for screens that were
drawing perfectly well. Forty-eight of them carried their own copy of the loop, and the rule
against it had been written in prose twice by then.

The checker is structural: a screen read with a counter incremented and compared against the
stability threshold inside the same window is a settle loop, whatever the counter is called. It
asks no semantic question, needs no build, and its coverage detector refuses a corpus that has lost
the helper.

## Decision ledger

Every decision in `plan/module-decisions.md` and `docs/decisions/0001-scaffold.md` is bound here.
Where a decision has several parts, a rule holds the mechanically checkable part and the ledger
names what requires tests or review. A claim that a check exists here means it has been run; future
work is named as future work.

| Recorded decision | Binding |
|---|---|
| One runtime Go binary and inward package boundaries | `ARCH-LAYER-1` checks import direction. The choice to split a process is a design judgement reviewed when proposed. The `oraclecmp` developer tool is a separate executable, not a second emulator service. |
| `internal/httpx` and `internal/app` own transport and wiring; snapshot framing lives in `internal/platform/snapcodec` and bus policy in `internal/bus` | `ARCH-LAYER-1` and `ARCH-PLATFORM-1` hold the import boundaries. No rule pins those exact names: a coordinated rename is allowed when the design remains intact. |
| One bus `Device` interface, every device serialisable from creation | `ARCH-SNAP-1` discovers hardware-shaped types, requires the methods and a live field-complete contract test. The bus's address-decoding behaviour is held by its own tests. |
| Errors are values; a bad guest instruction halts visibly | None, because the CPU is not built yet and a static return-type check cannot prove visible halt behaviour. Phase 2 integration tests must exercise a bad instruction. |
| No goroutine in the instruction loop; `icount` drives device time | `ARCH-DET-1` scans the existing clock and automatically enrols future CPU and device files. Replay determinism remains an integration-level requirement in phase 4. |
| Shared `hexfmt`, `statehash`, `snapcodec`, `instrument` and `clock` | `ARCH-PLATFORM-1` holds the shared layer's dependency direction. Each primitive's behaviour is held by its own tests; a rule forbidding a second implementation would need semantic equivalence it cannot establish from names. |
| No database, auth, tenancy, API envelope, cache or queue | None, because these are scope decisions. A new requirement may justify one; blanket banned-package checks would reject a deliberate change rather than detect accidental drift. |
| One static page and TypeScript module, no SPA framework | None yet, because the browser client is a phase 5 deliverable. Its dependency and bundle review will make the choice visible when code exists. |
| Structured JSON logs with `icount` | None as a conformance rule, because `internal/logging/logging_test.go` already exercises emitted JSON records and their instruction counts. Whether events are diagnostically useful is an operator judgement. |
| No Sentry | None, because this is an observability scope decision; the boot and oracle gates are the chosen error-detection mechanism. |
| Public TLS via Traefik | None yet, because deployment is a phase 5 deliverable; its production gate must exercise the served TLS route. A source string cannot prove a route is publicly reachable. |
| Developer surfaces restricted to loopback | `ARCH-DEV-1` exercises the HTTP loader, GDB listener and Compose mapping. Future instrument listeners must use a validated path or extend this rule before exposure. |
| Firmware stays outside source and images | `ARCH-FW-1` scans tracked source and the exported runtime image, with separate probes. |
| Explicit dependency set from ADR 0001 | `ARCH-MODULE-1` checks the parsed module declaration against the approved WebSocket module and version. A new dependency or version requires a deliberate decision and rule edit. |
| Go toolchain version in ADR 0001 | None as a conformance rule yet: the recorded host ceiling was disproved, and `gort-4sx.15` owns the version update and vulnerability recheck before public deployment. |

## Not mechanisable

- The single-binary choice is visible in the build outputs, but whether a new process boundary is
  *justified* is a design judgement reviewed when proposed.
- Errors being useful to a viewer requires boot and interaction tests; a static assertion that a
  function returns `error` would not show whether a bad guest instruction halts visibly.
- The choice to omit a database, auth, a cache, a queue and an SPA framework is revisited through
  architecture review if the product gains a requirement for them. A forbidden-package list would
  misclassify a deliberate new requirement as a violation.
- Structured logs carrying the relevant instruction count are checked by logging tests; whether
  the chosen events are diagnostically useful requires an operator review.

## Considered and rejected

An empty-corpus pass and a checker with no negative probe are rejected: both make a green result
possible without observing the claimed subject. A source-only scan cannot establish that firmware
is absent from a built image.

## Tooling

The harness uses Python's standard-library TOML parser and a tracked-file corpus. Each enforced
rule records its actual tool and rationale in `conformance/rules.toml`: Go's AST parser for source
syntax, `go list` for import edges, the real config loader and Docker Compose for binding, Go's
JSON test stream for device contracts, and a built/exported Docker image for firmware containment.
No architecture-only package dependency was needed.

Rule changes are recorded by the staged-digest commit guard in
`scripts/conformance/rule-guard.sh`. It is an audit trail and speed bump. Forge branch protection
and CODEOWNERS review are not configured here, so this local guard is not an independent approval
boundary.
