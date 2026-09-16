# Security audit — Phase 1 (Foundation)

**Date:** 2026-09-16 · **Reviewer:** p1-security (Fable 5.1, a different model from the builders; read-only on product code) · **Tree audited:** `23382d8` plus the uncommitted `cmd/oraclecmp` / `statehash/compare*` edits present in the working tree · **Bead:** `gort-6ar.13`

**Verdict: 0 Critical · 0 High · 2 Medium · 2 Low · 5 Informational.** Nothing blocks the phase. The loopback negative is **proved with one stated limit** (below). Both Mediums are *missing controls*, not active exposures, and each has a small, specific fix.

---

## 1. The loopback negative

The claim under audit: *developer surfaces bind to localhost only, and nothing else in the tree widens the bind.*

### What was walked

| Path by which the listen address is set or could be | Finding |
|---|---|
| `internal/config/config.go:74` — `Defaults.HTTPAddr = "127.0.0.1:8099"` | Loopback. Pinned by `TestTheDefaultListenerBindsLoopback` (`config_test.go:263`). |
| `internal/config/config.go:55` — `GORETROTV_HTTP_ADDR` env override | **Accepts any value, no validation** — see M-1. `.env.example:53` says "do not widen it on a host"; nothing enforces that sentence. |
| `cmd/goretrotv/main.go:78` — the only consumer of `cfg.HTTPAddr` | Passed verbatim to `httpx.NewServer`. |
| `internal/httpx/server.go:47` — `ListenAndServe` | **The only listener construction in the non-test tree.** `grep -rn 'net\.Listen\|ListenAndServe\|http\.Serve(\|httptest\.NewServer' cmd internal` returns exactly this one site (the test tree uses `httptest.NewRecorder` only — no sockets). |
| `internal/app/app.go:27-33` — pprof | `MountPprof` registers on the **same mux**; there is no second `http.Server`. Enabling pprof cannot widen the bind. Verified live (§1.2). |
| `docker-compose.yml:25` — `GORETROTV_HTTP_ADDR: ":8099"` | A **literal**, not `${...}`: the operator's `.env` cannot change what the container binds. Inside the container it is all-interfaces by necessity. |
| `docker-compose.yml:28` — `"127.0.0.1:${GORETROTV_PORT:-8099}:8099"` | The host IP is a literal; only the port substitutes. A `GORETROTV_PORT` of `0.0.0.0:8099` or `8099:8099` produces a malformed mapping that compose rejects rather than a wider one. |
| Host firewall | Docker 29.6.1, `iptables -P FORWARD DROP`, `DOCKER-USER` present — the modern daemon's "unpublished/loopback-published ports are unreachable from other hosts" posture is in force. Not probed from a second host. |
| `Dockerfile:40` — `EXPOSE 8099` | Metadata only; `docker run -P` would publish to `0.0.0.0`. `ctl.sh` never does this. Informational (I-4). |
| `tools/boot-gate.sh:124` | Forces `127.0.0.1:$port`. |
| `.github/workflows/ci.yml` | Never starts the server. |
| `ctl.sh run` (native) | Sources `.env` with `set -a`, so a host `.env` reaches the loader — this is the path M-1 describes. |
| The shared `traefik-demosrv` container (`0.0.0.0:80/443`) | Not attached to this stack; `docker-compose.yml` carries no Traefik labels or external network. The service is currently **not publicly reachable at all** — publication is phase 5 (`gort-4sx.6`). |

### 1.2 Live verification (audit-owned build in the scratchpad, real firmware, random loopback port)

| Case | `ss -ltnp` bound | `/health` | `/debug/pprof/` | `/debug/pprof/cmdline` | malformed request | SIGTERM |
|---|---|---|---|---|---|---|
| `GORETROTV_HTTP_ADDR=127.0.0.1:P`, pprof off | `127.0.0.1:P` | 200 | 404 | 404 | 400, process alive | exit 0, `"msg":"stopped"` |
| same, `GORETROTV_ENABLE_PPROF=true` | `127.0.0.1:P` (unchanged) | 200 | 200 | 200 | 400 | exit 0 |
| `GORETROTV_HTTP_ADDR=:P`, pprof off | **`*:P`** | 200 | 404 | 404 | 400 | exit 0 |

Zero non-JSON lines reached stderr in any case.

### 1.3 Verdict

