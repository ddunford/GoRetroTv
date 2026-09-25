# GoRetroTV

A Pace 2500N Sky Digibox (1998–2002), running **its own firmware**, in Go — with a browser for a
screen and a handset.

Not a Sky-lookalike interface. The real flash image, the real MIPS, the real menus, the real guide.

## Start here

1. **`SPEC.md`** — what this is and what v1 means.
2. **`docs/reference/digibox-emulation.md`** — ~6,000 lines of measured facts about this firmware.
   Every address, constant and behaviour claimed anywhere else traces back to it. It is evidence,
   not documentation: it records how each thing was measured, and which earlier readings were
   withdrawn.
3. **`docs/reference/digibox-emulator-skill.md`** — the CPU reference, the firmware's conventions,
   and the measurement discipline this domain demands.
4. **`docs/reference/lessons-from-the-browser-emulator.md`** — mistakes already paid for once.

## The oracle

`reference/digibox-boot.html` is the **working browser emulator this port is derived from**. It is
kept deliberately: it boots the same flash image, draws the same interface, and shows real listings
— so it is the one independent check the Go port has. The port is correct exactly as far as the two
agree, instruction for instruction (`SPEC.md` FR-6).

Deleting it would make every later "is this right?" unanswerable.

## Firmware

Not in the repository and not redistributable — see `firmware/MANIFEST.md`. A clone is inert until
someone supplies the images.

## Licence

GoRetroTV is free software under the GNU General Public License version 2. See `LICENSE`. The
included Sky/OpenTV Huffman dictionary is derived from `jcdutton/loadepg`; its origin and the local
correction are recorded in `THIRD_PARTY_NOTICES.md` and `dictionaries/MANIFEST.md`.

## Lineage

This replaces an earlier project in which the emulator began as one epic among many. That work —
610 commits, a FastAPI backend, the listings-reconstruction pipeline, ErsatzTV playout, 464 tests
and a conformance harness — is preserved separately by the maintainer. The listings reconstruction
is planned to return as a later phase; the playout stack stays archived unless video lands.
