// WHICH PRIVATE DESCRIPTOR DOES THIS BOX CONSUME ONCE A NAMESPACE IS DECLARED? -- sky-eluc.12,
// stage four, and the one the channel list depends on.
//
// WHAT THE PREVIOUS THREE STAGES ESTABLISHED, each with its control:
//
//   * The box's hardware match table on a plain boot is 0x40 (NIT, network 0x20) and 0x73 (TOT)
//     only. ONE NIT makes it add 0x4A/ext=0x1000 -- the BAT, at Sky's own bouquet base. So the
//     ladder is NIT first, then bouquet.
//   * A section is memcpy'd out of the ring and parsed on the copy, so a ring watch sees only the
//     copier (0x800FA958). Watching the ring reported all 128 private tags as consumed; they were
//     the memcpy.
//   * The transport descriptor loop is walked by a generic iterator, FUN_800ad590(loop, off,
//     wantedTag, cb), which reads ONLY tag and length and calls back on a match. Swept with tags
//     0x80..0xFF the callback NEVER fired -- every descriptor walked, none consumed. Swept with
//     0x01..0x7F, in the same run, exactly one did: tag 0x5F.
//   * Decompiling that callback: it tests `tag - 0x5F`, assembles bytes 2..5 into a 32-bit value,
//     and compares it against the constants 2 and 5 -- so it is the DVB private_data_specifier
//     descriptor, and this firmware distinguishes specifier 0..1, 2..4 and >=5.
//
// WHICH EXPLAINS THE NEGATIVE RATHER THAN SITTING BESIDE IT. In DVB a private descriptor (0x80..
// 0xFF) has no meaning until a private_data_specifier_descriptor has said WHOSE private namespace
// applies. The box skipped all 128 because none of them was in any declared namespace -- the parser
// was conformant and the feed was missing the one descriptor that gives the others meaning.
//
// SO DECLARE ONE AND SWEEP AGAIN. Loop = [0x5F, 4, specifier] then 0x80..0xFF then a service_list.
// Three passes, and the two controls are the point rather than decoration:
//
//   specifier 2   the middle class the firmware's own constants carve out
//   specifier 0   inside the 0..1 class -- if this consumes the same tags, the specifier VALUE is
//                 not what unlocks them and the finding would be about the descriptor's presence
//   specifier 9   above the >=5 boundary -- the other side of the same question
//
// A result where all three behave identically is still a result: it would say the namespace value
// is not read for this purpose, and it would say so with evidence rather than by omission.
//
// THE SPECIFIER GOES FIRST IN THE LOOP because DVB scopes it to the descriptors that FOLLOW it.
// Putting it last would be a well-formed section that declares nothing about anything, and the
// negative it produced would be about placement rather than about the box.

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
function u32(v){ return [(v >>> 24) & 0xFF, (v >>> 16) & 0xFF, (v >>> 8) & 0xFF, v & 0xFF]; }
function serviceList(sid){ return [0x41, 3].concat(u16(sid), [0x01]); }

var BODY_LEN = 4;
function buildLoop(spec){
  var descs = [], layout = [];
  function add(b, what){ layout.push({ off: descs.length, len: b.length, tag: b[0], what: what }); descs = descs.concat(b); }
  add([0x5F, 4].concat(u32(spec)), 'specifier');
  for (var i = 0x80; i <= 0xFF; i++) add([i, BODY_LEN, i, 0xA5, i, 0x5A], 'probe');
  add(serviceList(0x0064), 'control-last');
  return { descs: descs, layout: layout };
}
var MARK = 0xFF, MARK_PATTERN = [MARK, BODY_LEN, MARK, 0xA5, MARK, 0x5A];

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

// The iterator's own reads. Anything else inside the loop is a callback, and a callback is the box
// consuming a descriptor.
var WALKER = { '0x800AD5B4': 1, '0x800AD5B6': 1, '0x800AD5F2': 1, '0x800AD5F4': 1 };
// The private_data_specifier handler, found in stage three. Expected on every pass -- it is what
// reads the 0x5F body -- so it is named rather than counted as a discovery.
var SPEC_CB = /^0x800CAE[89ABC]/;

