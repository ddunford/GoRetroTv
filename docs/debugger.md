# Firmware debugger

<!-- anchor: internal/gdbstub/server.go -->
<!-- fingerprint: sha256:521a61054802db240d86ffb8bf1ab58fa0ae68823c3a412dceb77b7a47a9d6ab @ 2026-09-22 -->

The diagnostic runner exposes one all-stop GDB Remote Serial Protocol session.
It drives the same single-threaded instruction advance as a normal firmware run.
The listener accepts only literal loopback addresses; it refuses `0.0.0.0` and
other public binds. Start from the private post-acquisition snapshot to inspect
a pressable machine without waiting for the cold boot:

```bash
./ctl.sh build
bin/firmwaretrace -snapshot-in snapshots/post-acquisition.snapshot \
  -gdb-addr 127.0.0.1:2345 -interval 2000000000 -state-hash
```

In a second local terminal, use a GDB build with MIPS support. There is no
ELF file for this raw flash image, so set the architecture and byte order
before attaching:

```text
gdb-multiarch -q
(gdb) set architecture mips
(gdb) set endian big
(gdb) target remote 127.0.0.1:2345
(gdb) info registers pc sp
(gdb) x/1wx 0x80000000
(gdb) break *0x800d35e4
(gdb) continue
(gdb) stepi
(gdb) detach
```

The sample breakpoint address is from the current private post-acquisition
snapshot; use `info registers pc` to choose an address for another image.
Breakpoints compare guest PCs without patching firmware. Read, write and
access watchpoints observe guest bus operations during CPU execution. A GDB
memory command can itself read or write emulated devices, so prefer DRAM or
flash addresses for inspection. Detach ends the diagnostic process after it
prints its final instruction count and any requested hashes.

The real-client acceptance run attached `gdb-multiarch`, read PC `0x800D35E0`
and SP `0x80128E7C`, read memory at `0x80000000`, continued to a breakpoint
at `0x800D35E4`, single stepped to `0x800D35E8`, and detached. The session
retired two guest instructions. A second GDB session set a read watchpoint on
`0x801072D8` and stopped when the firmware loaded that address. A request for
`-gdb-addr 0.0.0.0:23457` was rejected before opening a listener. Full client
output is in private `.artifacts/gdb-real-client.log` and
`.artifacts/gdb-watch-client.log`.

The protocol follows GDB's [remote protocol](https://sourceware.org/gdb/current/onlinedocs/gdb.html/Remote-Protocol.html)
and [MIPS register packet format](https://www.sourceware.org/gdb/current/onlinedocs/gdb.html/MIPS-Register-packet-Format.html).
