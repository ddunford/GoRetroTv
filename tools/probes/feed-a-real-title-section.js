// sky-02me.21 -- FEED A REAL SKY TITLE SECTION AND SEE WHETHER THE GUIDE TAKES IT.
//
// THE FORMAT, from openTVtoXML's opentv_read_titles() and confirmed against what this box reads:
// a title section is a channel id, two bytes the hardware unit filters on, and then records --
// event id, length, tag 0xB5, start, duration, genre, rating, Huffman title. The reader BREAKS if
// the tag is not 0xB5, which is exactly the tag this project's own sweep found being consumed.
//
// THE TWO BYTES AT data[8..9] ARE STAMPED FROM THE MATCH UNIT, NOT COMPUTED. The reference readers
// call them the MJD, and 0x9E8B is MJD 40587 (1 Jan 1970). Two live tests failed to move them: once
// by setting the clock after the line-up, and once with a 1998 TDT on air from the first
// instruction (240 sections, 0 refused). So the date reading is REFUTED for this box and the field
// is something else -- and it does not matter, because the unit states exactly what it will accept
// and this reads it back rather than theorising. The SDT's unit constrains the same two bytes to
// its original_network_id, so the field is an identifier the box already holds.
//
// THE RECORDS ARE PRECOMPUTED by scripts/skyepg/title_section.py, whose Huffman encoder was
// round-tripped through a transcription of the reference DECODER (9/9, including a leading space
// and multi-character dictionary phrases) and whose section was walked back with the reference
// reader's own arithmetic. Embedding the bytes keeps the validated encoder in one place.
//
// TITLES ARE GENERIC PLACEHOLDERS ON PURPOSE. This project's rule is that an entry with no source
// is reconstructed and must never render as historical; real listings come from the scanned
// magazines with provenance. These exist to prove the pipe carries text.
//
// WHAT WOULD MAKE THIS A FALSE POSITIVE, and each is guarded: a section that is delivered but never
// copied (checked); a copy nothing reads (checked, per PC); reads that stop at the tag and never
// reach the TEXT (the offsets are reported, and the text begins at record+13); and a guide that
// redraws for some unrelated reason (the press alternates keys and the widget count is compared
// against the same press made before the feed).
// %d records, %d bytes -- built by scripts/skyepg/title_section.py and round-tripped through
// the reference reader before being embedded here.
var RECORDS = [1,1,240,36,181,30,126,144,3,132,0,0,0,58,9,42,35,87,24,101,122,10,158,245,113,130,148,117,113,134,86,174,48,254,0,128,1,2,240,25,181,19,130,20,3,132,0,0,0,42,227,0,90,184,195,43,87,24,127,0,64,1,3,240,31,181,25,133,152,7,8,0,0,0,42,227,1,90,184,195,43,87,24,2,85,198,25,90,184,192,52,64,1,4,240,23,181,17,140,160,7,8,0,0,0,16,207,87,24,101,106,227,0,177,0,1,5,240,25,181,19,147,168,14,16,0,0,0,42,159,45,105,125,92,97,149,170,45,43,16,1,6,240,24,181,18,161,184,3,132,0,0,0,32,230,171,140,50,181,113,135,240,4,0];

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function hx(v){ return '0x' + (v >>> 0).toString(16); }
var CRC_TAB = (function(){
  var t = new Int32Array(256), i, j, c;
  for (i = 0; i < 256; i++) { c = i << 24; for (j = 0; j < 8; j++) c = (c & 0x80000000) ? ((c << 1) ^ 0x04C11DB7) : (c << 1); t[i] = c; }
  return t;
})();
function crc32(b){ var c = -1, i; for (i = 0; i < b.length; i++) c = (c << 8) ^ CRC_TAB[((c >>> 24) ^ b[i]) & 0xFF]; return c >>> 0; }

var DISPATCH = 0x800297B0, KEYEVENT = 0x8006EA04;
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
           moved: s1.hash !== s0.hash, colours: s1.colours, drew: (c.newWidget || 0) > 0 };
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
var out = { question: 'does a well-formed Sky title section reach the guide', settledAfterSeconds: waited };

// The carousel puts the line-up on air, so the box subscribes to its own listings unaided.
if (window.__siCarousel().running === false) throw new Error('carousel not running -- use --carousel');
var unit = null;
for (var w = 0; w < 40 && !unit; w++) {
  unit = window.__siMatches().filter(function(x){ return x.tableId === 0xA1 || x.tableId === 0xA0; })[0] || null;
  if (!unit) await new Promise(function(r){ setTimeout(r, 4000); });
}
if (!unit) throw new Error('the box never subscribed to 0xA0/0xA1 -- nothing to feed');
out.unit = { tableId: hx(unit.tableId), ext: hx(unit.extension), bytes: unit.bytes };
var FILTER = [parseInt(unit.bytes[6], 16) & 0xFF, parseInt(unit.bytes[7], 16) & 0xFF];
// The unit's extension mask is 0xFFFC, so it covers four channel ids; feed the lowest, which is
// the first channel of the line-up rather than an arbitrary member of the range.
var CHANNEL = (unit.extension & unit.extensionMask) >>> 0;
out.feeding = { channelId: hx(CHANNEL), filterBytes: FILTER.map(hx) };

