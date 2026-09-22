# Firmware instruments

<!-- anchor: internal/instruments/instruments.go -->
<!-- anchor: internal/platform/instrument/instrument.go -->
<!-- fingerprint: sha256:2cfe9906589cbedf60ef3cdef1dcd8d5cfa795f311965b441bf988590dc52279 @ 2026-09-22 -->

`bin/firmwaretrace` can inspect a restored machine without changing guest state. Build it with `./ctl.sh build`. The `-steps` value is an **absolute instruction count**: for `snapshots/post-acquisition.snapshot` at 1,100,000,000 instructions, `-steps 1120000000` observes the next 20 million.

The following command exercises each collector during a real Sky-key run. Instrument output goes to stderr; checkpoint output goes to stdout. The `-instrument-max` cap limits detailed records while totals keep counting.

```sh
bin/firmwaretrace \
  -snapshot-in snapshots/post-acquisition.snapshot \
  -steps 1120000000 -interval 20000000 \
  -key 0x7D -key-at 1100000000 \
  -pc-hist -pc-range 0x800D35E0:0x800D35F0 -pc-control 0x800D35E0 \
  -read-watch 0x801072D8:0x801072DC \
  -write-watch 0x80000000:0x82000000 \
  -call-trace 0x800D35E0 -ocode-trace -instrument-max 4
```

PC ranges, address watches, control PCs and call PCs can be repeated. Bounds are half-open. An address watch accepts an optional executing-PC window as `lo:hi:fromPC:toPC`. DRAM watches match cached and uncached aliases and report the guest's actual virtual address. Instruction fetches are excluded from data-read watches. The o-code collector watches byte reads in the OpenTV CODE chunk from the interpreter fetch instruction; it requires both that instruction and a matching byte read. Section injection remains available through repeatable `-section instruction:pid:hex` and the single-section `-section-at`, `-section-pid`, `-section-hex` flags.

Every requested collector requires guest instructions in its observation window. Each PC range, address watch, call target and o-code trace also requires a matching subject. An absent subject exits with a harness error instead of reporting a misleading zero. For example, `-steps 1100000000 -pc-hist` on the snapshot above exits with `instrument examined nothing`; `-steps 1100000001 -read-watch 0x81000000:0x81000004` exits with the same error for its missing address range.

The real 1.1B→1.12B run produced 20,000,000 examined instructions; 7,141,157 hits in the selected PC range; 1,785,165 watched reads; 766,212 watched writes; 1,785,759 call samples; and 26,332 o-code reads. The four-record cap was respected for every detailed log. Its local captures are `.artifacts/instruments-real-1120m.log` and `.artifacts/instruments-real-1120m.stderr`.