**Proved, with one stated limit.** No code path constructs a listener from anything but the audited config value, the default is loopback and is tested, enabling pprof does not widen it, and the container's only widening is a literal whose exposure is decided by a literal `127.0.0.1` port mapping. **The limit:** the control is a *default plus prose*, not an invariant of the loader — the third row above shows the env override widening the bind with no refusal, and the compose file legitimately exercises that same override. That is M-1, and it is what the phase-1a rule `ARCH-DEV-1` (`gort-87m.2`) should turn into a check that can fail.

Note for the record: in phase 1 the "developer surfaces" are `/health` and gated pprof. The gdb stub and instrument endpoints do not exist yet (phase 4); this audit says nothing about them.

---

## 2. Findings

### Medium

**M-1 · [A05] The listen address accepts any interface with no guard; the loopback decision is enforced by a default and a comment.**
- `internal/config/config.go:55` (field), `config.go:184-213` (`assign` — string fields are stored verbatim), `cmd/goretrotv/main.go:78`.
- Path to harm: an operator `.env` on the host with `GORETROTV_HTTP_ADDR=:8099` (or `0.0.0.0:8099`) is honoured silently — demonstrated in §1.2 row 3. With `GORETROTV_ENABLE_PPROF=true` also set, pprof (heap/goroutine dumps, `cmdline`, CPU profiles — a CPU-burn DoS lever) is on a public interface. In phase 4 the same override exposes the gdb stub, which is arbitrary read/write of the emulated machine. Two operator mistakes are needed today; one will do once phase 4 lands, because the second surface arrives with no bind of its own.
- Why Medium not High: no active exposure — the stack is not published, the default is right, and the one shipped widening (compose) is fenced by the port mapping.
- **Fix (recommended, small):** make widening a *named* act. Add `AllowNonLoopbackBind bool \`env:"GORETROTV_BIND_ALL_INTERFACES"\`` (default `false`) and in `config.Load`, after `assign`, parse `HTTPAddr` with `net.SplitHostPort`; if the host is empty, `0.0.0.0`, `::`, or resolves to a non-loopback IP (`net.ParseIP(host).IsLoopback()` false) and the flag is not set, return `fmt.Errorf("GORETROTV_HTTP_ADDR=%q binds a non-loopback interface; developer surfaces bind to localhost (CLAUDE.md). Set GORETROTV_BIND_ALL_INTERFACES=true only inside a container whose port mapping is loopback-only")`. Set the flag as a literal in `docker-compose.yml` beside `GORETROTV_HTTP_ADDR: ":8099"`, document it in `.env.example` (the completeness test will demand this), and add a test that `:8099` without the flag refuses and with it loads. `ARCH-DEV-1` then becomes greppable: the only setter of that flag in the tree is `docker-compose.yml`.
- Owner: whoever takes `gort-87m.2`; this should be folded into it rather than filed twice.

**M-2 · [A08/robustness] A forged member count in an aggregate snapshot kills the process with an unrecoverable `fatal error: out of memory`.**
- `internal/platform/snapcodec/set.go:136` — `names := make([]string, 0, count)` where `count` is the blob's own `uint32` claim, allocated **before** any check that the payload could hold that many members. `Reader.Words()` (`snapcodec.go:362`) gets this right (checks `remaining()` first); `OpenSet` does not.
- Proof: a well-formed one-member container from the real `SetWriter` with the count field overwritten to `0xFFFFFFFF`, fed to `OpenSet` via a `go test -overlay` probe (nothing written into the tree) under `ulimit -v 8000000`: `runtime: out of memory: cannot allocate 68719476736-byte block … fatal error: out of memory`, frame `snapcodec.OpenSet set.go:136`. This is a runtime `throw`, not a panic — **no `recover()` anywhere can catch it**, so the middleware `Recover` and the "errors are values, never take the process down" rule are both bypassed. On a host with generous overcommit the same input instead commits 64 GB of address space and reads as a hang.
- `bus.decodeSnapshot` (`internal/bus/snapshot.go:179`) does **not** have this bug — it uses an unsized map.
- Why Medium: today a snapshot is an operator-supplied file (SPEC FR-8: "to a file") with no network ingress, so the input is trusted-ish; but this is the shared primitive every phase-4 restore path and every replay/oracle comparison inherits, the failure is total, and the fix is one line.
- **Fix:** drop the capacity hint (`names := make([]string, 0)` or `var names []string`), or guard as `Words` does: each member costs at least 8 bytes (two length prefixes), so `if uint64(count)*8 > uint64(r.remaining()) { return nil, fmt.Errorf("snapcodec: %s: header claims %d members, %d payload bytes remain", name, count, r.remaining()) }` before allocating. Add the forged-count case to `set_test.go` alongside `TestAHeaderCountThatDisagreesWithItsContentsIsRefused`.
- Owner: `p1-platform`.

