# Third-party notices

## Sky/OpenTV UK Huffman dictionary

`dictionaries/skyuk.dict` is derived from `conf/sky_uk.dict` in
[`jcdutton/loadepg`](https://github.com/jcdutton/loadepg), commit
`74267d7089d1b8e4fb1c88f710af1666b9a907ed` (2011-10-02), written by Luca De Pieri and published by
James Courtier-Dutton under the GNU General Public License version 2.

GoRetroTV changes the upstream line ` =0001000` to `=0001000`. This makes that code an empty
terminator entry instead of a duplicate SPACE entry, matching the dictionary parser and the
firmware's decoded output. The modified file is distributed under GPL-2.0 with GoRetroTV; see
`LICENSE` and `dictionaries/MANIFEST.md`.
