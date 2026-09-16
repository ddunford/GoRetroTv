// WHICH TABLE COSTS THE DRAWING? -- sky-02me.18, the narrowing that names the code path.
//
// Established in hand-fed-and-still-drawing.js: sixteen sections pushed BY HAND, with no carousel
// and nothing repeating, are enough to stop the application building any screen at all. The keys
// still arrive, the task list is still 42, and the surface stays parked on the menu the control
// press drew. So it is not a rate, and the feed is now a cheap reproduction rather than a
// two-minute broadcast.
//
// That feed sent four tables at once. This sends them ONE AT A TIME and presses after each, so the
// answer is a table rather than "SI". A table names a parser, a parser names a code path, and that
// is most of the gate tracing done before it starts.
//
// THE ORDER IS SIMPLEST-FIRST, DELIBERATELY. The TDT is the smallest table in DVB and the only one
// with no CRC at all -- if the clock alone costs the drawing then the cause is not in any of the
// acquisition machinery and the search collapses to something very small. After that the natural
// ladder: NIT, SDT, a plain BAT, and finally a BAT carrying the line-up, which is the only one that
// makes the box subscribe to 0xA1.
//
// EVERY STAGE PRESSES TWICE, ALTERNATING sky AND tv guide, because that is a pair of transitions a
// healthy box always redraws for. Pressing the SAME key twice would be the trap: a box that is
// already on the menu has no reason to rebuild it, and "no widgets" would then mean "nothing
// changed" rather than "it refuses" -- a false positive that would end the search on stage one.
//
// STAGE 0 IS THE CONTROL AND IT IS ASSERTED. Both its presses must draw before anything is fed. And
// the walk STOPS at the first stage that goes quiet: once the box has stopped drawing, every later
// stage is a reading about a box that was already broken.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 240) {
  await new Promise(function(r){ setTimeout(r, 1000); });
  waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
if (window.__siCarousel().running !== false)
  throw new Error('the carousel is RUNNING -- this probe feeds by hand and would be measuring the '
                + 'thing it is trying to isolate');

var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];
var DISPATCH = 0x800297B0, KEYEVENT = 0x8006EA04;
function inputHits(){
  var r = window.__pcHits(DISPATCH, KEYEVENT);
  if (r[h(DISPATCH)] === undefined || r[h(KEYEVENT)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- a harness failure, not a zero');
  return { dispatcher: r[h(DISPATCH)], keyEvents: r[h(KEYEVENT)] };
}
async function press(raw, label){
  window.__traceCalls(TRACE); window.__traceClear();
  var b = { input: inputHits(), blits: window.__blitLog().length, surface: surface() };
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  var a = { input: inputHits(), blits: window.__blitLog().length, surface: surface() };
  window.__traceCalls([]);
  return { key: label, keyEvents: a.input.keyEvents - b.input.keyEvents,
           widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
           blits: a.blits - b.blits, moved: a.surface.hash !== b.surface.hash,
           surface: a.surface, drew: (c.newWidget || 0) > 0 };
}
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}

var LINEUP = [
  { id: 0x0064, listings: 0x0BB8, channel: 101 },
  { id: 0x0065, listings: 0x0BB9, channel: 102 },
  { id: 0x0066, listings: 0x0BBA, channel: 103 },
  { id: 0x0067, listings: 0x0BBB, channel: 104 }
];
var STAGES = [
  { name: 'nothing fed -- the control', feed: null },
  { name: 'TDT only (the clock: smallest table in DVB, and the only one with no CRC)',
    feed: function(v){ return { tdt: window.__siTDT(1998, 1, 1, 12, v, 0) }; } },
  { name: 'NIT', feed: function(v){ return { nit: window.__siNIT(undefined, { version: v }) }; } },
  { name: 'SDT', feed: function(v){ return { sdt: window.__siSDT(undefined, { version: v }) }; } },
  { name: 'BAT, plain -- no private descriptor',
    feed: function(v){ return { bat: window.__siBAT(undefined, { version: v }) }; } },
  { name: 'BAT carrying the line-up -- the only one that makes the box subscribe to 0xA1',
    feed: function(v){ return { bat: window.__siBAT(undefined, { version: v, lineup: LINEUP }) }; } }
];

var out = { question: 'which table costs the application its ability to build a screen',
            settledAfterSeconds: waited, tablesAtStart: tablesWanted(), stages: [] };

for (var si = 0; si < STAGES.length; si++) {
  var st = STAGES[si];
  var row = { stage: si, fed: st.name, tablesBefore: tablesWanted() };
  if (st.feed) {
    // Twice, with different versions: one copy of a table cannot be told from a box that was busy.
    row.pushes = [st.feed(si * 2 + 1), st.feed(si * 2 + 2)].map(function(r){
      var k = Object.keys(r)[0]; return k + ': ' + (r[k].ok ? 'ok' : r[k].why); });
    await new Promise(function(r){ setTimeout(r, 7000); });
  }
  row.tablesAfter = tablesWanted();
  row.subscriptionChanged = row.tablesAfter !== row.tablesBefore;
  // Alternating, never the same key twice: a box already on the menu has no reason to rebuild it.
  row.presses = [ await press(0x7D, 'sky'), await press(0x80, 'tv guide') ];
  row.drewBoth = row.presses.every(function(p){ return p.drew; });
  row.keysArrived = row.presses.every(function(p){ return p.keyEvents > 0; });
  out.stages.push(row);

  if (si === 0 && !row.drewBoth)
    throw new Error('the CONTROL stage did not draw on both presses before anything was fed -- the '
                  + 'instrument or the state is wrong, so no later quiet stage would mean anything. '
                  + JSON.stringify(row.presses));
  if (!row.drewBoth) {
    out.culprit = st.name;
    out.verdict = 'THE DRAWING STOPS AT STAGE ' + si + ': ' + st.name
                + (row.keysArrived ? '. The keys still arrived, so it is the application refusing '
                   + 'to build a screen and not an input fault.'
                   : '. AND THE KEYS STOPPED ARRIVING TOO -- which makes this an input-path finding '
                   + 'rather than a drawing one, and changes where to look.');
    break;
  }
}
if (!out.culprit)
  out.verdict = 'EVERY STAGE STILL DREW. That contradicts hand-fed-and-still-drawing.js, which saw '
              + 'the same tables stop it -- so the difference is in HOW they were fed (four tables '
              + 'per round there, one table per stage here) and that difference is the next subject.';
out.tasksAtEnd = window.__tasks().n;
if (out.tasksAtEnd < 42)
  out.caveat = 'the box lost tasks during the walk (' + out.tasksAtEnd + ') -- something wedged it '
             + 'and the later stages are suspect';
return out;