### Low

**L-1 · [A09] `http.Server.ErrorLog` is nil, so net/http's own errors bypass the structured logger.**
- `internal/httpx/server.go:25-33`. `net/http` writes accept errors, "panic serving" (for panics outside our `Recover`), and — relevant to baseline entry GO-2026-5039 — unescaped client-supplied header bytes to `log.Default()` → raw stderr, interleaved with the JSON stream that every tool in this project queries. Zero such lines were observed in the live runs (§1.2), so this is hygiene, not an active leak.
- **Fix:** `ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError)` in `NewServer`.

**L-2 · [A05] Container hardening left at Docker defaults.**
- `docker-compose.yml` — no `read_only: true`, `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]`, or memory limit. Distroless static + `nonroot` + `CGO_ENABLED=0` already does most of the work; these close what remains. Note `read_only` will need a writable volume when phase 4 writes snapshots/traces.

### Informational (no plausible path to harm; recorded so nobody re-derives them)

- **I-1** `/health` discloses version + commit unauthenticated. Deliberate (the boot gate asserts identity on it) and harmless for an open-source emulator; phase 5 publishes it as-is.
- **I-2** `middleware.Recover` also recovers `http.ErrAbortHandler` and logs a stack for what is a client abort. Re-panic that sentinel (`if rec == http.ErrAbortHandler { panic(rec) }`). Noise, not security.
- **I-3** Compose defaults `GORETROTV_LOG_LEVEL` to `debug`. Nothing sensitive is logged at any level today.
- **I-4** `EXPOSE 8099` in the Dockerfile is a hint, not a publish; `docker run -P` would publish to all interfaces. `ctl.sh` is the only sanctioned runner.
- **I-5** pprof, when enabled, has no gate of its own — reachability *is* the bind. That is the recorded design; it is why M-1 matters more than it looks.

---

## 3. Supply chain — the 29 baseline vulnerabilities, triaged by reachability from untrusted input

`govulncheck v1.1.4` re-run independently: 168 OSV records scanned, 29 called at symbol level, **byte-identical to `tools/govulncheck-baseline.txt`** (`diff` empty). Toolchain `go1.22.2`.

**Is the baseline approach defensible?** Yes, with two caveats. The gate asks the one question a committer can act on ("anything NEW?"), it asserts the scan produced records before believing an empty result, and it reads the verdict from the comparison rather than from govulncheck's exit code (which is 0 in JSON mode even with findings — correctly noted in `vulncheck.sh:74`). Caveats: (a) `--write-baseline` rewrites the ID list wholesale with no per-entry reason, so the file cannot say *why* each is accepted — the triage below should live in it; (b) "called" is a static call-graph fact, not a reachability fact, and nobody had separated the two. That separation:

| Bucket | IDs | Verdict |
|---|---|---|
| **Executes on attacker-controlled bytes at the current listener** | GO-2025-3563 (chunked-body smuggling, fixed 1.23.8) · GO-2026-6089 (h2c preface read not under `ReadHeaderTimeout`, fixed 1.25.13) · GO-2026-5039 (raw header bytes in textproto errors, fixed 1.25.11) · GO-2026-4601 / GO-2025-4010 (IPv6 host-literal parsing in `ParseRequestURI`, fixed 1.25.8 / 1.24.8) | Reachable by any client that can reach the socket. **Impact today is nil** because the socket is loopback. In phase 5 behind Traefik: 3563 becomes a real smuggling primitive only if Traefik and Go disagree on chunk parsing (Traefik re-serialises requests, which largely defuses it — verify then); 6089 is defused because Traefik, not the client, opens the backend connection and never sends a stalled preface; 5039 lands in the 400 body / L-1's stderr; 4601/4010 need host-based routing decisions this server does not make. |
| **Reachable only with pprof enabled** | GO-2026-4341 (`ParseQuery` memory exhaustion, fixed 1.24.12) — trace: `pprof.Profile → Request.FormValue → ParseForm → ParseQuery`. Nothing else in the tree parses a query string. | Bounded by `MaxHeaderBytes` (1 MiB default) and gated by `GORETROTV_ENABLE_PPROF` + the bind. |
| **Not reachable from any input: TLS/x509/ASN.1/PEM machinery linked by `net/http` but never entered** | GO-2025-3373, -3447, -4007, -4008, -4009, -4011, -4013, -4155, -4175, GO-2026-4337, -4340, -4870, -4946, -4947, -5037, -5856, -5972, -6090 (18 IDs) | No `ListenAndServeTLS`, no `http.Transport`, no certificate or key is parsed anywhere. Static "called" via `crypto/tls.Conn` methods on the server's `net.Conn` interface. Dead weight in the baseline, correctly accepted. |
| **Not reachable: DNS / netip / OS semantics** | GO-2024-2824 (DNS resolver loop — no attacker-named lookups; `net.Listen` on a literal IP does no DNS) · GO-2024-2887 (netip Is* — internal to Listen) · GO-2026-4971 (Windows NUL) · GO-2025-3750 (`O_EXCL` semantics; `CreateTemp` reached only via multipart parsing, which nothing does) · GO-2026-4602 (`os.Root` — unused) | Accept. |

