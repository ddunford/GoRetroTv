# Test Plan: Phase 8 — test channel video and audio

## Test cases

- **TC-8.1: Firmware requests the media path** (covers: Phase 8 task 1) — a repeatable real-firmware
  probe reaches the `0x210` object callback and public audio/video start boundary from authentic
  DVB/device input, names every operand and includes cold and live controls.
- **TC-8.2: Standards-shaped test service** (covers: Phase 8 task 2) — PAT/PMT/PES/TS fixtures parse
  independently, continuity counters and timestamps advance correctly, and ffprobe identifies one
  MPEG-2 video stream plus one MP2 audio stream with the declared PIDs.
- **TC-8.3: Deterministic media device** (covers: Phase 8 task 3) — guest-programmed PID/DMA state
  alone admits media; snapshot/restore and replay reproduce byte-identical device state and the
  same instruction-count presentation sequence.
- **TC-8.4: Decoder supervision** (covers: Phase 8 task 4) — a real fixture decodes, malformed input
  and an exited/hung ffmpeg fail visibly, queues remain bounded, and shutdown leaves no process.
- **TC-8.5: Video composition and audio wire** (covers: Phase 8 task 5) — moving decoded frames are
  below transparent OSD pixels, OSD remains above video, timestamped audio survives reconnect, and
  autoplay refusal presents an operable unmute control rather than silently dropping sound.
- **TC-8.6: Tune and watch in the browser** (covers: Phase 8 task 6) — Playwright tunes the real
  test service, proves two separated video frames differ, verifies non-silent audio samples through
  the wire/control surface, and screenshots desktop/mobile light and dark states for inspection.
  Boot, oracle, snapshot, replay, guide and security gates remain green.
