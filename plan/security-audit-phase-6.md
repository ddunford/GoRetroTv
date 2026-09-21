# Security Review Report: Phase 6

Date: 2026-09-21
Scope: the broadcast Phase 6 added — DVB/OpenTV section builders and the Sky Huffman codec
(`internal/broadcast`), operator-supplied schedule ingestion with live reload
(`internal/multiplex`), the new `GORETROTV_LISTINGS_PATH` / `GORETROTV_DICTIONARY_PATH` /
`GORETROTV_BROADCAST_DATE` settings, the `dictionaries/` and `listings/` runtime mounts, the guide
changes in `web/`, and a re-check that Phase 5's public-surface conclusions still hold at
`47e1f01`. Commits `f29b877..47e1f01`.

This is the audit Phase 5 named as its own limit: *"It does not audit future Phase 6 broadcast
section parsing, listings ingestion, video playback."*

## Summary

**Phase 6 did not widen the public attack surface.** `git diff --stat f29b877..47e1f01 --
internal/web internal/httpx internal/app internal/gdbstub` is empty: no new route, no new WebSocket
message, no new accepted field. The new on-disk data is unreachable over the public route — live
probes for `/listings`, `/listings/default.json` and `/dictionaries/skyuk.dict` all returned 404.
Hostile parsing of the schedule and the dictionary is robust: fourteen malformed-dictionary cases
produced zero panics, and every `#nosec` annotation in the new code sits behind a real range check.

The one High is not a runtime exploit. It is that **a licence-integrity control the project asserts
in four places is false, and nothing would ever have failed.**

| Severity | Count |
|---|---:|
| Critical | 0 |
| High | 1 |
| Medium | 1 |
| Low | 2 |
| Informational | 2 |

## Findings

### F-1 [A08 / A05] The "never committed" Huffman dictionary is tracked, public, and unguarded — High

Tracked as `gort-a40`.

**Evidence.** `git ls-files '*.dict'` returns two files — `tools/skyepg/skyuk.dict` and
`web/data/skyuk.dict`. Both are blob `a041217d`, 13896 bytes, and both hash to
`c05d288d9d22403f97e5caade329579c5124cb180790bf726e62cca792cb300b` — *exactly* the digest
`dictionaries/MANIFEST.md` records for the file it describes as "required, local, gitignored, never
committed and never baked into a published image". `git ls-tree -l origin/master` shows both on the
pushed branch; `gh repo view ddunford/GoRetroTv --json visibility` returns `PUBLIC`. They have been
there since the first commit `67fd4f5` and `git log --diff-filter=D -- '*.dict'` is empty, so they
have never been removed.

**Nothing was watching.** `.gitignore:39` covers `/dictionaries/*.dict` only — one directory, and
neither copy is in it. `.dockerignore` has no dictionary entry at all, so the file is inside every
build context: proven by building a probe image from this repo, which copied it out with the
manifest digest intact. `ARCH-FW-1` matches the three firmware manifest names, sizes and digests
and the U202 magic (`conformance/checkers/firmware_image.py:31-42`); it has no concept of the
dictionary. The claim is made in four places — `.gitignore:35-39`, `dictionaries/MANIFEST.md`,
`docker-compose.yml:33-34`, `internal/broadcast/huffman.go:18-20` — and not one of them is backed
by a check.

**What is *not* wrong.** The built runtime image is clean. Exporting
`goretrotvdemosrvuk-goretrotv:latest` and scanning every layer found no dictionary bytes; the image
holds only `/goretrotv`, `/oraclecmp` and the copied `/web` assets. That is luck of the Dockerfile's
explicit `COPY` list, not a control — a future `COPY . .` would bake it in silently.

**Failure scenario.** The project's stated licence posture ("treated exactly like the firmware") is
not true and has not been since inception. The file is retrievable from a public repository and its
full history.

**Fix.** Owner decision, 2026-09-21: **remove from HEAD and mechanise; do not rewrite public
history.**

1. `git rm tools/skyepg/skyuk.dict web/data/skyuk.dict`. Neither is load-bearing — the Go suite and
   the production path use `dictionaries/skyuk.dict`, `web/data/` is served by nothing, and
   `tools/skyepg/huffman.py` only defaults to its neighbour. Repoint it.
2. Widen `.gitignore` beyond the one directory and add a dictionary entry to `.dockerignore`.
3. Arm a rule that fails on a recommitted or image-baked dictionary, with probes proving it can
   fail, mirroring `ARCH-FW-1`'s two halves.
4. Correct the four assertion sites so they state what is true, including that the blob remains in
   public history.

