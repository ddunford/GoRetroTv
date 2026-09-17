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

## Verified TDT handoff

The private `post-tdt` image was saved at instruction 470,000,000 after the guest
armed PID 20, 17 and 16 filters and consumed an eight-byte TDT section on PID 20.
The browser oracle and Go both recorded one demux interrupt, one section-task entry,
two length-reader entries, a nine-byte ring advance and cleared interrupt status.
The image's restored state hash is `C918AA06`; `./ctl.sh snapshot inspect post-tdt`
reports it without running any instructions. The image is an intermediate SI state;
NIT and SDT acquisition still need their own guest proof.

The private `si-registered` image is a later, clean state at instruction 650,000,000.
Its restored state hash is `32D1E63B`. By then the guest has polled demodulator
register 11 and programmed a NIT match for network `0x0020` plus a TOT match.
It has received no broadcast sections. Use it for controlled SI input experiments;
the guest's filters, rather than a guessed network ID, determine which sections
should be sent.
