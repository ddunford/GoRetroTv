// sky-02me.17 -- CLOSING THE 0xA1 SWEEP. The first run found 0xB5 and left eleven tags unchecked.
//
// WHAT THE FIRST SWEEP FOUND. Of 0x00..0xFF, exactly one tag made a non-iterator PC read a
// descriptor BODY: 0xB5, consumed at 0x800C6B42..0x800C6B56. Passes over 0x01..0x40, 0x41..0x80,
// 0xC1..0xFF and tag 0x00 were clean negatives that each reached their last descriptor, and both
// negative controls (unsigned payload, out-of-mask extension) went silent.
//
// WHY IT IS NOT FINISHED. The pass carrying 0xB5 walked 53 of its 64 descriptors and stopped --
// 0x81 + 52 is 0xB5 exactly, so the walk ended AT the consumed tag. That makes the silence of
// 0xB6..0xC0 'I could not check' rather than 'nothing is consumed', and the probe said so rather
// than reporting a clean sweep. Eleven tags is a small gap and an unchecked one is still unchecked,
// so this closes it: 0xB5 on its own, then the eleven, then the half that was genuinely covered
// re-run as this run's control.
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
// 0xB5 ALONE, to confirm the consumer fires on it and on nothing around it.
out.passes.push(await pass('A: 0xB5 alone -- the candidate', [0xB5]));
// THE GAP THE FIRST SWEEP LEFT. Pass C walked 53 of 64 and stopped exactly at 0xB5 (0x81 + 52),
// so 0xB6..0xC0 were never examined and their silence was "I could not check". This is the range
// that closes the sweep, and it is run WITHOUT 0xB5 in front of it so nothing can truncate it.
out.passes.push(await pass('B: 0xB6..0xC0 -- the eleven tags the first sweep never reached',
                           (function(){ var a=[]; for(var i=0xB6;i<=0xC0;i++) a.push(i); return a; })()));
// AND THE OTHER HALF OF PASS C, which the first run DID cover, re-run as this run's own control.
out.passes.push(await pass('C: 0x81..0xB4 -- the known negative, as this run\'s control',
                           (function(){ var a=[]; for(var i=0x81;i<=0xB4;i++) a.push(i); return a; })()));
var consumers = out.passes.filter(function(p){ return p.nonWalkerReaders && p.nonWalkerReaders.length; });
out.consumedIn = consumers.map(function(p){ return p.pass; });
out.coverageFailures = out.passes.filter(function(p){ return /HARNESS FAILURE/.test(p.coverage || ''); })
                                 .map(function(p){ return p.pass; });

// ---- the two negative controls, on a range that DID walk ---------------------------------------
var walked = out.passes.filter(function(p){ return p.lastDescriptorWalked; })[0];
if (walked) {
  var tagsForControl = range(0x41, 0x80);
  out.controlUnsigned  = await pass('control: the same loop, payload NOT signed (must go silent)',
                                    tagsForControl, { signed: false });
  out.controlExtension = await pass('control: signed, extension 0xBAD outside the unit mask (must go silent)',
                                    tagsForControl, { ext: 0x0BAD });
  out.controlsHeld = !(out.controlUnsigned.descriptorLoopAt && out.controlUnsigned.tagsWalked.length)
                  && !(out.controlExtension.descriptorLoopAt && out.controlExtension.tagsWalked.length);
} else {
  out.controlUnsigned = out.controlExtension = { skipped: 'no pass walked its loop, so there is nothing to vary' };
  out.controlsHeld = false;
}

out.tagZeroIsATerminator = (function(){
  var e = out.passes[4];
  if (!e || !e.tagsWalked) return 'unknown -- pass E did not produce a walk';
  return e.tagsWalked.indexOf('0x11') < 0 ? 'YES -- 0x11 behind it was never walked'
                                          : 'no -- 0x11 and beyond were walked';
})();
out.tablesAtEnd = tablesWanted();
out.tasksAtEnd = window.__tasks().n;
out.headline = consumers.length
  ? 'a callback fired in: ' + out.consumedIn.join(', ') + ' -- narrow within those passes next'
  : (out.coverageFailures.length
      ? 'NO CALLBACK, BUT ' + out.coverageFailures.length + ' pass(es) did not reach their last '
        + 'descriptor, so this is not a clean negative'
      : 'no tag in 0x00..0xFF made a non-iterator PC read a descriptor body, and every pass reached '
        + 'its last descriptor -- so the listings are NOT selected by tag in this loop, and the next '
        + 'subject is what else FUN_800c64cc is called with');
return out;
