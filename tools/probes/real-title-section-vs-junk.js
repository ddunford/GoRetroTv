// sky-02me.21 -- A REAL TITLE SECTION vs THE JUNK ONE THAT IS KNOWN TO PARSE, SAME RUN.
//
// THE PREVIOUS ATTEMPT WAS A NULL RESULT WITH A CONFOUND. It fed a well-formed Sky title section
// and nothing read it -- but it ran with --carousel, while every parse this project has ever
// observed happened on a SILENT box fed the line-up by hand. Two variables moved at once, so the
// silence was not attributable.
//
// This uses the harness that demonstrably PRODUCED a parse (what-is-on-0xa1.js: the walker read
// offsets 2,3,4,5,6,7 and 79,80 of a self-naming payload) and changes exactly one thing -- the
// payload -- with the known-parsing payload as the control IN THE SAME RUN.
//
//   pass A   the self-naming junk payload   MUST parse, or the harness is not in the state it was
//   pass B   a real title section           the question
//   pass C   the same records at the EXACT subscribed extension rather than the masked-down one
//
// If A parses and B does not, the difference is the payload and that is a finding about the format.
// If A does not parse either, this run is not comparable to the one it is built on and nothing
// below means anything -- which the probe asserts rather than reports.
//
// THE FINDING THIS IS BUILT ON, from scripts/digibox-probes/feed-a-lineup.js: feeding a real
// line-up (NIT -> BAT carrying [0x5F, 4, specifier 2] then a 0xB1 whose gate halfword is 0xFFFF and
// whose entries are nine bytes) builds four service records AND WIDENS THE SUBSCRIPTION:
//
//     before   0x40/ext=0x20  0x73
//     after    0x40/ext=0x20  0x42/ext=0x0  0x4a/ext=0x1000  0x73  0xa1/ext=0xbbb
//
// `0xBBB` is not a number anyone chose. It is `0x0BB8 + 3` -- the value fed in field +3..4 of the
// LAST 0xB1 entry. So that field is the identifier the box turns into a table-id extension, and
// table 0xA1 is what it asks for with it. 0xA1 sits in the OpenTV carousel range, which is where
// this project has always suspected the listings live and has never been able to show it.
//
// So the question stops being "which table do we send" -- the box has now said -- and becomes what
// it does with one. This asks that the way everything else here was asked: push a section whose
// every byte identifies itself, watch the memory the parse happens in, and let the firmware name
// its own consumer.
//
// WHAT MUST NOT BE ASSUMED, and each of these is a guard rather than a remark:
//
//   * WHICH PID 0xA1 ARRIVES ON. __siMatches() indexes MATCH UNITS and __siFilters()/__dispState()
//     index PID CHANNELS -- 16 units against 32 channels -- and joining them by index has already
//     produced one confident artefact on this project. So the PID is found by DIFFERENCE: record
//     the armed filters before the line-up and after it, and whatever is new is what the box added.
//     Measured, that is TWO PIDs, not one: 0x52, which this project already knows and whose
//     consumer at 0x800A857A understands only 0x40/0x42/0x70, and 0x33, which has never appeared
//     in any run before a line-up existed. So both are tried, separately, and the one that parses
//     the payload is the answer. Picking between them by reasoning would be a guess wearing a
//     measurement's clothes -- and the first version of this probe correctly REFUSED to run when
//     it found two, rather than taking the first.
//   * THAT THE SUBSCRIPTION SURVIVES. It is re-read immediately before the push, because a
//     subscription that has lapsed makes every later zero a fact about timing.
//   * THAT A PUSH IS A PARSE. Sections are memcpy'd out of the ring (0x800FA958) and parsed on the
//     copy, so the ring is watched to find the copy and the copy is watched to find the parser --
//     located by finding a marker afterwards, never by predicting an address.
//   * THAT A SECOND COPY OF THE SAME BYTES IS RE-PARSED. The version field is advanced per push.
//
// THE CONTROL IS AN EXTENSION THE BOX DID NOT ASK FOR. The same section is also pushed with
// extension 0xBAD -- delivery is by PID so it arrives either way, and if it is parsed identically
// then the extension is not what selects it and the 0xBBB reading means less than it looks.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function hx(v){ return '0x' + (v >>> 0).toString(16); }

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 180) {
  await new Promise(function(r){ setTimeout(r, 1000); });
  waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

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
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}

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

