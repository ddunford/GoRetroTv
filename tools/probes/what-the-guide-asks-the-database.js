// sky-02me.21 -- EVERY MODULE-12 CALL A GUIDE PRESS MAKES, AND WHICH BRANCH THE ANSWER TAKES.
//
// MODULE 12 IS THE EPG DATABASE. (12,0x23) resolves to 0x800CB770, which sits in the same
// neighbourhood as everything FUN_800c95d0 uses to FILL the store -- the per-event register at
// 0x800C587C, the notifies at 0x800C579C and 0x800C5710, the allocator at 0x800CCECC. Resolved
// from the live module table at the word in 0x8006E71C, with module 1 still landing on the array
// this project verified against the flash, so the walk is the checked one.
//
// AND THE GUIDE ASKS IT EXACTLY TWICE. Decompiled, 0x800CB770 is a lock/ask/unlock wrapper around
// 0x800CB69C, which presets the caller's out-parameter to 0xFFFFFFFF, sends request byte 0x91
// through 0x800AD608, and then takes one of two branches:
//
//     result 3 or 4, and the echoed key matches   ->  0x800A4040(ctx, u16, u16, u16)
//     anything else                               ->  0x800AC774(ctx)
//
// Which of those two runs is the whole question, and it is a COUNT rather than an argument. If the
// "otherwise" arm runs, the guide asked its database something and was told no -- and that is the
// gate, sitting in front of a store we have already proven holds 240 programmes.
//
// THE CENSUS IS OVER ALL 51 OF MODULE 12'S FUNCTIONS, not just the one. A single native in
// isolation cannot say whether the guide queried the database once and gave up or never really
// asked; the full list turns that into a reading. Every target is resolved through its MIPS16 thunk
// and MASKED -- a function pointer carries the ISA bit, and __pcHits normalises upward only, so an
// odd address counts zero on a function that ran. That cost this task a whole run.
//
// NO LINE-UP AND NO LISTINGS ARE FED, deliberately. The guide build is byte-identical on a box
// holding 240 of our programmes and on one holding none -- measured, instruction for instruction --
// so the cheapest box that reproduces it is the right one, and anything that DID differ would
// contradict that measurement and is worth knowing.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function b32(a){ var b = window.__peek(a, 4); return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0; }
function b16(a){ var b = window.__peek(a, 2); return ((b[0] << 8) | b[1]) & 0xFFFF; }
function resolveThunk(impl){
  var base = impl & ~1;
  for (var k = 0; k < 8; k++) {
    var hw = b16(base + k * 2);
    if ((hw >>> 11) === 0x16) {
      var pool = ((base + k * 2) & ~3) + (hw & 0xFF) * 4;
      return b32(pool) & ~1;
    }
  }
  return base;                     // the inline kind: the shim IS the function
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

var out = { question: 'which of its database it asks, and which answer it gets',
            settledAfterSeconds: waited };
var modtab = b32(0x8006E71C);
var arr12 = b32(modtab + 12 * 8), n12 = b32(modtab + 12 * 8 + 4);
if (b32(modtab + 1 * 8) !== 0x9FC29F04)
  throw new Error('module 1 no longer resolves to the flash array this project verified, so the '
                + 'table walk has moved and module 12\'s rows would be read out of the wrong place');
out.module12 = { array: h(arr12), count: n12 };

var SUBJECTS = [];
for (var f = 0; f < n12; f++) {
  var rec = b32(arr12 + 4 * f);
  SUBJECTS.push({ n: '(12,0x' + f.toString(16) + ')', a: resolveThunk(b32(rec)) & ~1 });
}
// The two arms of 0x800CB69C's decision, and the request function it goes through.
SUBJECTS.push({ n: 'ask wrapper 0x800CB770',        a: 0x800CB770 });
SUBJECTS.push({ n: 'ask body 0x800CB69C',           a: 0x800CB69C });
SUBJECTS.push({ n: 'request fn 0x800AD608',         a: 0x800AD608 });
SUBJECTS.push({ n: 'ARM: answered (0x800A4040)',    a: 0x800A4040 });
SUBJECTS.push({ n: 'ARM: not answered (0x800AC774)', a: 0x800AC774 });
SUBJECTS.push({ n: 'store: per-event register 0x800C587C', a: 0x800C587C });
SUBJECTS.push({ n: 'store: section parser 0x800C95D0',     a: 0x800C95D0 });
SUBJECTS.forEach(function(s){
  if (s.a & 1) throw new Error(s.n + ' resolved to the odd address ' + h(s.a) + '; __pcHits '
                             + 'normalises upward only and would count zero on a function that ran');
});
function census(){
  var r = window.__pcHits.apply(null, SUBJECTS.map(function(s){ return s.a; })), o = {};
  SUBJECTS.forEach(function(s){
    if (r[h(s.a)] === undefined)
      throw new Error('__pcHits could not find the key for ' + s.n + ' -- harness failure, not a zero');
    o[s.n + ' @' + h(s.a)] = r[h(s.a)];
  });
  return o;
}
function diff(a, b){ var o = {}; Object.keys(b).forEach(function(k){ if (b[k] !== a[k]) o[k] = b[k] - a[k]; }); return o; }

var WIDGET_TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x800CB770, name: 'ask',       args: 2 }
];
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
var lastKey = null;
async function press(raw, label){
  if (lastKey === raw) throw new Error('HARNESS: ' + label + ' repeats a key; a box already on that screen rebuilds nothing');
  lastKey = raw;
  window.__traceCalls(WIDGET_TRACE); window.__traceClear();
  var c0 = census(), s0 = surface();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 11000); });
  var lg = window.__traceLog(), n = {};
  lg.forEach(function(e){ n[e.name] = (n[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var s1 = surface();
  return { key: label, widgets: n.newWidget || 0, asks: n.ask || 0,
           askArgs: lg.filter(function(e){ return e.name === 'ask'; })
                      .map(function(e){ return e.a.map(h).join(', '); }),
           moved: s1.hash !== s0.hash, surface: s1, drew: (n.newWidget || 0) > 0,
           census: diff(c0, census()) };
}

out.sky   = await press(0x7D, 'sky');
if (!out.sky.drew) throw new Error('the settling sky press built no widgets, so the box is not drawing');
out.guide = await press(0x80, 'tv guide');
await window.__shot('guide-database-census');
if (!out.guide.drew)
  throw new Error('the guide press built no widgets, so this is not the press the probe means to measure');

var c = out.guide.census;
var answered = c['ARM: answered (0x800A4040) @0x800A4040'] || 0;
var not      = c['ARM: not answered (0x800AC774) @0x800AC774'] || 0;
out.headline = 'the guide press made ' + out.guide.asks + ' call(s) to (12,0x23) [' + out.guide.askArgs.join(' | ')
  + ']; the ANSWERED arm ran ' + answered + ' and the NOT-ANSWERED arm ran ' + not + '. '
  + 'Module-12 functions the press touched: '
  + Object.keys(c).filter(function(k){ return /^\(12,/.test(k); }).join(' ');
return out;