var out = { question: 'which private descriptor tags does this box consume once a namespace is declared',
            settledAfterSeconds: waited, tablesBefore: tablesWanted() };

if (!wants(0x4A)) {
  var n = window.__siNIT();
  if (!n.ok) throw new Error('the ladder-opening NIT was refused: ' + n.why);
  for (var w = 0; w < 10 && !wants(0x4A); w++) await new Promise(function(r){ setTimeout(r, 3000); });
}
out.tablesAfterNit = tablesWanted();
if (!wants(0x4A)) throw new Error('the box does not subscribe to table 0x4A');
if (!armedFilterForPid(0x0011)) throw new Error('no armed filter on PID 0x0011');

async function pass(spec){
  var L = buildLoop(spec), mark = L.layout.filter(function(d){ return d.tag === MARK; })[0];
  var r = { privateDataSpecifier: spec, descriptors: L.layout.length };

  var b0 = buildBat(L.descs);
  var p0 = window.__siPush(0x0011, b0.bytes);
  if (!p0.ok) { r.error = 'push refused: ' + p0.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var ringPhys = (parseInt(p0.at, 16) >>> 0) & 0x1FFFFFF;
  var found = window.__find(MARK_PATTERN, 0x80000000, 0x82000000, 24).filter(function(a){
    var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF;
    return !(q >= ringPhys && q < ringPhys + 1024);
  });
  if (!found.length) { r.error = 'the marker was not found outside the ring -- no copy to watch'; return r; }
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

  var again = window.__find(MARK_PATTERN, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (!again.length) { r.error = 'the marker is not in the watched region afterwards -- unmappable, not zero'; return r; }
  var loopAt = (parseInt(again[0], 16) >>> 0) - mark.off;
  r.descriptorLoopAt = h(loopAt);

  var byTag = {}, pcCover = {}, pcTags = {};
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
        if (off === d.off) e.tag++; else if (off === d.off + 1) e.len++;
        else { e.body++; if (!WALKER[rr.pc]) (pcTags[rr.pc] = pcTags[rr.pc] || {})['0x' + d.tag.toString(16)] = 1; }
        break;
      }
    }
  });

  var readers = Object.keys(pcCover).map(function(p){
    return { pc: p, bytes: Object.keys(pcCover[p]).length }; })
    .sort(function(a, b){ return b.bytes - a.bytes; });
  r.descriptorsWalked = Object.keys(byTag).reduce(function(n, t){ return n + byTag[t].tag; }, 0);
  r.specifierWasRead = byTag[0x5F] ? byTag[0x5F].body > 0 : false;
  var news = readers.filter(function(p){ return !WALKER[p.pc] && !SPEC_CB.test(p.pc); });
  r.consumerPcs = news.map(function(p){ return { pc: p.pc, bytes: p.bytes, tags: Object.keys(pcTags[p.pc] || {}) }; });
  r.privateTagsWithBodyRead = L.layout.filter(function(d){
    return d.what === 'probe' && byTag[d.tag].body > 0; }).map(function(d){ return '0x' + d.tag.toString(16); });
  r.answer = !r.specifierWasRead
    ? 'THE 0x5F BODY WAS NOT READ -- the specifier handler did not run, so this pass says nothing '
    + 'about what a declared namespace unlocks'
    : (r.privateTagsWithBodyRead.length
        ? 'private tags consumed: ' + r.privateTagsWithBodyRead.join(' ')
        : 'the namespace was read and NO private tag was consumed');
  return r;
}

out.spec2 = await pass(2);
out.spec0 = await pass(0);
out.spec9 = await pass(9);
out.summary = [out.spec2, out.spec0, out.spec9].map(function(p){
  return 'specifier ' + p.privateDataSpecifier + ': '
       + (p.error ? ('ERROR ' + p.error)
                  : ((p.specifierWasRead ? '0x5F read, ' : '0x5F NOT read, ')
                     + (p.privateTagsWithBodyRead.length ? p.privateTagsWithBodyRead.join(' ') : 'no private tag consumed')));
});
return out;
