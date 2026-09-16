# Phase 2 CPU oracle measurement

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
