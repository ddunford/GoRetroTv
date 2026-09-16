// sky-02me.21 -- WITH THE LINKAGE ANSWERED, WHAT ELSE DOES THE GUIDE ASK ITS DATABASE?
//
// THE GAP THIS CLOSES. Censusing module 12 on a box WITHOUT the linkage showed a guide press making
// exactly four database calls -- (12,0x6) x2, (12,0x8), (12,0x23), (12,0x26) x2 -- and (12,0x23),
// the linkage question, taking the NOT-ANSWERED arm. Giving the BAT a linkage_descriptor of type
// 0x91 flips that arm and costs **14,944 extra o-code instructions and three extra events**, proven
// A/B in one run against a BAT differing by seven bytes.
//
// Fourteen thousand instructions of new work is a lot of asking, and the earlier census could not
// see any of it: its subject list was the linkage path, not the module. So this censuses **all 51
// of module 12's functions** across three guide presses on a box that has the linkage, the clock,
// and 480 of our programmes stored -- and the question it answers is whether the guide, having been
// told where the EPG lives, then goes and asks for ROWS.
//
// A function that runs only on the linkage box is the thread; a census identical to the four-call
// one would mean the extra instructions are all drawing, and the row query is somewhere else again.
//
// EVERY TARGET IS RESOLVED THROUGH ITS MIPS16 THUNK AND MASKED. A function pointer carries the ISA
// bit and __pcHits normalises upward only, so an odd address counts zero on a function that ran --
// that cost this task a whole run and a confidently wrong "first rung that did not turn".

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function hx(v){ return '0x' + (v >>> 0).toString(16); }

var LADDER = [
  { a: 0x800C95D0, n: '01 sectionParser FUN_800c95d0' },
  { a: 0x800A3F7C, n: '02 extensionLookup' },
  { a: 0x800C70E0, n: '03 dayKeyToSlot(0x9E8B)' },
  { a: 0x800CCECC, n: '04 alloc' },
  { a: 0x800C92A4, n: '05 findBlock A' },
  { a: 0x800C91F8, n: '05 findBlock B' },
  { a: 0x8001DB18, n: '06 memcpy (shared -- context only)' },
  { a: 0x800C1540, n: '07 registerBlock' },
  { a: 0x800C1A00, n: '08 dayKeyToDate' },
  { a: 0x80074758, n: '09 dateToBaseTime' },
  { a: 0x800C64CC, n: '10 walkDescriptors(0xB5)' },
  { a: 0x800C6B38, n: '11 the 0xB5 callback' },
  { a: 0x800C587C, n: '12 PER-EVENT REGISTER' },
  { a: 0x800C579C, n: '13 notify(0x3EC / 0x3EA)' },
  { a: 0x800C5710, n: '14 final notify' },
  { a: 0x800C8DF8, n: '-- head fn (param_1 & 2 path)' },
  { a: 0x800C590C, n: '-- fn(svc,key,tidLow)' },
  { a: 0x800C59D0, n: '-- fn(svc) on completion' },
  { a: 0x800C9324, n: '-- fn(&head,svc,key,tidLow)' },
  { a: 0x800BECF0, n: '-- huffman' }
];
// AND THE ADDRESSES ARE MASKED, WHICH IS NOT TIDYING. Every one of these came out of a POOL WORD,
// and a MIPS16 function pointer carries the ISA bit, so they are all ODD. __pcHits normalises by
// counting `a` and `a|1` -- which is a normalisation UPWARD only: handed 0x800C64CD it counts
// 0x800C64CD twice and never looks at 0x800C64CC, where the profiler actually recorded the hits.
// The first run of this probe did exactly that and reported ZERO for eleven rungs of a ladder whose
// two hand-typed EVEN addresses counted perfectly -- a census that measured its own harness and
// looked like a finding, with a "first rung that did not turn" naming the wrong function. The
// existing guard passed it, because the key was present; presence was never the question.
LADDER.forEach(function(x){
  if (x.a & 1)
    throw new Error('ladder entry ' + x.n + ' is the ODD address ' + h(x.a) + '. Pool words carry the '
                  + 'MIPS16 ISA bit and __pcHits only normalises upward, so this would count zero on '
                  + 'a function that ran. Mask the bit where the address is written down.');
});
function census(){
  var r = window.__pcHits.apply(null, LADDER.map(function(x){ return x.a; })), o = {};
  LADDER.forEach(function(x){
    if (r[h(x.a)] === undefined)
      throw new Error('__pcHits could not find the key for ' + x.n + ' -- a census that cannot find '
                    + 'its own subject is a harness failure, never a count of zero');
    o[x.n] = r[h(x.a)];
  });
  return o;
}
function diff(a, b){ var o = {}; Object.keys(b).forEach(function(k){ if (b[k] !== a[k]) o[k] = b[k] - a[k]; }); return o; }

