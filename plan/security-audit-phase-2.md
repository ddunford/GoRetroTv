# Phase 2 security audit

Date: 2026-09-16  
Scope: the MIPS32/MIPS16 CPU, COP0 and interrupts, CPU checkpoints and snapshots, flash command sequencer, and the local `firmwaretrace` command. The HTTP listener and deployment configuration were checked for reachability, but Phase 2 adds no HTTP route.

## Findings

| Severity | Finding | Disposition |
|---|---|---|
| Critical | None | — |
| High | None | — |
| Medium | The host runs Go 1.22.2. `govulncheck` reports 32 accepted called standard-library advisories. Three newly observed call paths are confined to the existing local/CI device census checker: Go source parsing can exhaust the parser stack on crafted deeply nested source (`GO-2024-3105`, `GO-2024-3107`), and `exec.LookPath` is called with the fixed executable name `git`, which does not satisfy the `""`, `"."`, or `".."` precondition of `GO-2025-3956`. None is reachable from the emulator HTTP process or the new trace command. | Recorded in `tools/govulncheck-baseline.txt`; `gort-4sx.15` requires a supported toolchain before publication. |
| Low | None | — |

## Checks and limits

- **Access and exposure (A01, A05):** Phase 2 creates no route. The app registers `GET /health` and optionally pprof; pprof is off by default. Native binding defaults to `127.0.0.1`, and Compose publishes only to host loopback. `ARCH-DEV-1` and its negative probes passed. The public hostname returned 404 for `/` and `/health` on 2026-09-16, so there is no deployed GoRetroTV instance to audit through that hostname.
- **Injection and input handling (A03, A08):** The new CPU and flash paths execute guest instructions against the local bus; they do not run a shell, query a database, fetch URLs, or deserialize object graphs. Snapshot decoding uses the versioned `snapcodec` and validates lengths and control-flow alignment before replacing CPU state. The trace command's firmware directory is an operator CLI argument and firmware loading verifies images against its manifest.
- **Data and dependency integrity (A02, A06, A08):** Firmware is gitignored, excluded from the runtime image, and mounted read-only by Compose. The project declares no external Go modules. `./ctl.sh vuln` passed against the reviewed baseline: 32 accepted findings, zero new. The old Go toolchain remains a release blocker.
- **Logging and availability (A09, A10):** No Phase 2 network client, session, user account, tenant, or user-controlled URL exists. CPU and flash paths run synchronously; bad guest instructions halt with an error. `firmwaretrace` can produce large local output when an operator requests a long run or per-instruction tracing; it is not attached to an HTTP endpoint.
- **Secrets:** `.env.example` contains documented sample settings, Compose has no embedded credential, `.githooks/pre-commit` is executable and armed, and the tracked-file name check found no committed private-key or live env file. No secret file contents were read.

Verification: `./ctl.sh test`, `./ctl.sh lint`, `./ctl.sh gate`, `./ctl.sh conformance`, and `./ctl.sh vuln` passed. The local startup gate verified all three firmware images, `/health`, build version, and graceful stop. The full bootloader handoff acceptance check is still open as `gort-dji.11` because the CPU reaches an unmodelled video RAM port at instruction 3,204,424; see `docs/reference/cpu-oracle-measurement.md`.
