// sky-02me.18 -- THE O-CODE TRACE. Is the EPG's BYTECODE even running on a dead press?
//
// Eight native-level hypotheses are dead (docs/reference/digibox-emulation.md). Every one of them
// diffed MIPS. The EPG is an OpenTV BYTECODE application, so "build no screen" is far more likely
// a decision in o-code than in the native code every instrument so far has watched -- which is the
// single best explanation for why all eight missed.
//
// __readWatch over the EPG's CODE chunk, PC-filtered to the interpreter's MAIN FETCH SITE
// (0x80069298, which reads OPCODE bytes while every other site reads operands), logs the
// application's own program counter. This traces the same presses on a healthy box and on one
// that has parsed a BAT, and compares them.
//
// THE FIRST QUESTION IS BINARY AND THE ANSWER DECIDES EVERYTHING AFTER IT:
//   * dead press executes ~0 o-code   -> the interpreter is never entered. The fault is NATIVE,
//                                        in whatever delivers the key event to the VM.
//   * dead press executes o-code      -> the application is running and CHOOSING not to draw, and
//                                        the divergence point in the two streams IS the decision.
//
// THE CAP IS RAISED ON PURPOSE AND THAT IS NOT A DETAIL. A sky press executes 34,067 bytecode
// instructions and a tv guide press 40,000-50,000 -- measured on this box, first run of this
// probe. The watch's default 4,000 keeps a TENTH of that, and a diff over two truncated openings
// would report "identical" for two presses that part company later. {max} is passed explicitly and
// `capped` is asserted on, because a capped trace and a short one look the same in the output.
//
// THE FEED ORDER IS THE WHOLE REPRODUCTION AND THE FIRST RUN OF THIS PROBE GOT IT WRONG. Pushing
// NIT then BAT back to back does NOT reproduce anything: delivery is by PID, the box is not
// subscribed to 0x4A until it has PARSED the NIT, and a BAT that arrives before that is dropped
// unread -- indistinguishable, from outside, from a BAT that was harmless. That run reported both
// presses drawing 62 widgets and would have read as "the o-code is identical, the fault is
// elsewhere". It was caught by the match table in its own output: no 0x4A had opened yet.
// So the NIT gets 8 seconds and the subscription is ASSERTED before the BAT is sent.
//
// AND THE REPRO IS ASSERTED, NOT ASSUMED. If the fed press still draws, this probe says so and
// concludes nothing: a trace of a box that never went wrong is a trace of nothing.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
var OCODE_LO = 0x9FC4A400, OCODE_LEN = 353188;
var MAIN_FETCH = 0x80069298, MAIN_FETCH_S = '0x80069298';
var TRACE_MAX = 120000;
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
  return { dispatcher: r[h(DISPATCH)], keyEvents: r[h(KEYEVENT)] };
}
function tablesWanted(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
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
async function press(raw, label, tag, from){
  assertRedrawable(raw, tag);
  window.__traceCalls(TRACE); window.__traceClear();
  var b = { input: inputHits(), blits: window.__blitLog().length, surface: surface() };
  var w = window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN,
                             { fromPc: [MAIN_FETCH, MAIN_FETCH + 2], max: TRACE_MAX });
  if (w.max !== TRACE_MAX)
    throw new Error('this page\'s __readWatch does not take {max} -- the trace would silently cap '
                  + 'at ' + w.max + ' and a capped diff reports "identical" for streams that part '
                  + 'later. Reload the page from the current source.');
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var lg = window.__readWatchLog();
  window.__readWatch();
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name] || 0) + 1; });
  window.__traceCalls([]);
  var a = { input: inputHits(), blits: window.__blitLog().length, surface: surface() };

  // byPc is counted OUTSIDE the cap, so this total is exact however long the press ran -- and it
  // is what tells a truncated trace from a short one.
  var mainTotal = null;
  (lg.byPc || []).forEach(function(s){
    var p = s.split(' x'); if (p[0] === MAIN_FETCH_S) mainTotal = parseInt(p[1], 10);
  });
  var stream = lg.all.map(function(e){ return e.at; });
  return {
    tag: tag, key: label, pressedFrom: from,
    keyEvents: a.input.keyEvents - b.input.keyEvents,
    widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
    blits: a.blits - b.blits,
    moved: a.surface.hash !== b.surface.hash, surface: a.surface,
    drew: (c.newWidget || 0) > 0,
    ocode: { instructions: mainTotal, logged: stream.length, capped: !!lg.capped, cap: lg.max },
    stream: stream.join(' ')
  };
}

// ---- state assertions: a reading from a box in the wrong state is a reading about nothing ------
var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
if (window.__siCarousel().running !== false)
  throw new Error('the carousel is RUNNING -- this probe feeds by hand and would be measuring the thing it isolates');
window.__profile(true);

var out = { question: 'does the EPG bytecode run on a dead press, and where do the two streams part',
            settledAfterSeconds: waited, tablesAtStart: tablesWanted(), presses: [] };
function add(p){ out.presses.push(p); return p; }

// ---- the control. Alternating keys, twice round, so the fed presses have a partner made from
// the SAME starting screen: control-guide-2 is pressed from the menu, and so is fed-guide.
add(await press(0x7D, 'sky',      'control-sky-1',   'the boot screen'));
add(await press(0x80, 'tv guide', 'control-guide-1', 'the menu'));
add(await press(0x7D, 'sky',      'control-sky-2',   'the guide'));
add(await press(0x80, 'tv guide', 'control-guide-2', 'the menu'));
if (!out.presses.every(function(p){ return p.drew; }))
  throw new Error('a CONTROL press did not draw before anything was fed -- every later quiet '
                + 'reading would be meaningless. ' + JSON.stringify(out.presses.map(function(p){
                    return p.tag + ':' + p.widgets; })));
