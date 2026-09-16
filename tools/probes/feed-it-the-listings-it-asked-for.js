// IS IT WAITING FOR THE LISTINGS IT JUST ASKED FOR? -- sky-02me.18, and the one thing never tried.
//
// THE STATE OF PLAY. A parsed BAT makes the box subscribe to table 0xA1 on PID 0x33 with the
// per-service id from the 0xB1 entries as the extension -- it asks for its own listings. It then
// spends about two minutes writing a service list and a favchn list to the EEPROM, consuming a
// hundred pool slots, AND STOPS: counters frozen, 42 tasks all in their healthy states, eight of
// them still being scheduled, nothing blocked. A key press arrives at the input layer and builds
// zero widgets. The RTOS is fine. It is an application-level refusal.
//
// AND EVERY PRESS EVER MEASURED IN THAT STATE WAS ON A BOX THAT HAD ASKED FOR LISTINGS AND RECEIVED
// NONE. That is the untested variable. An EPG that has just acquired a channel list and is waiting
// for the event data it requested is not obviously wrong to decline to build a screen -- and nobody
// has ever answered it.
//
// THE HONEST WEAKNESS OF THE IDEA, stated before the run rather than after: it also refuses to
// rebuild the BOX OFFICE menu, which has no obvious need for listings. So the hypothesis is not
// clean. It is still worth one run, because it is cheap, it is on the direct path to the thing that
// is actually wanted, and a negative narrows the refusal to something that is not about data at all.
//
// WHAT CAN AND CANNOT BE FED. The section's frame is known and measured: table 0xA1 on PID 0x33,
// extension within the unit's 0xFFFC mask, payload[0..1] = 0x9E 0x8B, (payload[3] & 0x70) == 0, and
// a tag/length descriptor loop from payload offset 6. WHAT IS NOT KNOWN is which descriptor tag
// carries the programmes -- that is sky-02me.17, still open. So these sections are WELL-FORMED AND
// EMPTY OF MEANING: the right envelope, a walkable loop, no content the box can use. If the refusal
// is "I am waiting for a section on 0xA1", an empty one may satisfy it. If the refusal is "I am
// waiting for PROGRAMMES", it will not, and that is a different and useful answer.
//
// CONTROLS: a healthy press first, asserted. The repro asserted. The pushes asserted as accepted --
// a refused push would make the silence afterwards a fact about delivery. And presses BOTH before
// and after the listings, so "it still does not draw" is a comparison rather than a single reading.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
var CRC_TAB = (function(){
  var t = new Int32Array(256), i, j, c;
  for (i = 0; i < 256; i++) { c = i << 24; for (j = 0; j < 8; j++) c = (c & 0x80000000) ? ((c << 1) ^ 0x04C11DB7) : (c << 1); t[i] = c; }
  return t;
})();
function crc32(b){ var c = -1, i; for (i = 0; i < b.length; i++) c = (c << 8) ^ CRC_TAB[((c >>> 24) ^ b[i]) & 0xFF]; return c >>> 0; }
function u16(v){ return [(v >>> 8) & 0xFF, v & 0xFF]; }

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 240) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');

var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
async function press(raw, label, seconds){
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, (seconds || 10) * 1000); });
  var w = window.__traceLog().filter(function(x){ return x.name === 'newWidget'; }).length;
  window.__traceCalls([]);
  var s1 = surface();
  return { key: label, widgets: w, moved: s1.hash !== s0.hash, colours: s1.colours, hash: s1.hash };
}
function tables(){
  return window.__siMatches().map(function(x){
    return '0x' + x.tableId.toString(16) + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}
function armedPid(pid){ return window.__siFilters().some(function(f){ return parseInt(f.pid,16) === pid && f.armed; }); }

var LINEUP = [
  { id: 0x0064, listings: 0x0BB8, channel: 101 },
  { id: 0x0065, listings: 0x0BB9, channel: 102 },
  { id: 0x0066, listings: 0x0BBA, channel: 103 },
  { id: 0x0067, listings: 0x0BBB, channel: 104 }
];

var out = { settledAfterSeconds: waited, tablesAtStart: tables() };
out.healthy = await press(0x7D, 'sky, healthy');
if (!out.healthy.widgets) throw new Error('the healthy press built no widgets -- no healthy half');

// ---- give it a line-up, which is what makes it ask for listings --------------------------------
out.feed = { nit: [window.__siNIT(undefined,{version:101}), window.__siNIT(undefined,{version:102})].map(function(r){return r.ok?'ok':r.why;}) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined,{version:101,lineup:LINEUP}), window.__siBAT(undefined,{version:102,lineup:LINEUP})]
                 .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 10000); });
