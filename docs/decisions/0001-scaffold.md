# ADR 0001: The scaffold — Go 1.22, one module, no external dependencies

**Status:** accepted
**Date:** 2026-09-16

## Context

`plan/phase-1-foundation.md` TASK-1.1 calls for the module layout, `ctl.sh`, Dockerfile,
docker-compose and Makefile. Two constraints from outside the plan shaped the result.

**The toolchain is fixed at Go 1.22.2.** `CLAUDE.md` pins "Go 1.22+", the host has 1.22.2, and this
machine cannot fetch a newer one — building against a `go 1.25` directive fails with
`download go1.25 for linux/amd64: toolchain not available`. That is not a preference; it is the
ceiling everything here builds under.

**The house scaffold's default dependency set does not fit under that ceiling.** `chi/v5@latest`
resolves to v5.3.2, which requires Go ≥ 1.23. The same two-major support policy governs the OTel
SDK, testcontainers-go and sentry-go. Adopting them would mean pinning each to a walked-back version
and re-walking them on every update.

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
  repeated by every future agent who runs `go get`.
- **Upgrade the host's Go.** Not available from inside this environment; `dl.google.com` is
  unreachable. Worth doing outside it, and it is the trigger above.
- **Root layout with a single binary.** Rejected: TASK-1.10's `oraclecmp` is a second binary that
  must share the checkpoint encoding, and two modules would mean two definitions of it.
