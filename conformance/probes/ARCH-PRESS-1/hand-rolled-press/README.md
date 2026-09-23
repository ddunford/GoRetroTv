# A probe with its own settle loop

Adds a firmware probe that counts identical screen samples itself instead of calling
`pressAndLetItFinish`. `ARCH-PRESS-1` must report `press-loop-outside-helper`.

This is the exact shape that broke thirty-one probes on 2026-09-23: a settle loop with no paint
tail returns a half-drawn menu, the next press lands in it and is swallowed, and the probe reports
`drew 00000000` for a screen that was drawing perfectly well. Forty-eight copies of it existed and
the rule against them had been written down twice.