if (out.presses.some(function(p){ return p.ocode.instructions === null; }))
  throw new Error('the main fetch site ' + MAIN_FETCH_S + ' is not in byPc on a press that DREW -- '
                + 'the watch is not seeing the interpreter, so every o-code number here is a '
                + 'harness failure rather than a count');
out.controlCapped = out.presses.filter(function(p){ return p.ocode.capped; }).map(function(p){ return p.tag; });

// ---- the NIT, which OPENS the subscription and must not itself be the culprit -------------------
out.nitPushes = [window.__siNIT(undefined, { version: 11 }), window.__siNIT(undefined, { version: 12 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 8000); });
out.tablesAfterNit = tablesWanted();
if (!window.__siMatches().some(function(x){ return x.tableId === 0x4A; }))
  throw new Error('the box did not subscribe to table 0x4A after the NIT (' + out.tablesAfterNit
                + ') -- a BAT sent now would be DROPPED unread and every later zero would be about '
                + 'delivery rather than about parsing. That is exactly how the first run of this '
                + 'probe measured a box that never went wrong.');
// The intermediate control, which also parks the box on the MENU for the comparison below.
add(await press(0x7D, 'sky', 'after-nit-sky', 'the guide'));
if (!out.presses[4].drew)
  throw new Error('THE NIT ALONE STOPPED THE DRAWING -- which contradicts the bisect, where two '
                + 'more stages drew after it. The NIT is the subject, not the BAT.');

// ---- the BAT, plain. This is the minimal repro ---------------------------------------------------
window.__eeTxClear();
out.batPushes = [window.__siBAT(undefined, { version: 11 }), window.__siBAT(undefined, { version: 12 })]
  .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 9000); });
out.tablesAfterBat = tablesWanted();
out.batWasParsed = out.tablesAfterBat !== out.tablesAfterNit;

// Pressed from the MENU, exactly as control-guide-2 was.
add(await press(0x80, 'tv guide', 'fed-guide', 'the menu'));
add(await press(0x7D, 'sky',      'fed-sky',   out.presses[5].drew ? 'the guide' : 'the menu (the guide press drew nothing)'));
out.eepromTransactionsDuringFeed = window.__eeTx().length;

var fedGuide = out.presses[5], fedSky = out.presses[6];
out.reproduced = !fedGuide.drew && !fedSky.drew;

// ---- and the state AFTER the append loop has finished, which is the one that outlives it --------
if (out.reproduced) {
  var quiet = [], settleS = 0;
  window.__eeTxClear();
  for (var w2 = 0; w2 < 48; w2++) {
    await new Promise(function(r){ setTimeout(r, 5000); });
    settleS += 5;
    quiet.push(window.__eeTxClear());
    if (quiet.length >= 3 && quiet.slice(-3).every(function(x){ return x === 0; })) break;
  }
  out.eepromPerFiveSeconds = quiet;
  out.quiescentAfterSeconds = settleS;
  add(await press(0x80, 'tv guide', 'quiescent-guide', 'the menu'));
}

out.tasksAtEnd = window.__tasks().n;
if (out.tasksAtEnd < 42)
  out.caveat = 'the box lost tasks during the walk (' + out.tasksAtEnd + ') -- the fed readings are suspect';

// ---- the comparison, stated as numbers rather than as a verdict ---------------------------------
function cmp(a, b){
  var A = a.stream ? a.stream.split(' ') : [];
  var B = b.stream ? b.stream.split(' ') : [];
  var i = 0; while (i < A.length && i < B.length && A[i] === B[i]) i++;
  var r = { control: a.tag, fed: b.tag,
            controlInstructions: a.ocode.instructions, fedInstructions: b.ocode.instructions,
            controlLogged: A.length, fedLogged: B.length,
            controlCapped: a.ocode.capped, fedCapped: b.ocode.capped,
            controlWidgets: a.widgets, fedWidgets: b.widgets,
            commonPrefix: i };
  if (i < A.length && i < B.length) {
    r.divergeAfter = i;
    r.divergeControl = A[i];
    r.divergeFed = B[i];
    r.lastCommon = A.slice(Math.max(0, i - 10), i).join(' ');
    r.controlNext = A.slice(i, i + 10).join(' ');
    r.fedNext = B.slice(i, i + 10).join(' ');
  } else if (A.length === B.length) {
    r.divergeAfter = 'the logged streams are identical'
                   + ((a.ocode.capped || b.ocode.capped) ? ' UP TO THE CAP -- which is not the same as identical' : '');
  } else {
    r.divergeAfter = 'one stream simply ENDS at ' + i + ' -- shorter, not different';
  }
  return r;
}
out.compare = [cmp(out.presses[3], fedGuide), cmp(out.presses[2], fedSky)];
if (out.presses[7]) out.compare.push(cmp(out.presses[3], out.presses[7]));

out.headline = out.reproduced
  ? ('REPRODUCED. control tv guide ran ' + out.presses[3].ocode.instructions
     + ' o-code instructions and built ' + out.presses[3].widgets + ' widgets; the fed tv guide press ran '
     + fedGuide.ocode.instructions + ' and built ' + fedGuide.widgets)
  : ('NOT REPRODUCED -- the fed presses still drew (' + fedGuide.widgets + ' / ' + fedSky.widgets
     + ' widgets), so every trace below is of a box that never went wrong and NOTHING here is '
     + 'evidence about the fault. Tables after the BAT: ' + out.tablesAfterBat);
return out;
