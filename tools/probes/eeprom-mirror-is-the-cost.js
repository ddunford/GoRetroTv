// sky-02me.18 -- IS THE NVRAM MIRROR WHAT COSTS THE INTERFACE? The A/B, in the harness.
//
// THE DIAGNOSIS THIS TESTS. After a parsed BAT the EPG's o-code enters a counted loop at
// 0x9FC85F41..0x9FC85F93 -- decoded and validated against the trace, back edge `41 ad` at
// 0x9FC85F92 targeting 0x9FC85F41 exactly -- whose body is one scall (2,0x28) and one
// scall (2,0x2D) per service and whose counter is tested against DS[0x00019A20]. It TERMINATES.
// While it runs the interpreter never returns to its event loop at 0x9FC4A538, so a key press is
// not refused, it is simply never looked at. Measured: 258 bytecode instructions per nine-second
// press, six identical passes, zero widgets -- against 41,895 instructions and 249 widgets on a
// healthy tv guide press.
//
// So the question is not why the box refuses. It is why one pass of a counted loop takes 1.3
// SECONDS. The suspect is ours: every I2C STOP on the EEPROM built a 16,384-character string,
// base64-ed it and called localStorage.setItem SYNCHRONOUSLY, for an eight-byte record.
//
// THE MEASUREMENT, and it is a difference rather than a number. Same feed, same keys, same
// harness; the only variable is ?eesync=1, which restores write-through. If batching the mirror
// is what frees the interface then the loop's duration collapses and the box draws; if it changes
// nothing then the diagnosis is wrong and no bigger version of it is worth trying.
//
// WHAT IT MUST NOT DO IS CHANGE WHAT THE FIRMWARE READS. The byte array IS the device and is
// written immediately in both configurations -- only the localStorage mirror is batched -- so
// eepromBytesNotFF is reported at the end of both runs as the check that the same data landed.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
var DISPATCH = 0x800297B0, KEYEVENT = 0x8006EA04;
var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function inputHits(){
  var r = window.__pcHits(DISPATCH, KEYEVENT);
  if (r[h(DISPATCH)] === undefined || r[h(KEYEVENT)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- harness failure, not a zero');
  return r[h(KEYEVENT)];
}
function ee(){ return window.__i2cState().eeprom; }
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
  var k0 = inputHits(), s0 = surface(), b0 = window.__blitLog().length;
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var s1 = surface();
  return { tag: tag, key: label, keyEvents: inputHits() - k0,
           widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
           blits: window.__blitLog().length - b0,
           moved: s1.hash !== s0.hash, colours: s1.colours, drew: (c.newWidget || 0) > 0 };
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
if (ee().writeThrough === undefined)
  throw new Error('this page has no writeThrough flag -- it predates the batched mirror, so this '
                + 'run cannot say which configuration it measured and neither number would mean '
                + 'anything. Reload from the current source.');
window.__profile(true);

// SAY WHICH MACHINE THIS IS. Both configurations run the identical probe, so the output has to
// carry the variable or the two runs cannot be told apart afterwards.
var out = { question: 'does batching the NVRAM mirror free the interface after a parsed BAT',
            configuration: ee().writeThrough ? 'WRITE-THROUGH (?eesync=1, the old behaviour)'
                                             : 'BATCHED (the default)',
            writeThrough: ee().writeThrough,
            settledAfterSeconds: waited, tablesAtStart: tablesWanted(), eeAtStart: ee() };

out.control = await press(0x7D, 'sky', 'control-sky');
if (!out.control.drew)
  throw new Error('the CONTROL press did not draw before anything was fed -- nothing below means anything');

out.nitPushes = [window.__siNIT(undefined, { version: 31 }), window.__siNIT(undefined, { version: 32 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 8000); });
out.tablesAfterNit = tablesWanted();
if (!window.__siMatches().some(function(x){ return x.tableId === 0x4A; }))
  throw new Error('the box did not subscribe to 0x4A after the NIT -- a BAT now would be DROPPED unread');

var eeBefore = ee(), t0 = performance.now();
out.batPushes = [window.__siBAT(undefined, { version: 31 }), window.__siBAT(undefined, { version: 32 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });

// THE PRESS THAT USED TO BE DEAD, made at the same moment in both runs.
out.rightAfterTheBat = await press(0x80, 'tv guide', 'right-after-the-bat');
out.tablesAfterBat = tablesWanted();

// HOW LONG THE NVRAM WORK TAKES. Polled on the write counter rather than slept on, so the number
// is the loop's duration and not the probe's patience. Three still samples end it.
var samples = [], last = ee().writes, still = 0, secs = 0;
for (var w = 0; w < 150 && still < 3; w++) {
  await new Promise(function(r){ setTimeout(r, 2000); });
  secs += 2;
  var now = ee().writes;
  samples.push(now - last);
  still = (now === last) ? still + 1 : 0;
  last = now;
}
out.nvramWorkSeconds = secs - 6;                 // the three still samples are not work
out.writesPerTwoSeconds = samples;
out.eeAfter = ee();
out.eeDelta = { writes: out.eeAfter.writes - eeBefore.writes,
                saves: out.eeAfter.saves - eeBefore.saves,
                saveMs: out.eeAfter.saveMs - eeBefore.saveMs };
out.wallSecondsFromBat = +((performance.now() - t0) / 1000).toFixed(1);

// sky, NOT tv guide: the press right after the BAT drew the guide, so a second tv guide press
// has nothing to rebuild and its zero would read as the fault rather than as no transition.
out.afterTheWork = await press(0x7D, 'sky', 'after-the-work');
out.tasksAtEnd = window.__tasks().n;
out.eepromBytesNotFF = out.eeAfter.bytesNotFF;
out.headline = 'configuration ' + out.configuration
             + ': the NVRAM work took ' + out.nvramWorkSeconds + 's ('
             + out.eeDelta.writes + ' byte writes, ' + out.eeDelta.saves + ' whole-device saves, '
             + out.eeDelta.saveMs + 'ms inside the save itself); the press right after the BAT built '
             + out.rightAfterTheBat.widgets + ' widgets and the press after the work built '
             + out.afterTheWork.widgets;
return out;
