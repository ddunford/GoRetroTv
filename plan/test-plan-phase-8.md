# Test Plan: Phase 8 — configured channel video and audio

## Test cases

- **TC-8.1: Firmware requests the media path** (covers: Phase 8 task 1) — a repeatable real-firmware
  probe reaches the `0x210` object callback and public audio/video start boundary from authentic
  DVB/device input, names every operand and includes cold and live controls.
- **TC-8.2: Guide-configured media selection and standards-shaped service** (covers: Phase 8 task 2)
  — each firmware-selected `service_id` resolves to that service's current guide programme; a
  configured programme or service-default test-pattern source activates with its real
  channel/programme names, an unconfigured service stays inactive, and switching services replaces
  rather than retains the prior source.
  PAT/PMT/PES/TS fixtures parse independently, continuity counters and timestamps advance correctly,
  and ffprobe identifies one MPEG-2 video stream plus one MP2 audio stream with the declared PIDs.
- **TC-8.3: Deterministic media device** (covers: Phase 8 task 3) — guest-programmed PID/DMA state
  alone admits media; snapshot/restore and replay reproduce byte-identical device state and the
  same instruction-count presentation sequence.
- **TC-8.4: Decoder supervision** (covers: Phase 8 task 4) — a real fixture decodes, malformed input
  and an exited/hung ffmpeg fail visibly, queues remain bounded, and shutdown leaves no process.
- **TC-8.5: Video composition and audio wire** (covers: Phase 8 task 5) — moving decoded frames are
  below transparent OSD pixels, OSD remains above video, timestamped audio survives reconnect, and
  autoplay refusal presents an operable unmute control rather than silently dropping sound.
- **TC-8.6: Tune and watch in the browser** (covers: Phase 8 task 6) — Playwright tunes a configured
  service and an unconfigured service through the real firmware, proves two separated video frames
  differ only for the configured current programme, verifies non-silent audio samples through the
  wire/control surface, and screenshots desktop/mobile light and dark states for inspection.
  Boot, oracle, snapshot, replay, guide and security gates remain green.
- **TC-8.7: Scheduled file and folder playout** (covers: Phase 8 task 7) — two services resolve to
  different configured media sources; selecting either halfway through its programme begins at the
  same broadcast-relative media offset a continuously tuned receiver would have reached. A folder
  advances across lexically ordered files by their probed durations. Traversal outside the media
  root, missing/unsupported files, ambiguous or empty folders, insufficient non-looping duration,
  and changed files fail visibly. Looping wraps by the exact probed playlist duration, and a
  reconnect derives the current offset again rather than restarting at zero.
