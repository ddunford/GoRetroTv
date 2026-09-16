// WHERE DOES THE DRAWING STOP WHEN A BROADCAST IS RUNNING? -- sky-02me.18.
//
// Measured with the A/B in scripts/../guidestate: on a settled box, pressing sky then tv guide
// moves the surface from 12 colours to 37 and then 34 when nothing is being broadcast, and leaves
// it BYTE-IDENTICAL for thirty seconds when the carousel is on. Acquisition itself is healthy --
// 124 sections, 0 refused, and the box subscribes to its own listings at 0xA1/ext=0xBBB unaided.
// So acquisition works and the interface stops.
//
// THIS DOES NOT GO STRAIGHT TO THE SUSPECT. skyGatesTick() is the obvious candidate and obvious
// candidates are what this project distrusts; jumping there would mean testing a theory instead of
// locating a fault. The question that has to be answered first is WHICH LAYER goes quiet, and the
// counts separate them cleanly:
//
//   the key never arrives            -> an input-path problem, and nothing about drawing
//   no widgets built                 -> the application is not building a screen at all
//   widgets but no damage/blits      -> it builds one and declines to rasterise it
//   widgets and blits, no surface    -> it rasterises and the content or destination is wrong
//
// Run it BOTH WAYS -- plain, and with --carousel -- and the pair is the measurement. The probe
// reports which state it was actually in rather than trusting the flag, because a run that thinks
// it is broadcasting and is not would produce a matched pair that means nothing.
//
// THE CONTROL IS THE PRESS ITSELF. Raw 0x7D on a settled silent box builds 62 widgets and 11 blits
// and moves the screen -- so if the silent half of the pair does not show that, the instrument is
// wrong and the broadcasting half says nothing. That check is asserted, not eyeballed.
//
// TASK STATES ARE READ TOO, because a task blocked somewhere new is the cheapest possible name for
// what changed. Nucleus status: 0 ready, 2 sleep, 3 mailbox, 4 queue, 5 pipe, 6 semaphore, 7 event.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 180) {
  await new Promise(function(r){ setTimeout(r, 1000); });
  waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

// WHICH STATE IS THIS ACTUALLY IN? Read it off the page, never off the flag that was passed.
var car = window.__siCarousel();
var state = {
  broadcasting: car.running !== false,
  sectionsSent: car.sent === undefined ? 0 : car.sent,
  refused: car.refused === undefined ? 0 : car.refused,
  tables: window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' '),
  settledAfterSeconds: waited
};

function taskCensus(){
  var t = window.__tasks(), by = {};
  t.tasks.forEach(function(x){ by[x.status] = (by[x.status] || 0) + 1; });
  return { n: t.n, byStatus: by,
           // Named individually because "one task moved" is the finding, and a histogram hides it.
           each: t.tasks.map(function(x){ return x.name + ':' + x.status; }).sort().join(' ') };
}

// The input path, from the gate's own assertions: if these do not move, the key never arrived and
// every drawing count below is about a press that did not happen.
var DISPATCH = 0x800297B0, KEYEVENT = 0x8006EA04;
function inputHits(){
  var r = window.__pcHits(DISPATCH, KEYEVENT);
  if (r[h(DISPATCH)] === undefined || r[h(KEYEVENT)] === undefined)
    throw new Error('__pcHits did not return the keys it was asked for -- a harness failure, not a zero');
  return { dispatcher: r[h(DISPATCH)], keyEvents: r[h(KEYEVENT)] };
}

var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget',     args: 1 },
  { pc: 0x80082604, name: 'apply',         args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',        args: 2 },
  { pc: 0x80085128, name: 'setWindowRoot', args: 2 }
];

async function press(raw, label){
  window.__traceCalls(TRACE);
  window.__traceClear();
  var before = { input: inputHits(), blits: window.__blitLog().length, surface: surface(),
                 tasks: taskCensus() };
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  var after = { input: inputHits(), blits: window.__blitLog().slice(before.blits),
                surface: surface(), tasks: taskCensus() };
  window.__traceCalls([]);
  return {
    key: label + ' (raw ' + h(raw).slice(-2) + ')',
    pressedOn: before.surface,
    input: { dispatcher: after.input.dispatcher - before.input.dispatcher,
             keyEvents: after.input.keyEvents - before.input.keyEvents },
    widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
    rebind: c.setWindowRoot || 0,
    blits: after.blits.length,
    blitNotes: after.blits.slice(0, 6).map(function(b){ return b.note || JSON.stringify(b); }),
    surface: after.surface,
    screenMoved: after.surface.hash !== before.surface.hash,
    tasksBefore: before.tasks.each === after.tasks.each ? 'unchanged' : before.tasks.each,
    tasksAfter: before.tasks.each === after.tasks.each ? 'unchanged' : after.tasks.each,
    layer: (after.input.keyEvents - before.input.keyEvents) === 0
             ? 'THE KEY NEVER ARRIVED -- an input-path problem, nothing to do with drawing'
         : (c.newWidget || 0) === 0
             ? 'the key arrived and NO WIDGET WAS BUILT -- the application is not making a screen'
         : after.blits.length === 0
             ? 'widgets were built and NOTHING BLITTED -- it makes a screen and declines to rasterise it'
         : after.surface.hash === before.surface.hash
             ? 'it blitted and the surface did not move -- the content or the destination is wrong'
             : 'it drew'
  };
}

var out = { question: 'which layer goes quiet when a broadcast is running', state: state,
            tasksAtRest: taskCensus() };
out.sky   = await press(0x7D, 'sky');
out.guide = await press(0x80, 'tv guide');

// THE CONTROL, AND IT IS ASSERTED RATHER THAN EYEBALLED. On a silent box raw 0x7D is known to build
// widgets and move the screen; if it does not, this run's instrument is wrong and the broadcasting
// half of the pair would be a reading about the instrument.
out.controlHeld = state.broadcasting ? 'n/a -- this is the broadcasting half of the pair'
                : (out.sky.screenMoved && out.sky.widgets > 0);
if (!state.broadcasting && !out.controlHeld)
  throw new Error('on a SILENT box the sky key built ' + out.sky.widgets + ' widgets and moved the '
                + 'screen: ' + out.sky.screenMoved + '. That is the known-good control failing, so '
                + 'nothing this probe reports can be trusted.');
out.verdict = out.sky.layer;
out.note = 'run this plain AND with --carousel; the pair is the measurement, and `state.broadcasting` '
         + 'says which half this one is rather than trusting the flag';
return out;