var OCODE_LO = 0x9FC4A400, OCODE_LEN = 353188;
var MAIN_FETCH = 0x80069298, MAIN_FETCH_S = '0x80069298', HEAD = '0x9FC4A538';
var TRACE_MAX = 250000;
function armOcode(){
  var w = window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN,
                             { fromPc: [MAIN_FETCH, MAIN_FETCH + 2], max: TRACE_MAX });
  if (w.max !== TRACE_MAX) throw new Error('__readWatch does not take {max} on this page');
}
function readOcode(){
  var lg = window.__readWatchLog(); window.__readWatch();
  var total = null;
  (lg.byPc || []).forEach(function(s){ var p = s.split(' x'); if (p[0] === MAIN_FETCH_S) total = parseInt(p[1], 10); });
  var st = lg.all.map(function(e){ return e.at; });
  var segs = [], cur = [];
  st.forEach(function(a){ if (a === HEAD && cur.length) { segs.push(cur); cur = [a]; } else cur.push(a); });
  if (cur.length) segs.push(cur);
  return { instructions: total, logged: st.length, capped: !!lg.capped,
           events: segs.length, sizes: segs.map(function(s){ return s.length; }),
           segments: segs.map(function(s){ return s.join(' '); }) };
}

// MJD -> calendar, the standard inverse, ROUND-TRIPPED against the page's own mjd() rather than
// trusted: __siTDT returns the MJD it computed in its `utc` string, so feeding the date back and
// comparing is a check the arithmetic cannot quietly fail.
function fromMjd(m){
  var jd = m + 2400001, a = jd + 32044, b = Math.floor((4 * a + 3) / 146097);
  var c = a - Math.floor(146097 * b / 4), d = Math.floor((4 * c + 3) / 1461);
  var e = c - Math.floor(1461 * d / 4), f = Math.floor((5 * e + 2) / 153);
  return { d: e - Math.floor((153 * f + 2) / 5) + 1,
           m: f + 3 - 12 * Math.floor(f / 10),
           y: 100 * b + d - 4800 + Math.floor(f / 10) };
}
function setClock(y, mo, d, hh){
  var r = window.__siTDT(y, mo, d, hh, 0, 0);
  window.__siTOT(y, mo, d, hh, 0, 0);
  return r;
}

var WIDGET_TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
// The two arms of the guide's one database question, plus the ask itself, counted per press. A
// count is the right instrument here: the arms are distinct addresses and only one of them can run.
function b16at(a){ var b = window.__peek(a, 2); return ((b[0] << 8) | b[1]) & 0xFFFF; }
function b32at(a){ var b = window.__peek(a, 4); return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0; }
function resolveThunk(impl){
  var base = impl & ~1;
  for (var k = 0; k < 8; k++) {
    var hw = b16at(base + k * 2);
    if ((hw >>> 11) === 0x16) return b32at(((base + k * 2) & ~3) + (hw & 0xFF) * 4) & ~1;
  }
  return base;
}
var MODTAB = b32at(0x8006E71C);
if (b32at(MODTAB + 1 * 8) !== 0x9FC29F04)
  throw new Error('module 1 no longer resolves to the flash array this project verified -- the '
                + 'module table walk has moved and every module-12 row would come from the wrong place');
var ARR12 = b32at(MODTAB + 12 * 8), N12 = b32at(MODTAB + 12 * 8 + 4);
if (!(N12 > 0 && N12 < 4096))
  throw new Error('module 12 declares ' + N12 + ' functions, which is not a usable count');
var ARMS = [
  { a: 0x800CB770, n: 'ask (12,0x23)' },
  { a: 0x800CB69C, n: 'ask body' },
  { a: 0x800AD608, n: 'linkage search' },
  { a: 0x800A4040, n: 'ARM: ANSWERED' },
  { a: 0x800AC774, n: 'ARM: not answered' }
];
for (var mf = 0; mf < N12; mf++)
  // The array holds POINTERS TO 8-byte {implementation, descriptor} records, not the records --
  // two dereferences, and dropping one hands resolveThunk the record address instead. That is
  // exactly what the first run did, and 0x9FC2C08C is the record for (12,0x23).
  // AND `& ~1` IN JAVASCRIPT YIELDS A *SIGNED* 32-BIT INT, so 0x800CB770 masked comes out negative
  // and compares unequal to the literal. __pcHits and hex32 both apply >>> 0 so the census itself
  // was never affected -- it was the self-check that failed, on an address that was correct.
  ARMS.push({ a: (resolveThunk(b32at(b32at(ARR12 + 4 * mf))) & ~1) >>> 0,
              n: '(12,0x' + mf.toString(16) + ') @RESOLVED' });
