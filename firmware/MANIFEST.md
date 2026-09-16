# Firmware — what belongs here, and why it is not in git

These are **Pace's**, not ours. They are gitignored and must never be committed or baked into a
published image. Nothing in this repository runs without them.

| File | Bytes | MD5 | What it is |
|---|---|---|---|
| `FLASH_U202.bin` | 2097152 | `7541fb4884d7…` | The Pace 2500N's first flash chip: the bootloader at `0x0`–`0x1FFFF` and the application image from `0x20000`. The EPG's o-code CODE chunk lives at `0x9FC4A400` (353,188 bytes) and its resources directly above. |
| `FLASH_U203.bin` | 2097152 | `723840c66d19…` | The second flash chip. The board has two and the emulator models both; `?u203=0` unmapped it in the browser version, which is how its absence was told apart from its contents. |
| `application-ram-image.bin` | 1029144 | `b220ead81b5a…` | The application **after** the bootloader decompresses it, based at `0x800009F4`. Not a separate artefact of the machine — a capture, kept because Ghidra needs it alongside the flash: half of every function is unreadable without both. |

Full checksums are in the archived predecessor at
`/opt/workspaces/development/archive/skytv.demosrv.uk-2026-09-16`.

**Why the RAM image is kept rather than regenerated.** It can be reproduced by running the
bootloader, but every Ghidra seed, every pool-word resolution and every address in
`docs/reference/digibox-emulation.md` is expressed against this exact copy. Regenerating it risks a
different layout and silently invalidating six thousand lines of measured addresses.
