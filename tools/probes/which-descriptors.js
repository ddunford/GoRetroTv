// WHICH DESCRIPTOR TAGS DOES THIS BOX ACTUALLY READ THE BODY OF? -- sky-eluc.12.
//
// Sky's channel list is a BOUQUET, and the firmware has a builder for it by name (SKY_BAT_SVL,
// alongside ASTRA_SDT_SVL and OTV_NIT_SVL), so the box is expecting one. What we do not know is
// the PRIVATE descriptor that carries the logical channel numbers: its layout is not established
// here, and __siBAT() says so in its own comment rather than inventing one. A guessed private
// descriptor is a fact nobody measured dressed as a feed.
//
// THE PARSER IS THE SPECIFICATION, so ask it. Broadcast a table whose transport descriptor loop
// carries one descriptor of every private tag 0x80..0xFF, read-watch the section ring, and see
// which BODIES get read. Every conformant parser reads every tag and length byte to skip what it
// does not want, so a tag read proves nothing -- a BODY read is the discriminator.
//
// IT CLIMBS THE LADDER RATHER THAN ASSUMING THE TOP OF IT. Measured with si-what-is-armed.js: on
// a plain boot this box's hardware match table carries only 0x40 (NIT, network 0x20) and 0x73
// (TOT), and stays that way for 220M instructions. No SDT, no BAT. That is a box on the first
// rung of a scan -- learn the network, then the transports, then the bouquet -- so the NIT is
// both the table it is certainly subscribed to AND the thing that should widen the table. So:
// probe the NIT first, watch what the match table becomes, and probe the BAT only if the box
// asks for one. A BAT sent to a box that never subscribed would be a reading about subscription.
//
// FOUR THINGS THIS HAS TO GUARD AGAINST, and each of them would otherwise return a clean zero:
//
//   1. A BULK COPY READS EVERYTHING FROM ONE PC and is not a parser. If the section is copied out
//      of the ring and parsed on the copy, the ring watch sees the copier and nothing else. So
//      reads are grouped by PC and each PC's COVERAGE of the region is reported: a PC that reads
//      nearly all of it in order is a copier, and if that is the only reader the answer is "the
//      parse happens elsewhere" rather than "no tag is consumed".
//
//   2. A PARSER THAT BAILS AT THE FIRST UNKNOWN TAG would make the result a prefix while looking
//      like an answer. So there is a KNOWN-CONSUMED control descriptor at BOTH ENDS of the loop --
//      satellite_delivery (0x43) first and service_list (0x41) last, the two a transport loop
//      cannot be read without. Both controls read means the loop was walked end to end and a tag
//      with no body read was genuinely skipped. Only the first means the parser stopped, and the
//      result is reported as incomplete rather than as a finding.
//
//   3. A DUPLICATE VERSION IS DISCARDED WITHOUT PARSING. An SI manager that has already seen this
//      table at this version has no reason to look at it twice, and that discard is invisible from
//      outside. So the version is distinctive, and the section task's own PCs are counted before
//      and after: if delivery did not advance, every zero below is about delivery.
//
//   4. DELIVERY HERE IS BY PID, NOT BY MATCH UNIT. siPush() writes straight into the filter's ring
//      and sets the status bit, so a section reaches the firmware whenever the PID is armed --
//      the hardware table-id match is not in that path. So "no match unit for 0x4A" is a fact
//      about what the box has SUBSCRIBED to, not about whether bytes arrive, and it is reported
//      rather than used to refuse. Getting that backwards would have turned a subscription
//      finding into a delivery excuse.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }

// ---- preconditions ---------------------------------------------------------------------
// The box must be settled: its own readout is the signal, and it waits for BGLOAD to have RUN
// rather than merely to be standing still. A push into the flash-checking window is dropped, and
// a dropped section is indistinguishable from an ignored one.
var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 180) {
  await new Promise(function(r){ setTimeout(r, 1000); });
  waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's -- every reading below would be of a saturated machine');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

function armedFilterForPid(pid){
  return window.__siFilters().filter(function(f){ return parseInt(f.pid, 16) === pid && f.armed; })[0] || null;
}
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return (x.tableId === null ? '0x??' : '0x' + x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : '')
         + (x.extensionMask !== null && x.extensionMask !== 0xFFFF ? '&0x' + x.extensionMask.toString(16) : '');
  });
}
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }

if (!armedFilterForPid(0x0010))
  throw new Error('no armed filter on PID 0x0010 -- the demux should carry 0x10, 0x11 and 0x14 by '
                + 'the end of every boot, so this is the instrument failing and not the box');

