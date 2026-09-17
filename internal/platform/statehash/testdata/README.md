# Cold-boot oracle traces

`oracle-cold-boot.stream` is the browser oracle's original cold-boot trace. Its default
`skyGatesTick()` presentation policy changes guest RAM and one flash byte after 42 tasks exist.
That declared host patch is visible in the state hash near instruction 447,465,320.

`oracle-cold-boot-nogates.stream` was captured by `tools/oracle-cold-boot-capture.mjs` from a
temporary in-memory copy of `reference/digibox-boot.html` with only the declared Sky menu policy
disabled. The script serves the same two local firmware images on a fresh browser origin with
empty localStorage and `?cp=1000&si=0`, stops after 470,000,000 retired instructions, and verifies
the guest created 42 tasks. The stream has **470,000 checkpoints**, through instruction
469,999,000. The checked-in oracle is unchanged; its SHA-256 at capture was
`9d57489d7a65bf325315a0cd80564d254b18d56e31c20197421901f1204f61b5`.

To recapture, run `node tools/oracle-cold-boot-capture.mjs --out
internal/platform/statehash/testdata/oracle-cold-boot-nogates.stream` with a local Playwright
module (`PLAYWRIGHT_MODULE`) and browser executable (`PLAYWRIGHT_CHROME`) if they are not on the
default Playwright paths. The private `firmware/` images are required. `./ctl.sh oracle-gate`
runs the real Go firmware against this stream at the same 1,000-instruction cadence. This trace
isolates CPU, RAM and device agreement before the optional Sky menu policy.

`tools/oracle-tier2-gate.sh` proves the second verification tier on a real boot. It makes a
temporary browser copy with a single Status-register bit changed after instruction 100,123,
without changing the checked-in oracle or Go model. The first differing tier-1 checkpoint is at
101,000; the comparator directs tracing from the last matching checkpoint through that one
(`100,001..101,000`). Per-instruction comparison identifies instruction **100,123**, after 105
matching browser states, and prints both PC, ISA, Count, Status and full-state hashes. The bit
change is an explicit test mutation, never part of the full cold-boot fixture.
