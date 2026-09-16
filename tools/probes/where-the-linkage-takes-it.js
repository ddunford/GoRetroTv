// sky-02me.21 -- WHERE THE LINKAGE TAKES THE GUIDE, AND WHERE THAT GOES WRONG.
//
// The linkage_descriptor of type 0x91 made the guide's one database question succeed -- the
// ANSWERED arm at 0x800A4040 runs where 0x800AC774 used to. The guide then draws a second panel:
//
//     FOR YOUR INFORMATION
//     No satellite signal is being received
//
// over the same "Further schedule information is not available" bar. And it draws it WITHOUT
// touching the front end: zero demodulator reads and zero demodulator writes across every guide
// press, measured. So "no satellite signal" is a conclusion the application reached, not a status
// it read off hardware -- and the SDT already declares those services running_status 4 and
// free_CA_mode 0, so it is not the obvious answer either.
//
// The one thing the box DID reach for is worth keeping: on the press taken with "now" inside the
// day we fed, a new PID channel appeared -- **0x30**, the first of Sky's eight title PIDs. The
// linkage unblocked acquisition and the box went looking for another day.
//
// SO THIS DIFFS THE GUIDE'S BYTECODE WITH THE LINKAGE AGAINST THE SAME PRESS WITHOUT IT, IN ONE
// RUN. Two runs would vary the clock, the NVRAM and the day the box happens to be asking for; here
// the only difference between press A and press B is a second BAT carrying seven more bytes. The
// streams are segmented by the interpreter's event-loop head at 0x9FC4A538 and compared by CONTENT,
// because this box's o-code activity is periodic and a count difference between two windows has
// already fooled this task once.
//
// The arms census runs on both presses as its own control: if the not-answered arm does not run on
// A and the answered arm does not run on B, the two presses are not the two states this is meant to
// compare and the diff is between two unknowns.
//
// THE SECOND BAT COSTS A CHANNEL-LIST REBUILD. Pushing a new BAT version sends the box into the
// 74-140 second rebuild (sky-02me.18), whose blank screen reads exactly like the fault, so the wait
// is on EEPROM traffic going quiet and the press that follows is asserted to have drawn.

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

var OCODE_LO = 0x9FC4A400, OCODE_LEN = 353188, MAIN_FETCH_S = '0x80069298';
var HEAD = '0x9FC4A538', TRACE_MAX = 300000;
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
var ARMS = [
  { a: 0x800CB770, n: 'ask (12,0x23)' },
  { a: 0x800CB69C, n: 'ask body' },
  { a: 0x800AD608, n: 'linkage search' },
  { a: 0x800A4040, n: 'ARM: ANSWERED' },
  { a: 0x800AC774, n: 'ARM: not answered' }
];
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
async function press(raw, label, tag, from, trace){
  if (lastKeyPressed === raw)
    throw new Error('HARNESS: ' + tag + ' presses ' + hx(raw) + ' twice running; a box already on '
                  + 'that screen rebuilds nothing and the zero would read as the fault');
  lastKeyPressed = raw;
  window.__traceCalls(WIDGET_TRACE); window.__traceClear();
  var s0 = surface(), a0 = arms();
  if (trace) {
    // Unfiltered: the main fetch site reads opcode bytes and every other site reads OPERANDS, and
    // ocode-disasm.py needs both to decode rather than merely to list addresses.
    var w = window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN, { max: TRACE_MAX });
    if (w.max !== TRACE_MAX) throw new Error('__readWatch does not take {max} on this page');
  }
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 11000); });
  var ocode = null;
  if (trace) {
    var wl = window.__readWatchLog(); window.__readWatch();
    var total = null;
    (wl.byPc || []).forEach(function(s){ var p = s.split(' x'); if (p[0] === MAIN_FETCH_S) total = parseInt(p[1], 10); });
    var st = wl.all.filter(function(e){ return e.pc === MAIN_FETCH_S; }).map(function(e){ return e.at; });
    var segs = [], cur = [];
    st.forEach(function(a){ if (a === HEAD && cur.length) { segs.push(cur); cur = [a]; } else cur.push(a); });
    if (cur.length) segs.push(cur);
    ocode = { instructions: total, logged: st.length, capped: !!wl.capped,
              events: segs.length, sizes: segs.map(function(s){ return s.length; }),
              segments: segs.map(function(s){ return s.join(' '); }),
              csv: wl.all.map(function(e){ return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }).join(';') };
  }
  var lg = window.__traceLog(), c = {};
  lg.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var s1 = surface();
  return { tag: tag, key: label, pressedFrom: from, widgets: c.newWidget || 0,
           applies: c.apply || 0, damage: c.DAMAGE || 0, moved: s1.hash !== s0.hash,
           surface: s1, drew: (c.newWidget || 0) > 0, arms: armsDiff(a0, arms()), ocode: ocode };
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
var WITH_LINKAGE = false;   // press A is without, press B is with -- the one variable
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
  var linkage = WITH_LINKAGE
    ? [0x4A, 7].concat(u16(ids.tsid), u16(ids.networkId), u16(SERVICES[0]), [0x91]) : [];
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

// ---- PRESS A: the same box, BAT WITHOUT the linkage --------------------------------------------
out.presses = [];
out.presses.push(await press(0x7D, 'sky', 'sky-before-A', 'the boot screen', false));
if (!out.presses[0].drew)
  throw new Error('the settling sky press built no widgets, so the box is not drawing at all');
