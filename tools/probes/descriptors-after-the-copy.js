// WHICH DESCRIPTOR TAGS DOES THE BAT PARSER READ -- WATCHED WHERE THE PARSE ACTUALLY HAPPENS.
// sky-eluc.12, stage two.
//
// which-descriptors.js asked the question at the section ring and got a perfect, useless answer:
// every tag 0x80..0xFF had its body read, all 786 bytes, by a SINGLE PC at 100% coverage. That PC
// is 0x800FA958, and decompiling it settles what it is --
//
//     undefined1 * FUN_800fa93c(undefined1 *dst, undefined1 *src, int n)
//     { while (n = n + -1, -1 < n) { *dst = *src; dst++; src++; } return dst0; }
//
// -- the byte loop inside memcpy. So the section is copied out of the ring wholesale and parsed
// somewhere else, and a watch on the ring can only ever see the copier. "All 128 tags read" was
// the memcpy, and without the per-PC coverage grouping it would have been a spectacular false
// finding: 128 private descriptors, every one apparently consumed.
//
// SO FOLLOW THE COPY. The probe bodies are distinctive by construction ([tag, 0xA5, tag, 0x5A]
// behind a [tag, 4] header), so after a push the six bytes of one chosen tag exist in the ring AND
// wherever it was copied to. __find locates both across the whole 32 MB -- not the first eighth,
// which is its default trap.
//
// AND THE FIRST VERSION OF THIS PROBE PICKED THE WRONG ONE, in the shape this project keeps
// meeting. It pushed twice and watched the address the pattern was found at BOTH times, reasoning
// that a reused buffer would be the stable one. Measured, the copies are a RING:
//
//     push A (v9)   0x8019A49A        push B (v10)  0x8019A7C7     delta 0x32D = 813 = the section
//
// so the address present in both runs is the one push A wrote and nothing has yet overwritten --
// stable because it is DEAD. The watch was armed on a dead allocation, push C landed in the next
// slot, and it read back a clean, meaningless zero. "Stable across both pushes" was satisfied by
// exactly the state it was written to exclude.
//
// THE FIX IS TO STOP PREDICTING AN ADDRESS AND WATCH THE WHOLE REGION. Each copy family lives in a
// few KB; watching the region means wherever the next section lands it is caught, and the section
// is then located by finding the pattern AFTERWARDS and mapping reads relative to where it actually
// went. A prediction that is wrong returns zero; a region that is right cannot.
//
// TWO FAMILIES EXIST AND BOTH ARE WATCHED, one pass each. 0x8019Axxx advances by exactly the
// section length -- whole sections, a ring. 0x802A7xxx advances irregularly (704, 800) -- sized
// allocations rather than fixed slots, which is what a per-record store looks like.
//
// VERSIONS MUST DIFFER OR THE SECTION IS DISCARDED UNPARSED, and the section task's PCs are
// counted around every push so a zero can be attributed to delivery rather than to parsing.
//
// THE LADDER IS OPENED RATHER THAN ASSUMED. Measured in stage one: on a plain boot this box
// subscribes only to 0x40 (NIT, network 0x20) and 0x73 (TOT), and ONE NIT is what makes it add
// 0x4A/ext=0x1000 -- the BAT, at Sky's own bouquet base. So a plain NIT goes first and the BAT
// probes follow, which puts the question where SKY_BAT_SVL can answer it.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 180) {
  await new Promise(function(r){ setTimeout(r, 1000); });
  waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's -- every reading below would be of a saturated machine');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

