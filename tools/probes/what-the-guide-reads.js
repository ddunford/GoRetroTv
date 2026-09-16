// sky-02me.21 -- WHAT DOES THE GUIDE READ WHEN IT DECIDES IT HAS NOTHING TO SHOW?
//
// The box PARSES the Sky title records we generate -- our bytes, at our offsets, both controls
// silent -- and the guide still says "Further schedule information is not available". The static
// reading says why the title path cannot be the store: the 0xA1 walk's callback writes start and
// duration into a STACK LOCAL at sp+0x10 and the caller immediately doubles it and compares. That
// is a time LOOKUP the guide consults, not the rows it draws.
//
// So the next thing to learn is not a fifth section format. It is what the GUIDE itself does, and
// the guide is OpenTV BYTECODE. Every instrument that has measured this box so far watched MIPS.
//
// TWO MEASUREMENTS, ONE RUN, and they answer different halves:
//
//   1. A FULL o-code trace of a tv guide press -- every read of the EPG's CODE chunk with the PC
//      that made it, UNFILTERED, so the operand bytes are in it too and scripts/ocode-disasm.py
//      can decode the executed path rather than merely list addresses. Taken twice: once before
//      any listings exist and once on a box holding our parsed records. If the two streams are
//      identical the sections changed nothing the guide looks at; if they part, the parting IS the
//      decision.
//   2. __pcHits across the TWELVE call sites that hold the 0xB5 callback as a pool word, plus the
//      callback itself, the walker, the Huffman decompressor and the two carousel parsers --
//      sampled at four points, so each window is attributable. Which caller runs says which
//      subsystem asks the time question; whether 0x800BECF0 EVER runs is the single best marker
//      for "a compressed module arrived and was accepted", and our own title text is Huffman.
//
// THE LINE-UP IS COPIED VERBATIM FROM listings-on-the-guide.js, which parses. Four runs of a
// hand-rolled variant of that probe measured the harness rather than the box; the rule earned
// there is to copy the working probe and change ONE thing. The one thing changed here is what is
// watched during the presses.
//
// THE PARSE IS PROVED BY THE CALLBACK, NOT BY A MEMORY MARKER. __pcHits on 0x800C6B38 (the 0xB5
// callback's real entry) rising across the push window is a direct statement that our records were
// walked and the tag consumed -- cheaper than the find-the-copy dance and not subject to which of
// the three copies the search happened to land on. If it does not rise, every later silence is
// about delivery and this probe says so rather than concluding.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function hx(v){ return '0x' + (v >>> 0).toString(16); }

var OCODE_LO = 0x9FC4A400, OCODE_LEN = 353188;
var MAIN_FETCH_S = '0x80069298';
var TRACE_MAX = 300000;
var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];

// The twelve call sites that hold 0x800C6B39 as a pool word, each pairing with a FUN_800c64cc
// call a few bytes later, plus the four other markers. Named so the output reads as evidence.
var SITES = [
  { a: 0x800C6B38, n: 'cb0xB5-entry' },
  { a: 0x800C64CC, n: 'walker-FUN_800c64cc' },
  { a: 0x800C1610, n: 'site-800c1610' },
  { a: 0x800C194E, n: 'site-800c194e' },
  { a: 0x800C2FD6, n: 'site-800c2fd6' },
  { a: 0x800C309A, n: 'site-800c309a' },
  { a: 0x800C35B6, n: 'site-800c35b6' },
  { a: 0x800C6F46, n: 'site-800c6f46' },
  { a: 0x800C72D2, n: 'site-800c72d2 (the time compare)' },
  { a: 0x800C7380, n: 'site-800c7380' },
  { a: 0x800C7496, n: 'site-800c7496' },
  { a: 0x800C7592, n: 'site-800c7592' },
  { a: 0x800C75FC, n: 'site-800c75fc' },
  { a: 0x800C7722, n: 'site-800c7722' },
  { a: 0x800BECF0, n: 'huffman-0x800BECF0' },
  { a: 0x800C95D0, n: 'carousel-parser-0x800C95D0' },
  { a: 0x800C9CA0, n: 'carousel-parser-0x800C9CA0' },
  { a: 0x800C9BC4, n: 'walker-callsite-in-carousel-0x800C9BC4' },
  { a: 0x800C9BF4, n: 'walker-callsite-in-carousel-0x800C9BF4' }
];
function census(){
  var r = window.__pcHits.apply(null, SITES.map(function(s){ return s.a; }));
  var o = {};
  SITES.forEach(function(s){
    if (r[h(s.a)] === undefined)
      throw new Error('__pcHits did not return the key for ' + s.n + ' -- a census that cannot find '
                    + 'its own subject is a harness failure, never a count of zero');
    o[s.n] = r[h(s.a)];
  });
  return o;
}
function censusDiff(a, b){
  var o = {};
  Object.keys(b).forEach(function(k){ if (b[k] !== a[k]) o[k] = b[k] - a[k]; });
  return o;
}

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

