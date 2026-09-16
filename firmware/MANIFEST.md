# Firmware — what belongs here, and why it is not in git

These are **Pace's**, not ours. They are gitignored and must never be committed or baked into a
published image. Nothing in this repository runs without them.

`internal/firmware` reads this file and refuses to start the machine unless every image below is
present and matches. The table is parsed, not decorative: the columns and their order are the
format, and a digest recorded as anything other than its full length is rejected by name rather
than skipped. It used to hold truncated MD5s (`7541fb4884d7…`), which look like a record and verify
nothing — running a different ROM silently is the failure all of this exists to prevent.

**SHA-256 is the integrity check. MD5 is kept as the provenance identifier**, because that is how
these images are named in the outside world and in our own record: `docs/reference/digibox-emulation.md`
opens by identifying `FLASH_U202.bin` as md5 `7541fb4884d72b03858f7175217c2177`, Colibri's published
JTAG dump of a Pace 2500N. Dropping the MD5 would break the link between this repository and the
only public account of where the image came from.

| File | Bytes | SHA-256 | MD5 | What it is |
|---|---|---|---|---|
| `FLASH_U202.bin` | 2097152 | `32f6b2e84d0c4c4ec280a72f2d080e4b6fa5e68e8438ef712f34f97f63d38c7f` | `7541fb4884d72b03858f7175217c2177` | The Pace 2500N's first flash chip: the bootloader at `0x0`–`0x1FFFF` and the application image from `0x20000`. The EPG's o-code CODE chunk lives at `0x9FC4A400` (353,188 bytes) and its resources directly above. |
| `FLASH_U203.bin` | 2097152 | `7832d619b63e369bdd4a0f3de8dd73c98a99285d96c94a8bb0a39fe829ce1f40` | `723840c66d194e1a2689d5f754043f95` | The second flash chip. The board has two and the emulator models both; `?u203=0` unmapped it in the browser version, which is how its absence was told apart from its contents. |
| `application-ram-image.bin` | 1029144 | `a77bdacde0c9c35d8ac8d48d8b01fef382681e617233e8fc5a7eaad5ae7b7428` | `b220ead81b5a0deebef6108c416dc44d` | The application **after** the bootloader decompresses it, based at `0x800009F4`. Not a separate artefact of the machine — a capture, kept because Ghidra needs it alongside the flash: half of every function is unreadable without both. |

The MD5s above were measured here on 2026-09-16 and `FLASH_U202.bin`'s agrees with the independent
copy recorded in `docs/reference/digibox-emulation.md`. The SHA-256s were measured at the same time
and have no earlier record to agree with, which is worth knowing: they check that the image has not
changed since, not that it is the same image somebody else has.

**Why the RAM image is kept rather than regenerated.** It can be reproduced by running the
bootloader, but every Ghidra seed, every pool-word resolution and every address in
`docs/reference/digibox-emulation.md` is expressed against this exact copy. Regenerating it risks a
different layout and silently invalidating six thousand lines of measured addresses.