var CRC_TAB = (function(){
  var t = new Int32Array(256), i, j, c;
  for (i = 0; i < 256; i++) { c = i << 24; for (j = 0; j < 8; j++) c = (c & 0x80000000) ? ((c << 1) ^ 0x04C11DB7) : (c << 1); t[i] = c; }
  return t;
})();
function crc32(b){ var c = -1, i; for (i = 0; i < b.length; i++) c = (c << 8) ^ CRC_TAB[((c >>> 24) ^ b[i]) & 0xFF]; return c >>> 0; }
function u16(v){ return [(v >>> 8) & 0xFF, v & 0xFF]; }
function bcd8(v, digits){
  var s = String(v); while (s.length < digits) s = '0' + s;
  var out = []; for (var i = 0; i < digits; i += 2) out.push(parseInt(s.substr(i, 2), 16) & 0xFF);
  return out;
}
function satellite(){ return [0x43, 11].concat(bcd8(1177800, 8), bcd8(282, 4), [0x81], bcd8(275000, 8).slice(0, 4)); }
function serviceList(sid){ return [0x41, 3].concat(u16(sid), [0x01]); }

var BODY_LEN = 4, MARK = 0xC3;                    // the tag whose six bytes are the search pattern
var descs = [], layout = [];
(function(){
  function add(bytes, what){
    layout.push({ off: descs.length, len: bytes.length, tag: bytes[0], what: what });
    descs = descs.concat(bytes);
  }
  add(satellite(), 'control-first');
  for (var i = 0x80; i <= 0xFF; i++) add([i, BODY_LEN, i, 0xA5, i, 0x5A], 'probe');
  add(serviceList(0x0064), 'control-last');
})();
var markOff = layout.filter(function(d){ return d.tag === MARK; })[0].off;
var PATTERN = [MARK, BODY_LEN, MARK, 0xA5, MARK, 0x5A];

function armedFilterForPid(pid){
  return window.__siFilters().filter(function(f){ return parseInt(f.pid, 16) === pid && f.armed; })[0] || null;
}
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return (x.tableId === null ? '0x??' : '0x' + x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : '');
  }).join(' ');
}

var LISR = 0x800041B4, SECTASK = 0x80004520;
function hits(){
  var r = window.__pcHits(LISR, SECTASK);
  // __pcHits keys with the page's hex32, which UPPERCASES; a lower-case lookup returns undefined
  // rather than a count, and reads as a perfectly plausible zero.
  if (r[h(LISR)] === undefined || r[h(SECTASK)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- a census that cannot find '
                  + 'its own subject is a harness failure, not a count of zero');
  return { lisr: r[h(LISR)], task: r[h(SECTASK)] };
}

var VERSION = 8;
function buildBat(){
  VERSION = (VERSION + 1) & 0x1F;
  var ids = window.__siIds();
  var bq = (ids.bouquetIdMask !== null && ids.bouquetId !== null &&
            ((0x1001 & ids.bouquetIdMask) === (ids.bouquetId & ids.bouquetIdMask)))
         ? 0x1001 : ids.bouquetId;                // authentic where the filter still accepts it
  var ts = u16(ids.tsid).concat(u16(ids.networkId),
               [0xF0 | ((descs.length >>> 8) & 0x0F), descs.length & 0xFF], descs);
  var name = [0x47, 3, 0x53, 0x6B, 0x79];
  var len = 5 + 2 + name.length + 2 + ts.length + 4;
  var s = [0x4A, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(bq), [0xC1 | ((VERSION & 0x1F) << 1), 0x00, 0x00],
            [0xF0 | ((name.length >>> 8) & 0x0F), name.length & 0xFF], name,
            [0xF0 | ((ts.length >>> 8) & 0x0F), ts.length & 0xFF], ts);
  var c = crc32(s);
  return { bytes: s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]),
           bouquetId: bq, version: VERSION };
}

var out = { question: 'where is a BAT section parsed, and which private descriptor bodies are read there',
            settledAfterSeconds: waited, tablesBefore: tablesWanted() };

// ---- open the ladder: one plain NIT ------------------------------------------------------
if (!wants(0x4A)) {
  var n = window.__siNIT();
  if (!n.ok) throw new Error('the ladder-opening NIT was refused: ' + n.why);
  for (var w = 0; w < 10 && !wants(0x4A); w++) await new Promise(function(r){ setTimeout(r, 3000); });
}
out.tablesAfterNit = tablesWanted();
if (!wants(0x4A))
  throw new Error('the box still does not subscribe to table 0x4A after a NIT -- stage one measured '
                + 'that it does, so this run is of a different machine state');
