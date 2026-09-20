# Test Plan: Phase 6 — The broadcast

## Test Cases
- [x] **TC-6.1: Sections are accepted** (covers: TASK-6.1, TASK-6.9) — CRC verified by the firmware, not by us.
  **Result:** `internal/broadcast/firmware_test.go` boots the real `FLASH_U202`/`U203` images from the
  post-acquisition snapshot and pushes a built NIT at the guest. Addressed to the network the guest's
  own match unit is asking for, the firmware reads the section and keeps fetching — 37 fetches, accepted.
  With the network id moved by one the section still reaches the parser and is discarded after its
  header — 9 fetches, not accepted — so the test proves the guest's own acceptance rather than ours.
  `internal/dvb/crc_test.go` anchors the CRC against the standard `123456789` check vector, and
  `internal/broadcast/sections_test.go` covers section length, the descriptor loops and MJD/BCD times.
  `go test ./internal/broadcast ./internal/dvb` passes with the private firmware installed and skips
  without it. TASK-6.9's cross-check of the Huffman encoder is a separate case and is not covered here.
- [x] **TC-6.2: The line-up builds service records** (covers: TASK-6.2, TASK-6.9) — N services, 18 bytes apart,
  with the four flag bits unpacked; a gate halfword the guest does not admit must decode no entries.
  *(The case used to say "other than `0xFFFF`". It is corrected rather than ticked as written: the
  box admits `0x0000` too, which nobody had tested — see the Result.)*
  **Result:** `internal/broadcast/lineup_firmware_test.go` pushes a real BAT at the restored
  post-acquisition snapshot and counts the guest's own parser PCs, so "did it decode N services"
  is answered by the firmware rather than by reading our own bytes back. Four services in,
  `0x800BF826` (entry field read) and `0x800BF814` (the `mult` by `0x12` that gives the eighteen-byte
  record stride) each run exactly four times, and so does each of the four flag-unpack stores
  `0x800BF860/6C/78/96` — all four are asserted separately, because three of them firing would look
  like success at any coarser granularity. The bouquet id and network id are read off the box's own
  match unit (unit 3 is `4a/ff 10/ff 00/ff`, so bouquet `0x1000` exactly); choosing them instead is
  how you broadcast a section the hardware never delivers, which from outside is indistinguishable
  from one the box ignored.
  **Negative controls, and two of them corrected the plan.** A gate of `0x1234`, `0x0001` or
  `0xFFFE` decodes nothing; specifier 9 in place of 2 decodes nothing. But sweeping nine gate values
  found **`0x0000` decodes as readily as `0xFFFF`**, by a different path through the firmware, and
  moving the `0x5F` *after* the `0xB1` still decoded everything — so on this box the specifier's
  value is load-bearing and its position is not. Both refine claims the record had drawn from a
  single control each, and both are written up in `docs/reference/digibox-emulation.md → gort-qbn.2`.
  The builder still emits the `0xFFFF` gate with the specifier first.
  **Falsified, not assumed:** reordering the descriptors in the builder was run against the real
  firmware to see whether the test would catch it — it did not, which is how the position finding
  surfaced. `TestOracleLineupDescriptorVector` pins the Go bytes to the untouched oracle's own
  `lineupDescriptors()` output (`5f0400000002b114ffff…`), so the builder matches an implementation
  a box has already been measured accepting. `go test -race ./...`, `ctl.sh lint` and the boot gate
  pass.
- [x] **TC-6.3: The linkage makes the guide ask and be answered** (covers: TASK-6.3, TASK-6.9) — with it the
  answered arm runs; without it the not-answered arm does. Both directions asserted.
  **Result:** `internal/broadcast/linkage_firmware_test.go` feeds a real BAT to the restored
  post-acquisition snapshot, presses `tv guide`, and counts the two arms of the guide's single
  database question at the addresses the record read off the firmware — `0x800A4040` on success,
  `0x800AC774` on failure:

  | linkage_type | answered arm | not-answered arm |
  |---|---|---|
  | `0x91` | **1** | 0 |
  | `0x90` | 0 | **2** |

  Both directions, because both arms end in a drawn screen and "the guide showed something" is not
  evidence of anything. The negative case patches the linkage_type byte and repairs the CRC rather
  than removing the descriptor, so the section keeps every length and the only thing that differs is
  the one byte the firmware actually tests. It patches **both** loops and fails the test if it finds
  fewer than two, since changing one would leave the other answering and the run would prove nothing.
  `TestBATCarriesTheLinkageInBothDescriptorLoops` holds the both-loops requirement structurally, and
  `TestLinkageCarriesTheFieldsTheGuideReads` pins the bytes to the field offsets the callback reads
  (`4a0712340020006491`) rather than to a public table. A transport that declares no service is
  refused outright: a linkage naming a plausible default would answer the guide with a service that
  does not exist. `go test -race ./...`, `ctl.sh lint` and the boot gate pass.
  **Not yet visible on the demo host.** Nothing wires `internal/broadcast` to the running machine —
  only its own tests import it — so the live box still receives no SI at all. The remaining phase-6
  tasks are that work.