// THE CENSUS MUST PROVE ITS OWN RESOLUTION. The first run of this probe reported ZERO for all 51
// generated entries while the hand-written entry for the SAME function, 0x800CB770, counted 1 --
// which is not a fact about the box, it is a table walk landing somewhere else. A generated entry
// that disagrees with a known-good hand-written one is the cheapest possible self-check and it is
// asserted rather than eyeballed, because "the guide asks its database nothing" is exactly the kind
// of confident wrong conclusion this project keeps paying for.
ARMS.forEach(function(x){ x.n = x.n.replace('@RESOLVED', '@' + h(x.a)); });
var gen23 = ARMS.filter(function(x){ return /^\(12,0x23\)/.test(x.n); })[0];
if (!gen23 || (gen23.a >>> 0) !== 0x800CB770)
  throw new Error('the generated entry for (12,0x23) resolved to ' + (gen23 ? h(gen23.a) : 'nothing')
                + ', but that function is known to be 0x800CB770 -- the module-12 table walk is '
                + 'landing somewhere else and every count it produced would be about the wrong '
                + 'addresses. Module 12 array ' + h(ARR12) + ', count ' + N12 + '.');
ARMS.forEach(function(x){
  if (x.a & 1) throw new Error(x.n + ' is the odd address ' + h(x.a) + '; __pcHits normalises upward only');
});
function arms(){
  var r = window.__pcHits.apply(null, ARMS.map(function(x){ return x.a; })), o = {};
  ARMS.forEach(function(x){
    if (r[h(x.a)] === undefined) throw new Error('__pcHits could not find ' + x.n + ' -- harness failure');
    o[x.n] = r[h(x.a)];
  });
  return o;
}
function armsDiff(a, b){ var o = {}; Object.keys(b).forEach(function(k){ if (b[k] !== a[k]) o[k] = b[k] - a[k]; }); return o; }

var lastKeyPressed = null;
async function press(raw, label, tag, from){
  if (lastKeyPressed === raw)
    throw new Error('HARNESS: ' + tag + ' presses ' + hx(raw) + ' twice running; a box already on '
                  + 'that screen rebuilds nothing and the zero would read as the fault');
  lastKeyPressed = raw;
  window.__traceCalls(WIDGET_TRACE); window.__traceClear();
  var s0 = surface(), a0 = arms();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 11000); });
  var lg = window.__traceLog(), c = {};
  lg.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var s1 = surface();
  return { tag: tag, key: label, pressedFrom: from, widgets: c.newWidget || 0,
           applies: c.apply || 0, damage: c.DAMAGE || 0, moved: s1.hash !== s0.hash,
           surface: s1, drew: (c.newWidget || 0) > 0, arms: armsDiff(a0, arms()) };
}

var CRC_TAB = (function(){ var t = new Int32Array(256), i, j, c;
  for (i = 0; i < 256; i++) { c = i << 24; for (j = 0; j < 8; j++) c = (c & 0x80000000) ? ((c << 1) ^ 0x04C11DB7) : (c << 1); t[i] = c; }
  return t; })();
function crc32(b){ var c = -1, i; for (i = 0; i < b.length; i++) c = (c << 8) ^ CRC_TAB[((c >>> 24) ^ b[i]) & 0xFF]; return c >>> 0; }
function u16(v){ return [(v >>> 8) & 0xFF, v & 0xFF]; }
function u32(v){ return [(v >>> 24) & 0xFF, (v >>> 16) & 0xFF, (v >>> 8) & 0xFF, v & 0xFF]; }
function bcd8(v, d){ var s = String(v); while (s.length < d) s = '0' + s;
  var o = []; for (var i = 0; i < d; i += 2) o.push(parseInt(s.substr(i, 2), 16) & 0xFF); return o; }
