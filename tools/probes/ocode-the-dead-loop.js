// sky-02me.18 -- WHAT IS THE 43-INSTRUCTION O-CODE LOOP THE DEAD PRESS SPINS IN?
//
// ocode-healthy-vs-dead.js established it: after a parsed BAT a key press runs 258 bytecode
// instructions over 43 distinct addresses, 0x9FC85F41..0x9FC85F92, six identical passes in nine
// seconds -- and never reaches the application's event loop at 0x9FC4A538. A healthy press runs
// 41,895 instructions over 8,228 addresses. The application is not declining to draw; its o-code
// is somewhere else entirely, and that is why eight native-level hypotheses all missed.
//
// This captures the loop as a FULL trace so scripts/ocode-disasm.py can read it. Full means
// UNFILTERED: the disassembler learns operand lengths from the bytes consumed between two
// main-site reads, so a trace filtered to the main fetch site teaches it that every opcode has
// zero operands -- a table that is wrong everywhere and looks fine.
//
// TWO TRACES, AND THE CONTROL IS NOT OPTIONAL. The operand table is measured from a HEALTHY press,
// which executes a wide slice of the image; the dead loop is 43 addresses and would teach the
// decoder almost nothing about anything. The healthy trace is also what --check validates the
// listing against. Feeding the disassembler only the dead loop would produce a listing whose
// boundaries nothing corroborates, and a decoder run over bytes does not fail -- it produces
// plausible output.
//
// THE REPRO IS ASSERTED. The control press must draw and the fed press must not. If either fails
// this probe says so and returns no listing, because a trace of a box that never went wrong is a
// trace of nothing -- which is exactly what the first run of the A/B probe produced when its NIT
// and BAT were pushed back to back and the BAT was dropped unread.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
var OCODE_LO = 0x9FC4A400, OCODE_LEN = 353188;
var TRACE_MAX = 60000;
var DISPATCH = 0x800297B0, KEYEVENT = 0x8006EA04;
var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];
function inputHits(){
  var r = window.__pcHits(DISPATCH, KEYEVENT);
  if (r[h(DISPATCH)] === undefined || r[h(KEYEVENT)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- harness failure, not a zero');
  return { dispatcher: r[h(DISPATCH)], keyEvents: r[h(KEYEVENT)] };
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
  window.__traceCalls(TRACE); window.__traceClear();
  var b = inputHits(), blits0 = window.__blitLog().length;
  var w = window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN, { max: TRACE_MAX });
  if (w.max !== TRACE_MAX) throw new Error('this page\'s __readWatch does not take {max}');
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var lg = window.__readWatchLog();
  window.__readWatch();
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var a = inputHits();
  var mainSite = {}; (lg.byPc || []).forEach(function(s){ var p = s.split(' x'); mainSite[p[0]] = parseInt(p[1], 10); });
  return { tag: tag, key: label, keyEvents: a.keyEvents - b.keyEvents,
           widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
           blits: window.__blitLog().length - blits0,
           drew: (c.newWidget || 0) > 0,
           entries: lg.all.length, capped: !!lg.capped, byPc: lg.byPc,
           opcodeFetches: mainSite['0x80069298'] === undefined ? null : mainSite['0x80069298'],
           trace: lg.all.map(function(e){ return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }) };
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

var out = { question: 'what is the o-code loop a dead press spins in', settledAfterSeconds: waited,
            tablesAtStart: tablesWanted() };

// The healthy press: the operand table, and the corroboration --check needs.
out.healthy = await press(0x7D, 'sky', 'healthy-sky');
if (!out.healthy.drew)
  throw new Error('the CONTROL press did not draw before anything was fed -- nothing below would mean anything');
if (out.healthy.opcodeFetches === null)
  throw new Error('the interpreter main fetch site is not in byPc on a press that DREW -- harness failure');

out.nitPushes = [window.__siNIT(undefined, { version: 21 }), window.__siNIT(undefined, { version: 22 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 8000); });
out.tablesAfterNit = tablesWanted();
if (!window.__siMatches().some(function(x){ return x.tableId === 0x4A; }))
  throw new Error('the box did not subscribe to 0x4A after the NIT -- a BAT now would be DROPPED unread');
out.batPushes = [window.__siBAT(undefined, { version: 21 }), window.__siBAT(undefined, { version: 22 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 9000); });
out.tablesAfterBat = tablesWanted();

out.dead = await press(0x80, 'tv guide', 'dead-guide');
out.reproduced = !out.dead.drew;
if (!out.reproduced)
  out.warning = 'THE FED PRESS STILL DREW (' + out.dead.widgets + ' widgets) -- the loop was not '
              + 'reproduced and the "dead" trace is of a healthy box. Conclude nothing from it.';

// The loop, characterised without decoding anything: distinct addresses, span, and how many times
// the sequence repeats. A period that divides the length exactly is what makes it a LOOP rather
// than a walk that happens to stay in one region.
var MAIN = '0x80069298';
var ops = out.dead.trace.map(function(r){ return r.split(','); })
                        .filter(function(r){ return r[0] === MAIN; })
                        .map(function(r){ return r[1]; });
var uniq = {}; ops.forEach(function(a){ uniq[a] = 1; });
var keys = Object.keys(uniq).sort();
var period = 0;
for (var p = 1; p <= ops.length / 2; p++) {
  var ok = true;
  for (var k = p; k < ops.length; k++) if (ops[k] !== ops[k - p]) { ok = false; break; }
  if (ok) { period = p; break; }
}
out.loop = { opcodes: ops.length, distinct: keys.length,
             lo: keys[0], hi: keys[keys.length - 1],
             period: period, passes: period ? +(ops.length / period).toFixed(2) : null,
             firstPass: period ? ops.slice(0, period).join(' ') : null };
out.tasksAtEnd = window.__tasks().n;
return out;