// A box already on a screen rebuilds nothing, so the same key twice running reports zero widgets
// for a perfectly healthy box. Mechanised rather than remembered -- it caught two probes in one
// session, both of which had the screen they pressed FROM in their own output.
var lastKeyPressed = null;
async function press(raw, label, tag, from, trace){
  if (lastKeyPressed === raw)
    throw new Error('HARNESS: ' + tag + ' presses raw ' + hx(raw) + ' twice running; a box already on '
                  + 'that screen has nothing to rebuild and would read as the fault. Alternate.');
  lastKeyPressed = raw;
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface(), b0 = window.__blitLog().length;
  var w = null;
  if (trace) {
    // UNFILTERED on purpose: the main fetch site reads opcode bytes and every other site reads
    // OPERANDS, so a PC-filtered trace is a list of addresses while an unfiltered one is a
    // program. ocode-disasm.py needs the operands to derive its lengths and to decode.
    w = window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN, { max: TRACE_MAX });
    if (w.max !== TRACE_MAX)
      throw new Error('this page\'s __readWatch does not take {max}; the trace would cap at '
                    + w.max + ' and a capped stream reads exactly like a short one');
  }
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 11000); });
  var lg = null;
  if (trace) { lg = window.__readWatchLog(); window.__readWatch(); }
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var s1 = surface();

  var r = { tag: tag, key: label, pressedFrom: from,
            widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
            blits: window.__blitLog().length - b0,
            moved: s1.hash !== s0.hash, surface: s1, drew: (c.newWidget || 0) > 0 };
  if (lg) {
    var mainTotal = null;
    (lg.byPc || []).forEach(function(s){
      var p = s.split(' x'); if (p[0] === MAIN_FETCH_S) mainTotal = parseInt(p[1], 10);
    });
    if (mainTotal === null && r.drew)
      throw new Error('the interpreter fetch site ' + MAIN_FETCH_S + ' is not in byPc on a press that '
                    + 'DREW -- the watch is not seeing the interpreter, so every number here is a '
                    + 'harness failure rather than a measurement');
    r.ocode = { instructions: mainTotal, reads: lg.reads, capped: !!lg.capped, cap: lg.max,
                byPc: lg.byPc };
    // The trace in exactly the CSV shape scripts/ocode-disasm.py reads: pc,at,size,icount.
    // Returned as ONE string, because 150,000 objects pretty-printed is megabytes of whitespace.
    r.csv = lg.all.map(function(e){ return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }).join(';');
    // The opcode stream alone, for the diff -- the addresses fetched from the main site, in order.
    r.stream = lg.all.filter(function(e){ return e.pc === MAIN_FETCH_S; })
                     .map(function(e){ return e.at; }).join(' ');
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
function serviceListDesc(ids){
  var b = []; ids.forEach(function(s){ b = b.concat(u16(s), [0x01]); }); return [0x41, b.length].concat(b);
}
var SERVICES = [0x0064, 0x0065, 0x0066, 0x0067];
var LINEUP = SERVICES.map(function(sid, i){
  return { sid: sid, f2: 0x01, f34: 0x0BB8 + i, f56: 0x1770 + i, ch: 0x0ABC + i, flags: 0x5 };
});
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
    body = body.concat(u16(e.sid), [e.f2], u16(e.f34), u16(e.f56), u16(((e.ch << 4) | e.flags) & 0xFFFF));
  });
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

// The records are the ones listings-on-the-guide.js fed and this box parsed: 12 titles covering a
// full 24 hours back to back from 00:00, built by scripts/skyepg/title_section.py and each walked
// back with a transcription of the reference reader.
var RECORDS = [2,1,240,24,181,18,0,0,14,16,0,0,0,58,235,29,218,174,48,202,244,74,2,0,2,2,240,23,181,17,14,16,14,16,0,0,0,58,9,42,35,87,24,101,69,24,64,2,3,240,23,181,17,28,32,14,16,0,0,0,5,26,174,48,202,213,198,31,192,16,2,4,240,29,181,23,42,48,14,16,0,0,0,42,227,15,197,92,97,149,137,152,230,204,171,140,20,188,64,2,5,240,24,181,18,56,64,14,16,0,0,0,56,242,139,45,127,87,24,101,80,206,32,2,6,240,26,181,20,70,80,14,16,0,0,0,56,219,50,215,245,113,134,86,174,48,254,0,128,2,7,240,28,181,22,84,96,14,16,0,0,0,56,161,111,39,58,184,193,166,174,48,202,213,22,149,136,2,8,240,25,181,19,98,112,14,16,0,0,0,58,167,171,140,50,161,87,24,101,122,37,16,2,9,240,36,181,30,112,128,14,16,0,0,0,58,9,42,35,87,24,101,122,10,158,245,113,130,148,117,113,134,86,174,48,254,0,128,2,10,240,31,181,25,126,144,14,16,0,0,0,42,227,1,90,184,195,43,87,24,2,85,198,25,90,184,192,52,64,2,11,240,25,181,19,140,160,14,16,0,0,0,42,159,45,105,125,92,97,149,170,45,43,16,2,12,240,24,181,18,154,176,14,16,0,0,0,32,230,171,140,50,181,113,135,240,4,0];
var SIGNATURE = [0x9E, 0x8B];
function payload(){ return [SIGNATURE[0], SIGNATURE[1]].concat(RECORDS); }
if ((payload()[3] & 0x70) !== 0)
  throw new Error('payload[3] & 0x70 is not zero, so the match unit refuses every section and every '
                + 'silence below would be about the filter rather than about the guide');
