#!/usr/bin/env python3
"""Build a Sky/OpenTV EPG TITLE section — the thing we push to make the guide show programmes.

THE LAYOUT, taken from openTVtoXML's opentv_read_titles() rather than from a guess, and confirmed
against what this box actually reads (it read offsets 2,3,4,5,6,7 of a self-naming payload, which
is event id / length / tag / first data byte — the record, not a descriptor loop):

    data[0]      table id: titles are 0xA0..0xA4 and 0xB0  (summaries 0xA8..0xAB, 0xB1)
    data[1..2]   0xB0 | length hi, length lo   (DVB section syntax, length counts from data[3])
    data[3..4]   CHANNEL ID   <- the value fed in BAT 0xB1 entry +3..4
    data[5]      version / current_next
    data[6..7]   section_number / last_section_number
    data[8..9]   the two bytes the hardware match unit filters on
    data[10..]   records, each advancing by LENGTH + 4:

      r+0..1   event id
      r+2..3   flags in bits 7 and 6, then a 12-bit LENGTH: ((r[2] & 0x0f) << 8) | r[3]
      r+4      0xB5                    <- the tag; the reader BREAKS if it is not 0xB5
      r+5      descriptor length; the text is (this - 7) bytes
      r+6..7   start time, seconds into the day = value * 2
      r+8..9   duration,   seconds            = value * 2
      r+10     genre id
      r+11     unread by the reference reader
      r+12     rating, low nibble
      r+13..   Huffman-compressed title

THE LENGTH IS THE BODY, AND THE READER ADVANCES BY LENGTH + 4 -- WHICH IS NOT WHAT openTVtoXML
DOES. Its loop advances by packet_length alone, so a writer following it emits records four bytes
too long and this box desynchronises after the first one: it lands inside record 2, reads the
descriptor tag as a length, and runs off the end of the section. Measured rather than reasoned --
a payload of TWELVE records was walked as TWO, twenty times over, and the firmware's own loop
(FUN_800c95d0, `local_84 += local_8a + 4`, with `memcpy(dst, section + local_84 + 4, local_8a)`)
says the same thing: the field counts the bytes AFTER the four-byte header. Where the reference
implementation and the running box disagree, the box is the specification; the divergence is
recorded here so nobody re-derives the reference's version and calls it a fix.

DATA[8..9] IS STAMPED FROM THE MATCH UNIT, NOT COMPUTED. The reference implementations call it the
MJD, and 0x9E8B is indeed MJD 40587 (1 Jan 1970, the epoch of a box with no clock). But a live test
on this box did NOT move those bytes when the clock moved, and that test could not separate "not the
date" from "the unit is programmed once, before I changed it". Rather than ship a theory, the
builder takes the two bytes as an argument so the caller can read them off __siMatches() and stamp
exactly what the box says it will accept. Whatever they mean, matching them is what gets the section
through — and this is the same "let the box tell us" method that found every other rung here.
"""
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parent))
from huffman import encode_title

CRC_POLY = 0x04C11DB7


def crc32_mpeg(data):
    crc = 0xFFFFFFFF
    for byte in data:
        crc ^= byte << 24
        for _ in range(8):
            crc = ((crc << 1) ^ CRC_POLY) & 0xFFFFFFFF if crc & 0x80000000 else (crc << 1) & 0xFFFFFFFF
    return crc


