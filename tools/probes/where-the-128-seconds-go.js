// sky-02me.18 -- WHERE DOES THE TIME GO? The NVRAM mirror was refused; this asks the next question.
//
// ESTABLISHED. After a parsed BAT the EPG's o-code runs a counted loop at 0x9FC85F41..0x9FC85F93
// (decoded and --check validated; back edge `41 ad` at 0x9FC85F92 targets 0x9FC85F41 exactly)
// whose body is one scall (2,0x28) and one scall (2,0x2D) per service, bounded by DS[0x00019A20].
// It terminates. While it runs the interpreter never returns to its event loop at 0x9FC4A538, so a
// key press is never LOOKED at -- 258 bytecode instructions per press against a healthy 41,895.
//
// REFUSED, and by the instrument built to test it: the localStorage mirror. Whole-device saves cost
// 289 ms across 383 of them out of 128 SECONDS -- 0.2%. A fix that changes nothing means the
// diagnosis is wrong rather than too small, so the batching was reverted rather than enlarged.
//
// THE NEXT QUESTION IS WHETHER THE BOX IS WORKING OR WAITING, and the two have opposite fixes.
// 128 s at this box's ~2.7M instructions/s is ~345M instructions -- most of a whole cold boot --
// for 7,798 EEPROM byte writes, or ~44,000 instructions a byte. Either the driver genuinely burns
// that, or it is parked in a wait and the wall clock is our timer scaling rather than work.
//
//   hottest PC is an idle/wait loop  -> WAIT-BOUND. The fix is timing: what the driver waits for
//                                       and how long our model makes that take.
//   hottest PC is driver code        -> COMPUTE-BOUND. The fix is in the path itself.
//
// Sampled across the loop rather than read once, because a single reading cannot tell a steady
// state from a phase. __pcHist has no clear function -- successive __rangeHits are CUMULATIVE and
// must be differenced -- so every figure here is a difference between two samples.
//
// THE PRESSES ALTERNATE AND THE SECOND ONE IS MADE FROM A CHECKED STATE. The previous probe pressed
// tv guide twice: the first drew the guide and the second had nothing to redraw, so its zero
// widgets read as deadness and was not. Here the press DURING the loop and the press AFTER it are
// different keys, and each reports the screen it was made from.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
var DISPATCH = 0x800297B0, KEYEVENT = 0x8006EA04;
var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];
function icount(){ return parseInt(document.getElementById('icount').textContent.replace(/,/g, ''), 10); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function ee(){ return window.__i2cState().eeprom; }
function keyEvents(){
  var r = window.__pcHits(KEYEVENT);
  if (r[h(KEYEVENT)] === undefined) throw new Error('__pcHits did not return its key -- harness failure');
  return r[h(KEYEVENT)];
}
// A PRESS THAT CANNOT REDRAW IS A HARNESS FAILURE, NOT A ZERO. A box already on a screen has no
// reason to rebuild it, so pressing the same key twice running reports 0 widgets for a perfectly
// healthy box -- which reads exactly like the fault these probes exist to measure. It caught two
// probes in one session: one pressed tv guide while already on the guide and called it dead, and
// one pressed sky while already on the menu and called that dead too. Both times the screen the
// press was made FROM was in the probe's own output and neither probe looked. So the check is
// mechanised here rather than left as a thing to remember: alternate, or fail loudly.
var lastKeyPressed = null;
function assertRedrawable(raw, tag){
  if (lastKeyPressed === raw)
    throw new Error('HARNESS: ' + tag + ' presses raw 0x' + raw.toString(16) + ' twice running. A box '
                  + 'already on that screen has nothing to rebuild, so the widget count would be 0 for '
                  + 'a healthy box and would read as the fault. Alternate the keys.');
  lastKeyPressed = raw;
}
async function press(raw, label, tag){
  assertRedrawable(raw, tag);
  var from = surface();
  window.__traceCalls(TRACE); window.__traceClear();
  var k0 = keyEvents(), b0 = window.__blitLog().length;
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var to = surface();
  return { tag: tag, key: label, pressedFromColours: from.colours, keyEvents: keyEvents() - k0,
           widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
           blits: window.__blitLog().length - b0, moved: to.hash !== from.hash,
           colours: to.colours, drew: (c.newWidget || 0) > 0 };
}
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');
window.__profile(true);

var out = { question: 'is the box working or waiting during the 128 seconds after a parsed BAT',
            settledAfterSeconds: waited, tablesAtStart: tablesWanted() };

// THE IDLE BASELINE, so "hot" has something to be hot against. Same instruments, same window
// length, a box with nothing fed -- and it is what says whether the busiest PC during the loop is
// busy because of the loop or busy always.
// __rangeHits returns { total, distinctPCs, hottest: [[hex, count], ...] } -- READ OFF THE PAGE
// after a first version of this probe assumed it returned an array and died with
// "i1.hot.slice is not a function". A harness error is the honest outcome there; the trap would
// have been a shape that happened to survive and produce numbers about the wrong thing.
function hits(){ return window.__rangeHits(0x80000000, 0x80200000, 400); }
function snapshot(){
  var r = hits();
  return { icount: icount(), total: r.total, hot: r.hottest, ee: ee(), tasks: window.__tasks().n };
}
function hotMap(r){
  var m = {};
  r.hottest.forEach(function(e){ m[e[0]] = e[1]; });
  return m;
}
var i0 = snapshot();
await new Promise(function(r){ setTimeout(r, 10000); });
var i1 = snapshot();
out.idleBaseline = { seconds: 10, instructions: i1.icount - i0.icount,
                     profiledTotal: i1.total - i0.total,
                     eeWrites: i1.ee.writes - i0.ee.writes,
                     hottest: (function(){
                       var a = hotMap(hits()), b = {};
                       i0.hot.forEach(function(e){ b[e[0]] = e[1]; });
                       var d = [];
                       Object.keys(a).forEach(function(k){ var n = a[k] - (b[k] || 0); if (n > 0) d.push([k, n]); });
                       d.sort(function(x, y){ return y[1] - x[1]; });
                       return d.slice(0, 8).map(function(e){ return e[0] + ' x' + e[1]; });
                     })() };

out.control = await press(0x7D, 'sky', 'control-sky');
if (!out.control.drew)
  throw new Error('the CONTROL press did not draw before anything was fed -- nothing below means anything');

out.nitPushes = [window.__siNIT(undefined, { version: 41 }), window.__siNIT(undefined, { version: 42 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 8000); });
out.tablesAfterNit = tablesWanted();
if (!window.__siMatches().some(function(x){ return x.tableId === 0x4A; }))
  throw new Error('the box did not subscribe to 0x4A after the NIT -- a BAT now would be DROPPED unread');

out.batPushes = [window.__siBAT(undefined, { version: 41 }), window.__siBAT(undefined, { version: 42 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
// Let the parse start before sampling -- the loop is what this measures, not the seconds before it.
await new Promise(function(r){ setTimeout(r, 9000); });
out.tablesAfterBat = tablesWanted();

// SAMPLES ACROSS THE LOOP. Each one differences the cumulative histogram against the last, so the
// hot list is "hot during THIS ten seconds" rather than "hot since the boot".
var samples = [], prev = snapshot(), prevHot = hotMap(hits());
for (var s = 0; s < 18; s++) {
  await new Promise(function(r){ setTimeout(r, 10000); });
  var now = snapshot(), nowHot = hotMap(hits()), delta = [];
  Object.keys(nowHot).forEach(function(a){
    var d = nowHot[a] - (prevHot[a] || 0);
    if (d > 0) delta.push([a, d]);
  });
  delta.sort(function(x, y){ return y[1] - x[1]; });
  var instrs = now.icount - prev.icount, eew = now.ee.writes - prev.ee.writes;
  samples.push({ t: (s + 1) * 10, instructions: instrs, eeWrites: eew,
                 instructionsPerEeByte: eew ? Math.round(instrs / eew) : null,
                 hottest: delta.slice(0, 6).map(function(e){ return e[0] + ' x' + e[1]; }),
                 topShare: delta.length ? +(100 * delta[0][1] / delta.reduce(function(a, b){ return a + b[1]; }, 0)).toFixed(1) : null });
  prev = now; prevHot = nowHot;
  if (eew === 0 && s > 1) break;                 // the loop has finished
}
out.samples = samples;
out.loopSeconds = samples.length * 10;

// The press DURING the loop -- made only if the loop is still running, and it says so either way.
out.stillRunningWhenPressed = samples.length && samples[samples.length - 1].eeWrites > 0;
// tv guide, NOT sky: the control press already put the box on the menu, and pressing sky again
// there rebuilds nothing. That is precisely the reading this probe's first run got wrong.
out.afterTheLoop = await press(0x80, 'tv guide', 'tv-guide-after-the-loop');
out.eeAtEnd = ee();
out.tasksAtEnd = window.__tasks().n;
out.headline = 'the loop ran about ' + out.loopSeconds + 's; the busiest PC per ten-second window is '
             + (samples.length ? samples[Math.floor(samples.length / 2)].hottest[0] : 'n/a')
             + ' and the idle baseline\'s busiest is ' + (out.idleBaseline.hottest[0] || 'n/a');
return out;