- [x] **TC-6.4: Twelve records arrive as twelve** (covers: TASK-6.4, TASK-6.9) — and with the length field
  computed openTVtoXML's way, the box must read two. The wrong version has to be shown failing.
  **Proved, at the byte level:** `internal/broadcast/titles_test.go` builds a twelve-record section
  and walks it twice. The firmware's arithmetic — `local_84 += local_8a + 4` with
  `memcpy(dst, section + local_84 + 4, local_8a)`, which tvheadend reads the same way — finds
  **12 of 12**. openTVtoXML's, advancing by the field alone, finds **1 of 12, silently**, on the
  identical bytes. The count the broken walk lands on is deliberately **not** pinned: where it
  falls apart depends on how long the records happen to be, and the record's own measurement saw
  two with its records. Asserting an exact number would be attaching a figure from one fixture to
  another, which is the failure this file keeps recording. What is invariant is that the reference
  reads a fraction of what was broadcast and reports nothing.
  The codec is pinned byte-for-byte to the validated Python encoder it is a port of
  (`TestHuffmanMatchesTheValidatedEncoder`), which was itself checked by round-tripping through a
  transcription of the reference decoder — and separately round-trips here, because an encoder can
  be self-consistently wrong: pack eight bits into byte 0 instead of six and encode/decode still
  agree while the box reads nonsense.
  **And against the box, which is the half that matters.** `titles_firmware_test.go` feeds a clock
  and a line-up, reads the listings request off the box's own match unit — **table `0xA3`,
  extension `0x0BB8`, MJD `0xC67E`, PID `0x36`** — and pushes twelve programmes to it. The guest's
  consumer ladder turns end to end: `sectionParser` 1, `extensionLookup` 1, `dayKeyToSlot` 1,
  `findBlock` 1, `registerBlock` 1, then `walkDriver`, `walkDescriptors` and `perEventRegister` at
  **12 each**. With the lengths overstated openTVtoXML's way and the CRC repaired — every other byte
  identical — the box registers **2**, which is the count the record measured and the number this
  case was written around.
  **It only became provable after a demux fix.** `Demux.Match` read the low halfword of every match
  word, but units 8..15 carry their rule in the HIGH half — sixteen units packed two to a word.
  The box's entire listings subscription therefore read back as `00/00`, which looks like a filter
  nobody programmed rather than one we could not see, and sections addressed by guesswork were
  collected by the interrupt handler and silently dropped. Found by logging the guest's own writes
  to `+0x148`/`+0x144` through the bus observer: unit 2 wrote `42ff42fb` and means `42/fb` in the
  low half, unit 8 wrote `a3fe0000`, `0bff0000`, `b8ff0000`, `c6ff0000`, `7eff0000` and means a
  complete title filter in the high one.
- [x] **TC-6.5: Clock first changes the request** (covers: TASK-6.5, TASK-6.9) — proved by
  `TestTheClockTableChoosesTheDayAndThePIDTheBoxAsksFor`, four cases, falsified twice against the
  product (the MJD anchor one day out, and a TOT carrying a zero MJD).

  **The case as originally written could not be run, and finding out why is the result.** It read
  "with a clock the box asks for a real MJD; without one it asks for 40587". Neither half holds on
  this fixture: it is a *warm* box and wakes already holding a day (MJD 50814), so "without a clock"
  is not a state it can be put into — and the table that sets the clock is the **TOT**, not the TDT
  every earlier run in this package fed. The box's match units carry `0x73` and nothing matches
  `0x70`. What is proved instead, which is the thing the task needs:

  | fed | requested MJD | PID |
  |---|---|---|
  | nothing | 50814, the day it woke with | `0x36` |
  | a TDT for 1998-06-15 | 50814 — **unmoved** | `0x36` |
  | a TOT for 1998-06-15 | 50979 = 1998-06-15 | `0x33` |
  | a TOT for 1998-03-06 | 50878 = 1998-03-06 | `0x36` |

  and in every case the PID is `0x30 | (MJD mod 8)`, checked rather than assumed. The full sweep and
  the open question it raised are in `docs/reference/digibox-emulation.md`.
- [ ] **TC-6.6: Sections are addressed as the box asks** (covers: TASK-6.6, TASK-6.9).
- [ ] **TC-6.7: The in-world clock face matches real time** (covers: TASK-6.7, TASK-6.9) — 1:1, London both ends.
- [ ] **TC-6.8: A live edit reaches the guide; a broken one does not break it** (covers: TASK-6.8, TASK-6.9).
- [ ] **TC-6.13: Every day in the eight-day rotation can be fed listings** (covers: TASK-6.13) — sweep
  eight consecutive days; each must program a title match unit, not merely arm a PID. The instrument
  must dump all sixteen match units unconditionally, because a census narrowed to `0xAn` cannot see
  the half of this that is about the PID.
- [ ] **TC-6.9: The guide shows correct now and next** (covers: TASK-6.10) — SPEC success criterion 4.