var A1_VERSION = 0;
function buildA1(ext){
  A1_VERSION = (A1_VERSION + 1) & 0x1F;
  var p = payload();
  var len = 5 + p.length + 4;
  var s = [0xA1, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(ext), [0xC1 | ((A1_VERSION & 0x1F) << 1), 0x00, 0x00], p);
  var c = crc32(s);
  return s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]);
}

// ---- state assertions ------------------------------------------------------------------------
var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
if (window.__siCarousel().running !== false)
  throw new Error('the carousel is RUNNING -- this probe feeds by hand and would be measuring the '
                + 'thing it isolates');
window.__profile(true);

var out = { question: 'what does the guide read when it decides it has nothing to show',
            settledAfterSeconds: waited,
            before: { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) } };
out.census = { atStart: census() };

// ---- the line-up, so the box asks for 0xA1 ----------------------------------------------------
if (!wants(0x4A)) {
  var n0 = window.__siNIT();
  if (!n0.ok) throw new Error('the ladder-opening NIT was refused: ' + n0.why);
  for (var w0 = 0; w0 < 10 && !wants(0x4A); w0++) await new Promise(function(r){ setTimeout(r, 3000); });
}
if (!wants(0x4A))
  throw new Error('no 0x4A subscription after the NIT -- a BAT now would be dropped unread and every '
                + 'later zero would be about delivery. Tables: ' + tablesWanted());
for (var round = 0; round < 3; round++) {
  window.__siNIT(undefined, { version: round + 1 });
  window.__siSDT(undefined, { version: round + 1 });
  var br = pushBat();
  if (!br.ok) throw new Error('the BAT was refused: ' + br.why);
  await new Promise(function(r){ setTimeout(r, 7000); });
}
out.afterLineup = { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) };
if (!wants(0xA1))
  throw new Error('the box did not subscribe to 0xA1 after the line-up, so this run is of a '
                + 'different state and nothing below is attributable. Tables: ' + tablesWanted());
var a1 = matchFor(0xA1);
out.a1Match = a1;

// THE PID BY DIFFERENCE, never by index -- 16 match units against 32 PID channels, and joining
// them has already produced one confident artefact on this project.
var newPids = out.afterLineup.filters.filter(function(p){ return out.before.filters.indexOf(p) < 0; });
out.newFilters = newPids;
var LISTINGS_PID = 0x33;
if (newPids.map(function(p){ return parseInt(p, 16); }).indexOf(LISTINGS_PID) < 0)
  throw new Error('PID 0x33 is not among the filters the line-up opened (' + newPids.join(' ') + ') -- '
                + 'listings-on-the-guide.js measured that it is, so pushing there now would be a '
                + 'reading about some other filter');

// ---- wait out the NVRAM rebuild before taking ANY baseline -------------------------------------
// A baseline press taken from a box still rebuilding its channel list builds zero widgets at 12
// colours, which reads exactly like the fault. sky-02me.18.
var eeLast = window.__i2cState().eeprom.writes, eeStill = 0, eeSecs = 0;
for (var q = 0; q < 90 && eeStill < 3; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  eeSecs += 3;
  var nowW = window.__i2cState().eeprom.writes;
  eeStill = (nowW === eeLast) ? eeStill + 1 : 0;
  eeLast = nowW;
}
out.rebuildQuietAfterSeconds = eeSecs;
out.census.afterLineup = census();

// ---- MEASUREMENT 1a: the guide press with a line-up and NO listings ----------------------------
out.presses = [];
out.presses.push(await press(0x7D, 'sky', 'settle-sky', 'the boot screen', false));
if (!out.presses[0].drew)
  throw new Error('the settling sky press built no widgets, so the box is not drawing at all and '
                + 'every later quiet reading would be about that rather than about the listings');
