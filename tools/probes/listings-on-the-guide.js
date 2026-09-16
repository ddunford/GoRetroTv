// sky-02me.21 -- THE SAME PROBE THAT PARSES, WITH REAL TITLE RECORDS AS THE PAYLOAD.
//
// WHY THIS IS A COPY AND NOT A REWRITE. Four runs of a hand-rolled variant all reported "copied and
// NOT parsed" -- including its own known-good control -- and the reason was the harness, not the
// payload: the variant never located the PARSE BUFFER at 0x802A76xx and sat watching a first-stage
// copy at 0x801A85xx that nothing re-reads. Re-running this file verbatim parsed first time. So the
// payload is the only thing changed here, and every other line is left alone.
//
// THE PAYLOAD. Instead of 96 self-naming bytes, the two bytes the match unit demands followed by
// real Sky/OpenTV TITLE RECORDS, built by scripts/skyepg/title_section.py -- whose Huffman encoder
// round-trips through a transcription of the reference decoder and whose section is walked back
// with openTVtoXML's own arithmetic. Record layout: event id, length, tag 0xB5, start, duration,
// genre, rating, Huffman text.
//
// THE MARKER STAYS AT PAYLOAD OFFSET 8, which is where this harness looks for it -- it just lands
// inside the first record now rather than in filler. That is deliberate: moving the marker is what
// broke the variant's ability to find the parse buffer at all.
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

var out = { question: 'does this box parse a REAL Sky title section',
            settledAfterSeconds: waited,
            before: { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) } };

// ---- get the box asking for 0xA1 -----------------------------------------------------------
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
out.afterLineup = { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) };
if (!wants(0xA1))
  throw new Error('the box did not subscribe to table 0xA1 after the line-up -- feed-a-lineup.js '
                + 'measured that it does, so this run is of a different state and nothing below '
                + 'would be attributable. Tables: ' + tablesWanted());
out.a1Match = matchFor(0xA1);
var a1 = out.a1Match;

// THE PID BY DIFFERENCE, never by index. __siMatches indexes match units and __siFilters indexes
// PID channels -- 16 against 32 -- and joining them has already produced one confident artefact.
var newPids = out.afterLineup.filters.filter(function(p){ return out.before.filters.indexOf(p) < 0; });
out.newFilters = newPids;
if (!newPids.length)
  throw new Error('no new armed filter appeared for 0xA1 -- there is nowhere attributable to push, '
                + 'and pushing to an old filter would be a reading about that filter');

