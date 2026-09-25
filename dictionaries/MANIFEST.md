# Dictionaries — provenance and format

The Sky/OpenTV EPG carries its programme text Huffman-compressed against a dictionary the box
already holds. Writing that stream — which is what a listings feed has to do — needs the same
dictionary. The included UK dictionary comes from the GPL-2.0 `jcdutton/loadepg` project and may be
redistributed under those terms.

The exact source is `jcdutton/loadepg`, commit `74267d7089d1b8e4fb1c88f710af1666b9a907ed`,
`conf/sky_uk.dict`. GoRetroTV removes the leading space from its ` =0001000` entry so that it is
parsed as the empty terminator rather than another SPACE leaf. That modified form has SHA-256
`c05d288d9d22403f97e5caade329579c5124cb180790bf726e62cca792cb300b`. The upstream project and this
modified copy are distributed under GPL-2.0; see `LICENSE` and `THIRD_PARTY_NOTICES.md`.

## What must be here

| File | Bytes | Lines | Entries | SHA-256 |
|---|---:|---:|---:|---|
| `skyuk.dict` | 13896 | 512 | 447 | `c05d288d9d22403f97e5caade329579c5124cb180790bf726e62cca792cb300b` |

**512 lines, 447 codeable values, and the gap is the point.** Sixty-five lines map SPACE, one line
maps the terminator, and the rest map a value each. The duplicates are NOT alternative spellings:
space is coded `110` in three bits and then appears sixty-two more times as twenty-seven-bit leaves,
which are the flattened tree's padding. **A value's real code is the SHORTEST one**, and a loader
that keeps the last duplicate emits a filler the box does not read back as a space — which is how
every title on the guide lost its spaces.

The count was recorded as 446 until 2026-09-20. It is 447 because the line `==<bits>`, the code for
the equals character, was being read as a malformed empty value and dropped; reading a single
character before anything else, as below, recovers it.

## The format, read off openTVtoXML's `huffman_read_dictionary()` rather than guessed

One entry per line, `<value>=<bits>`, parsed by three attempts **in this order**:

    "%c=%[^\n]"        a single character — so a line beginning with a space maps SPACE
    "%[^=]=%[^\n]"     a multi-character phrase
    "=%[^\n]"          an EMPTY value: the terminator node, which emits nothing

The ordering is load-bearing. Splitting naively on `=` mis-parses both the space line and any
phrase containing `=`.

## Alternative source

The strings are also plain and NUL-terminated at `0xC8408` in `FLASH_U202.bin`, alphabetised,
though the code table after them is undocumented and nobody has reversed it yet.
