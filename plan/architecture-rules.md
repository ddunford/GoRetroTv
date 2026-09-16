# Architecture rules

This catalogue records which architecture decisions have executable checks. **Arming** says
whether a violation could be committed today; **state** says whether a checker and a proof that it
can fail have been installed. `pending` and `deferred` rules enforce nothing. A self-arming rule
must discover new subjects automatically and refuse an empty subject set when its first subject is
due to exist.

## Rules

| Rule | Invariant | Source decision | Class | Arming | State |
|---|---|---|---|---|---|
| `ARCH-DEV-1` | Developer surfaces bind only to loopback | Deployment and access | STATIC | SELF-ARMING | deferred — gort-87m.2 |
| `ARCH-SNAP-1` | Every device state field is captured by Snapshot and Restore | The core patterns | STATIC | SELF-ARMING | pending — gort-87m.3 |
| `ARCH-DET-1` | CPU and devices use instruction time, with no wall clock or goroutine in the instruction loop | The core patterns | STATIC | SELF-ARMING | pending — gort-87m.4 |
| `ARCH-LAYER-1` | Core and device packages do not import outward transport packages | Application structure | STATIC | SELF-ARMING | pending — gort-87m.5 |
| `ARCH-FW-1` | Firmware bytes are absent from git and built images | Deployment and access | STATIC | ARMED | pending — gort-87m.6 |
| `ARCH-PLATFORM-1` | The shared platform layer does not import domain packages | The shared / platform layer | STATIC | ARMED | **enforced** |
| `ARCH-MODULE-1` | The Go module graph contains only the project module until a dependency is deliberately approved | Dependencies | STATIC | ARMED | pending — gort-87m.11 |

## Rule definitions

### `ARCH-DEV-1`

The public demo must not expose the gdb stub or instrument endpoints. Binding either to a public
interface is the deliberate violation. The surfaces are not built yet; this rule must enrol them
when they arrive.

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

The module graph currently contains only the project module. Adding a dependency without an
explicit architecture decision is the deliberate violation. This currently has a CI assertion;
its conformance probe is owed by `gort-87m.11`.

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

The harness uses Python's standard-library TOML parser and a tracked-file corpus. Rule checkers
will record their own tool choice in the registry when enforced. Go import rules may use the
existing Go tooling if it can report the exact import edge; adding a dependency requires a measured
gap in those tools.
