# Test Plan: Phase 1a — Architecture conformance

## Prerequisites
- Phase 1 complete; `ctl.sh conformance` present

## Test Cases

- [x] **TC-1a.1: The gate runs and reports per rule** (covers: TASK-1a.1)
  **Expected:** each rule reports ARMED / SELF-ARMING / DEFERRED with a reason; a deferred rule names
  the task that owes it.

- [x] **TC-1a.2: A public-bound developer surface is refused** (covers: TASK-1a.2)
  **Steps:** probe — bind the gdb stub to `0.0.0.0`.
  **Expected:** ARCH-DEV-1 fails. This is the rule that matters most: the demo host is public.

- [x] **TC-1a.3: A device missing Snapshot is refused** (covers: TASK-1a.3)
  **Steps:** probe — add a device with unexported state and no serialisation.
  **Expected:** ARCH-SNAP-1 fails, naming the type and the field.

- [x] **TC-1a.4: Wall-clock in the core is refused** (covers: TASK-1a.4)
  **Steps:** probe — introduce `time.Now()` into the instruction loop; and separately a `go`
  statement.
  **Expected:** ARCH-DET-1 fails on each independently.

- [x] **TC-1a.5: An inward-pointing violation is refused** (covers: TASK-1a.5)
  **Steps:** probe — import `internal/web` from a device.
  **Expected:** ARCH-LAYER-1 fails.

- [ ] **TC-1a.6: Firmware in the tree is refused** (covers: TASK-1a.6)
  **Steps:** probe — commit a file with the flash image's magic.
  **Expected:** ARCH-FW-1 fails. Not redistributable is a licence fact, not a preference.

- [ ] **TC-1a.7: Every detector is caught on its own** (covers: TASK-1a.7)
  **Steps:** run the stop-gate, which neutralises each detector in turn.
  **Expected:** each rule's own probe goes uncaught while its siblings stay caught. A rule caught
  only by a sibling is not armed.

- [x] **TC-1a.8: Domain logic in the shared layer is refused** (covers: TASK-1a.9)
  **Steps:** probe — import `internal/device/demux` from `internal/platform`.
  **Expected:** ARCH-PLATFORM-1 fails, naming the import. A shared kernel fails by accumulating the
  domain, not by being absent.
