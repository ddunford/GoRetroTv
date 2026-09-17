# Local machine snapshots

Snapshots contain the full 32 MB RAM and both firmware flash images. They stay in
`snapshots/`, which is gitignored, and the control script makes the directory private (`0700`)
and each image private (`0600`). Do not copy an image into a repository or a public artifact.

The library is addressed by lowercase names of up to 64 letters, digits or hyphens:

```bash
./ctl.sh snapshot save booted -steps 470000000 -interval 470000000 -tasks
./ctl.sh snapshot list
./ctl.sh snapshot inspect booted
./ctl.sh snapshot run booted -steps 470001000 -state-hash
```

`save` refuses to replace an existing name. `run` uses an **absolute** instruction endpoint:
the example runs 1,000 more guest instructions after a snapshot saved at 470,000,000.
Handset inputs and their `-key-at` time use the same absolute retired-instruction count.
`inspect` restores the image without retiring another instruction and reports its current state
and raw display hash. The wrapper builds the binary first; restore timing should be measured
against `bin/firmwaretrace` after a build, rather than including compiler time.

For a different private library directory, set `GORETROTV_SNAPSHOT_DIR` when calling `ctl.sh`.
The supplied firmware in `firmware/` must match the image's machine; the loader verifies it
before constructing the bus. A mismatched or incomplete snapshot is refused rather than loaded
partially.

## Post-acquisition checkpoint

The private `post-acquisition` image is a **warm**, pressable machine at instruction
1,100,000,000. Its EEPROM came from a real 470M cold boot (SHA-256
`e63dc6f8c46c486a1db56a77280b6cc4b4c139316e243b682868de98225aa1b0`).
The warm boot uses the measured Sky menu gate policy. The guest then receives three
TDT/TOT clock pairs, a NIT for its requested network `0x0020`, a BAT for bouquet
`0x1000`, and an SDT, and finishes its finite service-list rebuild. It has 42 tasks,
PID `0x52` armed, and NIT/BAT/SDT/TOT match units. The same acquisition sequence
agrees with the unchanged browser oracle; see `plan/test-plan-phase-3a.md`.

`./ctl.sh snapshot inspect post-acquisition` reports state hash `04E99A24` and a
blue baseline surface hash `9825B318`. A raw Sky key (`0x7D`) after restore reaches
the guest input routine twice and draws the Box Office menu by instruction
1,120,000,000: surface hash `F3634409`, 37 distinct bytes, and state hash
`F51114FC`. Reproduce the press with:

```bash
./ctl.sh snapshot run post-acquisition -steps 1120000000 \
  -key 0x7D -key-at 1100000000 -pc-hit 0x8006EA04 -surface-hash -state-hash
```

The built binary restored this 38.8 MB image in **0.74 seconds** on the development
host, measured with `/usr/bin/time` and no compilation in the timing. The first
post-restore key run produced the same framebuffer and state hashes as the source
image's uninterrupted run. `snapshots/post-acquisition.snapshot` is private and
gitignored. On a new host with the verified private firmware in `firmware/`, run
`./ctl.sh snapshot seed`. It performs the cold EEPROM boot, warm SI acquisition,
finite service-list rebuild and Sky-key verification before installing the named
image. It refuses to replace an existing image.

Snapshot format v2 includes the demodulator's indirect pointer and the EEPROM
contents inside the I²C controller. Older v1 images are refused because they omitted
those child devices; the old `post-tdt` and `si-registered` files were removed from
the named library and retained only in private `.artifacts/` for diagnosis.

## Record and replay inputs

`firmwaretrace` can record host inputs at absolute retired-instruction counts and
replay them from the same initial snapshot. A recording contains a SHA-256 digest
of that snapshot, ordered handset and DVB section inputs, the final instruction
count, and a SHA-256 digest of the raw 720×576 framebuffer. Replay refuses a
different snapshot and fails if the final framebuffer bytes differ. Keep these
recordings in `.artifacts/` when they contain private broadcast data.

```bash
./ctl.sh snapshot run post-acquisition -steps 1120000000 \
  -key 0x7D -key-at 1100000000 \
  -record-out .artifacts/sky-key.inputs.json -surface-hash
./ctl.sh snapshot run post-acquisition \
  -replay-in .artifacts/sky-key.inputs.json -surface-hash
```

The recording above restores at 1,100,000,000 instructions, presses Sky and ends
at 1,120,000,000. Its framebuffer SHA-256 is
`1bffc82b335571138a8c80c8589a0da52a4dcb317b634183311e2c4a7a3f0b74`.
The same interface records timed `-section` deliveries; repeat that flag for a
sequence. Record/replay use the default CSI acknowledgement policy, with EEPROM
state supplied by the snapshot.