// ---- section building --------------------------------------------------------------------
// Built here rather than through __siNIT()/__siBAT(), because the point is a descriptor loop
// neither of them will write. The CRC is the firmware's own check on this construction: a wrong
// one is dropped and the controls would then read nothing, so a mis-built section fails as a
// harness error rather than as a finding about descriptors.
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
// The two controls: a transport descriptor loop cannot be read without them. satellite_delivery
// tells the box where the transport is; service_list tells it which services are on it, and the
// SVL builder must read that body to learn a service exists at all.
function satellite(){ return [0x43, 11].concat(bcd8(1177800, 8), bcd8(282, 4), [0x81], bcd8(275000, 8).slice(0, 4)); }
function serviceList(sid){ return [0x41, 3].concat(u16(sid), [0x01]); }

var BODY_LEN = 4, VERSION = 9;
function probeLoop(){
  var descs = [], layout = [], i;
  function add(bytes, what){
    layout.push({ off: descs.length, len: bytes.length, tag: bytes[0], what: what });
    descs = descs.concat(bytes);
  }
  add(satellite(), 'control-first');
  for (i = 0x80; i <= 0xFF; i++) add([i, BODY_LEN, i, 0xA5, i, 0x5A], 'probe');
  add(serviceList(0x0064), 'control-last');
  return { bytes: descs, layout: layout };
}

// ---- the experiment, run once per table ---------------------------------------------------
var LISR = 0x800041B4, SECTASK = 0x80004520;
function hits(){
  var r = window.__pcHits(LISR, SECTASK);
  // __pcHits keys with the page's hex32, which UPPERCASES. A lower-case lookup returns undefined,
  // not a count, and reads as a perfectly plausible zero -- which is how two natives were once
  // reported as never called out of a 236-entry census.
  if (r[h(LISR)] === undefined || r[h(SECTASK)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- the census cannot find '
                  + 'its own subject, which is a harness failure and not a count of zero');
  return { lisr: r[h(LISR)], task: r[h(SECTASK)] };
}

function run(name, tableId, pid, ext){
  var p = probeLoop(), descs = p.bytes, layout = p.layout;
  var ids = window.__siIds();
  var ts = u16(ids.tsid).concat(u16(ids.networkId),
               [0xF0 | ((descs.length >>> 8) & 0x0F), descs.length & 0xFF], descs);
  // The outer loop differs by table: a NIT opens with network_descriptors, a BAT with
  // bouquet_descriptors. Same shape, different name, and both take a name descriptor.
  var outer = tableId === 0x40 ? [0x40, 3, 0x53, 0x6B, 0x79]     // network_name "Sky"
                               : [0x47, 3, 0x53, 0x6B, 0x79];    // bouquet_name "Sky"
  var len = 5 + 2 + outer.length + 2 + ts.length + 4;
  var section = [tableId, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(ext), [0xC1 | ((VERSION & 0x1F) << 1), 0x00, 0x00],
            [0xF0 | ((outer.length >>> 8) & 0x0F), outer.length & 0xFF], outer,
            [0xF0 | ((ts.length >>> 8) & 0x0F), ts.length & 0xFF], ts);
  var c = crc32(section);
  section = section.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]);
  if (section.length > 1024)
    return { table: name, error: 'section is ' + section.length + ' bytes -- over the 1021 DVB '
           + 'long-section limit, so a refusal would be about the length rather than the descriptors' };

  var f = armedFilterForPid(pid);
  if (!f) return { table: name, error: 'no armed filter on PID 0x' + pid.toString(16) };
  var DESC_AT = section.length - 4 - descs.length;

  var before = hits();
  // The whole ring rather than the descriptor bytes alone: the landing address is only known
  // after the push, and watching wider and mapping afterwards cannot miss a read that watching
  // narrower would have excluded by a wrong guess at the layout.
  var ringBase = parseInt(f.ring, 16);
  window.__readWatch(ringBase, ringBase + 0x3000);
  var push = window.__siPush(pid, section);
  if (!push.ok) { window.__readWatch(); return { table: name, error: 'push refused: ' + push.why }; }
  return { pending: true, name: name, section: section, descs: descs, layout: layout,
           DESC_AT: DESC_AT, push: push, before: before, ext: ext, tableId: tableId };
}