function satellite(){ return [0x43, 11].concat(bcd8(1177800, 8), bcd8(282, 4), [0x81], bcd8(275000, 8).slice(0, 4)); }
function serviceListDesc(ids){ var b = []; ids.forEach(function(s){ b = b.concat(u16(s), [0x01]); }); return [0x41, b.length].concat(b); }
var SERVICES = [0x0064, 0x0065, 0x0066, 0x0067];
var LINEUP = SERVICES.map(function(sid, i){ return { sid: sid, f2: 0x01, f34: 0x0BB8 + i, f56: 0x1770 + i, ch: 0x0ABC + i, flags: 0x5 }; });
function armed(){ return window.__siFilters().filter(function(f){ return f.armed; }); }
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }
function matchFor(tid){ return window.__siMatches().filter(function(x){ return x.tableId === tid; }); }
function tablesWanted(){ return window.__siMatches().map(function(x){
  return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
       + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' '); }
var BAT_VERSION = 0;
function pushBat(){
  BAT_VERSION = (BAT_VERSION + 1) & 0x1F;
  var ids = window.__siIds();
  var bq = (ids.bouquetIdMask !== null && ids.bouquetId !== null &&
            ((0x1001 & ids.bouquetIdMask) === (ids.bouquetId & ids.bouquetIdMask))) ? 0x1001 : ids.bouquetId;
  var body = u16(0xFFFF);
  LINEUP.forEach(function(e){ body = body.concat(u16(e.sid), [e.f2], u16(e.f34), u16(e.f56), u16(((e.ch << 4) | e.flags) & 0xFFFF)); });
  // THE LINKAGE THE GUIDE HUNTS FOR. Body is exactly what 0x800CB660 reads: transport_stream_id,
  // original_network_id, service_id, linkage_type -- and the ONID is the box's network id, which is
  // what __siSDT() already uses for the services this same BAT declares, so the two agree by
  // construction rather than by coincidence.
  var linkage = [0x4A, 7].concat(u16(ids.tsid), u16(ids.networkId), u16(SERVICES[0]), [0x91]);
  var descs = [0x5F, 4].concat(u32(2), [0xB1, body.length].concat(body), serviceListDesc(SERVICES), satellite(), linkage);
  var ts = u16(ids.tsid).concat(u16(ids.networkId), [0xF0 | ((descs.length >>> 8) & 0x0F), descs.length & 0xFF], descs);
  var name = [0x47, 3, 0x53, 0x6B, 0x79].concat(linkage);
  var len = 5 + 2 + name.length + 2 + ts.length + 4;
  var s = [0x4A, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(bq), [0xC1 | ((BAT_VERSION & 0x1F) << 1), 0x00, 0x00],
            [0xF0 | ((name.length >>> 8) & 0x0F), name.length & 0xFF], name,
            [0xF0 | ((ts.length >>> 8) & 0x0F), ts.length & 0xFF], ts);
  var c = crc32(s);
  return window.__siPush(0x0011, s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]));
}
// THE TWO BYTES ARE READ OFF THE MATCH UNIT, NEVER ASSUMED. filterBytes is set from __siMatches()
// below; 0x9E 0x8B is what every run so far has been told to send, and if the clock changes it this
// probe follows rather than arguing.
var FILTER_BYTES = [0x9E, 0x8B];
var TABLE_ID = 0xA1;          // replaced by whatever the match unit names, before any section is built
var RECORDS = [2,1,240,20,181,18,0,0,14,16,0,0,0,58,235,29,218,174,48,202,244,74,2,0,2,2,240,19,181,17,14,16,14,16,0,0,0,58,9,42,35,87,24,101,69,24,64,2,3,240,19,181,17,28,32,14,16,0,0,0,5,26,174,48,202,213,198,31,192,16,2,4,240,25,181,23,42,48,14,16,0,0,0,42,227,15,197,92,97,149,137,152,230,204,171,140,20,188,64,2,5,240,20,181,18,56,64,14,16,0,0,0,56,242,139,45,127,87,24,101,80,206,32,2,6,240,22,181,20,70,80,14,16,0,0,0,56,219,50,215,245,113,134,86,174,48,254,0,128,2,7,240,24,181,22,84,96,14,16,0,0,0,56,161,111,39,58,184,193,166,174,48,202,213,22,149,136,2,8,240,21,181,19,98,112,14,16,0,0,0,58,167,171,140,50,161,87,24,101,122,37,16,2,9,240,32,181,30,112,128,14,16,0,0,0,58,9,42,35,87,24,101,122,10,158,245,113,130,148,117,113,134,86,174,48,254,0,128,2,10,240,27,181,25,126,144,14,16,0,0,0,42,227,1,90,184,195,43,87,24,2,85,198,25,90,184,192,52,64,2,11,240,21,181,19,140,160,14,16,0,0,0,42,159,45,105,125,92,97,149,170,45,43,16,2,12,240,20,181,18,154,176,14,16,0,0,0,32,230,171,140,50,181,113,135,240,4,0];
var A1_VERSION = 0;
function buildA1(ext){
  A1_VERSION = (A1_VERSION + 1) & 0x1F;
  var p = [FILTER_BYTES[0], FILTER_BYTES[1]].concat(RECORDS);
  var len = 5 + p.length + 4;
  var s = [TABLE_ID, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(ext), [0xC1 | ((A1_VERSION & 0x1F) << 1), 0x00, 0x00], p);
  var c = crc32(s);
  return s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]);
}

