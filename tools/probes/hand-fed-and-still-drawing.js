// DOES A HAND-FED BOX STILL DRAW? -- sky-02me.18, and the cheapest question that could collapse it.
//
// Measured in where-drawing-stops.js: with the repeating SI CAROUSEL running, a key press arrives at
// the input layer exactly as it does on a silent box and the application then builds ZERO widgets.
// Not rasterisation, not the blitter -- it declines to make a screen at all.
//
// BUT EVERY MEASUREMENT OF THAT USED THE CAROUSEL. The probes that fed this box by hand -- one-shot
// __siPush of a NIT, an SDT and a BAT -- never pressed a key afterwards, so there is NO evidence
// either way about a hand-fed box. Two very different worlds fit every observation so far:
//
//   the REPETITION is the problem   a carousel re-sending four tables for ever keeps the SI manager
//                                   permanently busy, and a one-shot feed would draw fine. If so
//                                   sky-02me.18 is about rate, listings are reachable today, and
//                                   the fix is a slower or finite feed.
//   SI ITSELF is the problem        any acquisition takes the application somewhere it will not
//                                   draw, and the rate is irrelevant. Then the gate tracing in
//                                   sky-02me.18 is the only way through.
//
// This separates them, and it is one run.
//
// THE CONTROL COMES FIRST AND IS ASSERTED. Raw 0x7D on a settled silent box builds 62 widgets and
// moves the screen. That press happens BEFORE anything is fed, so if it fails the box was never
// drawing and every quiet reading afterwards would be meaningless.
//
// AND THE FEED HAS TO HAVE LANDED, or a box that still draws proves only that nothing reached it.
// The check is the same one the line-up work used: the match table must widen to include 0xA1, which
// the box only subscribes to after it has parsed a 0xB1 line-up. No widening, no test.
//
// COLD ONLY. scripts/digibox-probe.mjs keeps a persistent profile, so its runs are WARM by default,
// and a warm box with a broadcast does not reach 42 tasks at all (22 tasks, ~1.9G instructions).
// Run this with --cold.

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

var car = window.__siCarousel();
if (car.running !== false)
  throw new Error('the carousel is RUNNING -- this probe is about a hand-fed box and would be '
                + 'measuring the thing it is trying to rule out. Run it without --carousel and '
                + 'without ?si=1.');

var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget',     args: 1 },
  { pc: 0x80082604, name: 'apply',         args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',        args: 2 },
  { pc: 0x80085128, name: 'setWindowRoot', args: 2 }
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
  return { key: label + ' (raw ' + h(raw).slice(-2) + ')', pressedOn: b.surface,
           keyEvents: a.input.keyEvents - b.input.keyEvents,
           dispatcher: a.input.dispatcher - b.input.dispatcher,
           widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
           blits: a.blits - b.blits, surface: a.surface,
           drew: a.surface.hash !== b.surface.hash && (c.newWidget || 0) > 0 };
}

function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}
function wants(tid){ return window.__siMatches().some(function(x){ return x.tableId === tid; }); }

var out = { question: 'does a box fed by hand, with no repeating carousel, still draw',
            settledAfterSeconds: waited, carouselAtStart: car, tablesBefore: tablesWanted() };

// ---- the control, BEFORE anything is fed --------------------------------------------------
out.controlPress = await press(0x7D, 'sky, before any feed');
if (!out.controlPress.drew)
  throw new Error('the control press did not draw on a silent settled box -- the instrument or the '
                + 'state is wrong, so nothing after a feed would mean anything. '
                + JSON.stringify(out.controlPress));

// ---- feed by hand, one shot per table, no carousel -----------------------------------------
// The same line-up __siBAT()'s opts.lineup builds, which is the one measured in sky-eluc.12: the
// specifier, the 0xFFFF gate and nine-byte entries. Several rounds with rising versions, because a
// single copy cannot be told from a box that was not listening -- but SENT BY HAND, which is the
// whole point: no repetition after these stop.
var LINEUP = [
  { id: 0x0064, listings: 0x0BB8, channel: 101 },
  { id: 0x0065, listings: 0x0BB9, channel: 102 },
  { id: 0x0066, listings: 0x0BBA, channel: 103 },
  { id: 0x0067, listings: 0x0BBB, channel: 104 }
];
out.feed = [];
for (var round = 0; round < 4; round++) {
  var n = window.__siNIT(undefined, { version: round + 1 });
  var d = window.__siSDT(undefined, { version: round + 1 });
  var b = window.__siBAT(undefined, { version: round + 1, lineup: LINEUP });
  var t = window.__siTDT(1998, 1, 1, 12, round, 0);
  out.feed.push({ round: round, nit: n.ok ? 'ok' : n.why, sdt: d.ok ? 'ok' : d.why,
                  bat: b.ok ? 'ok' : b.why, tdt: t.ok ? 'ok' : t.why, tables: tablesWanted() });
  await new Promise(function(r){ setTimeout(r, 7000); });
}
await new Promise(function(r){ setTimeout(r, 8000); });
out.tablesAfterFeed = tablesWanted();
out.feedLanded = wants(0xA1);
out.sectionsPushed = window.__siLog().length;
// The carousel must STILL be off -- nothing here should have started one, and if something did
// then this is the carousel experiment again wearing a different name.
out.carouselAtEnd = window.__siCarousel();

if (!out.feedLanded) {
  out.verdict = 'THE FEED DID NOT LAND -- the box never subscribed to 0xA1, so it has not reached '
              + 'the state this is meant to test. A box that still draws here would prove only '
              + 'that nothing reached it.';
  return out;
}

// ---- and now, does it still draw? -----------------------------------------------------------
out.afterFeed = [];
out.afterFeed.push(await press(0x80, 'tv guide, after the hand feed'));
out.afterFeed.push(await press(0x3C, 'back up, after the hand feed'));
out.afterFeed.push(await press(0x7D, 'sky again, after the hand feed'));

var drewAfter = out.afterFeed.filter(function(p){ return p.drew; }).length;
out.verdict = drewAfter
  ? 'THE BOX STILL DRAWS AFTER A HAND FEED (' + drewAfter + ' of ' + out.afterFeed.length
    + ' presses drew) -- so sky-02me.18 is about the CAROUSEL, not about SI. Listings are reachable '
    + 'with a one-shot or slow feed, and the fix is a rate rather than a gate.'
  : 'THE BOX DOES NOT DRAW AFTER A HAND FEED EITHER -- acquisition itself is what stops it, the '
    + 'repetition is irrelevant, and the gate tracing in sky-02me.18 is the only way through.';
out.tasksAtEnd = window.__tasks().n;
if (out.tasksAtEnd < 42)
  out.caveat = 'THE BOX LOST TASKS during the walk (' + out.tasksAtEnd + ') -- something wedged it, '
             + 'and the presses after that point are suspect.';
return out;
