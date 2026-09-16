// WHAT ARE THE NINE BYTES OF A 0xB1 ENTRY? -- sky-eluc.38, the last unknown before a real Sky
// line-up can be broadcast.
//
// ESTABLISHED BEFORE THIS (sky-eluc.12, evidence in docs/reference/digibox-emulation.md): one NIT
// widens the match table to 0x4A/ext=0x1000; a BAT's transport descriptor loop is walked by the
// generic iterator FUN_800ad590, which reads only tag and length; a private descriptor is skipped
// until a private_data_specifier_descriptor (0x5F) declares a namespace; and with specifier 2 --
// and only 2 -- the box consumes tag 0xB1 at 0x800BF7DA. Its prologue gives the outer shape:
//
//     800bf7da  lbu  v0,1(a0)      ; descriptor_length
//     800bf7de  addiu v0,-2        ; length - 2
//     800bf7e0  div  v0,a3         ; a3 = 9   -> entry count = (length - 2) / 9
//     800bf7e2  lbu  v0,2(a0)      ; body[0]
//     800bf7e4  lbu  a3,3(a0)      ; body[1]     -> a 16-bit field
//     800bf7ea  cmpi v0,0xffff     ; tested against 0xFFFF
//     800bf7ee  addiu v1,a0,0x2    ; v1 = &body[0]
//     800bf7fa  bteqz 0x800bf80c   ; if it IS 0xFFFF, go to the entry loop
//     800bf80c  addiu v1,0x2       ; step past the field -- entries begin at body[2]
//     800bf812  li   a1,0x12
//     800bf814  mult a2,a1         ; 18 bytes per OUTPUT record
//
// The previous probe gave 0xB1 a 4-byte body, so (4-2)/9 = 0 entries and the loop never ran. This
// gives it a real one.
//
// THE MEASUREMENT IS THE READ ORDER, NOT A GUESS AT FIELD WIDTHS. Each descriptor byte is recorded
// with every PC that read it and the icount at which it happened, so the parse reconstructs itself:
// consecutive offsets read by the same instruction pair are one field, and the order they are taken
// in is the order the record is built. A layout inferred that way is read off the firmware; a layout
// taken from a public Sky BAT table is a fact nobody here measured, which is exactly what
// __siBAT()'s own comment refuses to carry.
//
// EVERY BYTE IDENTIFIES ITSELF. Entry i byte j is 0xC0 + i*16 + j, so 0xC0..0xC8, 0xD0..0xD8,
// 0xE0..0xE8 -- unique across the whole loop, none of them zero (zero is the iterator's terminator),
// and a four-byte run of them is rare enough to search DRAM for, which is how the 18-byte output
// record is located afterwards.
//
// FOUR PASSES, AND THREE OF THEM ARE CONTROLS:
//   header 0xFFFF   the path the disassembly shows reaching the entry loop
//   header 0x1234   the other branch of that same test -- if the entries are read identically the
//                   field is not a gate, and if they are not, the difference IS the finding
//   N = 0           len = 2, so (len-2)/9 = 0: the entry loop must NOT run. A pass that reads
//                   entry bytes here would mean the count arithmetic is not what it appears to be
//   specifier 0     the negative -- 0xB1 must not be consumed at all. If it is, the instrument has
//                   drifted since the run that established specifier 2, and nothing above is safe
//
// THE COPY, NOT THE RING. Sections are memcpy'd out (0x800FA958) and parsed on the copy; a ring
// watch sees only the copier. The copy is located by finding an inert marker descriptor after a
// push and watching a few KB around it -- never by predicting an address, because a wrong
// prediction returns a clean zero that looks exactly like a parser reading nothing. Tag 0xFE is the
// marker and is inert by measurement: the specifier-2 sweep consumed 0xB1 and nothing else.

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

var MARK = [0xFE, 4, 0xFE, 0xA5, 0xFE, 0x5A];     // inert by measurement, used only to find the copy
function entryByte(i, j){ return (0xC0 + i * 16 + j) & 0xFF; }

function buildLoop(spec, header, nEntries){
  var descs = [], layout = [];
  function add(b, what){ layout.push({ off: descs.length, len: b.length, tag: b[0], what: what }); descs = descs.concat(b); }
  add([0x5F, 4].concat(u32(spec)), 'specifier');
  var body = u16(header);
  for (var i = 0; i < nEntries; i++) for (var j = 0; j < 9; j++) body.push(entryByte(i, j));
  add([0xB1, body.length].concat(body), 'b1');
  add(MARK, 'marker');
  add([0x41, 3].concat(u16(0x0064), [0x01]), 'control-last');
  return { descs: descs, layout: layout,
           b1: layout.filter(function(d){ return d.what === 'b1'; })[0],
           marker: layout.filter(function(d){ return d.what === 'marker'; })[0] };
}

function armedFilterForPid(pid){
  return window.__siFilters().filter(function(f){ return parseInt(f.pid, 16) === pid && f.armed; })[0] || null;
}
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }
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

// The iterator's own reads and the specifier handler's -- both expected on every pass, both named
// so that anything else is a discovery rather than noise.
var WALKER = { '0x800AD5B4': 1, '0x800AD5B6': 1, '0x800AD5F2': 1, '0x800AD5F4': 1 };
var SPEC_CB = /^0x800CAE[89ABCD]/;

var out = { question: 'the nine bytes of a 0xB1 entry, read off the consumer',
            settledAfterSeconds: waited };