var A = await press(0x80, 'tv guide', 'guide-A-no-linkage', 'the menu', true);
out.presses.push(A);
await window.__shot('guide-A-no-linkage');
if (!A.drew) throw new Error('press A built no widgets, so it is not the press this compares');
if (!A.arms['ARM: not answered'])
  throw new Error('press A did not take the NOT-ANSWERED arm (' + JSON.stringify(A.arms) + '), so the '
                + 'box is not in the state this diff means to compare from');
out.presses.push(await press(0x7D, 'sky', 'sky-between', 'the guide', false));

// ---- THE ONE VARIABLE: a second BAT, seven bytes longer ------------------------------------------
WITH_LINKAGE = true;
window.__eeTxClear();
var bl = pushBat();
if (!bl.ok) throw new Error('the linkage BAT was refused: ' + bl.why);
out.linkageBatPushed = true;
// The rebuild the new BAT version starts. Its blank screen reads exactly like the fault.
var rL = window.__i2cState().eeprom.writes, sL = 0, secL = 0;
for (var qq = 0; qq < 90 && sL < 3; qq++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  secL += 3;
  var nw2 = window.__i2cState().eeprom.writes;
  sL = (nw2 === rL) ? sL + 1 : 0; rL = nw2;
}
out.rebuildAfterLinkageSeconds = secL;

// ---- PRESS B: everything identical except those seven bytes -------------------------------------
var B = await press(0x80, 'tv guide', 'guide-B-with-linkage', 'the menu', true);
out.presses.push(B);
await window.__shot('guide-B-with-linkage');
if (!B.drew) throw new Error('press B built no widgets');
if (!B.arms['ARM: ANSWERED'])
  throw new Error('press B did not take the ANSWERED arm (' + JSON.stringify(B.arms) + '), so the '
                + 'linkage did not land and the diff would be between two copies of the same state');

// ---- THE DIFF, by CONTENT and per event ---------------------------------------------------------
function segs(p){ return p.ocode ? p.ocode.segments : []; }
var SA = segs(A), SB = segs(B);
var inA = {};
SA.forEach(function(s){ inA[s] = (inA[s] || 0) + 1; });
var novel = [], shared = [];
SB.forEach(function(s, i){
  if (inA[s]) { shared.push({ i: i, length: s.split(' ').length }); inA[s]--; }
  else novel.push({ i: i, length: s.split(' ').length, stream: s });
});
out.diff = {
  aInstructions: A.ocode.instructions, bInstructions: B.ocode.instructions,
  aCapped: A.ocode.capped, bCapped: B.ocode.capped,
  aEvents: A.ocode.events, bEvents: B.ocode.events,
  aSizes: A.ocode.sizes, bSizes: B.ocode.sizes,
  eventsOnlyInB: novel.map(function(n){ return { i: n.i, length: n.length }; }),
  eventsInBoth: shared
};
// The prefix diff of the BUILD, which is event 0 in both -- where inside one event they part.
var a0 = SA.length ? SA[0].split(' ') : [], b0 = SB.length ? SB[0].split(' ') : [];
var k = 0; while (k < a0.length && k < b0.length && a0[k] === b0[k]) k++;
out.diff.build = { aLength: a0.length, bLength: b0.length, commonPrefix: k };
if (k < a0.length && k < b0.length) {
  out.diff.build.lastCommon = a0.slice(Math.max(0, k - 14), k).join(' ');
  out.diff.build.aNext = a0.slice(k, k + 24).join(' ');
  out.diff.build.bNext = b0.slice(k, k + 24).join(' ');
} else {
  out.diff.build.note = (a0.length === b0.length) ? 'event 0 is identical in both'
                                                  : 'one build simply ENDS at ' + k;
}
// The addresses press B executed that press A never did, anywhere -- the new code.
var seenA = {};
SA.forEach(function(s){ s.split(' ').forEach(function(x){ seenA[x] = 1; }); });
var newAddrs = {};
SB.forEach(function(s){ s.split(' ').forEach(function(x){ if (!seenA[x]) newAddrs[x] = (newAddrs[x] || 0) + 1; }); });
var keys = Object.keys(newAddrs).sort(function(x, y){ return parseInt(x, 16) - parseInt(y, 16); });
var runs = [];
keys.forEach(function(x){
  var v = parseInt(x, 16);
  if (!runs.length || v > runs[runs.length - 1][1] + 8) runs.push([v, v]);
  else runs[runs.length - 1][1] = v;
});
out.diff.newCodeRuns = runs.map(function(r){ return h(r[0]) + '-' + h(r[1]) + ' (' + (r[1] - r[0] + 1) + ' bytes)'; });
out.diff.newAddressCount = keys.length;

out.csvA = A.ocode.csv;
out.csvB = B.ocode.csv;
delete A.ocode.segments; delete B.ocode.segments;
delete A.ocode.csv; delete B.ocode.csv;

out.headline = 'A (no linkage): ' + A.ocode.instructions + ' o-code, arms ' + JSON.stringify(A.arms)
  + ', ' + A.surface.hash + '/' + A.surface.colours + 'c. B (linkage): ' + B.ocode.instructions
  + ' o-code, arms ' + JSON.stringify(B.arms) + ', ' + B.surface.hash + '/' + B.surface.colours
  + 'c. Build parts after ' + out.diff.build.commonPrefix + ' of ' + out.diff.build.aLength
  + '. New code only B executed: ' + out.diff.newCodeRuns.join(' ');
return out;
