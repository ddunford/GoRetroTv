// sky-02me.21 -- HOW FAR ALONG ITS OWN CHAIN DOES THE BOX CARRY OUR TITLE RECORDS?
//
// WHAT IS NOW KNOWN, AND IT CHANGES THE QUESTION. The consumer of a 0xA1 section is
// FUN_800c95d0 -- measured, not guessed: it ran exactly 20 times for 20 sections pushed, while
// ALL TWELVE of the call sites that hold the 0xB5 callback as a pool word ran ZERO times. The
// static reading that put the consumption at 0x800C72D2, "the time compare", named a call site
// that never runs on this path; the walk is driven from 0x800C9BC4, inside FUN_800c95d0 itself.
//
// Decompiled, that function is the whole answer path, and every step of it is a POOL WORD whose
// callee address is therefore knowable without guessing:
//
//     extensionLookup(ext)        0x800A3F7D   must return 4 and an index < serviceCount
//     dayKeyToSlot(0x9E8B, tid&3) 0x800C70E1   must return 0..31, else the section is DROPPED
//     alloc                       0x800CCECD   the block the records are copied into
//     memcpy                      0x8001DB19   the record bodies, copied one per entry
//     registerBlock(block, ext)   0x800C1541
//     dayKeyToDate(0x9E8B)        0x800C1A01   0x9E8B IS read as a date here
//     dateToBaseTime              0x80074759   and turned into the base the start time is added to
//     walkDescriptors(.., 0xB5)   0x800C64CD   the walk we already see, once per record
//     PER-EVENT REGISTER          0x800C587D   once per record, AFTER the walk -- the payoff step
//     notify(svc, key, tid, id)   0x800C579D   0x3EC on entry, 0x3EA on completion
//     final notify                0x800C5711
//
// So "the guide does not draw them" stops being a mystery with one place to look and becomes a
// LADDER with eleven rungs, and the first rung that does not turn is the answer. That is what this
// counts. A census is the right instrument precisely because it cannot be argued with: each of
// these is a distinct address and __pcHits counts executions of exactly that address.
//
// AND THE SECOND HALF ASKS WHETHER THE APPLICATION IS EVER TOLD. Two of those rungs are
// NOTIFICATIONS, and a notification the EPG receives makes its bytecode run. So the o-code is
// traced across a feed window and across an IDLE WINDOW OF THE SAME LENGTH, and the two are
// segmented by the interpreter's event-loop head at 0x9FC4A538. An event the feed produced is an
// event the idle window does not have.
//
// THE CONTROL IS NOT OPTIONAL AND THE FIRST RUN OF THE SIBLING PROBE PROVED IT. A tv guide press
// on a fed box ran three MORE event-loop segments than the same press on an unfed one, which read
// as "the listings caused three events" -- and one of those three was byte-identical to a segment
// the unfed press already had, i.e. a PERIODIC timer that fires every so often and happened to
// land inside one window and not the other. The box's o-code activity is bursty and periodic, so a
// count difference between two windows means nothing without a same-length control.

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
  var descs = [0x5F, 4].concat(u32(2), [0xB1, body.length].concat(body), serviceListDesc(SERVICES), satellite());
  var ts = u16(ids.tsid).concat(u16(ids.networkId), [0xF0 | ((descs.length >>> 8) & 0x0F), descs.length & 0xFF], descs);
  var name = [0x47, 3, 0x53, 0x6B, 0x79];
  var len = 5 + 2 + name.length + 2 + ts.length + 4;
  var s = [0x4A, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(bq), [0xC1 | ((BAT_VERSION & 0x1F) << 1), 0x00, 0x00],
            [0xF0 | ((name.length >>> 8) & 0x0F), name.length & 0xFF], name,
            [0xF0 | ((ts.length >>> 8) & 0x0F), ts.length & 0xFF], ts);
  var c = crc32(s);
  return window.__siPush(0x0011, s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]));
}
var RECORDS = [2,1,240,24,181,18,0,0,14,16,0,0,0,58,235,29,218,174,48,202,244,74,2,0,2,2,240,23,181,17,14,16,14,16,0,0,0,58,9,42,35,87,24,101,69,24,64,2,3,240,23,181,17,28,32,14,16,0,0,0,5,26,174,48,202,213,198,31,192,16,2,4,240,29,181,23,42,48,14,16,0,0,0,42,227,15,197,92,97,149,137,152,230,204,171,140,20,188,64,2,5,240,24,181,18,56,64,14,16,0,0,0,56,242,139,45,127,87,24,101,80,206,32,2,6,240,26,181,20,70,80,14,16,0,0,0,56,219,50,215,245,113,134,86,174,48,254,0,128,2,7,240,28,181,22,84,96,14,16,0,0,0,56,161,111,39,58,184,193,166,174,48,202,213,22,149,136,2,8,240,25,181,19,98,112,14,16,0,0,0,58,167,171,140,50,161,87,24,101,122,37,16,2,9,240,36,181,30,112,128,14,16,0,0,0,58,9,42,35,87,24,101,122,10,158,245,113,130,148,117,113,134,86,174,48,254,0,128,2,10,240,31,181,25,126,144,14,16,0,0,0,42,227,1,90,184,195,43,87,24,2,85,198,25,90,184,192,52,64,2,11,240,25,181,19,140,160,14,16,0,0,0,42,159,45,105,125,92,97,149,170,45,43,16,2,12,240,24,181,18,154,176,14,16,0,0,0,32,230,171,140,50,181,113,135,240,4,0];
var A1_VERSION = 0;
function buildA1(ext){
  A1_VERSION = (A1_VERSION + 1) & 0x1F;
  var p = [0x9E, 0x8B].concat(RECORDS);
  var len = 5 + p.length + 4;
  var s = [0xA1, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
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
if (!wants(0xA1)) throw new Error('no 0xA1 subscription after the line-up: ' + tablesWanted());
var a1 = matchFor(0xA1);
var newPids = out.afterLineup.filters.filter(function(p){ return out.before.filters.indexOf(p) < 0; });
if (newPids.map(function(p){ return parseInt(p, 16); }).indexOf(0x33) < 0)
  throw new Error('PID 0x33 is not among the filters the line-up opened: ' + newPids.join(' '));

var eeLast = window.__i2cState().eeprom.writes, eeStill = 0, eeSecs = 0;
for (var q = 0; q < 90 && eeStill < 3; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  eeSecs += 3;
  var nowW = window.__i2cState().eeprom.writes;
  eeStill = (nowW === eeLast) ? eeStill + 1 : 0; eeLast = nowW;
}
out.rebuildQuietAfterSeconds = eeSecs;
out.census.afterLineup = census();

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
for (var rd = 0; rd < 5; rd++) {
  for (var ei = 0; ei < EXTS.length; ei++) {
    if (window.__siPush(0x33, buildA1(EXTS[ei])).ok) pushed++; else refused++;
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

out.after = { tables: tablesWanted(), tasks: window.__tasks().n };
var d = out.census.duringFeedWindow;
var rungs = LADDER.map(function(x){ return x.n; }).filter(function(n){ return /^\d/.test(n); });
var stopped = null;
for (var k = 0; k < rungs.length; k++) { if (!d[rungs[k]]) { stopped = rungs[k]; break; } }
out.headline = 'fed ' + pushed + ' sections. The ladder turns as far as: '
  + rungs.filter(function(n){ return d[n]; }).join(' | ')
  + (stopped ? ('  -- FIRST RUNG THAT DID NOT TURN: ' + stopped) : '  -- every rung turned')
  + '. Novel o-code events in the feed window: ' + novel.length
  + ' (idle window had ' + out.idleWindow.events + ' events, feed window ' + out.feedWindow.events + ').';
return out;
