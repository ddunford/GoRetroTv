// WHICH DESCRIPTOR TAG DOES THIS BOX ASK FOR IN A BAT TRANSPORT LOOP? -- sky-eluc.12, stage three.
//
// Stage two found the walk and decompiling it gave the specification outright:
//
//     FUN_800ad590(loop, off, wantedTag, userData)          // callback in $t0
//       tag = *desc; len = desc[1];
//       while (tag != 0) {
//         if (tag == wantedTag || wantedTag == 0) callback(desc, i, userData);
//         desc += len + 2; tag = *desc; len = desc[1];
//       }
//
// A generic descriptor-loop iterator that reads ONLY the tag and the length byte -- the callback is
// what reads a body. Measured on a loop of 130 descriptors (satellite_delivery, 128 private tags
// 0x80..0xFF, service_list): 0x800AD5B4/B6 read the first descriptor's tag and length, 0x800AD5F2/F4
// read 129 more, and NO other PC touched the loop at all. 1 + 129 = 130, so the walk covered every
// descriptor and the callback never fired once.
//
// THAT IS A CLEAN NEGATIVE AND IT NARROWS THE QUESTION. The wanted tag is not 0x43, not 0x41, and
// not anything in 0x80..0xFF -- if it were any of those the callback would have fired and its own PC
// would have appeared as a reader. What is left is 0x01..0x7F minus those two. So sweep it.
//
// THE RUN CARRIES ITS OWN CONTROL. Pass A sweeps 0x01..0x7F, pass B re-runs 0x80..0xFF. Pass B must
// reproduce the known negative: if it suddenly shows a callback, the instrument changed rather than
// the box, and pass A's positive would be worth nothing. A sweep whose control is in a different
// run is a sweep whose control is an assumption.
//
// TAG 0x00 CANNOT BE PROBED and that is not an omission -- it is the iterator's terminator. A
// descriptor with tag 0 ends the walk, so including one would truncate the loop and every tag after
// it would be reported as unread. Said here because "we swept every tag" would otherwise be false
// by one, and this project has already paid for a scanner that silently examined less than it said.
//
// THE COPY, NOT THE RING. A section is memcpy'd out of the ring (0x800FA958, the byte loop inside
// memcpy) and parsed on the copy, so a ring watch sees only the copier -- that is how the first
// version of this experiment reported all 128 private tags as consumed. The region is located by
// finding the probe pattern after a push and watching a few KB around it, never by predicting an
// address: a wrong prediction returns a clean zero that looks exactly like a parser reading nothing.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }

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
function bcd8(v, d){ var s = String(v); while (s.length < d) s = '0' + s;
  var o = []; for (var i = 0; i < d; i += 2) o.push(parseInt(s.substr(i, 2), 16) & 0xFF); return o; }
function satellite(){ return [0x43, 11].concat(bcd8(1177800, 8), bcd8(282, 4), [0x81], bcd8(275000, 8).slice(0, 4)); }
function serviceList(sid){ return [0x41, 3].concat(u16(sid), [0x01]); }

var BODY_LEN = 4;
// The search pattern must be a tag present in BOTH passes, or the copy cannot be located in one of
// them. 0x43 is in the loop of both as control-first, and its body is distinctive enough to find.
function buildLoop(lo, hi){
  var descs = [], layout = [];
  function add(b, what){ layout.push({ off: descs.length, len: b.length, tag: b[0], what: what }); descs = descs.concat(b); }
  add(satellite(), 'control-first');
  for (var i = lo; i <= hi; i++) {
    if (i === 0x43 || i === 0x41) continue;       // already present as the two controls
    add([i, BODY_LEN, i, 0xA5, i, 0x5A], 'probe');
  }
  add(serviceList(0x0064), 'control-last');
  return { descs: descs, layout: layout };
}
// A marker whose six bytes are unique and appear in both passes: tag 0x7F is in pass A, 0xFF in
// pass B, so each pass carries its own and says which it used rather than sharing one that is only
// in half the runs.
function markerFor(layout){
  var probes = layout.filter(function(d){ return d.what === 'probe'; });
  var d = probes[probes.length - 1];
  return { off: d.off, tag: d.tag, pattern: [d.tag, BODY_LEN, d.tag, 0xA5, d.tag, 0x5A] };
}