**The useful conclusion:** the four IDs fixable within the 1.22 line (2824, 2887, 3373, 3447) are all in the *unreachable* buckets. Every ID on the attacker-reachable path needs Go ≥ 1.23.8. Bumping to the newest 1.22.x buys nothing on the exposed surface; the toolchain upgrade recorded in ADR 0001 is the only fix that moves the needle, and it should be a prerequisite of phase 5's publication gate (`gort-4sx.6`) rather than a nice-to-have.

**Recommendation:** paste the four-bucket table into `tools/govulncheck-baseline.txt` as comments so the next reader inherits the triage, and have `vulncheck.sh` print the bucket of any NEW id's package (`net/http*`, `net/url`, `net/textproto` = "on the wire") so a new finding says whether it matters.

---

## 4. Secrets

- Tracked files (all 240) and the **full history of all refs** (58,383 added lines scanned; positive control string fired first) grepped for vendor key shapes (`sk-ant-`, `sk-proj-`, `ghp_`, `github_pat_`, `AKIA…`, `xox…`, Stripe live/test, PEM private-key headers) and `scheme://user:pass@` URLs: **zero hits**. Name-shaped assignments (`password|secret|api_key|token = <12+ chars>`): zero hits.
- No `.env` exists in the working tree (`./ctl.sh status` refuses for that reason). `.env.example` carries names and harmless values; `TestTheExampleFileCarriesNoSecrets` guards it in CI too.
- Hooks: `core.hooksPath=.githooks`, all five shims present and executable, beads delegation intact (`./ctl.sh hooks` green). The shared classifier `~/.claude/hooks/lib/sensitive-path.sh` is present on this machine; the frozen fallback in the hook is equivalent.
- **Pre-commit branches exercised:** the whole hook was run against a throwaway index (`GIT_INDEX_FILE`, a `--cacheinfo` entry, no file on disk) carrying one clean staged path → exit 0, no refusal — branches 1, 2, 4 and the Go hygiene block all run on the clean path. **Branch 3 (secret-bearing FILENAME) remains UNPROVEN by fixture:** every attempt to stage a name that would trip it — including via index-only entries with no file on disk — was blocked by the session's sensitive-path policy before execution. The classifier `sensitive_path_reason` was verified by reading (`.githooks/pre-commit:31-42`): `.env`/`.env.*` (minus the sample allowlist), SSH key names, `*.pem|*.key|*.p12|*.pfx|*.jks`, `credentials.*`, and `secrets/` directories. Someone outside this policy should run one refused-name commit and one allowed-name commit and record both.

## 5. Firmware

- `git log --all --name-only -- firmware/` shows only `firmware/MANIFEST.md` ever committed; no `*.bin` at any path in any ref; no blob >500 KB in the object store. `.gitignore:5` `firmware/*.bin`.
- Two independent controls keep it out of the image: `.dockerignore:24` excludes `firmware/`, **and** the Dockerfile copies only `go.mod`, `cmd/`, `internal/` (`Dockerfile:10-14`) — even with `.dockerignore` deleted, no firmware byte enters the build context's `COPY` set. Runtime gets it as `./firmware:/firmware:ro`.
- CI (`ci.yml`) has no `upload-artifact` step and no firmware; the boot gate refuses (does not skip) when firmware is absent.

## 6. Input handling (parsers)

Hand-walked; results as code, not as their comments:

