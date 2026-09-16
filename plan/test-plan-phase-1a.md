# Test Plan: Phase 1a — Architecture conformance

## Prerequisites
- Phase 1 complete; `ctl.sh conformance` present

## Test Cases

- [x] **TC-1a.1: The gate runs and reports per rule** (covers: TASK-1a.1)
  **Expected:** each rule reports ARMED / SELF-ARMING / DEFERRED with a reason; a deferred rule names
  the task that owes it.
  **Result:** `conformance/run.py` and `conformance/check_registry.py` report every registered rule's arming and result; `conformance/probes/ARCH-MODULE-1/dependency-added/apply.sh` proves a registered rule fails.

- [x] **TC-1a.2: A public-bound developer surface is refused** (covers: TASK-1a.2)
  **Steps:** probe — set the current developer-capable HTTP listener to `0.0.0.0` without the container override. The gdb stub is a later-phase surface.
  **Expected:** ARCH-DEV-1 fails. This is the rule that matters most: the demo host is public.
  **Result:** `conformance/probes/ARCH-DEV-1/guard-removed/apply.sh` proves the loader refuses a public bind; `conformance/probes/ARCH-DEV-1/public-port/apply.sh` proves Compose cannot publish it publicly.

- [x] **TC-1a.3: A device missing Snapshot is refused** (covers: TASK-1a.3)
  **Steps:** probe — add a device with unexported state and no serialisation.
  **Expected:** ARCH-SNAP-1 fails, naming the type and the field.
  **Result:** `conformance/probes/ARCH-SNAP-1/snapshot-methods-omitted/apply.sh` catches missing methods; `conformance/probes/ARCH-SNAP-1/field-omitted/apply.sh` catches an uncovered state field through the live device contract test.

- [x] **TC-1a.4: Wall-clock in the core is refused** (covers: TASK-1a.4)
  **Steps:** probe — introduce `time.Now()` into the instruction loop; and separately a `go`
  statement.
  **Expected:** ARCH-DET-1 fails on each independently.
  **Result:** `conformance/probes/ARCH-DET-1/wall-clock-in-cpu/apply.sh` and `conformance/probes/ARCH-DET-1/goroutine-in-device/apply.sh` each fail under their own detector.

- [x] **TC-1a.5: An inward-pointing violation is refused** (covers: TASK-1a.5)
  **Steps:** probe — import `internal/web` from a device.
  **Expected:** ARCH-LAYER-1 fails.
  **Result:** `conformance/probes/ARCH-LAYER-1/device-imports-http/apply.sh` is caught by the core-to-transport import detector.

- [x] **TC-1a.6: Firmware in the tree is refused** (covers: TASK-1a.6)
  **Steps:** probe — commit a file with the flash image's magic.
  **Expected:** ARCH-FW-1 fails. Not redistributable is a licence fact, not a preference.
  **Result:** `conformance/probes/ARCH-FW-1/flash-in-repository/apply.sh` is caught in tracked source; `conformance/probes/ARCH-FW-1/flash-in-image/apply.sh` is caught in the built image.

- [x] **TC-1a.7: Every detector is caught on its own** (covers: TASK-1a.7)
  **Steps:** run the stop-gate, which neutralises each detector in turn.
  **Expected:** each rule's own probe goes uncaught while its siblings stay caught. A rule caught
  only by a sibling is not armed.
  **Result:** `conformance/stop-gate.py` and `conformance/ablate.py` passed for every registered detector and subject; their source-corpus proofs include `conformance/probes/ARCH-SNAP-1/device-census-removed/apply.sh`.

- [x] **TC-1a.8: Domain logic in the shared layer is refused** (covers: TASK-1a.9)
  **Steps:** probe — import `internal/device/demux` from `internal/platform`.
  **Expected:** ARCH-PLATFORM-1 fails, naming the import. A shared kernel fails by accumulating the
  domain, not by being absent.
  **Result:** `conformance/probes/ARCH-PLATFORM-1/domain-import/apply.sh` is caught by the parsed import-graph checker.
