# ADR 0001: The scaffold — Go 1.22, one module, no external dependencies

**Status:** accepted
**Date:** 2026-09-16

## Context

`plan/phase-1-foundation.md` TASK-1.1 calls for the module layout, `ctl.sh`, Dockerfile,
docker-compose and Makefile. Two constraints from outside the plan shaped the result.

**The toolchain was believed to be fixed at Go 1.22.2, and that was wrong.** `CLAUDE.md` pins
"Go 1.22+" and the host's own Go is 1.22.2 (Ubuntu's `golang-1.22-go`). The scaffold concluded a
newer one could not be fetched, because a test build against a `go 1.25` directive failed with
`download go1.25 for linux/amd64: toolchain not available`.

**That failure was the test's fault, not the host's.** `go1.25` is not a release — toolchain names
carry a patch component (`go1.25.0`, `go1.27.1`). Since Go 1.21 toolchains are ordinary modules
served by `proxy.golang.org`, which this machine reaches, so `GOTOOLCHAIN=go1.27.1 go version`
downloads and runs fine. Measured afterwards: the whole tree builds, vets and passes `-race` under
1.27.1 with `go.mod` untouched, and `govulncheck` goes from **29 called standard-library
vulnerabilities to zero**. The ceiling was never real.

So the Go version is a live choice, tracked as `gort-4sx.15`, and **the decision below stands on
its second justification only** — which was always the stronger one.

**The house scaffold's default dependency set buys nothing this project's conventions want.**
`CLAUDE.md` already specifies "Go's own test framework", `plan/module-decisions.md` already records
"No Sentry" and no database, queue or cache. So testify, the OTel SDK, testcontainers-go and
sentry-go were rejected on their merits rather than on the toolchain, and that reasoning is
unaffected by the correction above. `chi` is the one genuine casualty: it was declined partly
because `chi/v5@latest` needs Go ≥ 1.23, and on a current toolchain that objection disappears —
leaving a free choice between it and Go 1.22's own method-and-wildcard `ServeMux`, which has so far
been sufficient.

## Decision

**One module at the repository root** (`github.com/ddunford/goretrotv`, lowercased from the git
remote), with binaries under `cmd/` sharing `internal/`. The `cmd/` shape rather than the root shape
because the phase already names a second binary: `oraclecmp` (TASK-1.10) shares the checkpoint
format with the emulator and must not re-declare it.

**No external dependencies.** The standard library covers every recorded requirement:

| Scaffold default | Replaced by | Why that is sufficient here |
|---|---|---|
| `go-chi/chi` | `net/http.ServeMux` | Go 1.22's ServeMux has method-and-wildcard patterns. The recorded HTTP surface is "a framebuffer stream, a key channel and a handful of developer endpoints" (`plan/module-decisions.md` → Not applicable), with no envelope and no pagination to standardise. |
| `caarlos0/env` + `godotenv` | `os.LookupEnv` + an explicit loader | TASK-1.15 requires asserting that the example env file covers every variable **by walking the config struct, not by eye** — that test needs `reflect` over our own tags either way. |
| `stretchr/testify` | `testing` | `CLAUDE.md` → Conventions already says "Go's own test framework; table-driven where the shape suits". |
| `google/uuid` | — | The correlation key in this project is `icount`, not a request id. There are no cross-service requests to correlate: it is one process. |
| OTel SDK, `sentry-go` | `log/slog` | `plan/module-decisions.md` → Observability records structured JSON logging with an `icount` field, and **"No Sentry"** explicitly. |
| `testcontainers-go` | — | No database, no queue, no cache — all four recorded as not applicable. There is no container to stand up for a test. |

`GOTOOLCHAIN=local` is exported by the Makefile and set in the Dockerfile so an accidental `go`
directive above 1.22 fails at once rather than hanging on a download that cannot succeed.

## Consequences

- A clone builds with nothing but a Go toolchain: `go.sum` does not exist, `go mod download` has
  nothing to fetch, and CI needs no module cache. Supply-chain surface for the emulator core is the
  standard library alone.
- `govulncheck` still matters — it reports standard-library vulnerabilities, which is now the only
  category we can have. `./ctl.sh vuln` runs it.
- **This is a floor, not a vow.** A later phase that genuinely needs a dependency should take one:
  the WebSocket transport (phase 5) and ffmpeg bindings (Video) are the expected candidates. What
  this decision rules out is acquiring dependencies *by default*, before a requirement names them.
- Revisit when the host can run a current Go: the pinning problem disappears and `chi` becomes a
  free choice again rather than a cost.

## Alternatives rejected

- **Walk every dependency back to a Go 1.22-compatible version.** Rejected: it buys router sugar and
  assertion helpers this project's own conventions say it does not want, and the walk-back has to be
  repeated by every future agent who runs `go get`. Moot now in any case — see below.
- **Upgrade the host's Go.** Recorded here as "not available from inside this environment, because
  `dl.google.com` is unreachable", and that was **wrong on both counts**. Toolchains come from the
  module proxy, not `dl.google.com`, and the proxy is reachable; the original test asked for a
  version string that does not exist. No upgrade of the *host's* Go is needed at all — a `go`
  directive plus dropping `GOTOOLCHAIN=local` is the whole change. Tracked as `gort-4sx.15`, a
  precondition of publishing, because it takes the tree from 29 called standard-library
  vulnerabilities to zero.

  **The lesson worth keeping is the shape of the mistake, not the fact of it.** A single failed
  command was read as a property of the environment and written into an architecture record, where
  it then justified a decision and was repeated to the user twice. The failing command was never
  re-examined — and its error message, `download go1.25 …: toolchain not available`, names a version
  that was never going to exist. A constraint discovered once should be re-tested before it becomes
  a premise, especially when it is load-bearing.
- **Root layout with a single binary.** Rejected: TASK-1.10's `oraclecmp` is a second binary that
  must share the checkpoint encoding, and two modules would mean two definitions of it.