`.dockerignore`, `conformance/`, `scripts/conformance/` and `plan/architecture-rules.md` are in
`rule-guard.sh`'s `PROTECTED` set — run `./ctl.sh authorise-rules "<why>"` first.

### F-2 [A04 / A05] The validator is narrower than the builders, so a valid schedule halts the box — Medium

Tracked as `gort-sgc`.

**Trust boundary:** operator-supplied file on a read-only production mount
(`./listings:/listings:ro`). Not internet-reachable. Availability only.

**Evidence.** `Listings.validate()` checks that a title is non-blank and stops there. Each of the
following is **accepted by `LoadListings` and then refused by a section builder**: channel 5000
(`BAT: channel 5000 exceeds twelve bits`), a non-ASCII service name (`SDT: non-ASCII printable byte
at 3`), a non-ASCII bouquet, a 300-byte service name (`SDT: descriptor too long`), a 300-byte
bouquet (`NIT: network name too long`), 86 services (`SDT: section length 1637 exceeds 1021`), a
250-character title (`internal/broadcast/titles.go:112` — 100 characters fits), and a channel whose
every title is unencodable (`records(): not even one programme fits in a title section`). The bad
edit is swapped in live: `Reload after channel=5000 edit: changed=true err=<nil>`.

**Propagation**, read from code: `multiplex.go:259/263/267/366` → `carousel.go:177/191` →
`multiplex.go:203` → `cmd/goretrotv/main.go:428-429` `return stopHalt, err` → `haltMachine`
(`main.go:352-357`) pushes phase `halted` to every browser. On reset, `transmitterFor`
(`main.go:345`) rebuilds over the same swapped guide, so it halts again at the next wave. **It does
not self-recover until the file is fixed.**

**Two claims this falsifies.** `listings.go:176` ("A MALFORMED EDIT KEEPS THE LAST GOOD SCHEDULE")
and `main.go:82` ("validated BEFORE the listener opens") are both true only to `validate()` depth.
TC-6.8 tests the JSON-malformed case, which does hold; the builder-refused case was never tested.

**A second, quieter defect.** One over-long title does *not* halt — it is dropped at
`multiplex.go:449-451` by a bare `continue` with no log (`records kept=1 (of 2)`). A viewer sees a
gap in the guide and nothing says why.

**Fix.** (a) Validate by construction — build NIT/SDT/BAT with a placeholder subscription and
`TitleSection` per quarter at load, so reload keeps the last good schedule for everything the
builders can refuse; log the dropped programme rather than dropping it silently. (b) At
`main.go:428-429`, go off air and set `transmitter = nil` instead of `stopHalt` — the choice
`transmitterFor` already makes at `main.go:298-306`. **A schedule fault is a host fault, and
reporting it as a guest halt misattributes it to the firmware**, which this project is careful
about everywhere else.

### F-3 [A04] The `services` key disconnects the viewer — Low

Tracked as `gort-p5x`.

**Trust boundary:** internet-reachable and unauthenticated, but **fail-closed and self-inflicted** —
the presser drops only their own connection.

**Confirmed on the live public demo**, by pressing the real button in a real browser and sampling
`#key-feedback` every 200ms:

```
t+200ms  "The handset is unavailable while disconnected."
t+600ms  "The handset is ready."
```

`web/index.html:60` offers `<button data-raw="0x7E">services</button>`. Every handset button ships
`disabled` and `web/app.ts:46` **enables them all** once the box is ready, so this is a live control
for every visitor. `web/app.ts:238` sends `Number("0x7E")` = 126; `decodeKey`
(`internal/web/transport.go:341`) rejects it because `handsetRaw` (`transport.go:360-369`) allows
`0x3c`, `0x58-0x5c`, `0x7d`, `0x80`, `0xcc`, `0xf5`, `0..9` and `0x6d-0x70` — not `0x7e`; and
`transport.go:257-258` closes the socket with `StatusPolicyViolation "invalid key"`.

`git log -S"0x7e" -- internal/web/transport.go` is empty and the switch is unchanged since
`1080fda`, so the button and the allowlist have disagreed since the key was put on the page.

**This contradicts the project's own record**, which states the handset answers exactly five codes
*including* `0x7E` services. Either the measurement is right and `handsetRaw` is missing a code, or
the button should not be there. Measure it; do not guess.

**What is not wrong.** The box is **not** reset and other viewers are unaffected — checked rather
than assumed: `runMachine`/`resetState` (`main.go:340-390`) reset only on `stopReset` or an explicit
reset request. The impact is a visible control that disconnects you, not a shared-state attack.

### F-4 [A05] Unbounded reads and quadratic re-encoding on the instruction loop — Low

Tracked as `gort-bek`. **Not timed — treat the magnitude as unmeasured.**