// ---- state assertions --------------------------------------------------------------------------
var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent)) throw new Error('never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');
window.__profile(true);

var out = { question: 'how far along its own chain does the box carry our title records',
            settledAfterSeconds: waited, before: { tables: tablesWanted(),
            filters: armed().map(function(f){ return f.pid; }) } };
out.census = { atStart: census() };

// ---- THE CLOCK, BEFORE ANYTHING ASKS FOR LISTINGS ----------------------------------------------
// TDT then TOT, repeated, because a single section cannot tell "ignored" from "not listening yet".
// 1 January 1998 is this product's in-world date and January is GMT, so the offset is genuinely
// zero rather than a placeholder. 19:00 is chosen so that "now" sits inside the day we then send,
// in the middle of a programme rather than on a boundary.
out.clock = { pushes: [] };
for (var ct = 0; ct < 3; ct++) {
  out.clock.pushes.push(window.__siTDT(1998, 1, 1, 19, 0, 0).utc);
  window.__siTOT(1998, 1, 1, 19, 0, 0);
  await new Promise(function(r){ setTimeout(r, 2500); });
}

if (!wants(0x4A)) {
  var n0 = window.__siNIT();
  if (!n0.ok) throw new Error('the ladder-opening NIT was refused: ' + n0.why);
  for (var w0 = 0; w0 < 10 && !wants(0x4A); w0++) await new Promise(function(r){ setTimeout(r, 3000); });
}
if (!wants(0x4A)) throw new Error('no 0x4A subscription after the NIT: ' + tablesWanted());
for (var round = 0; round < 3; round++) {
  window.__siNIT(undefined, { version: round + 1 });
  window.__siSDT(undefined, { version: round + 1 });
  var br = pushBat();
  if (!br.ok) throw new Error('the BAT was refused: ' + br.why);
  await new Promise(function(r){ setTimeout(r, 7000); });
}
out.afterLineup = { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) };
// THE TABLE ID IS THE BOX'S TO CHOOSE. The unit's tableIdMask is 0xFE, so 0xA0 and 0xA1 both reach
// the parser -- but the parser takes `tableId & 3` as part of the day-slot key, so sending the wrong
// one of the pair files our programmes under a slot the guide is not reading. Take the id the unit
// names and send exactly that.
var a1 = window.__siMatches().filter(function(x){ return x.tableId !== null && (x.tableId & 0xFE) === 0xA0; });
if (!a1.length)
  throw new Error('no 0xA0/0xA1 subscription after the line-up: ' + tablesWanted());
TABLE_ID = a1[0].tableId;
out.tableIdDemanded = hx(TABLE_ID);
out.tableIdMovedWithTheClock = TABLE_ID !== 0xA1;
// bytes[] indexes the FILTERABLE bytes, skipping the two length bytes: index 0 is section byte 0,
// indices 1 and 2 are the extension at bytes 3 and 4, so index 6 is section byte 8 -- payload[0].
// Derived, not counted off by eye, and asserted rather than hoped for.
var fb = (a1[0].bytes || []).slice(6, 8).map(function(x){
  var m = /^([0-9a-f]+)\/([0-9a-f]+)$/.exec(x); return m ? { v: parseInt(m[1], 16), mask: parseInt(m[2], 16) } : null; });
if (!fb[0] || !fb[1] || fb[0].mask !== 0xFF || fb[1].mask !== 0xFF)
  throw new Error('the 0xA1 unit does not constrain payload[0..1] to exact values on this run ('
                + JSON.stringify(a1[0].bytes) + '), so there is nothing to stamp and a guessed pair '
                + 'would be refused in silence');
FILTER_BYTES = [fb[0].v, fb[1].v];
out.filterBytesDemanded = '0x' + fb[0].v.toString(16) + ' 0x' + fb[1].v.toString(16);
out.filterBytesMovedWithTheClock = !(fb[0].v === 0x9E && fb[1].v === 0x8B);
// THE PID IS THE BOX'S TO CHOOSE TOO, AND IT MOVED WITH THE CLOCK. Every unclocked run opened
// 0x52 and 0x33 and this probe hardcoded 0x33 from them; with the clock set first the box opens
// 0x52, 0x37 and 0x36 instead. So the PID, the table id and the two payload bytes are all part of
// the same addressing and all three are read off the box rather than carried. 0x52 is excluded by
// name because this project has already measured its consumer (0x800A857A) and it understands only
// 0x40, 0x42 and 0x70 -- it is SI, not listings.
var newPids = out.afterLineup.filters.filter(function(p){ return out.before.filters.indexOf(p) < 0; })
  .map(function(p){ return parseInt(p, 16); }).filter(function(p){ return p !== 0x52; });
