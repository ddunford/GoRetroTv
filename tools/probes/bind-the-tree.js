// BIND THE WIDGET TREE TO THE VISIBLE PLANE BY HAND, and see whether it paints.
//
// THE CLAIM UNDER TEST. The only thing between the key press's widget tree and the 8 bpp plane is
// that nothing ever performs the binding operation, which the firmware has and names:
//
//   0x80085128  (1,0xE3) setWindowRoot(windowId, obj)
//        windowArray[windowId] + 0x60  = obj        // the window's ROOT OBJECT
//        setGateRecursive  (obj, 1)                 // ctx+0x0C = 1  on the whole subtree
//        setWindowRecursive(obj, windowId)          // ctx+0x28 = id on the whole subtree
//
// Measured 2026-09-14 by scripts/digibox-probes/who-binds-the-window.js: setWindowRoot NEVER RUNS,
// idle or on a key press, and +0x60 reads 0x00000000 on BOTH windows before and after a press. So
// no widget tree is the root of any plane, which is why apply() passes 0xFFFFFFFF or 0 as the
// window and damage() draws nothing.
//
// THE RESOLVER IS SUBTRACTION. 0x80085004 is `ctx = obj - 0x2C`, bounds-checked against the heap
// descriptor at 0x80147270. Proven against two independent captures: the gate break recorded
// ctx = 0x8043073C and the addChild trace recorded obj = 0x80430768, and 0x80430768 - 0x2C is
// exactly 0x8043073C. So this probe can resolve handles in JS without calling into the firmware --
// which matters, because setWindowRoot tail-jumps through a jump table and __call cannot complete
// such a function (it loses the harness's sentinel return address, as (1,0xE4) already showed).
//
//   ctx+0x0C  gate      ctx+0x14  parent    ctx+0x18  next sibling
//   ctx+0x1C  first child         ctx+0x28  window id
//
// So the binding is performed here as POKES -- the same writes setWindowRoot would make, walked
// over the same tree. If the plane's ring fills and the framebuffer changes, the diagnosis is
// proven and the remaining question is why the APPLICATION never calls (1,0xE3). If nothing moves,
// the diagnosis is wrong and this file's own rule applies: go and look, do not try a bigger poke.
//
// THE CLIP IS NOT THE OBSTACLE, and the earlier worry that it was rests on a misread. Window 1's
// rec+0x24 reads 0x00000000 and rec+0x28 reads 0x02D00240 -- those are PACKED HALFWORD PAIRS,
// (0,0) and (720,576). The clip is the whole screen.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }
function ctxOf(obj){ return (obj - 0x2C)>>>0; }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,4);
  return { hash: h(s), top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}

window.__profile(true);
var base = b32(0x80105E9C);
if (!base) throw new Error('window array pointer read as 0 -- probe starved or box not booted');
function rec(i){
  var r = base + i*100;
  return { win:i, kind:b32(r), produce:b32(r+0x50), consume:b32(r+0x54),
           bg:h(b32(r+0x5C)), rootObj:h(b32(r+0x60)),
           clipXY:h(b32(r+0x24)), clipWH:h(b32(r+0x28)) };
}

// ---- 1. watch the press build its tree -------------------------------------------------------
// SETTLE FIRST. A press 38 s after boot built FIVE widgets and added no children; the session that
// measured 62 widgets and 490 applies had waited 80 s. The box is still finishing its own startup
// draw at 38 s -- window 1's background reads 0x00000000 then and 0xDCDCDCDC afterwards -- so a
// press before it settles measures a different machine. Wait, then press.
window.__traceCalls([
  { pc: 0x80082A6C, name: 'newWidget', args: 1 },
  { pc: 0x80082604, name: 'apply',     args: 1 },
  { pc: 0x80085D10, name: 'addChild',  args: 2 },
  { pc: 0x80085D84, name: 'addChild2', args: 2 },
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 },
  { pc: 0x80085128, name: 'setWindowRoot', args: 2 }
]);
await new Promise(function(r){ setTimeout(r, 80000); });
var settled = { windows: [rec(0), rec(1)], counts0: (function(){
  var c = {}; window.__traceLog().forEach(function(e){ c[e.name] = (c[e.name]||0)+1; }); return c; })() };
window.__traceClear();
var before = { windows: [rec(0), rec(1)], blits: window.__blitLog().length, surface: surface() };
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 20000); });
var log = window.__traceLog();
var counts = {}; log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });

