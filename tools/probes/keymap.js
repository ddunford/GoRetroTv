// WHAT DOES EACH HANDSET BUTTON DO? -- sky-02me.2, the input to the rest of sky-02me.
//
// The box now draws its interface, so every other task in that epic depends on knowing which
// buttons it responds to. This presses each one on a settled box and records what changed: whether
// the widget layer rebuilt, whether anything reached the plane, whether the screen moved, and a PNG.
//
// THE BUTTON SET IS DERIVED FROM THE PAGE'S OWN HANDSET, not typed in here. Every button carries a
// data-raw attribute and the panel is a thin shell over remoteKeyRaw, so querying the DOM gets
// exactly the set a person can press -- and a button added later is included automatically. A
// hardcoded list is the enumeration trap this project has already paid for once, where a coverage
// probe named its subjects and silently stopped covering a third of them.
//
// EVERY ROW CARRIES THE SCREEN IT WAS PRESSED ON, and that is not decoration. The first version of
// this probe reported only what each press DID, and the write-up then attributed the arrow keys to
// the Box Office menu -- when the walk had already pressed standby and the TV Guide, so they landed
// on an empty guide where redrawing the same nothing is correct behaviour. Every number was right
// and the screen named in the conclusion was invented. So the `on` field below is taken BEFORE the
// press and travels with the row: a reader cannot attribute a result to a screen nobody verified,
// because the row says which one it was.
//
// STATE ACCUMULATES AND THAT IS DELIBERATE. Rebooting between presses would cost ~40 s each and
// lose the thing worth knowing: the box is a state machine, and what a button does depends on the
// screen it is pressed on. So this walks in order from the settled boot screen and reports a
// SEQUENCE. The first pass is a map of "what happened when pressed here", not "what this button
// means everywhere" -- and the difference is written into the result rather than left to a reader.
//
// CONTROLS. The box must be settled (a press at 38 s builds five widgets instead of 62), so this
// asserts 42 tasks before starting and again at the end -- a run that wedged the machine half way
// through would otherwise report a row of quiet buttons. And the FIRST button pressed is raw 0x7D,
// which is known to draw the menu: if that one shows no change, the instrument is wrong and every
// quiet row after it means nothing.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function safe(t){ return (t || '').replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '').toLowerCase() || 'btn'; }

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');

// The handset, read off the page.
var buttons = Array.from(document.querySelectorAll('[data-raw]')).map(function(el){
  return { raw: parseInt(el.getAttribute('data-raw'), 16),
           label: (el.textContent || '').replace(/\s+/g, ' ').trim() };
});
if (buttons.length < 20)
  throw new Error('only ' + buttons.length + ' handset buttons found in the DOM -- the selector is wrong');

// Put raw 0x7D first: it is the known-good control, and it also gets the box onto the menu so the
// rest of the walk happens somewhere interesting rather than on the boot screen.
buttons.sort(function(a, b){ return (a.raw === 0x7D ? -1 : 0) - (b.raw === 0x7D ? -1 : 0); });

await new Promise(function(r){ setTimeout(r, 80000); });

var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget',     args: 1 },
  { pc: 0x80082604, name: 'apply',         args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',        args: 2 },
  { pc: 0x80085128, name: 'setWindowRoot', args: 2 }
];
var rows = [], screens = {}, prev = surface();
for (var i = 0; i < buttons.length; i++){
  var btn = buttons[i];
  window.__traceCalls(TRACE);
  window.__traceClear();
  var blitsBefore = window.__blitLog().length;
  var on = surface();                       // the screen this press is being made ON
  window.__key(btn.raw, 0);
  await new Promise(function(r){ setTimeout(r, 5000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name]||0)+1; });
  var now = surface();
  var blits = window.__blitLog().slice(blitsBefore);
  if (now.hash !== on.hash) screens[now.hash] = (screens[now.hash] || 0) + 1;
  var row = { code: h(btn.raw).slice(-2), label: btn.label,
              on: { hash: on.hash, colours: on.colours },
              widgets: c.newWidget || 0, applies: c.apply || 0, damage: c.DAMAGE || 0,
              rebind: c.setWindowRoot || 0,
              blits: blits.length, screenMoved: now.hash !== prev.hash,
              colours: now.colours, hash: now.hash };
  rows.push(row);
  if (row.screenMoved || row.blits) await window.__shot('btn-' + row.code + '-' + safe(btn.label));
  prev = now;
}
window.__traceCalls([]);

var control = rows[0];
if (!control || control.code !== '7D' || !control.screenMoved)
  throw new Error('the control button raw 0x7D did not move the screen -- the instrument is wrong, ' +
                  'so no quiet row below means anything. control=' + JSON.stringify(control));

var tasksEnd = window.__tasks().n;
if (tasksEnd < 42) throw new Error('the box lost tasks during the walk (' + tasksEnd + ') -- ' +
                                   'something wedged it and the later rows are suspect');

return {
  buttonsFound: buttons.length,
  control: control,
  reacted: rows.filter(function(r){ return r.screenMoved || r.blits || r.widgets; }),
  quiet:   rows.filter(function(r){ return !r.screenMoved && !r.blits && !r.widgets; })
               .map(function(r){ return r.code + ' ' + r.label; }),
  all: rows,
  distinctScreensReached: Object.keys(screens).length,
  caveat: 'a SEQUENCE from the settled boot screen. Each row carries the screen it was pressed ON ' +
          'in its `on` field -- READ IT before attributing a result to any particular screen. This ' +
          'is not what each button means everywhere; for that, press it from a state you have ' +
          'checked, as scripts/digibox-probes/arrows-on-the-menu.js does.',
  tasks: tasksEnd
};