out.tablesAfterLineup = tables();
if (!/0xa1/.test(out.tablesAfterLineup))
  throw new Error('the box did not subscribe to 0xA1 after the line-up, so there is nothing to '
                + 'answer and this run cannot test the hypothesis. Tables: ' + out.tablesAfterLineup);
if (!armedPid(0x33)) throw new Error('PID 0x33 is not armed -- the listings cannot be delivered');

out.beforeListings = await press(0x7D, 'sky, after the line-up and BEFORE any listings');
if (out.beforeListings.widgets)
  throw new Error('the box is still drawing after the line-up -- the repro did not reproduce, so '
                + 'feeding listings would be answering a question nobody asked');

// ---- answer it: well-formed 0xA1 sections, one per service, repeated --------------------------
// 0x9E 0x8B is the signature the hardware match unit demands; payload[3] must have bits 6:4 clear;
// the descriptor loop starts at payload offset 6 and is walked by tag and length.
var A1_VERSION = 0;
function buildA1(ext){
  A1_VERSION = (A1_VERSION + 1) & 0x1F;
  var p = [0x9E, 0x8B, 0x00, 0x00, 0x00, 0x00];
  // Two small, well-formed descriptors so the walk has something to walk and terminates cleanly.
  p = p.concat([0x01, 0x04, 0xDE, 0xAD, 0xBE, 0xEF], [0x02, 0x02, 0x12, 0x34]);
  var len = 5 + p.length + 4;
  var s = [0xA1, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
    .concat(u16(ext), [0xC1 | ((A1_VERSION & 0x1F) << 1), 0x00, 0x00], p);
  var c = crc32(s);
  return s.concat([(c >>> 24) & 0xFF, (c >>> 16) & 0xFF, (c >>> 8) & 0xFF, c & 0xFF]);
}
out.listingsPushes = [];
for (var round = 0; round < 3; round++) {
  for (var li = 0; li < LINEUP.length; li++) {
    var r = window.__siPush(0x33, buildA1(LINEUP[li].listings));
    out.listingsPushes.push((r.ok ? 'ok ' : 'REFUSED ') + h(LINEUP[li].listings).slice(-4)
                            + (r.ok ? '' : ' -- ' + r.why));
  }
  await new Promise(function(r){ setTimeout(r, 6000); });
}
if (out.listingsPushes.some(function(x){ return /REFUSED/.test(x); }))
  throw new Error('a listings push was refused, so any silence below is about delivery: '
                + out.listingsPushes.filter(function(x){ return /REFUSED/.test(x); }).join(' | '));
await new Promise(function(r){ setTimeout(r, 10000); });

// ---- and now? -----------------------------------------------------------------------------------
out.afterListings = [ await press(0x7D, 'sky, after the listings', 12),
                      await press(0x80, 'tv guide, after the listings', 14) ];
out.tablesAtEnd = tables();
var drew = out.afterListings.filter(function(p){ return p.widgets > 0; });
out.verdict = drew.length
  ? 'IT DRAWS AGAIN. Answering the 0xA1 subscription with well-formed sections brought the interface '
  + 'back (' + drew.map(function(p){ return p.key + ': ' + p.widgets + ' widgets'; }).join(', ')
  + '). The box was WAITING for the listings it had asked for, not refusing -- so a line-up and a '
  + 'working interface can exist at once, and the only thing between here and a filled guide is '
  + 'sky-02me.17, the descriptor tag that carries the programmes.'
  : 'STILL NOTHING. Twelve well-formed 0xA1 sections were accepted on PID 0x33 across the four '
  + 'services the box asked about, and it still builds no widgets. So the refusal is NOT "waiting '
  + 'for a section on 0xA1" -- which is worth knowing, because it was the last cheap explanation. '
  + 'Either it wants sections with real CONTENT (sky-02me.17) or the refusal has nothing to do with '
  + 'listings at all.';
out.tasksAtEnd = window.__tasks().n;
return out;