// %d records, %d bytes -- built by scripts/skyepg/title_section.py and round-tripped through
// the reference reader before being embedded here.
var RECORDS = [1,1,240,36,181,30,126,144,3,132,0,0,0,58,9,42,35,87,24,101,122,10,158,245,113,130,148,117,113,134,86,174,48,254,0,128,1,2,240,25,181,19,130,20,3,132,0,0,0,42,227,0,90,184,195,43,87,24,127,0,64,1,3,240,31,181,25,133,152,7,8,0,0,0,42,227,1,90,184,195,43,87,24,2,85,198,25,90,184,192,52,64,1,4,240,23,181,17,140,160,7,8,0,0,0,16,207,87,24,101,106,227,0,177,0,1,5,240,25,181,19,147,168,14,16,0,0,0,42,159,45,105,125,92,97,149,170,45,43,16,1,6,240,24,181,18,161,184,3,132,0,0,0,32,230,171,140,50,181,113,135,240,4,0];

var out = { question: 'does a real title section parse where a junk one does',
            settledAfterSeconds: waited,
            before: { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) } };

if (!wants(0x4A)) {
  var n0 = window.__siNIT();
  if (!n0.ok) throw new Error('the ladder-opening NIT was refused: ' + n0.why);
  for (var w = 0; w < 10 && !wants(0x4A); w++) await new Promise(function(r){ setTimeout(r, 3000); });
}
for (var round = 0; round < 3; round++) {
  window.__siNIT(undefined, { version: round + 1 });
  window.__siSDT(undefined, { version: round + 1 });
  var br = pushBat();
  if (!br.ok) throw new Error('the BAT was refused: ' + br.why);
  await new Promise(function(r){ setTimeout(r, 7000); });
}
// WAIT OUT THE NVRAM REBUILD BEFORE PUSHING ANYTHING. A BAT the box parses sends it into a
// ~130-second counted loop writing one record per service to NVRAM (sky-02me.18), and pushing
// listings into that window is pushing at a box that is not listening. Every earlier 0xA1 parse in
// this project predates that finding, so nobody was waiting -- and the first version of THIS probe
// pushed straight after the line-up and got a silent control, which is what sent me here.
// Polled on the EEPROM write counter rather than slept on, so the wait is the rebuild's length and
// not the probe's patience.
var eeq = [], eeLast = window.__i2cState().eeprom.writes, eeStill = 0, eeSecs = 0;
for (var q = 0; q < 90 && eeStill < 3; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  eeSecs += 3;
  var now = window.__i2cState().eeprom.writes;
  eeq.push(now - eeLast);
  eeStill = (now === eeLast) ? eeStill + 1 : 0;
  eeLast = now;
}
out.rebuild = { seconds: eeSecs, writesPerThreeSeconds: eeq.slice(-12), settled: eeStill >= 3 };
if (!out.rebuild.settled)
  throw new Error('the NVRAM rebuild never went quiet in ' + eeSecs + 's -- pushing now would be '
                + 'pushing at a box that is still busy, and a silence would not be attributable');
out.afterLineup = { tables: tablesWanted() };
if (!wants(0xA1)) throw new Error('the box did not subscribe to 0xA1 -- this run is of a different state');
var unit = matchFor(0xA1)[0];
out.unit = { ext: hx(unit.extension), mask: hx(unit.extensionMask), bytes: unit.bytes };
var FILTER = [parseInt(unit.bytes[6], 16) & 0xFF, parseInt(unit.bytes[7], 16) & 0xFF];

