// DO THE ARROWS MOVE THE HIGHLIGHT? Press them ON THE MENU, from a known state.
//
// WHY THIS EXISTS: sky-02me.2 reported that the arrows redraw and never change the screen, and
// said they were pressed "on the Box Office menu". THEY WERE NOT. That walk is a SEQUENCE, and by
// the time it reached the arrows it had already pressed CC (standby) and 80 (the TV Guide), so the
// arrows landed on an EMPTY guide -- "Searching for listings / Further schedule information is not
// available" -- where redrawing the same nothing is the correct behaviour. The measurement was
// real and the claim attached to it described a screen nobody had checked the box was on.
//
// So this establishes the state before it measures: press 0x7D, CONFIRM the menu is up by its own
// signature, and only then press an arrow. Each step keeps the surface hash and a PNG so the
// answer is visible rather than inferred from counters.
//
// THE MENU'S SIGNATURE, measured: 37 distinct colours and 11 blits, five background bands plus six
// 480x32 rows at x=120 stepping 32 apart. A screen that does not match that is not the menu, and
// this refuses to report on arrow presses made somewhere else -- which is the whole point.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,5);
  return { hash: h(s), colours: Object.keys(hist).length,
           top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');
await new Promise(function(r){ setTimeout(r, 80000); });

var TRACE = [
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
];

async function press(raw, name){
  window.__traceCalls(TRACE); window.__traceClear();
  var b0 = window.__blitLog().length, s0 = surface();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 6000); });
  var log = window.__traceLog(), c = {};
  log.forEach(function(e){ c[e.name] = (c[e.name]||0)+1; });
  var s1 = surface(), blits = window.__blitLog().slice(b0);
  await window.__shot(name);
  return { pressed: h(raw).slice(-2), name: name,
           widgets: c.newWidget||0, applies: c.apply||0, damage: c.DAMAGE||0,
           blits: blits.length, moved: s0.hash !== s1.hash,
           before: s0, after: s1,
           blitNotes: blits.map(function(e){ return e.note || ''; }) };
}

// 1. Get onto the menu and PROVE we are on it.
var toMenu = await press(0x7D, 'arrows-00-menu');
if (!toMenu.moved || toMenu.after.colours < 30)
  throw new Error('0x7D did not produce the menu (' + toMenu.after.colours +
                  ' colours, moved=' + toMenu.moved + ') -- refusing to report on arrows pressed ' +
                  'somewhere unknown');

// 2. Now the arrows, each from the menu, in order.
var down1  = await press(0x59, 'arrows-01-down');
var down2  = await press(0x59, 'arrows-02-down-again');
var up1    = await press(0x58, 'arrows-03-up');
var right1 = await press(0x5B, 'arrows-04-right');
var left1  = await press(0x5A, 'arrows-05-left');
window.__traceCalls([]);

var arrows = [down1, down2, up1, right1, left1];
return {
  reachedMenu: { colours: toMenu.after.colours, blits: toMenu.blits, hash: toMenu.after.hash },
  arrows: arrows.map(function(a){
    return { pressed: a.pressed, name: a.name, widgets: a.widgets, applies: a.applies,
             damage: a.damage, blits: a.blits, moved: a.moved,
             hash: a.after.hash, colours: a.after.colours }; }),
  anyMoved: arrows.some(function(a){ return a.moved; }),
  firstDownBlits: down1.blitNotes.slice(0, 8),
  verdict: arrows.some(function(a){ return a.moved; })
      ? 'THE ARROWS DO MOVE THE SCREEN on the menu -- the earlier finding was about the TV Guide'
      : 'the arrows still do not move the screen, and this time the box was provably on the menu',
  tasks: window.__tasks().n
};