var VER = 0;
function buildSection(){
  VER = (VER + 1) & 0x1F;
  var payload = [(CHANNEL >> 8) & 0xFF, CHANNEL & 0xFF,
                 0xC1 | ((VER & 0x1F) << 1), 0x00, 0x00,
                 FILTER[0], FILTER[1]].concat(RECORDS);
  var len = payload.length + 4;
  var s = [0xA1, 0xB0 | ((len >> 8) & 0x0F), len & 0xFF].concat(payload);
  var c = crc32(s);
  return s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]);
}

out.before = await press(0x7D, 'sky, before the listings');
out.beforeGuide = await press(0x80, 'tv guide, before the listings');

// Locate the copy, then watch it while a second identical section arrives.
var first = window.__siPush(0x33, buildSection());
out.push = first.ok ? 'ok' : first.why;
if (!first.ok) throw new Error('the title section was REFUSED: ' + first.why);
await new Promise(function(r){ setTimeout(r, 8000); });
var MARK = RECORDS.slice(0, 8);
var ringPhys = (parseInt(first.at, 16) >>> 0) & 0x1FFFFFF;
var found = window.__find(MARK, 0x80000000, 0x82000000, 24).filter(function(a){
  var q = (parseInt(a, 16) >>> 0) & 0x1FFFFFF; return !(q >= ringPhys && q < ringPhys + 4096); });
out.copiesFound = found;
if (found.length) {
  var mid = parseInt(found.filter(function(a){ return (parseInt(a,16)>>>0) >= 0x80200000; })[0] || found[0], 16) >>> 0;
  window.__readWatch((mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0);
  window.__siPush(0x33, buildSection());
  await new Promise(function(r){ setTimeout(r, 8000); });
  var log = window.__readWatchLog();
  window.__readWatch();
  var again = window.__find(MARK, (mid - 0x2000) >>> 0, (mid + 0x2000) >>> 0, 8);
  if (again.length) {
    var at = parseInt(again[0], 16) >>> 0;       // the first record's first byte
    var pcs = {}, offs = {};
    log.all.forEach(function(rr){
      var a = parseInt(rr.at, 16), n = rr.size || 1, k;
      for (k = 0; k < n; k++) { var o = (a + k) - at;
        if (o < 0 || o >= RECORDS.length) continue;
        (pcs[rr.pc] = pcs[rr.pc] || {})[o] = 1; offs[o] = 1; }
    });
    out.readers = Object.keys(pcs).map(function(p){ return { pc: p, bytes: Object.keys(pcs[p]).length }; })
                        .sort(function(a,b){ return b.bytes - a.bytes; }).slice(0, 12);
    out.offsetsRead = Object.keys(offs).map(Number).sort(function(a,b){ return a-b; });
    // The first record is 36 bytes and its Huffman text starts at +13. Reads at or past 13 mean
    // the box went into the TEXT rather than stopping at the header.
    out.reachedTheText = out.offsetsRead.some(function(o){ return o >= 13 && o < 36; });
  } else out.error = 'the marker vanished from the watched region';
} else out.error = 'the section was delivered and never copied out of the ring';

// Repeat a few times, as a real carousel would, then look.
for (var k = 0; k < 4; k++) { window.__siPush(0x33, buildSection()); await new Promise(function(r){ setTimeout(r, 4000); }); }
await new Promise(function(r){ setTimeout(r, 8000); });
out.afterSky = await press(0x7D, 'sky, after the listings');
out.afterGuide = await press(0x80, 'tv guide, after the listings');
await window.__shot('guide-with-listings');
out.tasksAtEnd = window.__tasks().n;
out.guideChanged = out.afterGuide.colours !== out.beforeGuide.colours || out.afterGuide.applies !== out.beforeGuide.applies;
out.headline = (out.reachedTheText ? 'THE BOX READ INTO THE TITLE TEXT. ' : 'the box did not read past the record header. ')
             + 'guide before: ' + out.beforeGuide.widgets + ' widgets / ' + out.beforeGuide.applies
             + ' applies / ' + out.beforeGuide.colours + ' colours; after: ' + out.afterGuide.widgets
             + ' / ' + out.afterGuide.applies + ' / ' + out.afterGuide.colours;
return out;
