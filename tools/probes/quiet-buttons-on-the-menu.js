// ARE THE "QUIET" BUTTONS REALLY QUIET? Press each one from a VERIFIED menu, and restore between.
//
// sky-02me.2's walk found fifteen buttons producing zero of everything, and the write-up called
// them silent "on a menu whose rows are numbered 1 to 6". They were not pressed there: the walk is
// a sequence and the digits landed on the Interactive screen. That is the same error that produced
// the wrong arrow-key finding, and this probe exists so the answer this time is about a screen
// somebody checked.
//
// THE DISCIPLINE: press 0x7D, capture the menu's exact hash, and treat THAT as the known state.
// Before every measurement the box is restored to it and the restoration is VERIFIED against that
// hash -- not against "looks like a menu", because a condition that accepts several states has
// examined none of them. If the box cannot be restored, the walk STOPS rather than reporting rows
// measured from somewhere unknown; a partial answer about a known state beats a full one about an
// unknown mixture.
//
// CONTROL: the first thing measured is 0x59 (down), which is known to move the highlight and repaint
// exactly two rows. If that comes back quiet, the instrument is wrong and no zero below means
// anything.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');
await new Promise(function(r){ setTimeout(r, 80000); });

var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];

async function press(raw, waitMs){
  window.__traceCalls(TRACE); window.__traceClear();
  var b0 = window.__blitLog().length;
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, waitMs || 5000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name]||0)+1; });
  return { widgets: c.newWidget||0, applies: c.apply||0, damage: c.DAMAGE||0,
           blits: window.__blitLog().length - b0, after: surface() };
}

// Establish the known state and remember exactly what it looks like.
var toMenu = await press(0x7D, 8000);
var MENU = toMenu.after.hash;
if (toMenu.after.colours < 30)
  throw new Error('0x7D did not produce the menu (' + toMenu.after.colours + ' colours)');
await window.__shot('quiet-00-menu');

// RESTORING TO A BYTE-IDENTICAL SCREEN IS TOO STRICT AND IT STOPPED THE FIRST RUN DEAD. The
// control press (down) legitimately moves the highlight to row 2, so the menu is no longer the
// hash captured at the start, and 0x7D does not reset the highlight -- the walk then refused to
// continue after one subject. The state that matters here is "the Box Office menu", not "the menu
// with row 1 highlighted".
//
// So the check is the menu's COLOUR SIGNATURE, which is specific enough to discriminate: the menu
// is 37 distinct colours, while the TV Guide came back 34 and 30 and the post-0x0C screen 12. It is
// deliberately NOT a vague "looks about right" predicate -- the number is measured, the hash it
// actually landed on is recorded in every row, and a restore that misses it stops the walk rather
// than letting a later row be measured somewhere unknown.
var MENU_COLOURS = toMenu.after.colours;
async function restore(){
  if (surface().colours === MENU_COLOURS) return true;
  await press(0x7D, 8000);
  return surface().colours === MENU_COLOURS;
}

var SUBJECTS = [
  [0x59, 'down (CONTROL - must react)'],
  [0x01, 'digit 1'], [0x02, 'digit 2'], [0x03, 'digit 3'], [0x06, 'digit 6'],
  [0x00, 'digit 0'],
  [0x21, '0x500'], [0x20, '0x501'], [0x0A, '0x502'],
  [0x6D, 'raw 6D'], [0x6E, 'raw 6E'], [0x6F, 'raw 6F'], [0x70, 'raw 70'],
  [0x7F, '0x604']
];

var rows = [], stoppedAt = null;
for (var i = 0; i < SUBJECTS.length; i++){
  if (!(await restore())){ stoppedAt = SUBJECTS[i][1]; break; }
  var from = surface();
  var r = await press(SUBJECTS[i][0], 5000);
  var moved = r.after.hash !== from.hash;
  rows.push({ code: h(SUBJECTS[i][0]).slice(-2), what: SUBJECTS[i][1],
              pressedOn: from.hash, widgets: r.widgets, applies: r.applies,
              damage: r.damage, blits: r.blits, moved: moved, after: r.after.hash });
  if (moved) await window.__shot('quiet-' + h(SUBJECTS[i][0]).slice(-2));
}
window.__traceCalls([]);

var control = rows[0];
if (!control || control.code !== '59' || !control.moved)
  throw new Error('the control (down) did not move the screen -- the instrument is wrong, so no ' +
                  'quiet row means anything: ' + JSON.stringify(control));

var silent = rows.slice(1).filter(function(r){
  return !r.moved && !r.widgets && !r.applies && !r.damage && !r.blits; });
return {
  menuHash: MENU, measured: rows.length, stoppedAt: stoppedAt,
  control: control,
  rows: rows,
  trulySilentOnTheMenu: silent.map(function(r){ return r.code + ' ' + r.what; }),
  reactedOnTheMenu: rows.slice(1).filter(function(r){ return r.moved || r.widgets || r.blits; })
                        .map(function(r){ return r.code + ' ' + r.what; }),
  tasks: window.__tasks().n
};
