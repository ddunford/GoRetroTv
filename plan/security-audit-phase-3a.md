# Phase 3a security audit

Date: 2026-09-17  
Scope: the section demux, shared DRAM ring writes, board interrupt controller, snapshots, and the `firmwaretrace` wiring.

## Findings

No new Critical, High, Medium or Low finding in the reviewed Phase 3a code. An independent read-only reviewer examined section validation, ring bounds, MMIO indexing, snapshot restore, IRQ signalling and integration exposure. No new HTTP route, filesystem write, or network input path was introduced. `Demux.Push` currently has test callers only.

The Phase 2 release prerequisites remain open: supported Go toolchain (`gort-4sx.15`) and pinned container image digests (`gort-4sx.16`). They are not Phase 3a regressions.

## Verification and limits

`./ctl.sh test`, `./ctl.sh lint`, `./ctl.sh gate`, `./ctl.sh cpu-gate`, `./ctl.sh conformance` and `./ctl.sh vuln` passed. Conformance reported seven enforced rules passing, including snapshot contract coverage. `govulncheck` reported 32 accepted findings and zero new. The CPU gate matched eight oracle anchors through instruction 3,204,400 and identified the first unmodelled video RAM read at 3,204,424.

Guest section consumption, demux handler entry and SI acquisition are still open under `gort-l14.7` / TC-3a.5–3a.7, which depends on the Phase 3c real application handoff. Future public section-injection or snapshot-upload interfaces need their own exposure review when built.
