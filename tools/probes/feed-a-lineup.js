// FEED THE BOX A REAL CHANNEL LINE-UP -- sky-eluc.12.
//
// Everything this needs is now measured rather than guessed, and every number below has its
// evidence in docs/reference/digibox-emulation.md:
//
//   the ladder      one NIT widens the match table to 0x4A/ext=0x1000 (the BAT, Sky's bouquet base)
//   the namespace   a private descriptor is skipped until a private_data_specifier_descriptor
//                   (0x5F) declares one, and only specifier 2 unlocks anything here
//   the descriptor  tag 0xB1, consumed at 0x800BF7DA
//   the gate        the u16 at its body[0..1] must be 0xFFFF or NOT ONE ENTRY is decoded --
//                   a well-formed descriptor the box accepts and silently ignores
//   the entry       nine bytes: u16, u8, u16, u16, u16-packed, with N = (length - 2) / 9
//   the record      18 bytes; entry+0..1 -> rec[4..5], +2 -> rec[12], +3..4 -> rec[6..7],
//                   +5..6 -> rec[8..9], +7..8 >> 4 -> rec[10..11] and its bottom four bits
//                   unpacked one per byte into rec[13..16]
//
// WHAT IS STILL NOT MEASURED IS WHAT THE FIELDS MEAN, and this probe is built to name them rather
// than to assume them. Each field carries a value from its own decade, so whatever turns up in a
// record -- or on screen -- identifies which field it came from and nothing has to be inferred
// from a public BAT table:
//
//   +0..1  the service id, 0x0064..0x0067, which the NIT's service_list and the SDT both declare
//   +2     0x01
//   +3..4  0x0BB8 + i        (3000-odd: a value no other field can produce)
//   +5..6  0x1770 + i        (6000-odd)
//   +7..8  (0x0ABC + i) << 4 | 0b0101   so the stored 12-bit value is 0x0ABC+i and the four flags
//                                       are 0,1,0,1 -- which makes the UNPACKING falsifiable too
//
// THE RECORD IS THE PROOF AND IT IS SEARCHED FOR BY ITS OWN CONTENT. rec[4..12] should read
// 00 64 0B B8 17 70 0A BC 01 for the first service -- nine bytes that exist nowhere else in DRAM
// by construction. Finding it proves the whole chain end to end; not finding it says the chain
// broke, and the per-stage readings below say where.
//
// A REAL MULTIPLEX REPEATS AND A ONE-SHOT CANNOT BE TOLD FROM A BOX THAT WAS NOT LISTENING, so the
// set goes out several times with rising version numbers. The SDT goes with it because names are
// what make a line-up readable -- and whether this box parses one is itself a measurement: it has
// never subscribed to 0x42 in any run, though delivery is by PID and PID 0x0011 is armed, so the
// section certainly arrives.

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
  var b = []; ids.forEach(function(s){ b = b.concat(u16(s), [0x01]); });
  return [0x41, b.length].concat(b);
}

// The line-up. Four services, one per decade of probe value, so a number seen anywhere names the
// field it came from.
var SERVICES = [0x0064, 0x0065, 0x0066, 0x0067];
var LINEUP = SERVICES.map(function(sid, i){
  return { sid: sid, f2: 0x01, f34: 0x0BB8 + i, f56: 0x1770 + i, ch: 0x0ABC + i, flags: 0x5 };
});
function b1Descriptor(){
  var body = u16(0xFFFF);                          // the gate -- measured, not optional
  LINEUP.forEach(function(e){
    var packed = ((e.ch << 4) | e.flags) & 0xFFFF;
    body = body.concat(u16(e.sid), [e.f2], u16(e.f34), u16(e.f56), u16(packed));
  });
  return [0xB1, body.length].concat(body);
}
// rec[4..12] for the first service: the nine bytes that exist nowhere else in DRAM by construction.
function recordPattern(e){
  return u16(e.sid).concat(u16(e.f34), u16(e.f56), u16(e.ch), [e.f2]);
}

function armedFilterForPid(pid){
  return window.__siFilters().filter(function(f){ return parseInt(f.pid, 16) === pid && f.armed; })[0] || null;
}
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function nvramWritten(){
  var e = window.__eeprom(), n = 0;
  for (var i = 0; i < e.length; i++) if (e[i] !== 0xFF) n++;
  return n;
}

var VERSION = 0;
function pushBat(){
  VERSION = (VERSION + 1) & 0x1F;
  var ids = window.__siIds();
  var bq = (ids.bouquetIdMask !== null && ids.bouquetId !== null &&
            ((0x1001 & ids.bouquetIdMask) === (ids.bouquetId & ids.bouquetIdMask))) ? 0x1001 : ids.bouquetId;
  var descs = [0x5F, 4].concat(u32(2), b1Descriptor(), serviceListDesc(SERVICES), satellite());
  var ts = u16(ids.tsid).concat(u16(ids.networkId),
               [0xF0 | ((descs.length >>> 8) & 0x0F), descs.length & 0xFF], descs);
  var name = [0x47, 3, 0x53, 0x6B, 0x79];
  var len = 5 + 2 + name.length + 2 + ts.length + 4;
  var s = [0x4A, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(bq), [0xC1 | ((VERSION & 0x1F) << 1), 0x00, 0x00],
            [0xF0 | ((name.length >>> 8) & 0x0F), name.length & 0xFF], name,
            [0xF0 | ((ts.length >>> 8) & 0x0F), ts.length & 0xFF], ts);
  var c = crc32(s);
  var r = window.__siPush(0x0011, s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]));
  r.version = VERSION; r.bouquetId = hx(bq); r.bytes = s.length + 4;
  return r;
}

