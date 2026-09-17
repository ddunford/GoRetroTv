# Cold-boot oracle traces

`oracle-cold-boot.stream` is the browser oracle's original cold-boot trace. Its default
`skyGatesTick()` presentation policy changes guest RAM and one flash byte after 42 tasks exist.
That declared host patch is visible in the state hash near instruction 447,465,320.

`oracle-cold-boot-nogates.stream` was captured from a temporary copy of the unmodified
`reference/digibox-boot.html` with only `var skyGates = false` changed. The copy ran on a fresh
browser origin with empty localStorage and `?cp=100000&si=0`. The stream has 4,700 checkpoints
through instruction 469,900,000. The temporary page was used only for this measurement; the
repository oracle remains unchanged. This trace isolates CPU, RAM and device agreement before
the optional screen policy, and `./ctl.sh oracle-gate` compares a real Go firmware run against it.