if (!armedFilterForPid(0x0011)) throw new Error('no armed filter on PID 0x0011');

function phys(a){ return (typeof a === 'string' ? parseInt(a, 16) : a) >>> 0 & 0x1FFFFFF; }

async function push(){
  var b = buildBat();
  var before = hits();
  var p = window.__siPush(0x0011, b.bytes);
  if (!p.ok) return { error: 'push refused: ' + p.why };
  await new Promise(function(r){ setTimeout(r, 8000); });
  var after = hits();
  return { version: b.version, bouquetId: '0x' + b.bouquetId.toString(16), sectionBytes: b.bytes.length,
           landedAt: p.at, delivery: { lisr: after.lisr - before.lisr, sectionTask: after.task - before.task },
           found: window.__find(PATTERN, 0x80000000, 0x82000000, 24) };
}

// ---- map the copy families, by watching where the pattern MOVES -------------------------
out.pushA = await push();
out.pushB = await push();
if (out.pushA.error || out.pushB.error) { out.stop = 'a push was refused'; return out; }
if (!out.pushA.delivery.sectionTask || !out.pushB.delivery.sectionTask) {
  out.stop = 'the section task did not advance -- every zero below would be about delivery';
  return out;
}
var ringPhys = [phys(out.pushA.landedAt), phys(out.pushB.landedAt)];
function isRing(a){ var p = phys(a); return ringPhys.some(function(r){ return p >= r && p < r + 1024; }); }
var copies = out.pushB.found.filter(function(a){ return !isRing(a); });
// The LIVE slot is the one push B added, not the one both runs share -- see the note at the top.
var fresh = out.pushB.found.filter(function(a){
  return !isRing(a) && !out.pushA.found.some(function(b){ return phys(a) === phys(b); });
});
out.copies = { afterA: out.pushA.found.filter(function(a){ return !isRing(a); }),
               afterB: copies, freshInB: fresh,
               note: 'freshInB is where push B was copied to; an address in both is push A\'s slot '
                   + 'still resident, which is stable because it is dead' };
if (!copies.length) {
  out.stop = 'the pattern was not found anywhere outside the ring -- either the parse reads from '
           + 'the ring through a path this watch did not see, or the section was discarded.';
  return out;
}

// Group the copies into regions a few KB wide. Watching a REGION rather than an address is the
// whole correction: wherever the next section lands inside it, the reads are caught, and the
// section is located afterwards rather than predicted.
var regions = [];
copies.concat(out.copies.afterA).forEach(function(a){
  var p = phys(a);
  for (var i = 0; i < regions.length; i++)
    if (Math.abs(p - regions[i].mid) < 0x4000) { regions[i].seen.push(a); return; }
  regions.push({ mid: p, seen: [a] });
});
out.regions = regions.map(function(r){ return { around: h(0x80000000 + r.mid), seen: r.seen }; });

