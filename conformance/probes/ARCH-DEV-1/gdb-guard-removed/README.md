# Remove the GDB listener's non-loopback refusal

Disables the actual `gdbstub.Listen` guard while leaving its socket creation intact. The bind
checker must observe `0.0.0.0:0` and `:0` being accepted and report
`gdb-non-loopback-accepted` independently of the HTTP loader's detector.