var out = { question: 'does a real line-up build a service list, and what do the 0xB1 fields mean',
            settledAfterSeconds: waited,
            before: { tables: tablesWanted(), surface: surface(), nvram: nvramWritten(),
                      ids: window.__siIds() } };

// ---- rung one: the NIT ----------------------------------------------------------------
if (!wants(0x4A)) {
  var n = window.__siNIT();
  if (!n.ok) throw new Error('the ladder-opening NIT was refused: ' + n.why);
  for (var w = 0; w < 10 && !wants(0x4A); w++) await new Promise(function(r){ setTimeout(r, 3000); });
}
out.afterNit = { tables: tablesWanted(), ids: window.__siIds() };
if (!wants(0x4A)) throw new Error('the box does not subscribe to table 0x4A');
if (!armedFilterForPid(0x0011)) throw new Error('no armed filter on PID 0x0011');

// ---- broadcast the set, repeatedly ------------------------------------------------------
out.rounds = [];
for (var round = 0; round < 4; round++) {
  var nit = window.__siNIT(undefined, { version: round + 1 });
  var sdt = window.__siSDT(undefined, { version: round + 1 });
  var bat = pushBat();
  await new Promise(function(r){ setTimeout(r, 7000); });
  out.rounds.push({ round: round,
                    nit: nit.ok ? 'ok' : nit.why, sdt: sdt.ok ? 'ok' : sdt.why,
                    bat: bat.ok ? ('ok v' + bat.version + ' ' + bat.bytes + 'B') : bat.why,
                    tables: tablesWanted() });
}
await new Promise(function(r){ setTimeout(r, 8000); });

// ---- did a record get built? -------------------------------------------------------------
// Searched by its own content across the WHOLE 32 MB -- __find's default, but the default is worth
// writing out: a scan stopping at 0x80400000 answers about an eighth of the machine.
out.records = LINEUP.map(function(e, i){
  var pat = recordPattern(e);
  var at = window.__find(pat, 0x80000000, 0x82000000, 8);
  var r = { service: hx(e.sid), channel: hx(e.ch), foundAt: at };
  if (at.length) {
    // rec[4..12] is what was matched, so the record starts four bytes earlier.
    var base = (parseInt(at[0], 16) >>> 0) - 4;
    var bs = window.__peek(base, 18), o = [];
    for (var k = 0; k < bs.length; k++) o.push(('0' + bs[k].toString(16)).slice(-2));
    r.record = h(base) + ': ' + o.join(' ');
    r.decoded = {
      'rec[0..1] from transport context': hx((bs[0] << 8) | bs[1]),
      'rec[2..3] from transport context': hx((bs[2] << 8) | bs[3]),
      'rec[4..5]  = entry+0..1': hx((bs[4] << 8) | bs[5]),
      'rec[6..7]  = entry+3..4': hx((bs[6] << 8) | bs[7]),
      'rec[8..9]  = entry+5..6': hx((bs[8] << 8) | bs[9]),
      'rec[10..11]= entry+7..8 >> 4': hx((bs[10] << 8) | bs[11]),
      'rec[12]    = entry+2': hx(bs[12]),
      'rec[13..16]= the four flag bits': [bs[13], bs[14], bs[15], bs[16]].join(','),
      'rec[17]': hx(bs[17])
    };
    // The flags were sent as 0b0101, so bit3..bit0 must read 0,1,0,1. If they do, the unpacking
    // measured in sky-eluc.38 is confirmed on live data rather than only in the disassembly.
    r.flagUnpackingConfirmed = (bs[13] === 0 && bs[14] === 1 && bs[15] === 0 && bs[16] === 1);
  }
  return r;
});
out.recordsBuilt = out.records.filter(function(r){ return r.foundAt.length; }).length;

out.after = { tables: tablesWanted(), surface: surface(), nvram: nvramWritten(),
              ids: window.__siIds() };
out.widened = out.after.tables !== out.afterNit.tables;
out.nvramDelta = out.after.nvram - out.before.nvram;
out.screenChanged = out.after.surface.hash !== out.before.surface.hash;
out.verdict = out.recordsBuilt
  ? ('THE LINE-UP IS IN: ' + out.recordsBuilt + ' of ' + LINEUP.length + ' services built a record'
     + (out.widened ? ', AND the subscription widened to ' + out.after.tables : ', subscription unchanged'))
  : 'no record was built -- the chain broke; the per-stage readings above say where';
return out;