// ---- watch each region in turn -------------------------------------------------------------
out.passes = [];
for (var ri = 0; ri < regions.length && ri < 3; ri++) {
  var lo = (0x80000000 + regions[ri].mid - 0x2000) >>> 0;
  var hi = (0x80000000 + regions[ri].mid + 0x2000) >>> 0;
  var before = hits();
  window.__readWatch(lo, hi);
  var p = await push();
  var log = window.__readWatchLog();
  window.__readWatch();
  if (p.error) { out.passes.push({ region: h(lo) + '-' + h(hi), error: p.error }); continue; }

  // WHERE DID THIS ONE LAND? The pattern found inside the watched region and NOT present before
  // is this push's copy. Without that the reads cannot be mapped to descriptors at all, and a
  // guess at the address is how the previous version of this probe read back a clean zero.
  var landed = p.found.filter(function(a){
    var q = phys(a);
    return q >= phys(lo) && q < phys(hi) && !isRing(a);
  });
  var pass = { region: h(lo) + '-' + h(hi), version: p.version,
               delivery: p.delivery,
               patternInRegionAfterwards: landed,
               watch: { reads: log.reads, capped: log.capped ? 'CAPPED -- a sample, not a record' : false } };

  if (!log.reads) {
    pass.answer = 'NOTHING read this region during the push. Not "no tag is consumed" -- this '
                + 'region was not touched at all, so it is a staging or a stale buffer.';
    out.passes.push(pass); continue;
  }
  if (!landed.length) {
    pass.answer = 'the region WAS read, but this push\'s section is not in it afterwards, so the '
                + 'reads cannot be attributed to these descriptors. Unmappable, not zero.';
    pass.readers = (function(){
      var by = {}; log.all.forEach(function(r){ by[r.pc] = (by[r.pc] || 0) + 1; });
      return Object.keys(by).map(function(k){ return { pc: k, reads: by[k] }; })
        .sort(function(a, b){ return b.reads - a.reads; }).slice(0, 10);
    })();
    out.passes.push(pass); continue;
  }

  var loopAt = (parseInt(landed[0], 16) >>> 0) - markOff;
  var byTag = {}, pcCover = {}, touched = {};
  layout.forEach(function(d){ byTag[d.tag] = byTag[d.tag] || { tag: 0, len: 0, body: 0 }; });
  log.all.forEach(function(r){
    var a = parseInt(r.at, 16), n = r.size || 1, k;
    for (k = 0; k < n; k++) {
      var off = (a + k) - loopAt;
      if (off < 0 || off >= descs.length) continue;
      (pcCover[r.pc] = pcCover[r.pc] || {})[off] = 1;
      touched[off] = 1;
      for (var di = 0; di < layout.length; di++) {
        var d = layout[di];
        if (off < d.off || off >= d.off + d.len) continue;
        var e = byTag[d.tag];
        if (off === d.off) e.tag++; else if (off === d.off + 1) e.len++; else e.body++;
        break;
      }
    }
  });
  var pcs = Object.keys(pcCover).map(function(pp){
    var n = Object.keys(pcCover[pp]).length;
    return { pc: pp, bytes: n, coverage: +(100 * n / descs.length).toFixed(1) };
  }).sort(function(a, b){ return b.bytes - a.bytes; });
  var parserPcs = pcs.filter(function(x){ return x.coverage < 80; });
  var parserOffsets = {};
  parserPcs.forEach(function(x){ Object.keys(pcCover[x.pc]).forEach(function(o){ parserOffsets[o] = 1; }); });
  var probeTags = layout.filter(function(d){ return d.what === 'probe'; });
  var ctl = layout.filter(function(d){ return d.what.indexOf('control') === 0; }).map(function(d){
    var all = 0, byParser = 0, k;
    for (k = d.off + 2; k < d.off + d.len; k++) { if (touched[k]) all++; if (parserOffsets[k]) byParser++; }
    return { what: d.what, tag: '0x' + d.tag.toString(16), bodyBytesReadByAnyone: all, bodyBytesReadByParser: byParser };
  });

  pass.descriptorLoopAt = h(loopAt);
  pass.readers = pcs.slice(0, 12);
  pass.bulkCopiers = pcs.filter(function(x){ return x.coverage >= 80; });
  pass.parserCandidates = parserPcs.slice(0, 12);
  pass.controls = ctl;
  pass.tagsWhoseBodyAParserRead = probeTags.filter(function(d){
    for (var k = d.off + 2; k < d.off + d.len; k++) if (parserOffsets[k]) return true;
    return false;
  }).map(function(d){ return '0x' + d.tag.toString(16); });
  pass.answer = pcs.length === 0
    ? 'the region was read but not inside this section'
    : (parserPcs.length === 0
        ? 'EVERY PC that read this section covered ~all of it -- all copiers, so this is a staging '
        + 'buffer and the parse is further on'
        : (ctl.every(function(c){ return c.bodyBytesReadByParser > 0; })
            ? 'both controls were read by a non-copier, so the loop was genuinely walked and the '
            + 'tag list is an answer'
            : 'a control was NOT read by a non-copier -- treat the tag list as a prefix'));
  out.passes.push(pass);
}
return out;
