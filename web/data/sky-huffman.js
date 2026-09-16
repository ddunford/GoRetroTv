// Sky/OpenTV EPG Huffman ENCODER in JavaScript, so the demo page can build title records from a
// plain-text schedule at runtime instead of carrying byte arrays somebody pasted in.
//
// WHY THIS EXISTS. The records were pre-encoded by scripts/skyepg because the encoder was Python,
// which meant every change to the line-up was a generate-and-paste step and the page carried
// thousands of bytes nobody could read. With this the page fetches a JSON schedule and the
// dictionary, and the line-up becomes a data file you edit.
//
// IT IS A TRANSCRIPTION OF scripts/skyepg/huffman.py, WHICH IS ITSELF VALIDATED against a
// transcription of the reference DECODER. So the chain is: reference decoder <- Python encoder <-
// this. Nothing here is trusted on the strength of looking right -- scripts/skyepg/check_js_encoder.py
// encodes every title in the demo schedule with THIS file and decodes the result with the Python
// decoder, and fails on the first disagreement.
//
// THE TWO THINGS THAT BREAK AN IMPLEMENTATION OF THIS FORMAT, both inherited deliberately:
//
//   * THE DICTIONARY PARSE IS THREE-WAY and the order matters. A line is `<value>=<bits>`; a line
//     beginning with a space maps the SPACE character, and a line beginning with `=` is the
//     TERMINATOR, a node that emits nothing. Splitting naively on '=' mis-parses both.
//   * BYTE 0 CARRIES ONLY SIX BITS, starting at mask 0x20, and every later byte carries eight from
//     0x80. Eight bits in byte 0 produces convincing nonsense rather than an error.

(function (root) {
  'use strict';

  function readDictionary(text) {
    var table = Object.create(null), terminator = null;
    text.split(/\r?\n/).forEach(function (raw) {
      if (!raw) return;
      var eq = raw.indexOf('=');
      if (eq < 0) return;
      var value = raw.slice(0, eq), code = raw.slice(eq + 1);
      if (!code || !/^[01]+$/.test(code)) return;
      if (value === '') { terminator = code; return; }   // the empty-value node
      table[value] = code;
    });
    if (terminator === null) throw new Error('the dictionary has no terminator entry');
    return { table: table, terminator: terminator };
  }

  // Longest match first, exactly as the Python does: the dictionary holds phrases as well as single
  // characters, and a character-only encoding decodes correctly but is needlessly long.
  function encodeBits(text, dict) {
    var lengths = {}, k;
    for (k in dict.table) lengths[k.length] = true;
    var byLen = Object.keys(lengths).map(Number).sort(function (a, b) { return b - a; });
    var bits = [], i = 0;
    while (i < text.length) {
      var matched = false;
      for (var j = 0; j < byLen.length; j++) {
        var chunk = text.substr(i, byLen[j]);
        if (chunk.length === byLen[j] && dict.table[chunk] !== undefined) {
          bits.push(dict.table[chunk]); i += byLen[j]; matched = true; break;
        }
      }
      if (!matched)
        throw new Error('no code for ' + JSON.stringify(text.charAt(i)) + ' at ' + i
                      + ' -- the dictionary cannot express ' + JSON.stringify(text));
    }
    bits.push(dict.terminator);
    return bits.join('');
  }

  function pad(s, n) { while (s.length < n) s += '0'; return s; }

  function pack(bits) {
    var out = [parseInt(pad(bits.slice(0, 6), 6), 2) & 0x3F];   // byte 0: six bits, top two clear
    for (var i = 6; i < bits.length; i += 8) out.push(parseInt(pad(bits.substr(i, 8), 8), 2));
    return out;
  }

  function encodeTitle(text, dict) { return pack(encodeBits(text, dict)); }

  root.SkyHuffman = { readDictionary: readDictionary, encodeTitle: encodeTitle,
                      encodeBits: encodeBits, pack: pack };
})(typeof window !== 'undefined' ? window : globalThis);
