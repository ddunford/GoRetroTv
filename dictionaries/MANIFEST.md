# Dictionaries — what belongs here, and why it is not in git

The Sky/OpenTV EPG carries its programme text Huffman-compressed against a dictionary the box
already holds. Writing that stream — which is what a listings feed has to do — needs the same
dictionary, and **this one is not ours to redistribute**.

Every public implementation that ships such a file (`jcdutton/loadepg`, `dave-p/openTVtoXML`,
`vdr-projects/vdr-plugin-eepg`, `tvheadend`) is GPL, and the copy in use here carries no licence
header, so its provenance cannot be established from the file itself. It is therefore handled the
way the flash images are: **required, local, gitignored, never committed and never baked into a
published image.** `internal/broadcast` builds title sections only when it is present, and its
tests skip rather than fail when it is not — the same contract the firmware tests use.

## What must be here

| File | Bytes | Lines | Entries | SHA-256 |
|---|---:|---:|---:|---|
| `skyuk.dict` | 13896 | 512 | 446 | `c05d288d9d22403f97e5caade329579c5124cb180790bf726e62cca792cb300b` |

## The format, read off openTVtoXML's `huffman_read_dictionary()` rather than guessed

One entry per line, `<value>=<bits>`, parsed by three attempts **in this order**:

    "%c=%[^\n]"        a single character — so a line beginning with a space maps SPACE
    "%[^=]=%[^\n]"     a multi-character phrase
    "=%[^\n]"          an EMPTY value: the terminator node, which emits nothing

The ordering is load-bearing. Splitting naively on `=` mis-parses both the space line and any
phrase containing `=`.

## Getting one

It is not downloaded by anything here on purpose. Take it from a GPL EPG reader that ships one, or
extract it from your own box's flash — the strings are plain and NUL-terminated at `0xC8408` in
`FLASH_U202.bin`, alphabetised, though the code table after them is undocumented and nobody has
reversed it yet. Doing so would remove the third-party dependency entirely and is worth its own
task.