out.listingsPids = newPids.map(hx);
out.listingsPidsMovedWithTheClock = !(newPids.length === 1 && newPids[0] === 0x33);
if (!newPids.length)
  throw new Error('the line-up opened no PID that could carry listings (' + out.afterLineup.filters.join(' ')
                + '), so there is nowhere attributable to push and every later zero would be about that');

var eeLast = window.__i2cState().eeprom.writes, eeStill = 0, eeSecs = 0;
for (var q = 0; q < 90 && eeStill < 3; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  eeSecs += 3;
  var nowW = window.__i2cState().eeprom.writes;
  eeStill = (nowW === eeLast) ? eeStill + 1 : 0; eeLast = nowW;
}
out.rebuildQuietAfterSeconds = eeSecs;
out.census.afterLineup = census();

// ---- THE BASELINE GUIDE, on a box with a line-up and no listings --------------------------------
out.presses = [];
out.presses.push(await press(0x7D, 'sky', 'baseline-sky', 'the boot screen'));
if (!out.presses[0].drew)
  throw new Error('the settling sky press built no widgets, so the box is not drawing at all and '
                + 'every later reading would be about that rather than about the listings');
out.presses.push(await press(0x80, 'tv guide', 'baseline-guide', 'the menu'));
await window.__shot('guide-before-12-records');
if (!out.presses[1].drew)
  throw new Error('the BASELINE guide press built no widgets -- the box was not drawing before the '
                + 'listings were fed, so any later change would be it recovering rather than the '
                + 'listings arriving');
out.presses.push(await press(0x7D, 'sky', 'between-sky', 'the guide'));

// ---- THE IDLE CONTROL WINDOW, same length as the feed and taken FIRST ---------------------------
var WINDOW_MS = 30000;
var cIdle0 = census();
armOcode();
await new Promise(function(r){ setTimeout(r, WINDOW_MS); });
out.idleWindow = readOcode();
out.census.duringIdleWindow = diff(cIdle0, census());

// ---- THE FEED WINDOW ----------------------------------------------------------------------------
var EXTS = [];
for (var e = (a1[0].extension & a1[0].extensionMask) >>> 0; e <= a1[0].extension; e++) EXTS.push(e);
out.extensionsFed = EXTS.map(hx);
var cFeed0 = census();
armOcode();
var pushed = 0, refused = 0, t0 = Date.now();
out.keysFed = [];
for (var rd = 0; rd < 5; rd++) {
  // RE-READ WHAT THE BOX IS ASKING FOR, EVERY ROUND. It has already moved the table id, the PID and
  // the day between two runs of this probe; carrying any of them across five rounds would feed a day
  // nobody asked for and the silence would look like a format problem.
  var live = window.__siMatches().filter(function(x){ return x.tableId !== null && (x.tableId & 0xFE) === 0xA0; });
  if (live.length) {
    TABLE_ID = live[0].tableId;
    var lb = (live[0].bytes || []).slice(6, 8).map(function(x){
      var mm = /^([0-9a-f]+)\/([0-9a-f]+)$/.exec(x); return mm ? parseInt(mm[1], 16) : null; });
    if (lb[0] !== null && lb[1] !== null) FILTER_BYTES = [lb[0], lb[1]];
  }
  var key = hx(TABLE_ID) + '/' + hx((FILTER_BYTES[0] << 8) | FILTER_BYTES[1]);
  if (out.keysFed.indexOf(key) < 0) out.keysFed.push(key);
  for (var ei = 0; ei < EXTS.length; ei++) {
    // One section per extension per PID: the box named several and which of them carries titles is
    // its answer to give, not ours to pick. A section whose table id the unit on that PID does not
    // accept is refused, so this selects itself.
    var sec = buildA1(EXTS[ei]);
    for (var pj = 0; pj < newPids.length; pj++) {
      if (window.__siPush(newPids[pj], sec).ok) pushed++; else refused++;
    }
    await new Promise(function(x){ setTimeout(x, 1200); });
  }
}
var left = WINDOW_MS - (Date.now() - t0);
if (left > 0) await new Promise(function(x){ setTimeout(x, left); });
out.feedWindow = readOcode();
out.census.duringFeedWindow = diff(cFeed0, census());
out.sectionsPushed = pushed;
out.sectionsRefused = refused;
out.feedWindowMs = Date.now() - t0;