var baseGuide = await press(0x80, 'tv guide', 'baseline-guide', 'the menu', true);
out.presses.push(baseGuide);
await window.__shot('baseline-guide');
if (!baseGuide.drew)
  throw new Error('the BASELINE guide press built no widgets -- the box was not drawing before the '
                + 'listings were fed, so any later change would be it recovering rather than the '
                + 'listings arriving. That is exactly what an earlier run measured and reported as '
                + 'a finding.');
out.presses.push(await press(0x7D, 'sky', 'between-sky', 'the guide', false));

// ---- MEASUREMENT 2: feed the listings, and watch the twelve call sites -------------------------
var EXTS = [];
for (var e = (a1[0].extension & a1[0].extensionMask) >>> 0; e <= a1[0].extension; e++) EXTS.push(e);
out.extensionsFed = EXTS.map(hx);
var censusBeforeFeed = census();
var pushed = 0, refused = 0;
for (var rd = 0; rd < 5; rd++) {
  for (var ei = 0; ei < EXTS.length; ei++) {
    var pr = window.__siPush(LISTINGS_PID, buildA1(EXTS[ei]));
    if (pr.ok) pushed++; else refused++;
    await new Promise(function(x){ setTimeout(x, 1200); });
  }
}
out.sectionsPushed = pushed;
out.sectionsRefused = refused;
await new Promise(function(x){ setTimeout(x, 12000); });
out.census.afterFeed = census();
out.census.duringFeed = censusDiff(censusBeforeFeed, out.census.afterFeed);
// THE PARSE, PROVED BY THE CALLBACK ITSELF rather than by finding a copy in memory.
out.parsed = (out.census.duringFeed['cb0xB5-entry'] || 0) > 0;
if (!out.parsed)
  out.parseCaveat = 'the 0xB5 callback did not run during the feed window, so these sections were '
                  + 'NOT walked and every guide reading below is about a box that received nothing. '
                  + 'Treat the trace diff as void.';

// ---- MEASUREMENT 1b: the same press on a box holding the parsed records ------------------------
var censusBeforePress = census();
var fedGuide = await press(0x80, 'tv guide', 'fed-guide', 'the menu', true);
out.presses.push(fedGuide);
await window.__shot('fed-guide');
out.census.duringFedGuidePress = censusDiff(censusBeforePress, census());
out.presses.push(await press(0x7D, 'sky', 'after-sky', fedGuide.drew ? 'the guide' : 'the menu', false));

out.after = { tables: tablesWanted(), tasks: window.__tasks().n };
if (out.after.tasks < 42)
  out.caveat = 'the box lost tasks during the walk (' + out.after.tasks + ') -- the readings are suspect';

// ---- the comparison, as numbers ----------------------------------------------------------------
var A = baseGuide.stream ? baseGuide.stream.split(' ') : [];
var B = fedGuide.stream ? fedGuide.stream.split(' ') : [];
var i = 0; while (i < A.length && i < B.length && A[i] === B[i]) i++;
out.compare = {
  baselineInstructions: baseGuide.ocode.instructions, fedInstructions: fedGuide.ocode.instructions,
  baselineLogged: A.length, fedLogged: B.length,
  baselineCapped: baseGuide.ocode.capped, fedCapped: fedGuide.ocode.capped,
  baselineWidgets: baseGuide.widgets, fedWidgets: fedGuide.widgets,
  surfaceIdentical: baseGuide.surface.hash === fedGuide.surface.hash,
  commonPrefix: i
};
if (i < A.length && i < B.length) {
  out.compare.divergeAfter = i;
  out.compare.lastCommon = A.slice(Math.max(0, i - 12), i).join(' ');
  out.compare.baselineNext = A.slice(i, i + 12).join(' ');
  out.compare.fedNext = B.slice(i, i + 12).join(' ');
} else if (A.length === B.length) {
  out.compare.divergeAfter = 'the logged opcode streams are IDENTICAL'
    + ((baseGuide.ocode.capped || fedGuide.ocode.capped) ? ' UP TO THE CAP, which is not the same thing' : '');
} else {
  out.compare.divergeAfter = 'one stream simply ENDS at ' + i + ' -- shorter, not different';
}

out.headline = (out.parsed ? 'records PARSED (' + out.census.duringFeed['cb0xB5-entry'] + ' callback hits). '
                           : 'records NOT parsed -- the diff is void. ')
  + 'baseline guide ' + baseGuide.ocode.instructions + ' o-code instructions / ' + baseGuide.widgets
  + ' widgets; fed guide ' + fedGuide.ocode.instructions + ' / ' + fedGuide.widgets
  + '; streams ' + (typeof out.compare.divergeAfter === 'number'
                    ? ('part after ' + out.compare.divergeAfter) : out.compare.divergeAfter);
return out;