if (!wants(0x4A)) {
  var n = window.__siNIT();
  if (!n.ok) throw new Error('the ladder-opening NIT was refused: ' + n.why);
  for (var w = 0; w < 10 && !wants(0x4A); w++) await new Promise(function(r){ setTimeout(r, 3000); });
}
if (!wants(0x4A)) throw new Error('the box does not subscribe to table 0x4A');
if (!armedFilterForPid(0x0011)) throw new Error('no armed filter on PID 0x0011');

async function pass(name, spec, header, nEntries){
  var L = buildLoop(spec, header, nEntries);
  var r = { pass: name, specifier: spec, header: hx(header), entries: nEntries,
            b1Length: L.b1.len - 2, expectedEntryCount: Math.floor((L.b1.len - 2 - 2) / 9) };

  // locate the copy
  var b0 = buildBat(L.descs);
  var p0 = window.__siPush(0x0011, b0.bytes);
  if (!p0.ok) { r.error = 'push refused: ' + p0.why; return r; }
  await new Promise(function(x){ setTimeout(x, 8000); });
  var ringPhys = (parseInt(p0.at, 16) >>> 0) & 0x1FFFFFF;
  var found = window.__find(MARK, 0x80000000, 0x82000000, 24).filter(function(a){
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

  var again = window.__find(MARK, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (!again.length) { r.error = 'the marker is not in the watched region afterwards -- unmappable, not zero'; return r; }
  var loopAt = (parseInt(again[0], 16) >>> 0) - L.marker.off;
  r.descriptorLoopAt = h(loopAt);
  var b1At = loopAt + L.b1.off;

  // Every read that lands inside the 0xB1 descriptor, in the order it happened. This IS the layout:
  // consecutive offsets taken by the same instruction pair are one field.
  var seq = [], perOff = {};
  log.all.forEach(function(rr){
    var a = parseInt(rr.at, 16), n = rr.size || 1, k;
    for (k = 0; k < n; k++) {
      var off = (a + k) - b1At;
      if (off < 0 || off >= L.b1.len) continue;
      if (WALKER[rr.pc] || SPEC_CB.test(rr.pc)) { (perOff[off] = perOff[off] || []).push(rr.pc + '(walk)'); continue; }
      seq.push({ icount: rr.icount, pc: rr.pc, off: off, byte: hx(L.descs[L.b1.off + off]) });
      (perOff[off] = perOff[off] || []).push(rr.pc);
    }
  });
  seq.sort(function(a, b){ return a.icount - b.icount; });
  r.consumerReadOrder = seq.map(function(x){
    var what = x.off === 0 ? 'tag' : x.off === 1 ? 'length'
             : x.off < 4 ? ('header[' + (x.off - 2) + ']')
             : ('entry' + Math.floor((x.off - 4) / 9) + '.byte' + ((x.off - 4) % 9));
    return x.pc + ' -> +' + x.off + ' ' + what + ' = ' + x.byte;
  });
  r.offsetsReadByConsumer = Object.keys(perOff).filter(function(o){
    return perOff[o].some(function(p){ return p.indexOf('(walk)') < 0; }); }).map(Number).sort(function(a,b){ return a-b; });
  r.consumerPcs = (function(){
    var by = {}; seq.forEach(function(x){ by[x.pc] = (by[x.pc] || 0) + 1; });
    return Object.keys(by).map(function(k){ return k + ' x' + by[k]; }).sort();
  })();

  // The 18-byte output record, found by a four-byte run of the first entry's own bytes. Opportunistic
  // -- the read order above is the measurement, and this is corroboration when it lands.
  if (nEntries > 0) {
    var run4 = [entryByte(0, 0), entryByte(0, 1), entryByte(0, 2), entryByte(0, 3)];
    var hitsFound = window.__find(run4, 0x80000000, 0x82000000, 12).filter(function(a){
      var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF, lp = loopAt & 0x1FFFFFF;
      return !(q >= lp && q < lp + L.descs.length) && !(q >= ringPhys && q < ringPhys + 1024);
    });
    r.outputRecordCandidates = hitsFound;
    r.outputRecordBytes = hitsFound.slice(0, 3).map(function(a){
      var base = (parseInt(a, 16) >>> 0) - 8;       // a little before, to see the record's head
      var bs = window.__peek(base, 32), o = [];
      for (var i = 0; i < bs.length; i++) o.push(('0' + bs[i].toString(16)).slice(-2));
      return h(base) + ': ' + o.join(' ');
    });
  }

  r.answer = r.offsetsReadByConsumer.length
    ? 'the consumer read ' + r.offsetsReadByConsumer.length + ' byte(s) of this descriptor'
    : 'NO non-walker PC read inside this descriptor';
  return r;
}

out.headerFFFF   = await pass('entries, header 0xFFFF', 2, 0xFFFF, 3);
out.headerOther  = await pass('entries, header 0x1234 -- the other branch', 2, 0x1234, 3);
out.zeroEntries  = await pass('control: N = 0, the entry loop must not run', 2, 0xFFFF, 0);
out.specifierNil = await pass('control: specifier 0, 0xB1 must not be consumed', 0, 0xFFFF, 3);

out.controls = {
  zeroEntriesStayedQuiet: !!(out.zeroEntries && out.zeroEntries.offsetsReadByConsumer &&
                             out.zeroEntries.offsetsReadByConsumer.filter(function(o){ return o >= 4; }).length === 0),
  specifierNilStayedQuiet: !!(out.specifierNil && out.specifierNil.offsetsReadByConsumer &&
                              out.specifierNil.offsetsReadByConsumer.length === 0)
};
out.note = (out.controls.zeroEntriesStayedQuiet && out.controls.specifierNilStayedQuiet)
  ? 'both controls held in this run, so the read orders above are about the box'
  : 'A CONTROL DID NOT HOLD -- the instrument has drifted and the layout above is not trustworthy';
return out;