**Trust boundary:** operator-supplied files, read-only mount. Not internet-reachable.

`stampOf` (`listings.go:216-254`) reads and SHA-256 hashes every `*.json` in the schedule directory
on each line-up wave (`multiplex.go:249`, roughly every 60M instructions). `LoadListings`
(`listings.go:273`) has no size cap. `records()` (`multiplex.go:424-454`) re-encodes the growing
quarter once per programme, quadratic in programmes per channel with a Huffman encode inside each
pass. This is work on the loop, which is precisely what `gort-4sx.29` (3.2M instr/s against a 15M/s
NFR) is trying to claw back; whoever takes either should read both.

### Informational

**I-1.** `listings.json/` at the repo root is an empty root-owned directory left by the old
`./listings.json:/listings/listings.json` bind — the mount change is recorded in
`.conformance-authorised` row 91. Git cannot see an empty directory, so it will sit there
indefinitely. `sudo rmdir listings.json`.

**I-2.** `web/data/listings.json` and `web/data/sky-huffman.js` are tracked, have no consumer, and
are not served. Tracked with I-1 as `gort-a8a`; do it in the same pass as F-1, which removes
`web/data/skyuk.dict` from the same directory.

## Passed Checks

- **A01 access control / public surface.** No new routes — `internal/app/app.go:46-47` is still
  `GET /health`, `GET /ws` and an allowlisted asset map with an exact-path check. The Phase 6 diff
  against `internal/web internal/httpx internal/app internal/gdbstub` is **empty**; `handsetRaw` is
  unchanged. Live probes returned 404 for `/debug/pprof/`, `/instruments`, `/metrics`, `/trace`,
  `/.env`, `/.git/config`, and for the new data paths `/listings`, `/listings/default.json` and
  `/dictionaries/skyuk.dict`.
- **A02 cryptography.** Unchanged. Live headers at `47e1f01` still carry HSTS
  (`max-age=31536000`), a CSP naming `wss://goretrotv.demosrv.uk` with `object-src 'none'` and
  `frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff` and
  `Referrer-Policy: no-referrer`. The SHA-256 in `listings.go:221` is a reload content stamp, not an
  integrity control, and is not presented as one.
- **A03 injection / hostile parsing.** `DisallowUnknownFields` (`listings.go:279`); out-of-range
  scalars rejected by the decoder (`genre: 300` → `cannot unmarshal number 300 … uint8`). Fourteen
  hostile dictionaries produced **zero panics**: a 1 MB line → `bufio.Scanner: token too long`;
  no-terminator and empty-file → `has no terminator entry; refusing to guess one`; NUL, `=` and
  space single-character values parse correctly. Decode terminates on empty, all-ones, all-zeros
  and 4 KiB of `0xA5`. A 100,000-character title is refused; 200 records → `title section length
  4211 exceeds 1021`. Every `#nosec G115` in `sections.go` and `titles.go` follows an explicit range
  check — the annotations are honest, and the builders error rather than truncate.
- **A04 insecure design.** Every id is read off the box's own match units
  (`request.go:204-272`) rather than named in the schedule file — the schedule deliberately carries
  none of the facts the hardware settles. A zero extension mask matches nothing
  (`request.go:156-161`). Reload runs on the instruction loop with no goroutine
  (`listings.go:187-190`), consistent with the recorded decision. Gaps: F-2, F-3.
- **A05 misconfiguration.** `GORETROTV_BROADCAST_DATE` accepts only `now` or `YYYY-MM-DD` and is
  refused at startup otherwise (`main.go:246-258`); a schedule configured without a dictionary is
  refused rather than silently broadcasting nothing (`main.go:213-217`); `docker-compose.yml` still
  binds `127.0.0.1:${GORETROTV_PORT:-8099}:8099` with a literal `GORETROTV_BIND_ALL_INTERFACES`;
  all three new mounts are `:ro`. `ARCH-DEV-1` passes all seven probes. Gaps: F-1, F-4.
- **A06 vulnerable components.** `./ctl.sh vuln` → `vulncheck: 0 called vulnerabilities`.
  `npm audit --omit=dev --audit-level=moderate` and `npm audit --audit-level=moderate` → `found 0
  vulnerabilities`. `go vet ./...` clean; `golangci-lint` 0 issues; `go.mod`/`go.sum` unchanged in
  the range.
- **A07 authentication.** None by design — the box is shared and unauthenticated, unchanged.
- **A08 integrity.** `./ctl.sh conformance` → **7 rules, 7 enforced and passing, 26/26 probes
  caught**. `ARCH-FW-1` passes and an independent image export confirms no firmware bytes. Gap:
  F-1.
