// sky-02me.20 -- DOES TAG 0xBC CARRY THE PROGRAMMES? Nine-byte entries, fed and watched.
//
// WHERE THIS COMES FROM. sky-02me.17 swept all 256 tags of the 0xA1 payload and found exactly one
// consumed: 0xB5, at 0x800C6B42, which lifts two big-endian u16 out of desc[2..3] and desc[4..5]
// and returns 1. Two u16 and stop is a reference or a window, not a programme list.
//
// Decoding every addiu rX,-NN in 0x800C6000-0x800C7A00 from the MIPS16 EXTEND encoding found the
// rest of the family: a dispatcher at 0x800C6930 switching on tag-0xB3 with arms for 0xB3..0xB8,
// and -- the reason this probe exists -- 0x800C6C66, which handles tag 0xBC and immediately
// computes desc[1] / 9. NINE-BYTE ENTRIES, exactly the shape the BAT's 0xB1 uses for its
// per-service records (sky-eluc.38). That is what a programme list looks like.
//
// 0xBC GOES FIRST, AND THAT IS NOT A STYLE CHOICE. The 0xB5 callback returns 1 and FUN_800c64cc's
// loop is `if (cb(...) != 0) return`, so consuming a descriptor ENDS the walk -- which is why the
// sweep pass carrying 0xB5 covered only 53 of its 64 tags. A payload with 0xB5 in front of 0xBC
// might never reach 0xBC, and its silence would be meaningless. Pass C feeds exactly that ordering
// ON PURPOSE, as a positive control for the truncation claim itself.
//
// AND THE COVERAGE GUARD IS STILL THE POINT. Every pass asserts the walk reached its LAST
// descriptor. A pass that did not is "I could not check", never "nothing is consumed".
//
// THE STATE THIS BUILDS ON (docs/reference/digibox-emulation.md, "Table 0xA1 on PID 0x33"):
// feeding a line-up makes the box subscribe to 0xA1 with the per-service id from 0xB1 entry field
// +3..4 as the extension. Listings arrive on PID 0x33 -- NOT 0x52. The hardware match unit demands
// payload[0]=0x9E, payload[1]=0x8B and (payload[3] & 0x70)==0; without that signature nothing reads
// the payload at all, which is exactly what the first attempt measured. Signed, FUN_800c64cc walks
// the payload from offset 6 as a tag/length descriptor loop and calls back on the tag it wants:
//
//     int FUN_800c64cc(base, index, wantedTag, userData)          // callback in $t0
//       end = *(int *)(rec + 0x1C);
//       for (p = *(char **)(rec + 0x14); p < end; p += p[1] + 2)
//           if (*p == wantedTag || wantedTag == -1) callback(p, i++, userData);
//
// It walked the probe payload exactly (tag 0x46 len 71 -> tag 0x8F len 144 -> past the end, stop)
// and fired NO callback, because neither tag was it. So: sweep the space, and whichever tag makes
// a non-iterator PC read a descriptor BODY is the one the listings come in.
//
// TAG 0x00 IS REACHABLE HERE and that is the one way this sweep is easier than the BAT's. That loop
// terminated on a zero tag, so 0x00 could not be probed and "we swept every tag" was false by one.
// This one is bounded by a POINTER. But "reachable" is a prediction, not a fact, so 0x00 gets its
// own pass with two descriptors AFTER it: if those are still walked, 0x00 is a tag like any other,
// and if they are not, 0x00 terminates this loop too and the pass says so instead of reporting a
// silent negative.
//
// THE COVERAGE GUARD IS THE POSITIVE CONTROL, and without it every pass could report a clean
// negative while examining nothing. A sweep that says "no tag in 0x01..0x40 is consumed" is only
// worth something if the walk actually reached 0x40. So each pass asserts that the iterator read
// the tag byte of its LAST descriptor. A pass that cannot show that is a HARNESS failure and is
// reported as one -- "I could not check" and "I checked and found nothing" are different answers.
//
// THE COPY, NEVER THE RING. Sections are memcpy'd out of the ring (0x800FA958) and parsed on the
// copy, so a ring watch sees only the copier. The region is located by pushing once, finding the
// pass's own marker bytes and watching a few KB around them -- never by predicting an address,
// because a wrong prediction returns a clean zero indistinguishable from a parser reading nothing.
//
// THE TWO NEGATIVE CONTROLS THE ISSUE ASKS FOR run in this same run, on a payload that DID walk:
// unsigned (must go silent) and an extension outside the unit's 0xFFFC mask (must go silent). A
// control from a different run is a control that is an assumption.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function hx(v){ return '0x' + (v >>> 0).toString(16); }

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
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
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }
function matchFor(tid){ return window.__siMatches().filter(function(x){ return x.tableId === tid; }); }
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}
function armed(){ return window.__siFilters().filter(function(f){ return f.armed; }); }
var LISR = 0x800041B4, SECTASK = 0x80004520;
function hits(){
  var r = window.__pcHits(LISR, SECTASK);
  if (r[h(LISR)] === undefined || r[h(SECTASK)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- a harness failure, not a zero');
  return { lisr: r[h(LISR)], task: r[h(SECTASK)] };
}

// ---- get the box asking for 0xA1, exactly as what-is-on-0xa1.js did ---------------------------
var SERVICES = [0x0064, 0x0065, 0x0066, 0x0067];
var LINEUP = SERVICES.map(function(sid, i){
  return { sid: sid, f2: 0x01, f34: 0x0BB8 + i, f56: 0x1770 + i, ch: 0x0ABC + i, flags: 0x5 };
});
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

var out = { question: 'which descriptor tag does the 0xA1 listings callback want',
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
out.afterLineup = { tables: tablesWanted(), filters: armed().map(function(f){ return f.pid; }) };
if (!wants(0xA1))
  throw new Error('the box did not subscribe to 0xA1 after the line-up -- this run is of a '
                + 'different state and nothing below would be attributable. Tables: ' + tablesWanted());
out.a1Match = matchFor(0xA1);
var A1_PID = 0x33;
if (!armed().some(function(f){ return parseInt(f.pid, 16) === A1_PID; }))
  throw new Error('no armed filter on PID 0x33 -- pushing there would be a reading about nothing');

// ---- the payload ------------------------------------------------------------------------------
// payload[0..1] is the signature the match unit demands; payload[2..5] is the word the walk reads
// before the loop. Those four bytes keep the values the first successful parse used, because the
// only state in which the walk is KNOWN to run is the one it ran in. payload[3] must satisfy
// (payload[3] & 0x70) == 0 or the unit does not match at all.
var HEAD = [0x9E, 0x8B, 0x42, 0x0B, 0x44, 0x45];
var BODY_LEN = 4;
// The iterator's own reads, from the decompile. Anything ELSE that touches the loop is a callback.
var WALKER = { '0x800C64F2': 1, '0x800C64FA': 1, '0x800C651E': 1,
               '0x800C64F0': 1, '0x800C64F4': 1, '0x800C64FC': 1, '0x800C651C': 1, '0x800C6520': 1 };

function buildLoop(tags){
  var descs = [], layout = [];
  tags.forEach(function(t){
    layout.push({ off: descs.length, len: BODY_LEN + 2, tag: t });
    descs = descs.concat([t, BODY_LEN, t, 0xA5, t, 0x5A]);
  });
  return { descs: descs, layout: layout };
}
var A1_VERSION = 0;
function buildA1(descs, ext, signed){
  A1_VERSION = (A1_VERSION + 1) & 0x1F;
  var p = HEAD.slice().concat(descs);
  if (!signed) { p[0] = 0x40; p[1] = 0x41; }
  var len = 5 + p.length + 4;
  if (len > 4093) throw new Error('section too long (' + len + ') -- split the pass');
  var s = [0xA1, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(ext), [0xC1 | ((A1_VERSION & 0x1F) << 1), 0x00, 0x00], p);
  var c = crc32(s);
  return { bytes: s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]),
           version: A1_VERSION, loopOffsetInPayload: HEAD.length };
}

async function pass(name, tags, opts){
  opts = opts || {};
  var ext = opts.ext === undefined ? 0x0BBB : opts.ext;
  var signed = opts.signed === undefined ? true : opts.signed;
  var L = buildLoop(tags);
  var last = L.layout[L.layout.length - 1];
  var mark = { off: last.off, tag: last.tag, pattern: [last.tag, BODY_LEN, last.tag, 0xA5, last.tag, 0x5A] };
  var r = { pass: name, tags: tags.length, first: hx(tags[0]), last: hx(tags[tags.length - 1]),
            extension: hx(ext), signed: signed, marker: hx(mark.tag), loopBytes: L.descs.length };

  if (!wants(0xA1)) { r.error = 'the 0xA1 subscription lapsed -- a zero here would be about timing'; return r; }

  var b0 = buildA1(L.descs, ext, signed);
  var p0 = window.__siPush(A1_PID, b0.bytes);
  if (!p0.ok) { r.error = 'push refused: ' + p0.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var ringPhys = (parseInt(p0.at, 16) >>> 0) & 0x1FFFFFF;
  var found = window.__find(mark.pattern, 0x80000000, 0x82000000, 24).filter(function(a){
    var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF;
    return !(q >= ringPhys && q < ringPhys + 4096);
  });
  r.copiesFound = found;
  if (!found.length) {
    r.answer = 'DELIVERED AND NEVER COPIED OUT OF THE RING -- nothing took it';
    r.coverage = 'unknown: there was no copy to watch';
    return r;
  }
  var chosen = found.filter(function(a){ return (parseInt(a, 16) >>> 0) >= 0x80200000; })[0] || found[0];
  r.watching = chosen;
  var mid = parseInt(chosen, 16) >>> 0;
  var before = hits();
  window.__readWatch((mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0);
  var b1 = buildA1(L.descs, ext, signed);
  var p1 = window.__siPush(A1_PID, b1.bytes);
  if (!p1.ok) { window.__readWatch(); r.error = 'second push refused: ' + p1.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var log = window.__readWatchLog();
  window.__readWatch();
  r.version = b1.version;
  r.delivery = (function(){ var a = hits(); return { lisr: a.lisr - before.lisr, sectionTask: a.task - before.task }; })();
  r.watch = { reads: log.reads, capped: log.capped ? 'CAPPED -- a sample, not a record' : false };

  var again = window.__find(mark.pattern, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (!again.length) { r.error = 'the marker is not in the watched region afterwards -- unmappable, not zero'; return r; }
  var loopAt = (parseInt(again[0], 16) >>> 0) - mark.off;
  r.descriptorLoopAt = h(loopAt);

  var byTag = {}, pcCover = {};
  L.layout.forEach(function(d){ byTag[d.tag] = { tag: 0, len: 0, body: 0 }; });
  log.all.forEach(function(rr){
    var a = parseInt(rr.at, 16), n = rr.size || 1, k;
    for (k = 0; k < n; k++) {
      var off = (a + k) - loopAt;
      if (off < 0 || off >= L.descs.length) continue;
      (pcCover[rr.pc] = pcCover[rr.pc] || {})[off] = 1;
      var d = L.layout[Math.floor(off / (BODY_LEN + 2))];
      if (!d) continue;
      var e = byTag[d.tag];
      if (off === d.off) e.tag++; else if (off === d.off + 1) e.len++; else e.body++;
    }
  });
  r.readers = Object.keys(pcCover).map(function(p){
    var n = Object.keys(pcCover[p]).length;
    return { pc: p, bytes: n, coverage: +(100 * n / L.descs.length).toFixed(1) };
  }).sort(function(a, b){ return b.bytes - a.bytes; }).slice(0, 14);
  r.nonWalkerReaders = r.readers.filter(function(p){ return !WALKER[p.pc]; });
  r.tagsWalked = Object.keys(byTag).filter(function(t){ return byTag[t].tag > 0; })
                       .map(function(t){ return hx(Number(t)); });
  r.tagsWithBodyRead = Object.keys(byTag).filter(function(t){ return byTag[t].body > 0; })
                             .map(function(t){ return hx(Number(t)); });

  // THE COVERAGE GUARD. A pass that did not reach its last descriptor has examined a set nobody
  // chose, and its silence says nothing about the tags it never got to.
  r.lastDescriptorWalked = byTag[mark.tag] && byTag[mark.tag].tag > 0;
  r.walkedOf = r.tagsWalked.length + '/' + tags.length;
  if (!r.lastDescriptorWalked && signed && ext === 0x0BBB) {
    r.coverage = 'HARNESS FAILURE: the walk never reached the LAST descriptor of this pass ('
               + hx(mark.tag) + '), so a negative here is "I could not check", not "nothing is '
               + 'consumed". Walked ' + r.walkedOf + '. Split the pass or shorten the loop.';
  } else {
    r.coverage = signed && ext === 0x0BBB ? 'ok: the walk reached the last descriptor' : 'n/a (negative control)';
  }
  r.answer = r.nonWalkerReaders.length
    ? 'A CALLBACK FIRED -- ' + r.nonWalkerReaders.map(function(p){ return p.pc; }).join(', ')
      + ' read inside the loop without being the iterator'
    : 'only the iterator touched the loop';
  return r;
}

function range(lo, hi){ var a = []; for (var i = lo; i <= hi; i++) a.push(i); return a; }

out.passes = [];

// A 0xBC descriptor of N nine-byte entries. Every body byte is distinct, so any read address names
// its own (entry, field) rather than needing the offsets predicted in advance.
var ENTRIES = 4;
function bcDescriptor(){
  var body = [];
  for (var k = 0; k < ENTRIES * 9; k++) body.push((0x30 + k) & 0xFF);
  return { bytes: [0xBC, body.length].concat(body), entries: ENTRIES, bodyLen: body.length };
}
// A tail marker that is NOT consumed by anything, so it can locate the copy and prove the walk
// reached the end. 0x3C is in the swept-and-silent range.
var TAIL = [0x3C, BODY_LEN, 0x3C, 0xA5, 0x3C, 0x5A];

async function bcPass(name, descs, layout, opts){
  opts = opts || {};
  var ext = opts.ext === undefined ? 0x0BBB : opts.ext;
  var signed = opts.signed === undefined ? true : opts.signed;
  var last = layout[layout.length - 1];
  var mark = { off: last.off, tag: last.tag, pattern: descs.slice(last.off, last.off + 6) };
  var r = { pass: name, extension: hx(ext), signed: signed, loopBytes: descs.length,
            layout: layout.map(function(d){ return hx(d.tag) + '@' + d.off + '+' + d.len; }) };
  if (!wants(0xA1)) { r.error = 'the 0xA1 subscription lapsed -- a zero here would be about timing'; return r; }

  var b0 = buildA1(descs, ext, signed);
  var p0 = window.__siPush(A1_PID, b0.bytes);
  if (!p0.ok) { r.error = 'push refused: ' + p0.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var ringPhys = (parseInt(p0.at, 16) >>> 0) & 0x1FFFFFF;
  var found = window.__find(mark.pattern, 0x80000000, 0x82000000, 24).filter(function(a){
    var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF;
    return !(q >= ringPhys && q < ringPhys + 4096);
  });
  r.copiesFound = found;
  if (!found.length) { r.answer = 'DELIVERED AND NEVER COPIED OUT OF THE RING'; r.coverage = 'unknown'; return r; }
  var chosen = found.filter(function(a){ return (parseInt(a, 16) >>> 0) >= 0x80200000; })[0] || found[0];
  var mid = parseInt(chosen, 16) >>> 0;
  r.watching = chosen;
  window.__readWatch((mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0);
  var b1 = buildA1(descs, ext, signed);
  var p1 = window.__siPush(A1_PID, b1.bytes);
  if (!p1.ok) { window.__readWatch(); r.error = 'second push refused: ' + p1.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var log = window.__readWatchLog();
  window.__readWatch();
  r.watch = { reads: log.reads, capped: log.capped ? 'CAPPED -- a sample, not a record' : false };

  var again = window.__find(mark.pattern, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (!again.length) { r.error = 'the marker is not in the watched region afterwards'; return r; }
  var loopAt = (parseInt(again[0], 16) >>> 0) - mark.off;
  r.descriptorLoopAt = h(loopAt);

  var pcCover = {}, byDesc = {}, bcOffsets = {};
  layout.forEach(function(d){ byDesc[hx(d.tag)] = { tag: 0, len: 0, body: 0 }; });
  var bc = layout.filter(function(d){ return d.tag === 0xBC; })[0];
  log.all.forEach(function(rr){
    var a = parseInt(rr.at, 16), n = rr.size || 1, k;
    for (k = 0; k < n; k++) {
      var off = (a + k) - loopAt;
      if (off < 0 || off >= descs.length) continue;
      (pcCover[rr.pc] = pcCover[rr.pc] || {})[off] = 1;
      for (var di = 0; di < layout.length; di++) {
        var d = layout[di];
        if (off < d.off || off >= d.off + d.len) continue;
        var e = byDesc[hx(d.tag)];
        if (off === d.off) e.tag++; else if (off === d.off + 1) e.len++; else e.body++;
        if (bc && d.tag === 0xBC && off >= d.off + 2 && !WALKER[rr.pc])
          bcOffsets[off - d.off - 2] = (bcOffsets[off - d.off - 2] || 0) + 1;
        break;
      }
    }
  });
  r.readers = Object.keys(pcCover).map(function(p){
    var n = Object.keys(pcCover[p]).length;
    return { pc: p, bytes: n, coverage: +(100 * n / descs.length).toFixed(1) };
  }).sort(function(a, b){ return b.bytes - a.bytes; }).slice(0, 14);
  r.nonWalkerReaders = r.readers.filter(function(p){ return !WALKER[p.pc]; });
  r.tagsWalked = Object.keys(byDesc).filter(function(t){ return byDesc[t].tag > 0; });
  r.tagsWithBodyRead = Object.keys(byDesc).filter(function(t){ return byDesc[t].body > 0; });
  r.lastDescriptorWalked = byDesc[hx(mark.tag)] && byDesc[hx(mark.tag)].tag > 0;
  r.coverage = (signed && ext === 0x0BBB && !r.lastDescriptorWalked)
    ? ('HARNESS/EXPECTED: the walk did not reach the last descriptor (' + hx(mark.tag) + '). If a '
       + 'consumer fired this is the walk being STOPPED by it; if none did, this pass checked nothing.')
    : 'ok: the walk reached the last descriptor';
  // WHICH BYTES OF A NINE-BYTE ENTRY GET READ -- the thing that makes the layout readable later.
  if (bc) r.bcBodyOffsetsRead = Object.keys(bcOffsets).map(Number).sort(function(a, b){ return a - b; });
  r.answer = r.nonWalkerReaders.length
    ? 'A CONSUMER FIRED -- ' + r.nonWalkerReaders.map(function(p){ return p.pc; }).join(', ')
    : 'only the iterator touched the loop';
  return r;
}

// A: 0xBC FIRST, with a silent marker behind it.
(function(){
  var d = bcDescriptor(), descs = d.bytes.concat(TAIL);
  out.passes.push({ defer: ['A: 0xBC first, ' + d.entries + ' nine-byte entries, silent marker behind it',
                            descs, [{ off: 0, len: d.bytes.length, tag: 0xBC },
                                    { off: d.bytes.length, len: 6, tag: 0x3C }]] });
})();
// B: the marker ALONE, as this run's negative -- same harness, no 0xBC.
out.passes.push({ defer: ['B: the marker alone -- this run\'s negative', TAIL, [{ off: 0, len: 6, tag: 0x3C }]] });
// C: 0xB5 IN FRONT OF 0xBC, to show the truncation is real rather than asserted.
(function(){
  var b5 = [0xB5, 4, 0x11, 0x22, 0x33, 0x44], d = bcDescriptor();
  var descs = b5.concat(d.bytes, TAIL);
  out.passes.push({ defer: ['C: 0xB5 in FRONT of 0xBC -- 0xB5 should stop the walk before 0xBC',
                            descs, [{ off: 0, len: 6, tag: 0xB5 },
                                    { off: 6, len: d.bytes.length, tag: 0xBC },
                                    { off: 6 + d.bytes.length, len: 6, tag: 0x3C }]] });
})();

var resolved = [];
for (var pi = 0; pi < out.passes.length; pi++) {
  var a = out.passes[pi].defer;
  resolved.push(await bcPass(a[0], a[1], a[2]));
}
out.passes = resolved;

// The two controls the 0xA1 path always needs, on the payload that DID walk.
var d2 = bcDescriptor(), dl = d2.bytes.concat(TAIL);
var lay = [{ off: 0, len: d2.bytes.length, tag: 0xBC }, { off: d2.bytes.length, len: 6, tag: 0x3C }];
out.controlUnsigned  = await bcPass('control: NOT signed (must go silent)', dl, lay, { signed: false });
out.controlExtension = await bcPass('control: extension 0xBAD outside the mask (must go silent)', dl, lay, { ext: 0x0BAD });
// A CONTROL HELDS BY BEING SILENT, NOT BY BEING UNFINDABLE. The first version of this line asked
// whether __find located a copy at all -- and it does, because the payloads differ by two bytes and
// the marker still matches a STALE copy left by the previous pass. That reported controlsHeld=false
// over two controls whose own rows said "only the iterator touched the loop" with nothing walked:
// the fourth time on this project that a one-line verdict has been cruder than the table beside it.
// The control's actual requirement is that NOTHING WALKED and NOTHING WAS CONSUMED.
function silent(c){
  return !!c && (!c.tagsWalked || c.tagsWalked.length === 0)
             && (!c.nonWalkerReaders || c.nonWalkerReaders.length === 0);
}
out.controlsHeld = silent(out.controlUnsigned) && silent(out.controlExtension);
out.controlDetail = { unsigned: { walked: out.controlUnsigned.tagsWalked, answer: out.controlUnsigned.answer },
                      extension: { walked: out.controlExtension.tagsWalked, answer: out.controlExtension.answer } };

var A = out.passes[0], B = out.passes[1], C = out.passes[2];
out.bcIsConsumed = !!(A.nonWalkerReaders && A.nonWalkerReaders.length);
out.negativeHeld  = !!(B.nonWalkerReaders && B.nonWalkerReaders.length === 0);
out.b5TruncatesTheWalk = !!(C.tagsWalked && C.tagsWalked.indexOf('0xbc') < 0);
out.tablesAtEnd = tablesWanted();
out.tasksAtEnd = window.__tasks().n;
out.headline = out.bcIsConsumed
  ? ('0xBC IS CONSUMED by ' + A.nonWalkerReaders.map(function(p){ return p.pc; }).join(', ')
     + '; entry-body offsets read: ' + JSON.stringify(A.bcBodyOffsetsRead))
  : ('0xBC was walked and NOT consumed on this path (marker reached: ' + A.lastDescriptorWalked
     + '). The handler at 0x800C6C66 exists but is not the callback armed here -- the open half of '
     + 'sky-02me.20 is which caller arms the dispatcher at 0x800C6930.');
return out;
