# Security Review Report: Phase 5

Date: 2026-09-17
Scope: the public browser deployment at `https://goretrotv.demosrv.uk`, the WebSocket handset/framebuffer transport, the production Docker/Traefik overlay, static asset routing, pprof/developer-surface controls, private firmware/snapshot handling, and dependency/vulnerability state after Phase 5.

## Summary

No Critical or High findings were found. The public route serves the real browser emulator over TLS, but the developer surfaces remain unavailable through the public HTTP layer and direct TCP probes. The private firmware and post-acquisition snapshot are mounted read-only at runtime and are not present in tracked source or the built runtime image. A stale public asset finding discovered after the first audit pass was fixed and retested.

| Severity | Count |
|---|---:|
| Critical | 0 |
| High | 0 |
| Medium (fixed) | 1 |
| Low | 0 |
| Informational / accepted | 1 |

## Fixed Finding

1. **[A05] A public cache served stale stylesheet bytes after a deploy — Medium**
   - Evidence: the public Playwright asset-byte check found `/styles.css` from an older build while the live WebSocket served the new build. This could leave visitors with a mismatched page and handset layout.
   - Fix: the static route now sends `Cache-Control: no-store`; the HTML names the stylesheet and JavaScript modules with the running build version. The public Playwright check follows those versioned URLs and compares served bytes to the build artifacts.
   - Verification: `go test -race ./internal/app`, TypeScript typecheck, CSS lint, and `npm run test:e2e:public` passed against the deployed route after the fix (`gort-4sx.20`).

## Informational / Accepted

1. **[A05] `/health` exposes build identity by design**
   - Evidence: `https://goretrotv.demosrv.uk/health` returned `status=ok`, `version=ec44d06`, `commit=ec44d06`, and a build timestamp after the public rebuild.
   - Rationale: the project deliberately requires public health to name the running build so deployment and boot gates can prove they tested the intended binary. It does not expose secrets, local paths, firmware hashes, environment variables, or internal diagnostics.
   - Action: none for Phase 5. Revisit only if the demo host later carries account data, private viewer state, or admin surfaces.

## Passed Checks

- **A01 / access control and public surface:** the product has no accounts, tenant data, or state-changing HTTP API. Browser input is constrained to same-origin WebSocket handshakes and versioned key messages; server-side input is accepted only while the machine phase is `ready`, with a 1 KiB read limit and a 64-message key queue. Public requests for `/debug/pprof/`, `/debug/pprof/profile`, `/debug/pprof/cmdline`, `/instruments`, `/metrics`, `/trace`, `/.env`, `/.git/config`, `/swagger`, `/openapi.json`, `/docs`, and `/api/schema` returned 404.
- **A02 / cryptography:** TLS terminates at the existing Traefik route. Live headers included HSTS (`max-age=31536000`), CSP, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, and `Referrer-Policy: no-referrer`.
- **A03 / injection:** no database, shell execution, URL fetching, templates, or user-authored HTML exist in the public request path. WebSocket JSON decoding uses `DisallowUnknownFields`, rejects trailing values, and allowlists the exact handset raw codes.
- **A04 / insecure design:** the demo is intentionally shared and unauthenticated. Ready state is derived from verified machine state, and the browser cannot push keys before the server advertises `ready`.
- **A05 / misconfiguration:** `GORETROTV_ENABLE_PPROF=true` is refused outside `GORETROTV_ENV=development`; the public compose overlay forces `GORETROTV_ENABLE_PPROF=false`, removes host port publication, and routes only through Traefik. Public TCP probes to `goretrotv.demosrv.uk:23457` and `:8099` timed out; direct origin probes to `192.168.1.12:23457` and `:8099` were refused.
- **A06 / vulnerable components:** `./ctl.sh vuln` reported zero called Go vulnerabilities with Go 1.27.1. `npm audit --omit=dev --audit-level=moderate` and `npm audit --audit-level=moderate` both reported zero vulnerabilities. `./ctl.sh conformance` passed all 7 rules and 26 negative probes, including `ARCH-DEV-1` and `ARCH-FW-1`.
- **A08 / integrity:** the runtime image is built from pinned Go, Node and distroless image digests. `ARCH-FW-1` proved tracked source and the built runtime image contain no firmware image. Firmware and the private snapshot are runtime mounts only; the snapshot is mounted read-only.
- **A09 / logging:** the public request path logs request metadata and machine events; no credentials or account data exist. Startup logs list firmware byte counts and manifest path but do not print firmware contents.
- **A10 / SSRF:** the public app performs no outbound HTTP fetch from user input.

## Evidence Commands

- `./ctl.sh up-public` rebuilt and started the public image from commit `ec44d06`.
- `./ctl.sh health-public` returned `version=ec44d06`, `commit=ec44d06`.
- `npm run test:e2e:public` passed: public HTTPS asset bytes, developer-route 404s, `wss://goretrotv.demosrv.uk/ws`, exact initial framebuffer hash `A6A21DC5`, Sky raw key `0x7D`, and exact Box Office menu hash `FE8D1CCC`.
- `curl -fsSI https://goretrotv.demosrv.uk/` showed CSP, HSTS, frame deny, nosniff and no-referrer headers.
- `./ctl.sh vuln`, both npm audit commands, `./ctl.sh conformance`, `./ctl.sh test`, `./ctl.sh lint`, and `shellcheck ctl.sh` passed.
- Manual TCP probes: `goretrotv.demosrv.uk:23457` and `:8099` timed out; `192.168.1.12:23457` and `:8099` refused.

## Limitations

This audit covers the public Phase 5 demo surface. It does not audit future Phase 6 broadcast section parsing, listings ingestion, video playback, or any later interactive/transactional features.
