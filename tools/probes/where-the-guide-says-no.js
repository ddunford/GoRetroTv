// sky-02me.21 -- WHERE THE GUIDE DECIDES, read off the running machine rather than guessed.
//
// WHAT THE FIRST RUN ESTABLISHED (what-the-guide-reads.js, same day). A tv guide press is 44,935
// o-code instructions split by the interpreter's event loop head at 0x9FC4A538 into EIGHT events,
// the first of which is 37,610 instructions and IS the guide build. On a box holding our parsed
// Sky title records that build is BYTE-IDENTICAL -- all eight events, instruction for instruction
// -- to the same press on a box that has never seen a listings section. So the records change
// nothing the guide looks at, and the question is what it looks at INSTEAD.
//
// THE INSTRUMENT. The EPG's CODE chunk is 0x9FC4A400 + 353,188 bytes and its resources sit
// directly above it in the same flash. "Further schedule information is not available" is at
// 0x9FCAB279 -- found by searching the image, not assumed -- and nothing in the CODE chunk
// references it as a constant, which is consistent with this runtime addressing text by RESOURCE
// ID rather than by offset. So the reference cannot be found statically and the machine has to be
// asked: watch 0x9FC4A400..0x9FCB0000 in ONE range, and the rows divide themselves by address --
// below 0x9FCA0BE4 they are the o-code trace, at or above it they are the resource reads. Both
// carry icount, so the exact o-code instruction that was executing when the message was fetched
// is an arithmetic question rather than a search.
//
// TWO THINGS THAT WOULD MAKE THIS REPORT NOTHING, both asserted rather than hoped for:
//   * THE STRING MIGHT BE COPIED TO DRAM AT STARTUP and never read from flash again, in which case
//     the resource rows will not contain it. That is a real answer and the probe says so; it is not
//     the same as "the watch saw nothing", and the two are reported separately.
//   * THE WATCH MIGHT CAP. A press is ~64,000 reads over the code chunk alone, so the cap is raised
//     and `capped` is returned. A capped trace and a short one look identical in the output.
//
// AND THE DATA SEGMENT IS DUMPED, because it is readable and nobody has to infer it. DS resolves to
// 0x8045CB64 -- and that is CHECKED here rather than carried, by reading the word the earlier
// derivation was anchored on (DS[0x0001ACB0] must be 0x80477814). A dump taken against a wrong base
// is fiction that looks like evidence.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function hx(v){ return '0x' + (v >>> 0).toString(16); }

var CODE_LO = 0x9FC4A400, CODE_HI = 0x9FC4A400 + 353188;   // 0x9FCA0BE4
var WATCH_HI = 0x9FCB0000;
var MSG = 0x9FCAB279, MSG_LEN = 44;                        // "Further schedule information is not available"
var MAIN_FETCH_S = '0x80069298';
var TRACE_MAX = 400000;
// THE DATA SEGMENT IS LOCATED, NEVER CARRIED. Its first four bytes are the ASCII "RSRC" -- which
// is what the earlier derivation misread as a resource table before 0xAA turned out to be PUSH_DS.
//
// AND THE SELF-POINTER TEST THAT LOOKED LIKE THE OBVIOUS CHECK IS NOT ONE. The note recording
// DS = 0x8045CB64 anchors it on DS[0x0001ACB0] reading 0x80477814, which is DS + 0x1ACB0 -- a word
// pointing at itself. Tested here on a fresh boot it reads 0x00000001: that word is application
// STATE the box fills in later, not a structural property of the segment, so a probe gating on it
// refuses to run on a box that is perfectly fine. It is reported below and never asserted.
//
// WHAT DISCRIMINATES INSTEAD IS STRUCTURAL. "RSRC" appears TWICE in DRAM: once at 0x80078454,
// inside the decompressed application image (0x800009F4 + 1,029,144 = 0x800FC66C), which is the
// segment's INITIALISED TEMPLATE, and once in the heap above it, which is the live segment the
// interpreter pushes. Requiring exactly one candidate above the image is a test the box's state
// cannot break.
var DS = null, DS_ANCHOR_OFF = 0x0001ACB0;
var APP_IMAGE_END = 0x800FC66C;
var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];

function u32at(a){ var b = window.__peek(a, 4); return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0; }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}

