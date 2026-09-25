# Phase 4 security audit

Date: 2026-09-17
Scope: machine snapshots, input recordings, the local firmware instrument runner, the GDB remote protocol listener, current HTTP routes, container packaging and firmware privacy.

## Findings

No Critical or High finding in the Phase 4 implementation. GDB exposes guest registers and arbitrary guest memory to anyone who can connect, so its loopback restriction is the access control. `gdbstub.Listen` rejects wildcard, non-loopback IP and arbitrary hostname binds; `ServeListener` checks the actual listener address again. The runner uses that guarded listener and handles one client synchronously. Instrument collectors, section injection, snapshot paths and recorded inputs are command-line operations; none is registered on the HTTP mux. The current mux has only `/health` and optional pprof routes.

**Medium, conditional release risk (A01/A05):** `GORETROTV_ENABLE_PPROF=true` mounts profiling routes on the same HTTP mux as the public application. The current default and effective Compose configuration set it to `false`, and the Compose port is mapped to host loopback. When Phase 5 adds a public reverse-proxy route, a mistaken pprof enablement would expose process diagnostics through that route. Keep it disabled on the public listener or move it to a separately bound loopback listener. Verify this again in `gort-4sx.6` and the Phase 5 security audit `gort-4sx.8`; the present configuration does not expose it.

## Evidence

- A real `firmwaretrace` process restored the private post-acquisition snapshot and listened on `127.0.0.1:23491`. A TCP connection to the host's private LAN address on port `23491` was refused, while `127.0.0.1:23491` connected. The process exited normally after the local client disconnected. The existing real-client acceptance test also rejects `-gdb-addr 0.0.0.0:23457` and exercises register, memory, breakpoint and watchpoint operations.
- `python3 conformance/checkers/developer_bind.py` passed. It exercises the real config loader and GDB bind guard, and inspects effective Compose ports. Effective Compose has `host_ip: 127.0.0.1` and `GORETROTV_ENABLE_PPROF=false`.
- `go test -race ./internal/gdbstub ./internal/config ./internal/app ./internal/httpx/...` passed. `./ctl.sh vuln` reported zero called vulnerabilities with Go 1.27.1.
- Git tracks `firmware/MANIFEST.md` but no firmware image, snapshot or trace. `.gitignore` excludes the private data; the Dockerfile copies only `cmd` and `internal` into the build stage, and the runtime image mounts firmware read-only. Snapshots and recordings are written with mode `0600`; their decoders cap input at 128 MiB and 1 MiB respectively.
- `https://<configured-public-host>/health` returned HTTP 404 during this audit. The public URL is not yet serving this application, so the Phase 5 audit must verify the actual public route and its developer-surface isolation after deployment.

No authentication, database, upload, server-side URL fetch or browser client exists in this phase; those OWASP checks are outside this audit's reachable surface. Phase 5 introduces public WebSocket input and needs its own route, origin, message-size and rate-limit review.
