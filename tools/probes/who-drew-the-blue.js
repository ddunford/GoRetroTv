// WHICH NATIVE PRODUCED THE FOUR FILLS THAT ARE THE BLUE SCREEN, attributed by instruction count
// rather than by a two-second bucket.
//
// ANSWER, measured: (1,0xD2)(window=1, 0xDCDCDCDC) at 196111684, then (1,0xE4)(window=1) at
// 196316013, then the first fill at 196328206. Produce, then drain, then pixels.
//
// It exists because a per-native census disagreed, putting (1,0xE4) nowhere in eighty seconds -- a
// lower-case __pcHits lookup silently reading zero for every address with a hex letter in it. A
// two-second bucket could not have settled an ordering 204k instructions wide either way; the
// trace's own icount field can, and one instrument disagreeing with another is what caught the bug.
//
// Both the shim and the function it jumps to are watched for each candidate: if the dispatcher ever
// reached a native without running its thunk, the pair would disagree and say so.
function h(v){ return '0x'+(v>>>0).toString(16).padStart(8,'0'); }
window.__profile(true);
window.__traceCalls([
  { pc: 0x80082090, name: 'shim(1,0xD2)', args: 2 },   // set plane background colour
  { pc: 0x80084388, name: 'fn(1,0xD2)',   args: 2 },
  { pc: 0x80081C58, name: 'shim(1,0xE4)', args: 1 },   // drain the window command ring
  { pc: 0x800837A0, name: 'fn(1,0xE4)',   args: 1 },
  { pc: 0x80082190, name: 'shim(1,0x52)', args: 2 },
  { pc: 0x800859E0, name: 'fn(1,0x52)',   args: 2 }
]);
window.__traceClear();
// The fills happen about a minute after boot, so watch straight through them.
await new Promise(r=>setTimeout(r,100000));
var log = window.__traceLog();
var blits = window.__blitLog();
var counts = {};
log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });
// Interleave both streams in icount order, which is the whole point of the measurement.
var rows = log.map(function(e){ return { ic: e.icount, what: e.name + '(' + e.a.join(',') + ')' }; })
  .concat(blits.map(function(b){ return { ic: b.icount, what: 'BLIT ' + (b.note||'') }; }))
  .sort(function(x,y){ return x.ic - y.ic; });
return { counts: counts, traceN: log.length, blitN: blits.length,
         timeline: rows.map(function(r){ return r.ic + '  ' + r.what; }),
         tasks: window.__tasks().n };