// ---- which events the feed window has that the idle window does not ------------------------------
// By CONTENT, never by count: a periodic timer landing in one window and not the other is the exact
// artefact this control exists to refuse, and two events of the same length are the same event.
var idleSet = {};
out.idleWindow.segments.forEach(function(s){ idleSet[s] = (idleSet[s] || 0) + 1; });
var novel = [], repeated = [];
out.feedWindow.segments.forEach(function(s, i){
  if (idleSet[s]) { repeated.push({ i: i, length: s.split(' ').length }); idleSet[s]--; }
  else novel.push({ i: i, length: s.split(' ').length, opens: s.split(' ').slice(0, 10).join(' ') });
});
out.eventsOnlyInTheFeedWindow = novel;
out.eventsSeenInBothWindows = repeated;
delete out.idleWindow.segments;
delete out.feedWindow.segments;

// ---- AND NOW THE GUIDE ---------------------------------------------------------------------
out.presses.push(await press(0x80, 'tv guide', 'fed-guide', 'the menu'));
await window.__shot('guide-after-12-records');
out.presses.push(await press(0x7D, 'sky', 'after-sky', 'the guide'));

// ---- AND NOW MOVE "NOW" INTO THE DAY WE FED -----------------------------------------------------
var fedMjd = (FILTER_BYTES[0] << 8) | FILTER_BYTES[1];
var cal = fromMjd(fedMjd);
var chk = setClock(cal.y, cal.m, cal.d, 19);
out.clockMovedTo = { mjd: fedMjd, date: cal, reported: chk.utc };
if (chk.utc.indexOf('MJD ' + fedMjd) < 0)
  throw new Error('the MJD inverse disagrees with the page\'s own mjd(): asked for MJD ' + fedMjd
                + ' and the TDT reports "' + chk.utc + '". Every programme below would be on the '
                + 'wrong day and the guide press would prove nothing.');
for (var ck = 0; ck < 3; ck++) { setClock(cal.y, cal.m, cal.d, 19); await new Promise(function(r){ setTimeout(r, 2500); }); }
// A clock jump can send the box back into its channel-list rebuild, whose blank screen reads exactly
// like the fault. Wait it out on the EEPROM traffic, the same signal the earlier wait uses.
var eL = window.__i2cState().eeprom.writes, eS = 0, eSec = 0;
for (var qq = 0; qq < 60 && eS < 3; qq++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  eSec += 3;
  var nw = window.__i2cState().eeprom.writes;
  eS = (nw === eL) ? eS + 1 : 0; eL = nw;
}
out.clockMovedTo.quietAfterSeconds = eSec;
out.presses.push(await press(0x80, 'tv guide', 'guide-with-now-inside-the-fed-day', 'the menu'));
await window.__shot('guide-with-now-inside-the-fed-day');

var baseG = out.presses[1], fedG = out.presses[3];
var nowG = out.presses[5];
out.guideAfterClockMove = { widgets: nowG.widgets, surface: nowG.surface,
  changedFromBaseline: nowG.surface.hash !== baseG.surface.hash,
  changedFromFed: nowG.surface.hash !== fedG.surface.hash };
out.guide = {
  baseline: { widgets: baseG.widgets, surface: baseG.surface },
  fed:      { widgets: fedG.widgets,  surface: fedG.surface },
  changed:  baseG.surface.hash !== fedG.surface.hash
};

// THE EXPECTATION, STATED BEFORE THE RESULT IS READ. 20 sections x 12 records is 240; the run that
// found the defect read 40. Anything else is its own finding and is named rather than glossed.
// THE PREVIOUS RUN'S GUIDE HASH, asserted rather than quoted. If the baseline here is not that
// screen then this box is in a different state and the comparison below is between two unknowns.
out.baselineMatchesPreviousRun = baseG.surface.hash === '0xB4E3CAD8' && baseG.surface.colours === 26;
var perEvent = out.census.duringFeedWindow['12 PER-EVENT REGISTER'] || 0;
var walks    = out.census.duringFeedWindow['10 walkDescriptors(0xB5)'] || 0;
out.recordsPerSection = { expected: 12, perEventRegister: perEvent, descriptorWalks: walks,
  measuredPerSection: pushed ? +(perEvent / pushed).toFixed(2) : null,
  sectionsAccepted: out.census.duringFeedWindow['01 sectionParser FUN_800c95d0'] || 0,
  sectionsBuilt: pushed / (newPids.length || 1),
  verdict: (perEvent === (pushed / (newPids.length || 1)) * 12 * newPids.length
            || perEvent % 12 === 0 && perEvent > 0)
    ? 'EXACTLY TWELVE PER SECTION -- the length fix took'
    : 'NOT twelve per section (' + perEvent + ' registers over ' + pushed + ' sections) -- the walk '
    + 'still desynchronises, and the next thing to check is the record header rather than the guide' };

