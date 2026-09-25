# Security Review Report: Phase 7 and public release

Date: 2026-09-25
Scope: the current repository and complete Git history; Phase 7 changes since the Phase 6 review;
the browser media transport and playout work through `gort-video.6`; the public deployment at
`https://<configured-public-host>`; licensing and third-party provenance required for a public release.

## Summary

**No credential, firmware, programme-media, or developer-surface exposure was found.** Gitleaks
8.30.1 scanned all 379 commits (17.88 MB) and reported zero findings. A history-path census found
no firmware ROM/image or programme-media payload and no sensitive filename. The public host keeps
developer ports closed, returns 404 for sensitive/developer/data paths, and sends the expected CSP,
HSTS, framing, MIME-sniffing, and referrer protections.

The repository was already public when this review began. Its actual release blocker was licensing:
there was no root licence, while a historical Sky/OpenTV dictionary blob was derived from the GPL-2.0
`jcdutton/loadepg` project. The blob is not proprietary: its 512 lines match upstream except for the
documented removal of one leading space. GoRetroTV is now GPL-2.0, the dictionary is deliberately
included and attributed, and its modification is disclosed. No history rewrite is warranted.

| Severity | Open | Resolved here |
|---|---:|---:|
| Critical | 0 | 0 |
| High | 0 | 1 |
| Medium | 0 | 1 |
| Low | 0 | 2 |

## Resolved findings

### R-1 [A08 / licensing] Public code and GPL-derived data had no declared licence — High

**Evidence.** GitHub reported the repository `PUBLIC` with `licenseInfo: null`. The historical blob
`a041217d` (`tools/skyepg/skyuk.dict`) is the same 512-line code table as `loadepg` commit
`74267d7089d1b8e4fb1c88f710af1666b9a907ed`, `conf/sky_uk.dict`, apart from upstream line
` =0001000` being corrected to `=0001000`. Upstream contains `COPYING` with GPL version 2 and a
README directing recipients to it.

**Resolution.** Added the full GPL-2.0 text at `LICENSE`, an explicit README licence section,
`THIRD_PARTY_NOTICES.md`, and exact source/commit/hash/change provenance in
`dictionaries/MANIFEST.md`. The corrected dictionary is intentionally tracked. Claims that it was
unredistributable or of uncertain provenance were removed from live code/configuration. This also
resolves Phase 6 F-1 on better evidence than its earlier conservative owner decision.

### R-2 [A05 / release hygiene] Current documentation exposed private workstation topology — Medium

**Evidence.** Current files named absolute `/opt/workspaces/development/...` paths and a private LAN
address. `tools/digibox-probe.mjs` depended on an absolute path into the archived predecessor and
could not work for another clone.

**Resolution.** Current documentation now describes the separately maintained archive without its
host path; audit evidence describes the origin as a private LAN address without publishing it; and
the probe resolves Playwright from this repository. Historical commits retain old text, which is
not a credential and does not identify a publicly routable service.

### R-3 [A02 / secret detection] Secret scanning existed only as a local staged-file hook — Low

**Evidence.** `.githooks/pre-commit` rejects credential patterns in staged files, but CI did not
scan history. A bypassed/misconfigured local hook could therefore publish a secret without a remote
gate.

**Resolution.** CI now fetches complete history, downloads pinned Gitleaks 8.30.1, verifies the
official Linux archive SHA-256, and runs a redacted history scan. The same command passed locally:
379 commits, 17.88 MB, zero findings.

### R-4 [A05 / documentation accuracy] Runtime comments still claimed the dictionary was forbidden — Low

**Resolution.** `.gitignore` allows only the attributed `dictionaries/skyuk.dict`; unknown local
dictionaries remain ignored. Compose, configuration, broadcast code, README, and the dictionary
manifest now consistently state the GPL-2.0 provenance. The dictionary remains a read-only runtime
mount and is not copied into the container image.

## Passed checks

- **A01 access control / surface:** only the static allowlist, `/health`, and `/ws` are public.
  Live requests for `/.env`, `/.git/config`, `/debug/pprof/`, `/metrics`, `/instruments`,
  `/openapi.json`, `/swagger`, `/docs`, `/admin`, `/dictionaries/skyuk.dict`, and a firmware image
  returned 404. TCP ports 23457 and 8099 were not publicly reachable.
- **A02 cryptography and browser policy:** TLS deployment sends HSTS `max-age=31536000`; CSP limits
  code/assets/connections to self and the named WSS endpoint, with `object-src 'none'`,
  `frame-ancestors 'none'`, `base-uri 'self'`, and `form-action 'none'`; it also sends
  `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and `Referrer-Policy: no-referrer`.
- **A03 injection:** browser input remains a numeric handset allowlist; media state is server output,
  not executable markup. No new shell, SQL, template, or URL fetch boundary was added.
- **A04 design:** channel selection, demux PIDs, OSD, and timing remain firmware-owned. The browser
  receives requested/active state and media; it does not select channels or invent playback state.
- **A05 configuration:** developer listeners remain loopback-only; pprof is refused outside
  development; firmware, media, listings, and dictionary mounts are read-only.
- **A06 components:** `./ctl.sh vuln` reports zero called Go vulnerabilities and npm reported zero
  vulnerabilities. Runtime dependencies use permissive licences compatible with the project GPL.
- **A07 authentication:** none by design; the demonstration box is intentionally shared and carries
  no accounts or private user data.
- **A08 integrity:** firmware and media are absent from the current tree and all historical paths.
  `ARCH-FW-1` and the container-image probe pass. The complete Git history has no secret finding.
- **A09 logging:** no credential or media content is logged. Public health output contains only
  status/build identity/timestamp.
- **A10 SSRF:** no user-controlled outbound request path exists.

## Limitations

- This is a technical security and provenance review, not legal advice. GPL-2.0 is selected to
  satisfy the known dictionary provenance and the owner's open-source instruction.
- Git commit metadata and the tracked Beads issue export identify the maintainer and preserve
  engineering history. That is ordinary public-project metadata, not a secret; it has deliberately
  not been rewritten.
- Firmware and programme media must still be supplied lawfully by each operator. They remain
  excluded from Git and from the published container image.
