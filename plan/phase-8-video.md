# Phase 8: Configured channels with video and audio

## Outcome

Selecting a service on the unmodified Pace firmware resolves that service and the programme on air
through the editable TV-guide configuration. A programme with a configured media source displays
its video behind the guest OSD and produces audible synchronized sound in the browser; a programme
without one does not acquire a host-supplied picture. The first source is a deterministic moving
test pattern and tone, followed by scheduled files/folders through the same resolver. The signal is
a standards-shaped DVB service carried through the emulated transport and the guest-requested
media path; no guest RAM, firmware byte, PID, lifecycle state or decoder call is injected.

## Evidence and constraints

- Existing implementation: `internal/dvb/transport.go` packetizes sections, `internal/multiplex/`
  broadcasts SI/listings, `internal/device/osd/` composes the guest OSD, and `internal/web/` carries
  framebuffer frames. There is no PMT/PES builder, elementary-stream decoder, video plane or audio
  wire contract to duplicate.
- `plan/module-decisions.md` already reserves video for a separate phase and selects ffmpeg through
  CGo or a subprocess at the boundary. Use a subprocess so the emulator core remains no-CGo and
  deterministic.
- DVB TS 101 154 specifies MPEG-2 Main Profile at Main Level for MPEG-2 SDTV and recommends Layer II
  for MPEG-1 audio bitstreams: <https://www.etsi.org/deliver/etsi_ts/101100_101199/101154/02.02.01_60/ts_101154v020201p.pdf>.
- DVB describes video and audio as elementary streams carried by MPEG-2 TS and synchronized by its
  timing model: <https://dvb.org/?standard=carriage-of-synchronised-auxiliary-data-in-dvb-transport-streams>.

## Design boundary

The guest remains authoritative for tuning and stream selection. The tuned service is read from the
service-id extension of the EIT present/following filter the firmware arms while viewing; DVB's
`service_id` is the join key between the service and its PMT. The transmitter joins that id to the
current programme in the guide and only then resolves its optional media source. A deterministic
generated fixture (moving bars/clock plus a clearly audible tone cadence) is the first source kind;
files and folders extend the same programme-owned configuration rather than adding a second channel
map. The multiplex announces its PAT/PMT and packets only after the measured firmware path requests
them. The media device consumes the guest-programmed PIDs/DMA route, sends encoded payload to a
bounded ffmpeg subprocess, and composites decoded video beneath the existing OSD. Browser audio
uses a versioned wire message with instruction-count timestamps; mute/unlock/reconnect states are
explicit.

## Planned work

1. Measure and satisfy the `0x210` media-object event from `gort-qxl.7.3`; capture the guest's real
   PMT/PES PID and media-DMA requests before implementing either path. `[TC-8.1]`
2. Resolve the firmware-selected service and current guide programme to its configured media source,
   then add deterministic PAT, PMT, PES and TS generation for MPEG-2-video/MP2-audio sources through
   the single broadcast implementation. `[TC-8.2]`
3. Model the requested media registers/DMA with complete snapshot/restore state and instruction-
   count scheduling. `[TC-8.3]`
4. Decode through a supervised ffmpeg subprocess with bounded queues, visible failures and no CGo
   in the emulator core. `[TC-8.4]`
5. Composite the decoded video plane below the guest OSD and transport timestamped audio to the
   browser with reconnect, mute and autoplay-unlock behavior. `[TC-8.5]`
6. Prove the real user journey in a browser in light and dark modes: tune configured and
   unconfigured channels, observe media only on the matching current programme, hear the cadence,
   retain responsive layout, and preserve boot/oracle/replay gates.
   `[TC-8.6]`

## Ordering

The Beads graph is authoritative. Measurement precedes stream construction; stream construction
precedes device/decoder/browser integration; the browser acceptance and regression gates close the
phase.
