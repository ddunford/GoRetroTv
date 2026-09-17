# Phase 3b security audit

Date: 2026-09-17  
Scope: the blitter, descriptor DMA, video RAM, OSD window and CLUT parsing, snapshots, and trace-runner wiring.

## Findings and fixes

An independent read-only reviewer found two low-severity guest-state integrity faults in descriptor DMA. They did not demonstrate a host escape or remotely reachable exploit.

1. A source of zero and an inclusive end of `0xffffffff` wrapped the transfer length to zero in `uint32`. Channel 8 could report an empty upload as complete; channel 12 could execute stale blitter registers and report completion. Length is now computed in `uint64`, bounded to board DRAM, and channel 12 requires a complete 15-word command.
2. Adding field offsets to a descriptor pointer near `0xffffffff` wrapped the reads into low DRAM. The complete 40-byte descriptor is now checked before any field read. Physical and KSEG0/KSEG1 DRAM aliases remain supported, and source buffers are checked as complete ranges before transfer.

`TestDMARejectsWrappedLengthsAndDescriptorAddresses` checks both wraparound cases, a short blitter command, and a source buffer crossing the end of DRAM. Every invalid transfer must set a visible fault and leave completion clear.

The reviewer also checked allocation bounds, VRAM wrapping, display parsing, snapshot restore, IRQ updates and network exposure. No other concrete Phase 3b finding was reported. The Phase 2 release prerequisites remain open: supported Go toolchain (`gort-4sx.15`) and pinned container image digests (`gort-4sx.16`).

## Verification and limits

`./ctl.sh test`, `./ctl.sh lint`, `./ctl.sh cpu-gate`, `./ctl.sh conformance`, `./ctl.sh vuln`, and `./ctl.sh gate` passed after the fixes. Conformance enforced seven rules and caught all 21 mutation probes; vulnerability checking reported 32 accepted findings and zero new. The CPU gate matched nine oracle anchors through instruction 3,204,424. The complete drawn-menu comparison is still open under `gort-omj.6` / TC-3b.6 until the Phase 3c application boot and handset link are available. This audit does not claim the menu appeared.