// ---- a section whose every byte identifies itself --------------------------------------------
// The payload is unknown, so nothing is modelled: bytes carry 0x40 + k, unique and never zero, so
// any read address names its own offset.
//
// EXCEPT THE FIRST TWO, AND THAT IS THE WHOLE REASON THE FIRST RUN READ NOTHING. The 0xA1 match
// unit does not stop at the table id and the extension -- it matches two PAYLOAD bytes exactly:
//
//     bytes: ["a1/fe", "b/ff", "bb/fc", "0/0", "0/0", "0/0", "9e/ff", "8b/ff", "0/0", "0/70"]
//
// The array indexes the FILTERABLE bytes, skipping the two length bytes, and that mapping is
// derived rather than assumed: index 0 holds 0xA1, which is section byte 0, and indices 1 and 2
// hold 0x0BBB, which is the extension at section bytes 3 and 4. So index 6 is section byte 8 --
// the first payload byte -- and the unit demands:
//
//     payload[0] == 0x9E      payload[1] == 0x8B      (payload[3] & 0x70) == 0
//
// A payload beginning 0x40 0x41 is therefore not a 0xA1 section as far as this box is concerned,
// which is exactly what the first run measured: delivered, and read by nobody. Note also
// tableIdMask 0xFE (so 0xA0 and 0xA1 both match) and extension mask 0xFFFC, which covers
// 0x0BB8..0x0BBB -- all four services in one unit.
// %d records, %d bytes -- built by scripts/skyepg/title_section.py and round-tripped through
// the reference reader before being embedded here.
// TITLES covering a full 24 hours back to back from 00:00 (12 records, 318 bytes) and
// SUMMARIES for the same event ids (12 records, 462 bytes). Both built by
// scripts/skyepg/title_section.py and each walked back with the reference reader.
var RECORDS = [2,1,240,24,181,18,0,0,14,16,0,0,0,58,235,29,218,174,48,202,244,74,2,0,2,2,240,23,181,17,14,16,14,16,0,0,0,58,9,42,35,87,24,101,69,24,64,2,3,240,23,181,17,28,32,14,16,0,0,0,5,26,174,48,202,213,198,31,192,16,2,4,240,29,181,23,42,48,14,16,0,0,0,42,227,15,197,92,97,149,137,152,230,204,171,140,20,188,64,2,5,240,24,181,18,56,64,14,16,0,0,0,56,242,139,45,127,87,24,101,80,206,32,2,6,240,26,181,20,70,80,14,16,0,0,0,56,219,50,215,245,113,134,86,174,48,254,0,128,2,7,240,28,181,22,84,96,14,16,0,0,0,56,161,111,39,58,184,193,166,174,48,202,213,22,149,136,2,8,240,25,181,19,98,112,14,16,0,0,0,58,167,171,140,50,161,87,24,101,122,37,16,2,9,240,36,181,30,112,128,14,16,0,0,0,58,9,42,35,87,24,101,122,10,158,245,113,130,148,117,113,134,86,174,48,254,0,128,2,10,240,31,181,25,126,144,14,16,0,0,0,42,227,1,90,184,195,43,87,24,2,85,198,25,90,184,192,52,64,2,11,240,25,181,19,140,160,14,16,0,0,0,42,159,45,105,125,92,97,149,170,45,43,16,2,12,240,24,181,18,154,176,14,16,0,0,0,32,230,171,140,50,181,113,135,240,4,0];
var SUMMARIES = [2,1,240,32,185,30,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,189,117,142,237,87,24,101,122,37,14,194,0,2,2,240,31,185,29,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,189,4,149,17,171,140,50,162,141,216,64,2,3,240,31,185,29,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,162,141,87,24,101,106,227,15,224,118,16,2,4,240,37,185,35,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,181,113,135,226,174,48,202,196,204,115,102,85,198,10,95,216,64,2,5,240,32,185,30,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,188,121,69,150,191,171,140,50,168,103,236,32,2,6,240,34,185,32,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,188,109,153,107,250,184,195,43,87,24,127,3,176,128,2,7,240,36,185,34,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,188,80,183,147,157,92,96,211,87,24,101,106,139,74,251,8,2,8,240,33,185,31,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,189,83,213,198,25,80,171,140,50,189,18,246,16,2,9,240,44,185,42,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,189,4,149,17,171,140,50,189,5,79,122,184,193,74,58,184,195,43,87,24,127,3,176,128,2,10,240,39,185,37,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,181,113,128,173,92,97,149,171,140,1,42,227,12,173,92,96,27,216,64,2,11,240,33,185,31,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,181,79,150,180,190,174,48,202,213,22,149,246,16,2,12,240,32,185,30,56,170,227,12,173,92,96,235,245,113,134,86,174,48,30,171,140,50,176,115,85,198,25,90,184,195,248,29,132,0];
var SIGNATURE = [0x9E, 0x8B];
// payload[0..1] are the two bytes the hardware unit filters on; payload[2..] is where an OpenTV
// title section carries its records (section byte 10). The unit also masks payload[3] with 0x70 and
// requires zero there -- RECORDS[1] is the low byte of event id 0x0101, which is 0x01, so that
// holds. Asserted below rather than hoped for, because a section the unit refuses is delivered and
// silently unread, which looks exactly like a section it could not parse.
function payload(withSignature){
  var b = [SIGNATURE[0], SIGNATURE[1]].concat(RECORDS);
  if (!withSignature) { b[0] = 0x40; b[1] = 0x41; }
  return b;
}
var PAYLOAD_LEN = payload(true).length;
if ((payload(true)[3] & 0x70) !== 0)
  throw new Error('payload[3] & 0x70 is not zero, so the match unit will refuse every section and '
                + 'every silence below would be about the filter rather than the format');
var MARK = payload(false).slice(8, 16);             // eight distinctive bytes, to find the copy
var A1_VERSION = 0;
function buildA1(ext, withSignature){
  A1_VERSION = (A1_VERSION + 1) & 0x1F;
  var p = payload(withSignature);
  var len = 5 + p.length + 4;
  var s = [0xA1, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(ext), [0xC1 | ((A1_VERSION & 0x1F) << 1), 0x00, 0x00], p);
  var c = crc32(s);
  return { bytes: s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]),
           version: A1_VERSION, payloadAt: s.length - p.length };
}

