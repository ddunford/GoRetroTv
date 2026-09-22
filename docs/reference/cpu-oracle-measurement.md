# Phase 2 CPU oracle measurement

<!-- anchor: internal/cpu/core.go -->
<!-- anchor: internal/platform/statehash/statehash.go -->
<!-- fingerprint: sha256:99d9e6433d279311e101fe0036bd8165827cbae357175b3157861678b1759fb2 @ 2026-09-22 -->

The Go runner `go run ./cmd/firmwaretrace` loads the three images through the verified firmware
loader, starts at `0xBFC00000`, and emits the shared `statehash` format. Its `-interval` and
`-steps` flags select the checkpoint window. The browser oracle was served unchanged from
`reference/digibox-boot.html?cp=100&si=0`, with only the verified firmware files routed to its
existing sibling fetch paths. The page's `window.__checkpoints()` supplied the other stream.

The first MIPS32 branch revealed one inherited timing detail: the browser advances COP0 Count once
for a branch and its delay slot together, because its MIPS32 loop retires that pair at once. The
Go core now reproduces that behavior. Before the correction, checkpoint 6 disagreed; after it,
all 1,000 shared per-instruction checkpoints through instruction 1,091 agreed.

At interval 100, the streams first disagree at checkpoint **3,204,500**. Direct register traces
put the first different value at instruction **3,204,424**. The MIPS16 load at `0xBFC0AF5A`
reads word `0xB00020B0`, the video RAM data port. The oracle returns `0xAAAAAAAA`, which its
bootloader just wrote as a memory-test pattern. The Go bus reports that address as an unmapped
device and returns zero. PC and all compared registers agree immediately before that load. The
firmware then takes different comparison branches, as the measured record predicts. This is the
first missing peripheral, not a CPU decoder mismatch.

The oracle's own port model is documented in `reference/digibox-boot.html` at the `VIDEO RAM`
section; the Go video RAM model belongs to the drawing-device phase. The Go runner's unmapped
census names the wall instead of silently treating zero as hardware evidence. The CPU comparison
is therefore proved from reset through the first unmodelled read.

The Phase 2 integration command was `./ctl.sh cpu-gate`. At that point it built
`firmwaretrace`, checked eight oracle checkpoints spanning reset through instruction 3,204,400,
and asserted the video RAM read and first divergent sampled state. A deliberately corrupted
checkpoint-zero anchor failed the gate with exit 1. The full browser-vs-Go interval-100 comparison
above remains the measurement evidence; the committed anchor set is the fast regression gate.

**Phase 3b update (2026-09-17):** `internal/device/osd.Video` now models the 1 MB video RAM
address, data and mode ports at `0xB00020BC`, `0xB00020B0` and `0xB00020B4`.
`./ctl.sh cpu-gate` reproduces the previously divergent oracle checkpoint at instruction
3,204,500 (`0x8BC499D0`) and verifies that the MIPS16 load at instruction 3,204,424 returns
the firmware's `0xAAAAAAAA` test pattern. The port is no longer unmapped. The gate now has nine
independent oracle anchors; the former wall fixture was removed. Farther boot agreement belongs
to the later drawing and link tasks, and is not inferred from this one checkpoint.
