#!/usr/bin/env python3
"""Sky/OpenTV EPG Huffman codec, built from the published Sky UK dictionary.

WHY A CODEC AND NOT JUST A DECODER. Every public implementation of this format READS the stream;
we need to WRITE one, which means encoding. An encoder has a property a decoder does not: it can be
validated against the decoder it is meant to feed. So this file carries both halves and a
round-trip check, and nothing here is trusted until a string survives encode -> decode unchanged.

THE DICTIONARY FORMAT, read off openTVtoXML's huffman_read_dictionary() rather than guessed:
each line is `<value>=<bits>`, parsed by three sscanf attempts in order --
    "%c=%[^\\n]"        a single character (so a line starting with a space maps the SPACE char)
    "%[^=]=%[^\\n]"     a multi-character phrase
    "=%[^\\n]"          an EMPTY value: a node that emits nothing and resets to the root
That ordering matters: a naive split on '=' mis-parses both the space line and any phrase
containing '='.

THE BIT PACKING IS THE PART THAT WILL CATCH PEOPLE. The decoder starts the FIRST byte at mask 0x20
and every later byte at 0x80 -- so byte 0 contributes only its low SIX bits and its top two are
skipped entirely. That is not a typo in the reference implementation; it is the format.
"""
import sys
from pathlib import Path

DEFAULT_DICT = Path(__file__).resolve().parent / 'skyuk.dict'


def read_dictionary(path):
    """value -> bits, mirroring huffman_read_dictionary()'s three-way parse."""
    table, terminator = {}, None
    for raw in Path(path).read_text(encoding='latin-1').splitlines():
        if not raw or '=' not in raw:
            continue
        value, _, code = raw.partition('=')
        if not set(code) <= {'0', '1'} or not code:
            continue
        if value == '':
            terminator = code            # the empty-value node: emits nothing, resets to root
            continue
        table[value] = code
    if terminator is None:
        raise SystemExit('dictionary has no empty-value (terminator) entry -- refusing to guess one')
    return table, terminator


def build_tree(table, terminator):
    """The decoder's view: a binary tree of bits -> value."""
    root = {}
    for value, code in list(table.items()) + [('', terminator)]:
        node = root
        for b in code:
            node = node.setdefault(b, {})
        if 'v' in node and node['v'] != value:
            raise SystemExit('dictionary is ambiguous: %r and %r share a code' % (node['v'], value))
        node['v'] = value
    return root


def encode(text, table, terminator):
    """Longest-match-first, because the dictionary holds phrases as well as characters and a
    character-only encoding would still decode correctly but be needlessly long."""
    by_len = sorted({len(k) for k in table}, reverse=True)
    bits, i = [], 0
    while i < len(text):
        for n in by_len:
            chunk = text[i:i + n]
            if chunk in table:
                bits.append(table[chunk])
                i += n
                break
        else:
            raise SystemExit('no code for %r at %d -- the dictionary cannot express this string' % (text[i], i))
    bits.append(terminator)
    return ''.join(bits)


def pack(bits):
    """Bits -> bytes, with the format's first-byte quirk: byte 0 carries only six bits."""
    first, rest = bits[:6], bits[6:]
    out = [int(first.ljust(6, '0'), 2) & 0x3F]      # top two bits left clear
    for i in range(0, len(rest), 8):
        out.append(int(rest[i:i + 8].ljust(8, '0'), 2))
    return bytes(out)


def unpack(data):
    """Bytes -> bits, mirroring the decoder: first byte from mask 0x20, the rest from 0x80."""
    bits = []
    for i, byte in enumerate(data):
        mask = 0x20 if i == 0 else 0x80
        while mask:
            bits.append('1' if byte & mask else '0')
            mask >>= 1
    return ''.join(bits)


def decode(data, tree):
    """Exactly what the box's decoder does, so a round trip proves the encoder."""
    out, node = [], tree
    for bit in unpack(data):
        node = node.get(bit)
        if node is None:
            return ''.join(out), False       # ran off the tree: the encoder is wrong
        if 'v' in node:
            if node['v'] == '':
                return ''.join(out), True    # terminator
            out.append(node['v'])
            node = tree
    return ''.join(out), False               # ran out of bits without terminating


def encode_title(text, dict_path=DEFAULT_DICT):
    table, terminator = read_dictionary(dict_path)
    return pack(encode(text, table, terminator))


if __name__ == '__main__':
    path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_DICT
    table, terminator = read_dictionary(path)
    tree = build_tree(table, terminator)
    print('dictionary %s: %d entries, terminator %r' % (path, len(table), terminator))

    # THE ROUND TRIP IS THE WHOLE POINT. Titles from the era, plus the cases most likely to break an
    # encoder: a space-led string, punctuation, digits, and a phrase that should match a multi-char
    # dictionary entry rather than being spelled out.
    samples = ['The Simpsons', 'Sky News', 'Football', 'News at Ten',
               'The X-Files', 'Coronation Street', 'A', ' leading space', 'Digits 1998']
    bad = 0
    for s in samples:
        try:
            packed = pack(encode(s, table, terminator))
        except SystemExit as e:
            print('  ENCODE FAILED %-20r %s' % (s, e)); bad += 1; continue
        got, terminated = decode(packed, tree)
        ok = (got == s and terminated)
        if not ok: bad += 1
        print('  %-20r -> %2d bytes -> %-22r %s' % (s, len(packed), got, 'ok' if ok else 'MISMATCH'))
    print('\n%d of %d round-tripped' % (len(samples) - bad, len(samples)))
    sys.exit(1 if bad else 0)