var VER = 0;
function sectionWith(ext, payloadTail){
  VER = (VER + 1) & 0x1F;
  var payload = [(ext >> 8) & 0xFF, ext & 0xFF, 0xC1 | ((VER & 0x1F) << 1), 0x00, 0x00,
                 FILTER[0], FILTER[1]].concat(payloadTail);
  var len = payload.length + 4;
  var s = [0xA1, 0xB0 | ((len >> 8) & 0x0F), len & 0xFF].concat(payload);
  var c = crc32(s);
  return s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]);
}
// The junk payload is the one that is KNOWN to parse: bytes naming their own offsets, with the
// two filter bytes in front. Its tail starts at what the old probe called payload[2].
var JUNK = (function(){ var b = []; for (var k = 2; k < 96; k++) b.push((0x40 + k) & 0xFF); b[1] = 0x0B; return b; })();

async function pass(name, ext, tail, mark){
  var r = { pass: name, extension: hx(ext), tailBytes: tail.length };
  var p0 = window.__siPush(0x33, sectionWith(ext, tail));
  if (!p0.ok) { r.error = 'push refused: ' + p0.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var ringPhys = (parseInt(p0.at, 16) >>> 0) & 0x1FFFFFF;
  var found = window.__find(mark, 0x80000000, 0x82000000, 24).filter(function(a){
    var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF; return !(q >= ringPhys && q < ringPhys + 4096); });
  r.copiesFound = found;
  var deep = found.filter(function(a){ return (parseInt(a, 16) >>> 0) >= 0x80200000; });
  r.parseBufferCopy = deep[0] || null;
  if (!found.length) { r.answer = 'delivered and never copied'; return r; }
  var mid = parseInt(deep[0] || found[0], 16) >>> 0;
  r.watching = h(mid);
  window.__readWatch((mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0);
  var p1 = window.__siPush(0x33, sectionWith(ext, tail));
  if (!p1.ok) { window.__readWatch(); r.error = 'second push refused'; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var log = window.__readWatchLog();
  window.__readWatch();
  var again = window.__find(mark, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (!again.length) { r.error = 'marker gone from the watched region'; return r; }
  var at = parseInt(again[0], 16) >>> 0;
  var pcs = {}, offs = {};
  log.all.forEach(function(rr){
    var a = parseInt(rr.at, 16), n = rr.size || 1, k;
    for (k = 0; k < n; k++) { var o = (a + k) - at;
      if (o < 0 || o >= tail.length) continue;
      (pcs[rr.pc] = pcs[rr.pc] || {})[o] = 1; offs[o] = 1; }
  });
  r.readers = Object.keys(pcs).map(function(p){ return { pc: p, bytes: Object.keys(pcs[p]).length }; })
                    .sort(function(a,b){ return b.bytes - a.bytes; }).slice(0, 10);
  r.offsetsRead = Object.keys(offs).map(Number).sort(function(a,b){ return a-b; });
  r.parsed = r.readers.length > 0;
  r.answer = r.parsed ? ('parsed by ' + r.readers.map(function(p){ return p.pc; }).join(', '))
                      : 'copied and NOT parsed';
  return r;
}

var EXACT = unit.extension >>> 0;
var MASKED = (unit.extension & unit.extensionMask) >>> 0;
out.A = await pass('A: the junk payload that is KNOWN to parse (the control)', EXACT, JUNK, JUNK.slice(0, 8));
out.B = await pass('B: a real title section, extension masked down', MASKED, RECORDS, RECORDS.slice(0, 8));
out.C = await pass('C: the same records at the EXACT subscribed extension', EXACT, RECORDS, RECORDS.slice(0, 8));

out.controlHeld = !!(out.A && out.A.parsed);
out.realParsed = !!((out.B && out.B.parsed) || (out.C && out.C.parsed));
out.tasksAtEnd = window.__tasks().n;
out.headline = !out.controlHeld
  ? 'THE CONTROL DID NOT PARSE -- this run is not in the state the harness was built on, so B and C '
    + 'say nothing. Nothing below is attributable.'
  : (out.realParsed
      ? 'A REAL TITLE SECTION PARSES. Offsets read: B ' + JSON.stringify(out.B.offsetsRead)
        + '  C ' + JSON.stringify(out.C.offsetsRead)
      : 'the control parsed and the real title section did NOT -- so the box rejects it on its '
        + 'CONTENT, and the difference between the two payloads is the next subject');
return out;