function armedFilterForPid(pid){
  return window.__siFilters().filter(function(f){ return parseInt(f.pid, 16) === pid && f.armed; })[0] || null;
}
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).join(' ');
}
var LISR = 0x800041B4, SECTASK = 0x80004520;
function hits(){
  var r = window.__pcHits(LISR, SECTASK);
  if (r[h(LISR)] === undefined || r[h(SECTASK)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- a harness failure, not a zero');
  return { lisr: r[h(LISR)], task: r[h(SECTASK)] };
}

var VERSION = 0;
function buildBat(descs){
  VERSION = (VERSION + 1) & 0x1F;
  var ids = window.__siIds();
  var bq = (ids.bouquetIdMask !== null && ids.bouquetId !== null &&
            ((0x1001 & ids.bouquetIdMask) === (ids.bouquetId & ids.bouquetIdMask))) ? 0x1001 : ids.bouquetId;
  var ts = u16(ids.tsid).concat(u16(ids.networkId),
               [0xF0 | ((descs.length >>> 8) & 0x0F), descs.length & 0xFF], descs);
  var name = [0x47, 3, 0x53, 0x6B, 0x79];
  var len = 5 + 2 + name.length + 2 + ts.length + 4;
  var s = [0x4A, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(bq), [0xC1 | ((VERSION & 0x1F) << 1), 0x00, 0x00],
            [0xF0 | ((name.length >>> 8) & 0x0F), name.length & 0xFF], name,
            [0xF0 | ((ts.length >>> 8) & 0x0F), ts.length & 0xFF], ts);
  var c = crc32(s);
  return { bytes: s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]), version: VERSION };
}

var out = { question: 'which descriptor tag does this box ask for in a BAT transport loop',
            settledAfterSeconds: waited, tablesBefore: tablesWanted() };

if (!wants(0x4A)) {
  var n = window.__siNIT();
  if (!n.ok) throw new Error('the ladder-opening NIT was refused: ' + n.why);
  for (var w = 0; w < 10 && !wants(0x4A); w++) await new Promise(function(r){ setTimeout(r, 3000); });
}
out.tablesAfterNit = tablesWanted();
if (!wants(0x4A)) throw new Error('the box does not subscribe to table 0x4A -- stage one measured that it does');
if (!armedFilterForPid(0x0011)) throw new Error('no armed filter on PID 0x0011');

