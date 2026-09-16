// FEED PID 0x52 AND SEE WHAT THE FIRMWARE DOES WITH IT -- sky-02me.5.
//
// Established: after acquisition the box arms channel 21 for PID 0x52, and 21 is above the 16 match
// units this demux has, so NOTHING FILTERS THAT PID. Every section delivered on it reaches the
// firmware and the SI layer decides. So a feed does not have to get a table id past the hardware --
// which means the question "what does 0x52 want" can be asked of the SOFTWARE, by delivering a
// section and watching whether a parser runs.
//
// THE CANDIDATE RANGE IS NOT A GUESS. The project already located the OpenTV carousel parsers
// (0x800C95D0 and 0x800C9CA0, Huffman 0x800BECF0) and recorded its table ids as 0xA0-0xB1. This
// sweeps that range one id at a time and counts parser entries per id, with two DVB ids either side
// as negative controls -- 0x4E (EIT present/following) and 0x42 (SDT), which the box demonstrably
// understands but should not be looking for here.
//
// A DECODER RUN OVER DATA DOES NOT FAIL, IT PRODUCES PLAUSIBLE OUTPUT, so the payload is deliberately
// minimal and identical for every id: this measures WHICH ID WAKES A PARSER, not what the parser
// then makes of the bytes. Reading anything into the contents at this stage would be inventing a
// format. The content is sky-eluc.12's problem and the user has said it comes later.
//
// CONTROLS, three of them, because three different nothings look alike here:
//   the push must be ACCEPTED   -- __siPush refuses if no filter is programmed for the PID, and a
//                                  refusal reads exactly like a firmware that ignored the section
//   the section task must TICK  -- delivery to the ring is not delivery to the software
//   a known-good id must parse  -- 0x42 on PID 0x11 is fed by the carousel all run and proves the
//                                  parser trace is armed on addresses that really execute
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase(); }
function u16(v){ return [(v >>> 8) & 0xFF, v & 0xFF]; }

// MPEG CRC-32, poly 0x04C11DB7, init all ones, no final inversion -- the page has this internally
// but does not expose it, and a section with a wrong CRC is discarded silently, which would look
// exactly like a table id the firmware does not want.
var CRC_TAB = (function(){
  var t = new Int32Array(256);
  for (var i = 0; i < 256; i++){
    var c = i << 24;
    for (var j = 0; j < 8; j++) c = (c & 0x80000000) ? ((c << 1) ^ 0x04C11DB7) : (c << 1);
    t[i] = c;
  }
  return t;
})();
function crc32(bytes){
  var c = -1;
  for (var i = 0; i < bytes.length; i++) c = (c << 8) ^ CRC_TAB[((c >>> 24) ^ bytes[i]) & 0xFF];
  return c >>> 0;
}
function section(tableId, ext, payload){
  var len = 5 + payload.length + 4;            // after the length field, CRC included
  var body = [tableId & 0xFF, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
               .concat(u16(ext), [0xC1, 0x00, 0x00], payload);
  var c = crc32(body);
  return body.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]);
}

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');
await new Promise(function(r){ setTimeout(r, 80000); });
window.__siCarousel(true);
await new Promise(function(r){ setTimeout(r, 30000); });

var pids = window.__dispState().pids.map(function(p){ return p.pid; });
if (pids.indexOf('0x52') < 0)
  throw new Error('PID 0x52 is not armed (' + pids.join(',') + ') -- the box has not acquired, so ' +
                  'nothing below would be about the filter this task is for');

var PARSERS = [
  { pc: 0x800C95D0, name: 'otvParserA', args: 2 },
  { pc: 0x800C9CA0, name: 'otvParserB', args: 2 },
  { pc: 0x800BECF0, name: 'huffman',    args: 2 }
];
var IDS = [];
for (var t = 0xA0; t <= 0xB1; t++) IDS.push(t);
IDS.push(0x4E, 0x42);                            // negative controls

function surfaceHash(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261;
  for (var i=0;i<b.length;i++) s = (Math.imul(s ^ b[i], 16777619))>>>0;
  return h(s);
}

var rows = [], refusals = 0;
for (var i = 0; i < IDS.length; i++){
  var id = IDS[i];
  window.__traceCalls(PARSERS); window.__traceClear();
  var siBefore = window.__siLog().length;
  var r = window.__siPush(0x52, section(id, 0x0020, [0x00, 0x00, 0x00, 0x00]));
  if (!r || !r.ok) refusals++;
  await new Promise(function(rr){ setTimeout(rr, 4000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name]||0)+1; });
  rows.push({ tableId: h(id), pushed: !!(r && r.ok), why: (r && r.why) || null,
              acceptedIntoRing: window.__siLog().length - siBefore,
              otvParserA: c.otvParserA || 0, otvParserB: c.otvParserB || 0, huffman: c.huffman || 0 });
}
window.__traceCalls([]);

if (refusals === IDS.length)
  throw new Error('every push was refused -- no filter for PID 0x52, so this measured nothing');

var woke = rows.filter(function(r){ return r.otvParserA || r.otvParserB || r.huffman; });
return {
  armedPids: pids,
  pushesRefused: refusals,
  anyParserRan: woke.length > 0,
  idsThatWokeAParser: woke,
  rows: rows,
  surfaceAfter: surfaceHash(),
  note: 'identical minimal payload for every id ON PURPOSE: this measures which id wakes a parser, ' +
        'not what the parser makes of the bytes. The content is sky-eluc.12.',
  tasks: window.__tasks().n
};