var lastKeyPressed = null;
async function press(raw, label, tag, watch){
  if (lastKeyPressed === raw)
    throw new Error('HARNESS: ' + tag + ' presses ' + hx(raw) + ' twice running; a box already on that '
                  + 'screen rebuilds nothing and would read as the fault');
  lastKeyPressed = raw;
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface();
  if (watch) {
    var w = window.__readWatch(CODE_LO, WATCH_HI, { max: TRACE_MAX });
    if (w.max !== TRACE_MAX)
      throw new Error('__readWatch does not take {max} on this page; the trace would cap at ' + w.max);
  }
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 11000); });
  var lg = null;
  if (watch) { lg = window.__readWatchLog(); window.__readWatch(); }
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var s1 = surface();
  var r = { tag: tag, key: label, widgets: c.newWidget || 0, applies: c.apply || 0,
            damage: c.DAMAGE || 0, moved: s1.hash !== s0.hash, surface: s1,
            drew: (c.newWidget || 0) > 0 };
  if (lg) {
    var code = [], res = [];
    lg.all.forEach(function(e){
      var a = parseInt(e.at, 16) >>> 0;
      if (a < CODE_HI) code.push(e); else res.push(e);
    });
    var mainTotal = null;
    (lg.byPc || []).forEach(function(s){
      var p = s.split(' x'); if (p[0] === MAIN_FETCH_S) mainTotal = parseInt(p[1], 10);
    });
    if (mainTotal === null && r.drew)
      throw new Error('the interpreter fetch site is not in byPc on a press that DREW -- the watch is '
                    + 'not seeing the interpreter and every number here is a harness failure');
    r.watch = { reads: lg.reads, capped: !!lg.capped, cap: lg.max,
                codeRows: code.length, resourceRows: res.length, byPc: lg.byPc };
    // The o-code trace, in the CSV shape scripts/ocode-disasm.py reads. RESOURCE ROWS ARE EXCLUDED
    // and that is not tidying: steps() counts every non-main-site row between two fetches as
    // OPERAND bytes, so a resource read landing mid-instruction would inflate an operand length and
    // the decoder would drift -- silently, into plausible nonsense.
    r.csv = code.map(function(e){ return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }).join(';');
    // Every read of the module's resource region, with the icount that places it in the trace.
    r.resources = res.map(function(e){ return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }).join(';');
    var msg = res.filter(function(e){
      var a = parseInt(e.at, 16) >>> 0; return a >= MSG && a < MSG + MSG_LEN; });
    r.messageReads = msg.length;
    r.messageFirst = msg.length ? { pc: msg[0].pc, at: msg[0].at, icount: msg[0].icount } : null;
    r.messageNote = msg.length
      ? 'the message text WAS read out of the flash during this press'
      : (res.length
         ? 'the resource region was read ' + res.length + ' times during this press but NEVER inside '
         + 'the message string, so the text did not come from flash here -- a different answer from '
         + '"the watch saw nothing"'
         : 'NOTHING in the resource region was read at all during this press');
  }
  return r;
}

var CRC_TAB = (function(){
  var t = new Int32Array(256), i, j, c;
  for (i = 0; i < 256; i++) { c = i << 24; for (j = 0; j < 8; j++) c = (c & 0x80000000) ? ((c << 1) ^ 0x04C11DB7) : (c << 1); t[i] = c; }
  return t;
})();
function crc32(b){ var c = -1, i; for (i = 0; i < b.length; i++) c = (c << 8) ^ CRC_TAB[((c >>> 24) ^ b[i]) & 0xFF]; return c >>> 0; }
function u16(v){ return [(v >>> 8) & 0xFF, v & 0xFF]; }
function u32(v){ return [(v >>> 24) & 0xFF, (v >>> 16) & 0xFF, (v >>> 8) & 0xFF, v & 0xFF]; }
function bcd8(v, d){ var s = String(v); while (s.length < d) s = '0' + s;
  var o = []; for (var i = 0; i < d; i += 2) o.push(parseInt(s.substr(i, 2), 16) & 0xFF); return o; }
function satellite(){ return [0x43, 11].concat(bcd8(1177800, 8), bcd8(282, 4), [0x81], bcd8(275000, 8).slice(0, 4)); }
function serviceListDesc(ids){ var b = []; ids.forEach(function(s){ b = b.concat(u16(s), [0x01]); }); return [0x41, b.length].concat(b); }
var SERVICES = [0x0064, 0x0065, 0x0066, 0x0067];
var LINEUP = SERVICES.map(function(sid, i){
  return { sid: sid, f2: 0x01, f34: 0x0BB8 + i, f56: 0x1770 + i, ch: 0x0ABC + i, flags: 0x5 }; });
function armed(){ return window.__siFilters().filter(function(f){ return f.armed; }); }
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }
function matchFor(tid){ return window.__siMatches().filter(function(x){ return x.tableId === tid; }); }
var BAT_VERSION = 0;
function pushBat(){
  BAT_VERSION = (BAT_VERSION + 1) & 0x1F;
  var ids = window.__siIds();
  var bq = (ids.bouquetIdMask !== null && ids.bouquetId !== null &&
            ((0x1001 & ids.bouquetIdMask) === (ids.bouquetId & ids.bouquetIdMask))) ? 0x1001 : ids.bouquetId;
  var body = u16(0xFFFF);
  LINEUP.forEach(function(e){
    body = body.concat(u16(e.sid), [e.f2], u16(e.f34), u16(e.f56), u16(((e.ch << 4) | e.flags) & 0xFFFF)); });
  var descs = [0x5F, 4].concat(u32(2), [0xB1, body.length].concat(body), serviceListDesc(SERVICES), satellite());
  var ts = u16(ids.tsid).concat(u16(ids.networkId),
               [0xF0 | ((descs.length >>> 8) & 0x0F), descs.length & 0xFF], descs);
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
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');
window.__profile(true);

var out = { question: 'what does the guide read when it decides it has nothing to show',
            settledAfterSeconds: waited,
            before: { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) } };