- **A09 logging.** Phase 6's new log lines emit the configured schedule path, counts, the bouquet
  name, dictionary entry count, wave counters, requested PIDs/MJDs, and reload errors naming the
  operator's own channel and programme. No dictionary contents, no firmware bytes, no host paths
  beyond the operator's own. `main.go:73` logs firmware byte *counts*, not bytes.
- **A10 SSRF.** No outbound I/O added — `internal/multiplex` and `internal/broadcast` import no
  `net/http`, `os/exec` or `syscall`.

## Claims Hand-Walked

| Claim | Verdict |
|---|---|
| "never committed and never baked into a published image" (MANIFEST, compose:33, huffman.go:18-20) | **baked = true** (image export); **committed = FALSE** (F-1) |
| "A MALFORMED EDIT KEEPS THE LAST GOOD SCHEDULE" (`listings.go:176`) | True for JSON/`validate()` failures; **FALSE** for builder-refused schedules (F-2) |
| "loaded and validated BEFORE the listener opens" (`main.go:82`) | Only to `validate()` depth (F-2) |
| `#nosec G304` ×3 (`listings.go:223,273`; `huffman.go:52`) | **Honest.** `ReadDir` names cannot contain `/`, and non-default names must parse as `2006-01-02` (`listings.go:154`) — no traversal. Symlinks resolve to operator files on a `:ro` mount |
| "Pump is cheap when nothing is due" (`main.go:422`) | True (`multiplex.go:199-201` compares `NextDue`); the reload cost lands in the line-up wave instead (F-4) |
| `LiveClock` vs the icount rule | True — `LiveClock` reads the wall (`multiplex.go:110-113`) but carousel timing is icount-only (`carousel.go:151-196`); `ARCH-DET-1` passes and its census excludes `internal/multiplex`, consistent with the record |

## Limitations

- **F-2's final hop** (a `Pump` error reaching `stopHalt` and browsers seeing `halted`) is read from
  `main.go:428-429` and `352-357`, **not driven on real firmware**. Both halves either side — the
  validator accepting and the builder refusing, and the reload swapping the bad file in — were
  measured.
- **F-4 was not timed.** No instructions-per-second figure with a large schedule directory exists in
  the repo, so the magnitude is unknown.
- The audit covers Phase 6. It does not cover the Phase 7 screens, video, or the interactive/return
  path, none of which exist yet.
- This audit does not re-derive Phase 5's conclusions beyond confirming they still hold at
  `47e1f01`.
- The reviewer's session could not read the host `.env`; live public values were taken from
  `docker-compose.yml` and `docker-compose.traefik.yml` defaults.

## Evidence Commands

- `git diff --stat f29b877..47e1f01 -- internal/web internal/httpx internal/app internal/gdbstub` → empty
- `git ls-files '*.dict'`; `sha256sum` on both; `git ls-tree -l origin/master …`;
  `git log --all --diff-filter=A --name-only -- '*.dict'`; `gh repo view … --json visibility` → `PUBLIC`
- `docker build` of a probe image from this repo → dictionary present in the build context with the
  manifest digest; `docker save` + per-layer scan of `goretrotvdemosrvuk-goretrotv:latest` → no
  dictionary bytes
- `go test -race -count=1 -timeout 30m ./...` → **ok, 15m48s, zero cached, zero failures**
  (`internal/multiplex` 936s, `internal/broadcast` 202s)
- `./ctl.sh conformance` → 7/7 rules, 26/26 probes; `./ctl.sh lint` → 0 issues, hooks armed;
  `./ctl.sh vuln` → 0; both `npm audit` forms → 0
- `curl https://goretrotv.demosrv.uk/health` → `version=47e1f01`; `curl -I` for the header set;
  404 probes for the developer and data paths
- Live browser: pressing `services` on the real handset and sampling `#key-feedback` (F-3)
- `go test -overlay` harness for the validator/builder disagreement cases (F-2), tree untouched

## Method

Produced by a `/security-reviewer` agent running on a **different model** from the one that built
Phase 6, read-only, with every load-bearing finding re-verified independently before it was written
down. F-1, F-2's two halves and F-3 were each reproduced by hand; F-3 was confirmed against the live
public demo. Where the reviewer's reading and mine disagreed, the disagreement was resolved by
running the thing rather than by preferring either account — which is how the `AfterPump`
mis-citation in F-2's chain was caught and corrected to `main.go:428-429`.

## Gate

**F-1 blocks nothing technically but should land before the phase is called done**, because the
claim it falsifies is one the repository makes to anyone reading it. F-2, F-3 and F-4 are filed and
do not block. All four are tracked: `gort-a40`, `gort-sgc`, `gort-p5x`, `gort-bek`, plus `gort-a8a`
for the tidy-ups.