// THE OBJECT SET COMES FROM addChild ALONE, and that is a correction rather than a preference.
// (1,0x57) newWidget takes a CLASS -- an integer <= 6 -- so feeding its ARGUMENT into a set of
// objects and resolving it produces a reading of memory at (class - 0x2C), which duly reported a
// "root" of 0x02020000. An argument is not a return value. addChild's two arguments are both
// genuine handles, so the forest it describes is sound.
var newWidgetArgs = {};
log.forEach(function(e){ if (e.name === 'newWidget') newWidgetArgs[e.a[0]>>>0] = 1; });
var seen = {}, childOf = {};
log.forEach(function(e){
  if (e.name === 'addChild' || e.name === 'addChild2'){
    seen[e.a[0]>>>0] = true; seen[e.a[1]>>>0] = true; childOf[e.a[1]>>>0] = e.a[0]>>>0;
  }
});
var objs = Object.keys(seen).map(Number);

// ---- 2. climb to the roots --------------------------------------------------------------------
function rootOf(o){
  var cur = o;
  for (var i = 0; i < 64; i++){
    var p = b32(ctxOf(cur) + 0x14);
    if (!p || p === cur) return cur;
    cur = p;
  }
  return 0;   // cycle or runaway -- report it rather than guess
}
var roots = {};
objs.forEach(function(o){ var r = rootOf(o); roots[r] = (roots[r]||0)+1; });
// A second, independent reading of the same forest: a node nobody ever passed as a CHILD is a
// root, by the trace alone, with no memory read involved. If the two disagree the pointer walk is
// wrong and the disagreement is the finding.
var rootsByTrace = objs.filter(function(o){ return !childOf[o]; }).map(h);
var rootList = Object.keys(roots).map(Number).sort(function(a,b){ return roots[b]-roots[a]; });

// ---- 3. walk one root's subtree ---------------------------------------------------------------
function subtree(root){
  var out = [], stack = [root], seenN = {};
  while (stack.length && out.length < 2000){
    var n = stack.pop();
    if (!n || seenN[n]) continue;
    seenN[n] = true; out.push(n);
    var c = b32(ctxOf(n) + 0x1C);
    var guard = 0;
    while (c && guard++ < 500){ stack.push(c); c = b32(ctxOf(c) + 0x18); }
  }
  return out;
}

var target = rootList[0];
var nodes = target ? subtree(target) : [];
var fieldsBefore = nodes.slice(0, 6).map(function(n){
  return { obj:h(n), gate:h(b32(ctxOf(n)+0x0C)), win:h(b32(ctxOf(n)+0x28)) };
});

// ---- 4. perform the binding ------------------------------------------------------------------
poke32(base + 1*100 + 0x60, target);
nodes.forEach(function(n){ poke32(ctxOf(n) + 0x0C, 1); poke32(ctxOf(n) + 0x28, 1); });

// ---- 5. make it redraw and look ---------------------------------------------------------------
window.__traceClear();
window.__key(0x0C, 0);
await new Promise(function(r){ setTimeout(r, 15000); });
var log2 = window.__traceLog(), counts2 = {};
log2.forEach(function(e){ counts2[e.name] = (counts2[e.name]||0)+1; });
await window.__shot('bound-tree');

return {
  settled: settled,
  newWidgetArgs: Object.keys(newWidgetArgs).map(function(k){ return h(+k); }),
  rootsByTrace: rootsByTrace,
  press1: { counts: counts, objects: objs.length,
            addChild: log.filter(function(e){return e.name.indexOf('addChild')===0;}).slice(0,10)
                         .map(function(e){ return h(e.a[0])+' <- '+h(e.a[1]); }) },
  roots: rootList.slice(0,6).map(function(r){ return { root:h(r), reachedBy:roots[r] }; }),
  bound: { root: h(target), subtreeNodes: nodes.length, fieldsBefore: fieldsBefore },
  press2: { counts: counts2,
            damageArgs: log2.filter(function(e){return e.name==='DAMAGE';}).slice(0,12)
                            .map(function(e){ return e.a.map(h).join(','); }) },
  before: before,
  after: { windows: [rec(0), rec(1)], blits: window.__blitLog().length, surface: surface() },
  newBlits: window.__blitLog().slice(before.blits).slice(0,12).map(function(e){ return e.note || ''; }),
  tasks: window.__tasks().n
};