// THE DS BASE IS LOCATED, NEVER CARRIED. A dump against a wrong base is fiction that reads as data.
var rsrc = window.__find('RSRC', 0x80000000, 0x82000000, 64);
out.dsCandidates = rsrc.map(function(a){
  var base = parseInt(a, 16) >>> 0;
  return { base: h(base), aboveAppImage: base >= APP_IMAGE_END,
           wordAt0x1ACB0: h(u32at(base + DS_ANCHOR_OFF)),
           selfPointerHolds: u32at(base + DS_ANCHOR_OFF) === ((base + DS_ANCHOR_OFF) >>> 0) };
});
var good = out.dsCandidates.filter(function(c){ return parseInt(c.base, 16) >>> 0 >= APP_IMAGE_END; });
if (good.length !== 1)
  throw new Error('"RSRC" appears at ' + rsrc.length + ' addresses and ' + good.length + ' of them '
                + 'lie above the application image, so the data segment is not located and every '
                + 'offset below would name a word in something else. Candidates: '
                + JSON.stringify(out.dsCandidates));
DS = parseInt(good[0].base, 16) >>> 0;
out.dsBase = h(DS);

// ---- the line-up and the listings, so the box is in the state the question is about -------------
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

var EXTS = [];
for (var e = (a1[0].extension & a1[0].extensionMask) >>> 0; e <= a1[0].extension; e++) EXTS.push(e);
var cb0 = window.__pcHits(0x800C6B38)[h(0x800C6B38)];
if (cb0 === undefined) throw new Error('__pcHits could not find the 0xB5 callback key -- harness failure');
var pushed = 0;
for (var rd = 0; rd < 5; rd++) {
  for (var ei = 0; ei < EXTS.length; ei++) {
    if (window.__siPush(0x33, buildA1(EXTS[ei])).ok) pushed++;
    await new Promise(function(x){ setTimeout(x, 1200); });
  }
}
out.sectionsPushed = pushed;
await new Promise(function(x){ setTimeout(x, 12000); });
out.b5CallbackHits = window.__pcHits(0x800C6B38)[h(0x800C6B38)] - cb0;
if (!out.b5CallbackHits)
  out.parseCaveat = 'the 0xB5 callback did not run during the feed, so the records were not walked '
                  + 'and this press is of a box that received nothing';

// ---- the press, watched ------------------------------------------------------------------------
out.presses = [];
out.presses.push(await press(0x7D, 'sky', 'settle-sky', false));
if (!out.presses[0].drew) throw new Error('the settling sky press built no widgets');
var g = await press(0x80, 'tv guide', 'guide', true);
out.presses.push(g);
await window.__shot('guide-watched');
if (!g.drew) throw new Error('the watched guide press built no widgets, so it is not the press this '
                           + 'probe means to measure');

// ---- the data segment, dumped where the guide build reads it -----------------------------------
// The offsets are the ones the first run measured the BUILD reading, taken from its own output
// rather than chosen: they are what the guide consults, so their values are what it consulted.
var DS_FIELDS = [0x019A20, 0x01A0A4, 0x01A1BC, 0x013958, 0x013A28, 0x013A2C, 0x013A30, 0x013A34,
                 0x013A38, 0x035BBC, 0x035BC0, 0x02DC14, 0x029D28, 0x0252F8, 0x0252FC, 0x025300,
                 0x025304, 0x025308, 0x02530C, 0x025310, 0x025314, 0x02531C, 0x025A80, 0x02E114,
                 0x02E118, 0x02E11C, 0x012DBC, 0x013C20, 0x0146F0, 0x0146EC, 0x018868, 0x0193F4,
                 0x0329B0, 0x01364C, 0x013650, 0x013840, 0x01378C, 0x014044, 0x034B6C, 0x034B70];
out.ds = {};
DS_FIELDS.forEach(function(o){ out.ds['DS+0x' + o.toString(16).toUpperCase().padStart(6, '0')] = h(u32at(DS + o)); });

// THE 40-BYTE RECORD ARRAY THE GUIDE BUILD READS 301 TIMES. Its base is DS+0x0252FC and its stride
// is 40, both read off the executed o-code (index = ((i<<2) + i) << 3) rather than assumed -- the
// project's note that the stride is 4 stopped at the first shift and missed the second.
out.recordArray = { base: 'DS+0x0252FC', stride: 40, rows: [] };
for (var ri = 0; ri < 24; ri++) {
  var row = [], any = false;
  for (var fo = 0; fo < 40; fo += 4) {
    var v = u32at(DS + 0x0252FC + ri * 40 + fo);
    if (v) any = true;
    row.push(h(v));
  }
  out.recordArray.rows.push({ i: ri, nonZero: any, f: row });
}

out.after = { tables: tablesWanted(), tasks: window.__tasks().n };
out.headline = 'guide press: ' + g.widgets + ' widgets, ' + g.watch.codeRows + ' code reads / '
             + g.watch.resourceRows + ' resource reads, capped=' + g.watch.capped
             + '. Message string read ' + g.messageReads + ' times. ' + g.messageNote;
return out;