var LISR = 0x800041B4, SECTASK = 0x80004520;
function hits(){
  var r = window.__pcHits(LISR, SECTASK);
  if (r[h(LISR)] === undefined || r[h(SECTASK)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- a harness failure, not a zero');
  return { lisr: r[h(LISR)], task: r[h(SECTASK)] };
}
var WALKER = { '0x800AD5B4': 1, '0x800AD5B6': 1, '0x800AD5F2': 1, '0x800AD5F4': 1 };

async function pass(name, pid, ext, withSignature){
  var r = { pass: name, pid: hx(pid), extension: hx(ext), signature: !!withSignature };
  if (!wants(0xA1)) { r.error = 'the 0xA1 subscription has lapsed -- a zero here would be about timing'; return r; }

  var b0 = buildA1(ext, withSignature);
  var p0 = window.__siPush(pid, b0.bytes);
  if (!p0.ok) { r.error = 'push refused: ' + p0.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var ringPhys = (parseInt(p0.at, 16) >>> 0) & 0x1FFFFFF;
  var found = window.__find(MARK, 0x80000000, 0x82000000, 24).filter(function(a){
    var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF;
    return !(q >= ringPhys && q < ringPhys + 2048);
  });
  r.copiesFound = found;
  if (!found.length) {
    r.answer = 'the section was delivered and NEVER COPIED OUT OF THE RING -- nothing took it, '
             + 'which for 0xA1 is itself the finding';
    return r;
  }
  var chosen = found.filter(function(a){ return (parseInt(a, 16) >>> 0) >= 0x80200000; })[0] || found[0];
  r.watching = chosen;

  var mid = parseInt(chosen, 16) >>> 0;
  var before = hits();
  window.__readWatch((mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0);
  var b1 = buildA1(ext, withSignature);
  var p1 = window.__siPush(pid, b1.bytes);
  if (!p1.ok) { window.__readWatch(); r.error = 'second push refused: ' + p1.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var log = window.__readWatchLog();
  window.__readWatch();
  r.version = b1.version;
  r.delivery = (function(){ var a = hits(); return { lisr: a.lisr - before.lisr, sectionTask: a.task - before.task }; })();
  r.watch = { reads: log.reads, capped: log.capped ? 'CAPPED -- a sample, not a record' : false };

  var again = window.__find(MARK, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (!again.length) { r.error = 'the marker is not in the watched region afterwards -- unmappable, not zero'; return r; }
  var payloadAt = (parseInt(again[0], 16) >>> 0) - 8;

  var pcCover = {}, byOff = {};
  log.all.forEach(function(rr){
    var a = parseInt(rr.at, 16), n = rr.size || 1, k;
    for (k = 0; k < n; k++) {
      var off = (a + k) - payloadAt;
      if (off < 0 || off >= PAYLOAD_LEN) continue;
      (pcCover[rr.pc] = pcCover[rr.pc] || {})[off] = 1;
      (byOff[off] = byOff[off] || []).push(rr.pc);
    }
  });
  var pcs = Object.keys(pcCover).map(function(p){
    var n = Object.keys(pcCover[p]).length;
    return { pc: p, bytes: n, coverage: +(100 * n / PAYLOAD_LEN).toFixed(1) };
  }).sort(function(a, b){ return b.bytes - a.bytes; });
  r.payloadAt = h(payloadAt);
  r.readers = pcs.slice(0, 14);
  r.bulkCopiers = pcs.filter(function(p){ return p.coverage >= 80; });
  r.parserCandidates = pcs.filter(function(p){ return p.coverage < 80 && !WALKER[p.pc]; });
  r.offsetsRead = Object.keys(byOff).map(Number).sort(function(a, b){ return a - b; });
  r.readOrder = log.all.filter(function(rr){
    var o = parseInt(rr.at, 16) - payloadAt; return o >= 0 && o < PAYLOAD_LEN && !WALKER[rr.pc];
  }).slice(0, 60).map(function(rr){
    return rr.pc + ' -> +' + (parseInt(rr.at, 16) - payloadAt); });
  r.answer = r.parserCandidates.length
    ? 'A PARSER READ THE PAYLOAD -- ' + r.parserCandidates.map(function(p){ return p.pc; }).join(', ')
    : (r.bulkCopiers.length ? 'only bulk copiers touched it -- the parse is further on'
                            : 'nothing read the payload');
  return r;
}

// Every new PID gets the same section, so the one that parses it identifies itself.
out.byPid = [];
for (var pi = 0; pi < newPids.length; pi++) {
  var pid = parseInt(newPids[pi], 16);
  out.byPid.push(await pass('PID ' + newPids[pi] + ', ext 0xBBB, payload signed 9E 8B', pid, 0x0BBB, true));
}
// TWO CONTROLS, BOTH ON WHICHEVER PID ANSWERED, because a control on a silent PID proves nothing.
var responder = out.byPid.filter(function(x){ return x.parserCandidates && x.parserCandidates.length; })[0];
if (responder) {
  var rp = parseInt(responder.pid, 16);
  // The signature is the thing the first run got wrong, so vary it first: same PID, same
  // extension, payload starting 0x40 0x41. It must go silent, or the signature is not what
  // selects the section and the reading above is about something else.
  out.controlSignature = await pass('control: same PID and extension, payload NOT signed', rp, 0x0BBB, false);
  // And an extension the box did not ask for, to show the unit's extension mask is real.
  out.controlExtension = await pass('control: same PID, signed, extension 0xBAD outside the mask', rp, 0x0BAD, true);
} else {
  out.controlSignature = { skipped: 'no PID parsed the payload, so there is nothing to vary' };
  out.controlExtension = { skipped: 'no PID parsed the payload, so there is nothing to vary' };
}
out.after = { tables: tablesWanted() };

// ---- AND NOW LOOK AT THE GUIDE -------------------------------------------------------------
// The parse is established above; this asks the only question left, which is whether any of it
// reaches the screen. Feed every channel the unit's mask covers, repeatedly, as a real carousel
// would -- one section carries one channel's day, and a guide with one channel's worth of data may
// have nothing to draw a row from.
var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }, { pc: 0x80082604, name: 'apply', args: 1 }];
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
var lastKey = null;
async function press(raw, label){
  if (lastKey === raw) throw new Error('HARNESS: ' + label + ' repeats a key; a box already on that screen rebuilds nothing');
  lastKey = raw;
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 11000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var s1 = surface();
  return { key: label, widgets: c.newWidget || 0, applies: c.apply || 0,
           moved: s1.hash !== s0.hash, colours: s1.colours, hash: s1.hash };
}

var rp = 0x33;
out.guide = {};
// WAIT OUT THE NVRAM REBUILD BEFORE TAKING THE BASELINE. The previous run's "before" presses built
// ZERO widgets at 12 colours -- the blank screen of a box still rebuilding (sky-02me.18) -- so its
// "the guide changed" was the rebuild finishing, not the listings. A baseline taken from a box that
// cannot draw is not a baseline.
var eeLast = window.__i2cState().eeprom.writes, eeStill = 0, eeSecs = 0;
for (var q = 0; q < 90 && eeStill < 3; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  eeSecs += 3;
  var nowW = window.__i2cState().eeprom.writes;
  eeStill = (nowW === eeLast) ? eeStill + 1 : 0;
  eeLast = nowW;
}
out.guide.rebuildSeconds = eeSecs;
out.guide.beforeSky   = await press(0x7D, 'sky, before the listings');
out.guide.beforeGuide = await press(0x80, 'tv guide, before the listings');
await window.__shot('guide-before-listings');
if (!out.guide.beforeGuide.widgets)
  throw new Error('the BASELINE guide press built no widgets, so the box was not drawing before the '
                + 'listings were fed and any later change would be the box recovering rather than '
                + 'the listings arriving -- which is exactly what the previous run measured');

// Every extension the unit's mask covers, several rounds.
var EXTS = [];
for (var e = (a1[0].extension & a1[0].extensionMask) >>> 0; e <= a1[0].extension; e++) EXTS.push(e);
out.guide.extensionsFed = EXTS.map(hx);
var pushed = 0, refused = 0;
for (var round = 0; round < 5; round++) {
  for (var ei = 0; ei < EXTS.length; ei++) {
    var b1 = buildA1(EXTS[ei], true);
    var pr = window.__siPush(rp, b1.bytes);
    if (pr.ok) pushed++; else refused++;
    await new Promise(function(x){ setTimeout(x, 1200); });
  }
}
out.guide.sectionsPushed = pushed;
out.guide.sectionsRefused = refused;
await new Promise(function(x){ setTimeout(x, 12000); });

out.guide.afterSky   = await press(0x7D, 'sky, after the listings');
out.guide.afterGuide = await press(0x80, 'tv guide, after the listings');
await window.__shot('guide-after-listings');
out.guide.guideChanged = out.guide.afterGuide.hash !== out.guide.beforeGuide.hash;
out.guide.verdict = out.guide.guideChanged
  ? 'THE GUIDE SCREEN CHANGED after the listings were fed -- look at the screenshots'
  : 'the guide is byte-identical before and after, so the parse does not reach the screen yet';
out.note = responder
  ? 'the unsigned control must go silent and the out-of-mask extension must go silent; if either '
  + 'reads the payload the same way, that field is not what selects the section'
  : 'no parser touched a 0xA1 payload on any new PID even with the signature the match unit '
  + 'demands -- the next thing to vary is the rest of the header the unit does not constrain';
return out;
