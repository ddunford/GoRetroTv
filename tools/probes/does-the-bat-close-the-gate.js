// DOES PARSING A BAT CLOSE THE SCREEN GATE AGAIN? -- sky-02me.18, and a test of the hypothesis
// rather than an illustration of it.
//
// ESTABLISHED, and then CORRECTED BY THIS PROBE'S FIRST RUN. which-table-stops-the-drawing.js fed
// the tables cumulatively -- TDT, then NIT, then SDT, then a plain BAT -- and the drawing died at
// the BAT. The obvious reading was "a plain BAT costs the drawing". It does not: the first run of
// this probe pushed two plain BATs at a box that had been fed nothing else and it kept drawing
// perfectly, 62 and 249 widgets, with the gate word never written.
//
// THE DIFFERENCE IS THE NIT, and it is a difference in whether the BAT is PARSED AT ALL. A bare box
// subscribes only to 0x40 and 0x73; the NIT is what opens 0x4A/ext=0x1000. Delivery here is by PID,
// so a BAT pushed at an unsubscribed box arrives and is dropped -- which looks identical, from
// outside, to a BAT that was harmless.
//
// So the minimum that reproduces it is NIT then BAT, and that is what this feeds. "The stage at
// which it died" and "the table that does it" were never the same claim.
//
// THE FICTION ANSWERS EXACTLY TWO GATES, which is what makes this cheap to test. skyGatesTick()
// does two things and no more:
//
//     store(0x80054F84, 4, 0)             a DRAM word that reads 0xFFFFFFFF until it is answered
//     flash 0x9FC72FA1: 0x75 -> 0x76      one byte of the EPG's o-code
//
// It fires ONCE, when the task list first reaches 42. So if parsing a BAT writes that DRAM word
// back to something non-zero, the gate is shut again, nothing re-answers it, and the box stops
// drawing -- which would explain the whole of sky-02me.18 in one line and make the fix obvious.
//
// AND IF IT DOES NOT, THAT IS WORTH JUST AS MUCH. The hypothesis in the tracker says a service list
// is the moment the box first knows which service it is on, and that the application then waits for
// something about that service. If the gate word is untouched, that story is wrong as stated and the
// refusal is somewhere this project has not looked.
//
// THREE THINGS MAKE IT A TEST RATHER THAN A DEMONSTRATION:
//
//   * A WRITE WATCH, not a before-and-after read. A word that is written and restored would read
//     identical at both ends and the whole mechanism would be invisible. __writeWatch records the
//     PC of every write, so whatever touches it names itself.
//   * A CONTROL PRESS BEFORE THE FEED, asserted. If the box was not drawing beforehand there is
//     nothing to lose and no reading below means anything.
//   * AND THE REPAIR IS ATTEMPTED. If the word did change, re-zeroing it and pressing again is the
//     falsification: drawing returns and the mechanism is confirmed, or it does not and the word is
//     a symptom rather than the cause. A hypothesis that only ever gets confirmed is not being
//     tested.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
var GATE = 0x80054F84, OCODE = 0x9FC72FA1;

function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function gateWord(){ return h(window.__peek(GATE, 4).reduce(function(a, b){ return (a << 8) | b; }, 0) >>> 0); }
// The o-code gate is a FLASH byte and this page has no flash reader in its debug API, so it is
// reported as unreadable rather than guessed at. It also cannot change on its own: the application
// executes from flash and does not write it, so the DRAM gate is the one worth watching.
function ocodeByte(){ return 'not readable -- no flash peek in the debug API'; }

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
  throw new Error('the carousel is RUNNING -- this probe feeds one table by hand');

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
  return r[h(KEYEVENT)];
}
async function press(raw, label){
  window.__traceCalls(TRACE); window.__traceClear();
  var b = { k: inputHits(), blits: window.__blitLog().length, surface: surface() };
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  var a = { k: inputHits(), blits: window.__blitLog().length, surface: surface() };
  window.__traceCalls([]);
  return { key: label, keyEvents: a.k - b.k, widgets: c.newWidget || 0, applies: c.apply || 0,
           damage: c.DAMAGE || 0, blits: a.blits - b.blits,
           moved: a.surface.hash !== b.surface.hash, gate: gateWord(),
           drew: (c.newWidget || 0) > 0 };
}

var out = { question: 'does parsing a BAT write the screen gate shut again',
            settledAfterSeconds: waited,
            gateAtStart: gateWord(), ocodeAtStart: ocodeByte() };