function collect(st){
  var log = window.__readWatchLog();
  window.__readWatch();
  var after = hits();
  var at = parseInt(st.push.at, 16);
  var lo = at + st.DESC_AT, hi = lo + st.descs.length;
  var byTag = {}, pcCover = {}, touched = {};
  st.layout.forEach(function(d){ byTag[d.tag] = byTag[d.tag] || { tag: 0, len: 0, body: 0 }; });

  log.all.forEach(function(r){
    var a = parseInt(r.at, 16), n = r.size || 1, k;
    for (k = 0; k < n; k++) {
      var b = a + k;
      if (b < lo || b >= hi) continue;
      var off = b - lo;
      (pcCover[r.pc] = pcCover[r.pc] || {})[off] = 1;
      touched[off] = 1;
      for (var di = 0; di < st.layout.length; di++) {
        var d = st.layout[di];
        if (off < d.off || off >= d.off + d.len) continue;
        var e = byTag[d.tag];
        if (off === d.off) e.tag++; else if (off === d.off + 1) e.len++; else e.body++;
        break;
      }
    }
  });

  var pcs = Object.keys(pcCover).map(function(p){
    var n = Object.keys(pcCover[p]).length;
    return { pc: p, bytes: n, coverage: +(100 * n / st.descs.length).toFixed(1) };
  }).sort(function(a, b){ return b.bytes - a.bytes; });

  var ctl = st.layout.filter(function(d){ return d.what.indexOf('control') === 0; }).map(function(d){
    var body = 0, k;
    for (k = d.off + 2; k < d.off + d.len; k++) if (touched[k]) body++;
    return { what: d.what, tag: '0x' + d.tag.toString(16), at: h(lo + d.off), bodyBytesRead: body };
  });
  var walked = ctl.length === 2 && ctl[0].bodyBytesRead > 0 && ctl[1].bodyBytesRead > 0;
  var probeTags = st.layout.filter(function(d){ return d.what === 'probe'; }).map(function(d){ return d.tag; });
  var bodyRead = probeTags.filter(function(t){ return byTag[t].body > 0; });
  var tagOnly  = probeTags.filter(function(t){ return byTag[t].tag > 0 && byTag[t].body === 0; });

  return {
    table: st.name, tableId: '0x' + st.tableId.toString(16), extension: '0x' + st.ext.toString(16),
    version: VERSION, sectionBytes: st.section.length, landedAt: st.push.at,
    descriptorLoopAt: h(lo), descriptorBytes: st.descs.length,
    delivery: { lisr: after.lisr - st.before.lisr, sectionTask: after.task - st.before.task,
                note: 'both must be > 0, or every zero below is about delivery rather than parsing' },
    watch: { reads: log.reads,
             capped: log.capped ? 'CAPPED -- a sample of the busiest window, not a record of it' : false },
    readers: pcs.slice(0, 10),
    bulkCopiers: pcs.filter(function(p){ return p.coverage >= 80; }),
    controls: ctl,
    walkedWholeLoop: walked,
    answer: {
      tagsWithBodyRead: bodyRead.map(function(t){ return '0x' + t.toString(16); }),
      tagsSeenButSkipped: tagOnly.length,
      caveat: walked
        ? 'both controls were read, so the loop was walked end to end and a skipped tag was genuinely skipped'
        : 'THE LOOP WAS NOT WALKED END TO END -- this is a prefix, not an answer'
    },
    perTagNonZero: probeTags.filter(function(t){ return byTag[t].tag || byTag[t].body; })
      .map(function(t){ return '0x' + t.toString(16) + ' tag=' + byTag[t].tag + ' len=' + byTag[t].len + ' body=' + byTag[t].body; })
  };
}

var out = { question: 'which private descriptor tags does this box read the BODY of',
            settledAfterSeconds: waited, tablesWantedBefore: tablesWanted() };

// ---- rung 1: the NIT, which the box is certainly subscribed to ----------------------------
var ids0 = window.__siIds();
var st = run('NIT', 0x40, 0x0010, ids0.networkId);
if (st.error) { out.nit = st; }
else { await new Promise(function(r){ setTimeout(r, 8000); }); out.nit = collect(st); }

// ---- did the NIT widen the subscription? --------------------------------------------------
out.ladder = [];
for (var s = 0; s < 5; s++) {
  await new Promise(function(r){ setTimeout(r, 6000); });
  out.ladder.push({ tables: tablesWanted().join(' '), ids: (function(){
    var x = window.__siIds();
    return 'net=0x' + (x.networkId >>> 0).toString(16) + ' bq=0x' + (x.bouquetId >>> 0).toString(16)
         + ' ts=0x' + (x.tsid >>> 0).toString(16); })() });
}
out.tablesWantedAfter = tablesWanted();
out.nitWidenedTheSubscription = out.tablesWantedAfter.length > out.tablesWantedBefore.length;

// ---- rung 2: the BAT, only if the box now asks for one -------------------------------------
if (wants(0x4A)) {
  var ids1 = window.__siIds();
  var bq = (ids1.bouquetIdMask !== null && ids1.bouquetId !== null &&
            ((0x1001 & ids1.bouquetIdMask) === (ids1.bouquetId & ids1.bouquetIdMask)))
         ? 0x1001 : ids1.bouquetId;               // authentic where the filter still accepts it
  var st2 = run('BAT', 0x4A, 0x0011, bq);
  if (st2.error) { out.bat = st2; }
  else { await new Promise(function(r){ setTimeout(r, 8000); }); out.bat = collect(st2); }
} else {
  out.bat = { skipped: 'the box still does not subscribe to table 0x4A, so a pushed BAT would be a '
            + 'reading about subscription rather than about descriptors. Delivery is by PID and '
            + 'would have worked; the parse is what would not be attributable.' };
}

return out;