def title_record(event_id, start_seconds, duration_seconds, title, genre=0x00, rating=0):
    """One programme. Times are seconds; the wire carries them halved, so they quantise to 2s."""
    if start_seconds % 2 or duration_seconds % 2:
        raise ValueError('start and duration are carried halved, so both must be even seconds')
    text = encode_title(title)
    body = bytes([
        (start_seconds // 2) >> 8 & 0xFF, (start_seconds // 2) & 0xFF,
        (duration_seconds // 2) >> 8 & 0xFF, (duration_seconds // 2) & 0xFF,
        genre & 0xFF,
        0x00,
        rating & 0x0F,
    ]) + text
    # descriptor length counts the seven fixed bytes plus the text; the reader recovers the text
    # length as (this - 7), which is where the 7 comes from.
    desc = bytes([0xB5, len(body)]) + body
    # The length counts the descriptor ONLY -- the reader adds the four header bytes itself. See
    # the divergence from openTVtoXML in this module's docstring; it is the whole reason a
    # well-formed section used to arrive as two records instead of twelve.
    length = len(desc)
    if length > 0x0FFF:
        raise ValueError('record too long for a 12-bit length')
    return bytes([
        event_id >> 8 & 0xFF, event_id & 0xFF,
        0xF0 | (length >> 8 & 0x0F), length & 0xFF,
    ]) + desc


def title_section(channel_id, filter_bytes, records, table_id=0xA1, version=0,
                  section_number=0, last_section_number=0):
    """filter_bytes: the two bytes the box's match unit demands at data[8..9]."""
    if len(filter_bytes) != 2:
        raise ValueError('filter_bytes must be exactly the two bytes at data[8..9]')
    payload = bytes([
        channel_id >> 8 & 0xFF, channel_id & 0xFF,
        0xC1 | ((version & 0x1F) << 1),
        section_number & 0xFF, last_section_number & 0xFF,
        filter_bytes[0] & 0xFF, filter_bytes[1] & 0xFF,
    ]) + b''.join(records)
    length = len(payload) + 4              # + CRC, counted from data[3]
    if length > 0x0FFD:
        raise ValueError('section too long — split across section_number')
    head = bytes([table_id & 0xFF, 0xB0 | (length >> 8 & 0x0F), length & 0xFF]) + payload
    return head + crc32_mpeg(head).to_bytes(4, 'big')


if __name__ == '__main__':
    # A worked example, checked field by field against the layout above rather than eyeballed.
    recs = [
        title_record(0x0001, 19 * 3600,          30 * 60, 'The Simpsons'),
        title_record(0x0002, 19 * 3600 + 30 * 60, 60 * 60, 'Sky News'),
    ]
    sec = title_section(0x0BBB, (0x9E, 0x8B), recs)
    print('section: table 0x%02X, %d bytes, %d records' % (sec[0], len(sec), len(recs)))
    print('  data[3..4] channel id   = 0x%04X' % ((sec[3] << 8) | sec[4]))
    print('  data[8..9] filter bytes = %02X %02X' % (sec[8], sec[9]))
    print('  declared length         = %d (want %d)' % (((sec[1] & 0x0F) << 8) | sec[2], len(sec) - 3))
    assert (((sec[1] & 0x0F) << 8) | sec[2]) == len(sec) - 3, 'section_length disagrees with the real length'
    assert crc32_mpeg(sec[:-4]) == int.from_bytes(sec[-4:], 'big'), 'CRC does not verify'

    # Walk it back with THE BOX'S arithmetic. An encoder validated only against itself proves
    # nothing -- and an encoder validated against the wrong reader proves less than nothing, because
    # it passes. This loop is openTVtoXML's, transcribed, with the one step the firmware does
    # differently: `off += length + 4`. Walked the reference's way this same section yields TWO
    # records and the assertion below fails, which is what makes it an instrument.
    from huffman import read_dictionary, build_tree, decode, DEFAULT_DICT
    tree = build_tree(*read_dictionary(DEFAULT_DICT))
    off, seen = 10, 0
    while off + 11 < len(sec):
        packet_length = ((sec[off + 2] & 0x0F) << 8) | sec[off + 3]   # the DESCRIPTOR length
        if sec[off + 4] != 0xB5:
            break
        event_id = (sec[off] << 8) | sec[off + 1]
        o = off + 4
        text_len = sec[o + 1] - 7
        start = (sec[o + 2] << 9) | (sec[o + 3] << 1)
        dur = (sec[o + 4] << 9) | (sec[o + 5] << 1)
        text, terminated = decode(sec[o + 9:o + 9 + text_len], tree)
        print('  event 0x%04X  %02d:%02d  %3d min  %-14r %s'
              % (event_id, start // 3600, start % 3600 // 60, dur // 60, text,
                 'ok' if terminated else 'NOT TERMINATED'))
        seen += 1
        off += packet_length + 4
    assert seen == len(recs), 'the reference reader recovered %d of %d records' % (seen, len(recs))
    print('\nthe reference reader recovered every record — the builder agrees with the format')


def summary_record(event_id, summary):
    """One programme's synopsis, from openTVtoXML's opentv_read_summaries().

        [event id:2][packet_length:2] then descriptors, each [tag][len][data]
        tag 0xB9 carries Huffman text, and MULTIPLE 0xB9 descriptors CONCATENATE

    The reader looks the summary up by (event_id, mjd) against a title it already holds, so a
    summary for an event the box has never seen a title for has nothing to attach to.
    """
    text = encode_title(summary)
    if len(text) > 0xFF:
        raise ValueError('summary text needs splitting across several 0xB9 descriptors')
    desc = bytes([0xB9, len(text)]) + text
    if len(desc) > 0x0FFF:
        raise ValueError('record too long for a 12-bit packet_length')
    return bytes([
        event_id >> 8 & 0xFF, event_id & 0xFF,
        0xF0 | (len(desc) >> 8 & 0x0F), len(desc) & 0xFF,
    ]) + desc


def summary_section(channel_id, filter_bytes, records, table_id=0xA8, version=0):
    """Summaries are tables 0xA8..0xAB and 0xB1; the header is the title header's shape."""
    return title_section(channel_id, filter_bytes, records, table_id=table_id, version=version)