// The presses come after the feed window so the ladder numbers above are of the feed alone. The
// BASELINE press has to happen before the feed, which is why it sits here as a pair with the after
// press and both are made from the menu.
out.after = { tables: tablesWanted(), tasks: window.__tasks().n };
var d = out.census.duringFeedWindow;
var rungs = LADDER.map(function(x){ return x.n; }).filter(function(n){ return /^\d/.test(n); });
var stopped = null;
for (var k = 0; k < rungs.length; k++) { if (!d[rungs[k]]) { stopped = rungs[k]; break; } }
function armOf(p){ return (p.arms['ARM: ANSWERED'] ? 'ANSWERED x' + p.arms['ARM: ANSWERED']
                         : (p.arms['ARM: not answered'] ? 'not-answered x' + p.arms['ARM: not answered']
                            : 'the guide did not ask at all')); }
out.theAsk = { baselineGuide: armOf(baseG), fedGuide: armOf(fedG), afterClockMove: armOf(nowG) };
out.module12 = { array: h(ARR12), count: N12 };
out.databaseCalls = {};
[['baseline-guide', baseG], ['fed-guide', fedG], ['after-clock-move', nowG]].forEach(function(pair){
  var c = pair[1].arms, only = {};
  Object.keys(c).forEach(function(k){ if (/^\(12,/.test(k)) only[k] = c[k]; });
  out.databaseCalls[pair[0]] = only;
});
out.headline = 'DATABASE CALLS on the linkage box: ' + JSON.stringify(out.databaseCalls['after-clock-move'])
  + '. THE ASK: ' + armOf(baseG) + ' / ' + armOf(fedG) + ' / ' + armOf(nowG) + '. '
  + 'AFTER THE CLOCK MOVE the guide is '
  + (out.guideAfterClockMove.changedFromFed ? 'DIFFERENT' : 'byte-identical')
  + ' (' + nowG.surface.hash + '/' + nowG.surface.colours + 'c, ' + nowG.widgets + ' widgets). '
  + 'Days fed: ' + out.keysFed.join(' ') + '. BEFORE the clock move the guide was '
  + (out.guide.changed ? 'CHANGED' : 'byte-identical') + ' ('
  + baseG.surface.hash + '/' + baseG.surface.colours + 'c -> ' + fedG.surface.hash + '/'
  + fedG.surface.colours + 'c). ' + out.recordsPerSection.verdict + '. fed ' + pushed
  + ' sections. The ladder turns as far as: '
  + rungs.filter(function(n){ return d[n]; }).join(' | ')
  + (stopped ? ('  -- FIRST RUNG THAT DID NOT TURN: ' + stopped) : '  -- every rung turned')
  + '. Novel o-code events in the feed window: ' + novel.length
  + ' (idle window had ' + out.idleWindow.events + ' events, feed window ' + out.feedWindow.events + ').';
// ---- AND FINALLY, WHAT DATE THE BOX READS OUT OF THE KEY ----------------------------------------
// LAST, ON PURPOSE. __call cannot complete a function that tail-jumps through a jump table and
// leaves the machine wedged when it does, so everything worth measuring is already in `out` before
// this runs. A scratch buffer is checked to be zero before it is used and restored afterwards.
var SCRATCH = 0x81F00000;
var pre = window.__peek(SCRATCH, 32);
out.dateReadBack = { scratchWasClear: Array.prototype.every.call(pre, function(b){ return b === 0; }) };
if (out.dateReadBack.scratchWasClear) {
  var c = window.__call(0x800C1A00, true, (FILTER_BYTES[0] << 8) | FILTER_BYTES[1], SCRATCH, 0, 0);
  out.dateReadBack.call = c;
  out.dateReadBack.buffer = Array.prototype.map.call(window.__peek(SCRATCH, 32), function(b){
    return ('0' + b.toString(16)).slice(-2); }).join(' ');
  window.__poke(SCRATCH, Array.prototype.slice.call(pre));
} else {
  out.dateReadBack.skipped = 'the scratch address was not clear, so writing to it would disturb '
                           + 'live memory and any later reading would be about that';
}
return out;