- `internal/firmware/manifest.go` — `bufio.Scanner` capped at 1 MiB/line; header must match exactly; rows must have 5 cells; file column rejects `/` and `\` (`:196`), so a manifest cannot name a path outside its directory; digests must be full-length hex; duplicate rows refused; zero rows refused. **Sound.**
- `internal/firmware/loader.go` — `os.Stat` size check **before** `os.ReadFile` (`:106`), re-checked after; SHA-256 is the integrity check, MD5 is provenance only (annotated). A manifest claiming a 4 GB image cannot make the loader allocate it unless the file really is that size. **Sound.**
- `internal/platform/snapcodec/snapcodec.go` — `Open` validates every header length against `len(blob)` before slicing; `take` refuses over-reads with a sticky error; `Bytes()`/`Words()` check `remaining()` before allocating; `Done` catches under-consumption. **Sound** — the one exception is M-2 in `set.go`.
- `internal/bus/snapshot.go` — loop breaks on the sticky error; unsized map; device set checked before any device is touched; rollback on partial failure with a standing refusal if rollback fails. **Sound.**
- `internal/memory/{ram,flash}.go` — fast path is a `uint64` bound check; straddling accesses are byte-looped with a per-byte bound (`ram.go:142`, `flash.go:76`); `Restore` validates `len(contents)` and `words` against the machine before any copy. The bus guarantees `off < size ≤ 32 MiB`, so `off+i` cannot wrap. **Sound.**
- `internal/bus/bus.go:207,225` — the only two `panic` calls in the non-test tree, both on an invalid `Size` that only our own CPU can supply. Acceptable per the file's own reasoning.
- `internal/platform/statehash/stream.go` — line-capped scanner, strict header/trailer, monotonic icount check, END-count cross-check. `oraclecmp` inputs are operator-named files. **Sound.**

## 7. OWASP Top 10 — applicability

| Cat | Status |
|---|---|
| A01 Access control | N/A by recorded decision (no auth, no tenancy, shared box). The only access control is the bind — §1, M-1. |
| A02 Crypto | N/A — no passwords, sessions, tokens or TLS in-process. MD5 use is provenance-only and annotated. |
| A03 Injection | N/A — no DB, no shell-out, no templates. Access log via `slog` JSON escapes `r.URL.Path`. |
| A04 Insecure design | pprof gated; health minimal; no state-changing endpoints. Pass. |
| A05 Misconfig | **M-1, L-2, I-3, I-4.** No debug mode; distroless nonroot; `GOTOOLCHAIN=local` pinned everywhere. |
| A06 Vulnerable components | §3. Zero third-party modules (CI asserts `go list -m all` = 1). stdlib triage above. |
| A07 Auth failures | N/A. |
| A08 Integrity | Firmware manifest verification (§5,6); snapshot framing (§6, M-2); CI actions pinned by major (`checkout@v7`, `setup-go@v7`, `golangci-lint-action@v9` + linter `v2.1.6`), `permissions: contents: read`. Pass. |
| A09 Logging | Structured JSON, access log per request with remote addr; **L-1**. |
| A10 SSRF | N/A — the process makes no outbound requests. |

## 8. Claims verified by hand-walk vs. taken at face value

**Verified as code:** the pprof gate and same-mux mounting (`app.go`); single listener site (grep + live `ss`); compose literals; firmware never in history (log + blob scan); Dockerfile COPY set independent of `.dockerignore`; manifest path-separator refusal; loader size-before-read; every `snapcodec.Reader` bound (and the one it misses, M-2, proved by execution); `bus.decodeSnapshot` loop termination; `vulncheck.sh`'s 29-set reproduced independently; hook branches 1/2/4 executed; `go test ./...`, `go test -race ./...`, `go vet ./...` all green on this tree.

**Taken at face value:** Docker ≥ 28's LAN-isolation of loopback-published ports (checked only that `FORWARD` policy is `DROP` and `DOCKER-USER` exists; not probed from a second host); `traefik-demosrv` not sharing a network with this stack (the stack is not running, so there is no network to inspect); the `server.go:59` shutdown-context comment (read, agrees with the code, not exercised beyond the graceful-stop rows in §1.2).

## 9. Not looked at

The gdb stub, WebSocket transport, broadcast/DVB section parsing and the browser client (phases 4–6, not built). Traefik labels/publication (phase 5). The oracle page `reference/digibox-boot.html` and `tools/probes/*.js` as executable code (they are the measuring instrument, not the product). `internal/platform/statehash/statehash.go` hashing internals (CPU-fed, no external input). Pre-commit branch 3 by fixture (§4). External recon (Step 3c) against a deployed instance — there is none.