async function pass(name, lo, hi){
  var L = buildLoop(lo, hi), mark = markerFor(L.layout);
  var r = { pass: name, tags: h(lo).slice(-2) + '..' + h(hi).slice(-2),
            descriptors: L.layout.length, marker: '0x' + mark.tag.toString(16) };

  // Locate the copy: push once un-watched, find the marker, watch a few KB around it, push again.
  var b0 = buildBat(L.descs);
  var p0 = window.__siPush(0x0011, b0.bytes);
  if (!p0.ok) { r.error = 'push refused: ' + p0.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var ringPhys = (parseInt(p0.at, 16) >>> 0) & 0x1FFFFFF;
  var found = window.__find(mark.pattern, 0x80000000, 0x82000000, 24).filter(function(a){
    var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF;
    return !(q >= ringPhys && q < ringPhys + 1024);
  });
  r.copiesFound = found;
  if (!found.length) { r.error = 'the marker was not found outside the ring -- no copy to watch'; return r; }

  // 0x802A7xxx is where stage two found the parse; prefer a copy there, but say which was chosen
  // rather than silently taking the first.
  var chosen = found.filter(function(a){ return (parseInt(a, 16) >>> 0) >= 0x80200000; })[0] || found[0];
  r.watching = chosen;
  var mid = parseInt(chosen, 16) >>> 0;
  var before = hits();
  window.__readWatch((mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0);
  var b1 = buildBat(L.descs);
  var p1 = window.__siPush(0x0011, b1.bytes);
  if (!p1.ok) { window.__readWatch(); r.error = 'second push refused: ' + p1.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var log = window.__readWatchLog();
  window.__readWatch();
  r.version = b1.version;
  r.delivery = (function(){ var a = hits(); return { lisr: a.lisr - before.lisr, sectionTask: a.task - before.task }; })();
  r.watch = { reads: log.reads, capped: log.capped ? 'CAPPED -- a sample, not a record' : false };

  // The loop base: the marker, found again inside the watched region, minus its offset.
  var again = window.__find(mark.pattern, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (!again.length) { r.error = 'the marker is not in the watched region afterwards -- unmappable, not zero'; return r; }
  var loopAt = (parseInt(again[0], 16) >>> 0) - mark.off;
  r.descriptorLoopAt = h(loopAt);

  var byTag = {}, pcCover = {};
  L.layout.forEach(function(d){ byTag[d.tag] = byTag[d.tag] || { tag: 0, len: 0, body: 0 }; });
  log.all.forEach(function(rr){
    var a = parseInt(rr.at, 16), n = rr.size || 1, k;
    for (k = 0; k < n; k++) {
      var off = (a + k) - loopAt;
      if (off < 0 || off >= L.descs.length) continue;
      (pcCover[rr.pc] = pcCover[rr.pc] || {})[off] = 1;
      for (var di = 0; di < L.layout.length; di++) {
        var d = L.layout[di];
        if (off < d.off || off >= d.off + d.len) continue;
        var e = byTag[d.tag];
        if (off === d.off) e.tag++; else if (off === d.off + 1) e.len++; else e.body++;
        break;
      }
    }
  });
  r.readers = Object.keys(pcCover).map(function(p){
    var n = Object.keys(pcCover[p]).length;
    return { pc: p, bytes: n, coverage: +(100 * n / L.descs.length).toFixed(1) };
  }).sort(function(a, b){ return b.bytes - a.bytes; }).slice(0, 14);

  // THE WALKER IS KNOWN AND IS NOT THE ANSWER. 0x800AD5B4/B6 and 0x800AD5F2/F4 are the iterator's
  // own tag-and-length reads; anything ELSE that touches the loop is a callback, and a callback is
  // the box consuming a descriptor.
  var WALKER = { '0x800AD5B4': 1, '0x800AD5B6': 1, '0x800AD5F2': 1, '0x800AD5F4': 1 };
  var others = r.readers.filter(function(p){ return !WALKER[p.pc]; });
  r.nonWalkerReaders = others;
  r.tagsWithBodyRead = Object.keys(byTag).filter(function(t){ return byTag[t].body > 0; })
                             .map(function(t){ return '0x' + Number(t).toString(16); });
  r.descriptorsWalked = (function(){
    var n = 0; Object.keys(byTag).forEach(function(t){ if (byTag[t].tag) n += byTag[t].tag; }); return n;
  })();
  r.answer = others.length
    ? 'A CALLBACK FIRED -- ' + others.map(function(p){ return p.pc; }).join(', ')
      + ' read inside the loop without being the iterator, so a descriptor here IS consumed'
    : 'only the iterator touched the loop: every tag in this range was walked and skipped';
  return r;
}

out.passA = await pass('A: the low half', 0x01, 0x7F);
out.passB = await pass('B: the known negative, re-run as this run\'s own control', 0x80, 0xFF);
out.controlHeld = !!(out.passB && out.passB.answer && out.passB.answer.indexOf('only the iterator') === 0);
out.note = out.controlHeld
  ? 'pass B reproduced the negative in this same run, so pass A\'s result is about the box'
  : 'PASS B DID NOT REPRODUCE THE KNOWN NEGATIVE -- the instrument changed, and pass A is not trustworthy';
return out;