// ---- the control, before anything is fed ----------------------------------------------------
out.control = [ await press(0x7D, 'sky, before the BAT'), await press(0x80, 'tv guide, before the BAT') ];
if (!out.control.every(function(p){ return p.drew; }))
  throw new Error('the control presses did not draw before anything was fed -- nothing below means '
                + 'anything. ' + JSON.stringify(out.control));

// ---- watch the gate word across the BAT ------------------------------------------------------
// A WRITE WATCH rather than a before-and-after read: a word written and then restored reads the same
// at both ends, and the mechanism would be invisible.
var w = window.__writeWatch(GATE, GATE + 4);
if (!w.watching) throw new Error('could not watch the gate word: ' + JSON.stringify(w));
out.gateBeforeBat = gateWord();
// The NIT first, because without it the box is not subscribed to 0x4A and the BAT is dropped rather
// than parsed. Pressing after the NIT alone is the intermediate control: the NIT must NOT be what
// kills it, or the minimal pair is wrong and the subject is the NIT.
out.nitPushes = [ window.__siNIT(undefined, { version: 11 }), window.__siNIT(undefined, { version: 12 }) ]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 8000); });
out.tablesAfterNit = window.__siMatches().map(function(x){
  return '0x' + x.tableId.toString(16) + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
out.afterNit = [ await press(0x7D, 'sky, after the NIT only') ];
if (!out.afterNit[0].drew) {
  out.verdict = 'THE NIT ALONE STOPPED THE DRAWING -- which contradicts the bisect, where two more '
              + 'stages drew after it. The NIT is the subject, not the BAT.';
  window.__writeWatch();
  return out;
}
out.batPushes = [ window.__siBAT(undefined, { version: 11 }), window.__siBAT(undefined, { version: 12 }) ]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 9000); });
var wlog = window.__writeWatchLog();
window.__writeWatch();
out.gateAfterBat = gateWord();
out.gateWrites = { count: wlog.writes, byPc: wlog.byPc,
                   all: wlog.all.slice(0, 12).map(function(x){
                     return x.pc + ' -> ' + x.at + ' = ' + x.val + ' @' + x.icount; }) };
out.ocodeAfterBat = ocodeByte();

// ---- is it actually dead? --------------------------------------------------------------------
out.afterBat = [ await press(0x7D, 'sky, after the BAT'), await press(0x80, 'tv guide, after the BAT') ];
out.stoppedDrawing = !out.afterBat.some(function(p){ return p.drew; });
if (!out.stoppedDrawing) {
  out.verdict = 'THE BOX KEPT DRAWING after a NIT and two plain BATs, so even the minimal pair does '
              + 'not reproduce it. Something else in the bisect\'s cumulative feed is required -- the '
              + 'TDT and the SDT are what remain.';
  return out;
}

// ---- the repair, which is the falsification -------------------------------------------------
// If the gate word is what shut the drawing, re-answering it must bring the drawing back. If it
// does not, the word is a symptom and the hypothesis is wrong as stated.
out.repair = { gateBefore: gateWord() };
window.__poke(GATE, [0, 0, 0, 0]);
out.repair.gateAfterPoke = gateWord();
await new Promise(function(r){ setTimeout(r, 3000); });
out.repair.presses = [ await press(0x7D, 'sky, after re-zeroing the gate'),
                       await press(0x80, 'tv guide, after re-zeroing the gate') ];
out.repair.drewAgain = out.repair.presses.some(function(p){ return p.drew; });

out.verdict = out.gateWrites.count
  ? (out.repair.drewAgain
      ? 'CONFIRMED: the BAT parse writes the gate word (' + out.gateWrites.count + ' write(s), '
        + out.gateWrites.byPc.join(' ') + '), the box stops drawing, and RE-ZEROING IT BRINGS THE '
        + 'DRAWING BACK. That is the mechanism and the shape of the fix.'
      : 'HALF: the BAT parse DOES write the gate word, but re-zeroing it does not restore the '
        + 'drawing -- so the write is real and is not sufficient. Something else is also shut.')
  : (out.repair.drewAgain
      ? 'ODD: nothing wrote the gate word, yet poking it zero restored the drawing. Read the poke '
        + 'and the timing again before believing either half.'
      : 'REFUTED: nothing wrote the gate word and re-zeroing it changes nothing. The BAT stops the '
        + 'drawing by some other route, and the service-list hypothesis is wrong AS STATED. The '
        + 'o-code gate and the widget builder itself are the next subjects.');
out.tasksAtEnd = window.__tasks().n;
return out;
