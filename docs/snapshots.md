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
