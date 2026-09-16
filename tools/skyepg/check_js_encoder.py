#!/usr/bin/env python3
"""Prove the JavaScript encoder agrees with the reference decoder, title by title.

AN ENCODER VALIDATED AGAINST ITSELF PROVES NOTHING. scripts/skyepg/huffman.py earns its keep by
round-tripping through a transcription of openTVtoXML's DECODER; sky-huffman.js is a transcription
of that encoder, so it has to earn the same thing independently -- otherwise "it looks like the
Python" is the whole argument, and this format's two traps (the three-way dictionary parse and byte
0 carrying six bits) both produce plausible output rather than an error.

So: encode every title in the demo schedule with NODE, decode each result with the PYTHON decoder,
and fail on the first disagreement. Exit 1 on any mismatch, and exit 2 if nothing was checked --
a checker that silently examines zero titles looks exactly like one that found no faults.
"""
import json
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from huffman import read_dictionary, build_tree, decode, DEFAULT_DICT

SCHEDULE = HERE.parent.parent / 'frontend/public/digibox/listings.json'
# The encoder's single home is the directory the PAGE serves it from. A second copy under
# scripts/skyepg would be the one this checker validates while the page loaded the other.
ENCODER = HERE.parent.parent / 'frontend/public/digibox/sky-huffman.js'


def titles_from(schedule):
    for channel in schedule['channels']:
        for prog in channel['programmes']:
            yield channel['name'], prog['title']


def main():
    if not SCHEDULE.exists():
        sys.exit('%s does not exist -- nothing to check' % SCHEDULE)
    schedule = json.loads(SCHEDULE.read_text())
    titles = sorted({t for _, t in titles_from(schedule)})
    if not titles:
        print('the schedule holds no titles -- refusing to report success over an empty set')
        return 2

    # One node process for the whole set: it reads the dictionary the PAGE reads, not a copy.
    script = '''
      const fs = require('fs');
      require(%s);
      const dict = SkyHuffman.readDictionary(fs.readFileSync(%s, 'latin1'));
      const titles = JSON.parse(fs.readFileSync(0, 'utf8'));
      console.log(JSON.stringify(titles.map(t => SkyHuffman.encodeTitle(t, dict))));
    ''' % (json.dumps(str(ENCODER)), json.dumps(str(HERE.parent.parent / 'frontend/public/digibox/skyuk.dict')))
    proc = subprocess.run(['node', '-e', script], input=json.dumps(titles),
                          capture_output=True, text=True)
    if proc.returncode != 0:
        sys.exit('the JavaScript encoder failed: %s' % proc.stderr.strip())
    encoded = json.loads(proc.stdout)

    tree = build_tree(*read_dictionary(DEFAULT_DICT))
    bad = []
    for title, blob in zip(titles, encoded):
        text, terminated = decode(bytes(blob), tree)
        if not terminated or text != title:
            bad.append((title, text, terminated))
    print('%d distinct titles encoded in JavaScript and decoded by the reference decoder' % len(titles))
    if bad:
        for title, got, term in bad:
            print('  MISMATCH %r -> %r (terminated=%s)' % (title, got, term))
        return 1
    print('every one round-tripped unchanged')
    return 0


if __name__ == '__main__':
    sys.exit(main())
