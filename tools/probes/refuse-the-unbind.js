// IS THE BINDING SUFFICIENT? Let the application bind its screen and REFUSE ITS UNBIND.
//
// WHY THIS AND NOT THE EARLIER POKE. bind-the-tree.js bound the tree by hand AFTER the key press
// had built it, and it proved the routing: DAMAGE went 0 -> 48 calls carrying window 1, the ring
// advanced, five real fills landed. What it did NOT produce was a menu -- every one of those fills
// was the background colour, the surface hash did not move, and the screenshot is plain blue. That
// leaves the real question open, because a tree bound after the fact is not the same machine as a
// tree that was bound the whole time: whatever a widget emits when its geometry is set, it emitted
// while detached.
//
// So this binds nothing by hand. It lets the application do it -- it already does, correctly -- and
// stops it undoing it:
//
//     164838860  setWindowRoot(1, 0x80430A14)    the bind, left alone
//     164841592  setWindowRoot(1, 0x00000000)    the unbind, neutered here
//
// HOW, without a register-write API. setWindowRoot's whole body is behind `if (obj != rec[+0x60])`.
// At the unbind, obj is 0 and rec[+0x60] is the tree root, so the guard passes. Poking rec[+0x60]
// to 0 at the breakpoint makes it `0 != 0` -- false -- and the function returns having touched
// nothing. The field is then put back, so the window still names its root for anything else that
// reads it. Two pokes, no register writes, no patched firmware.
//
// WHAT EACH OUTCOME MEANS, decided before the run rather than after:
//   pixels that are not the background colour   -> the binding was the whole defect
//   damage and fills but still a flat blue page -> binding is necessary and NOT sufficient, and
//                                                  the next question is what a widget draws WITH
//   no damage at all                            -> the refusal did not take; check the fields
// The third is the control: the root's gate and window are read back before the key press, and if
// they are not 1 and 1 this probe has not tested what it claims to.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }
function ctxOf(o){ return (o - 0x2C)>>>0; }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,6);
  return { hash: h(s), distinctColours: Object.keys(hist).length,
           top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}

window.__profile(true);
var base = b32(0x80105E9C);
if (!base) throw new Error('window array pointer read as 0 -- probe starved or box not booted');
var REC1 = base + 1*100, SET_WINDOW_ROOT = 0x80085128;

async function stall(limitMs){
  var t0 = Date.now();
  while (Date.now() - t0 < limitMs){
    await new Promise(function(r){ setTimeout(r, 4); });
    var g = window.__regs();
    if ((parseInt(g.pc,16)>>>0) === SET_WINDOW_ROOT) return g;
  }
  return null;
}

window.__traceCalls([
  { pc: 0x80083830, name: 'DAMAGE',   args: 2 },
  { pc: 0x80082604, name: 'apply',    args: 1 },
  { pc: 0x80082A6C, name: 'newWidget',args: 1 }
]);
window.__breakAt([SET_WINDOW_ROOT]);

var g1 = await stall(90000);
if (!g1) throw new Error('never stalled at the bind -- nothing tested');
var bind = [parseInt(g1.a0,16)>>>0, parseInt(g1.a1,16)>>>0];
window.__resume();

var g2 = await stall(90000);
if (!g2) throw new Error('never stalled at the unbind -- nothing tested');
var unbind = [parseInt(g2.a0,16)>>>0, parseInt(g2.a1,16)>>>0];
var root = b32(REC1 + 0x60);
if (unbind[1] !== 0 || root === 0)
  throw new Error('second stall is not the unbind (a1=' + h(unbind[1]) + ', root=' + h(root) + ')');

poke32(REC1 + 0x60, 0);        // make the guard false: the call becomes a no-op
window.__resume();
await new Promise(function(r){ setTimeout(r, 500); });
poke32(REC1 + 0x60, root);     // and put the window's root back
window.__breakAt([]);
window.__resume();

// Settle, then check the CONTROL before pressing anything.
await new Promise(function(r){ setTimeout(r, 80000); });
var stillBound = { root: h(root), gate: h(b32(ctxOf(root)+0x0C)), win: h(b32(ctxOf(root)+0x28)) };
window.__traceClear();
var before = { blits: window.__blitLog().length, surface: surface(),
               produce: b32(REC1+0x50), consume: b32(REC1+0x54) };

window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });
var log = window.__traceLog(), counts = {};
log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });
await window.__shot('unbind-refused');

var newBlits = window.__blitLog().slice(before.blits);
return {
  bind: bind.map(h).join(','), unbind: unbind.map(h).join(','),
  control_rootStillBound: stillBound,
  press: { counts: counts,
           damageWindows: [...new Set(log.filter(function(e){return e.name==='DAMAGE';})
                                         .map(function(e){ return e.a[1]; }))] },
  ring: { before: [before.produce, before.consume],
          after: [b32(REC1+0x50), b32(REC1+0x54)] },
  blits: { before: before.blits, new: newBlits.length,
           notes: newBlits.slice(0, 20).map(function(e){ return e.note || ''; }),
           nonFill: newBlits.filter(function(e){ return (e.note||'').indexOf('fill') !== 0; }).length },
  surfaceBefore: before.surface, surfaceAfter: surface(),
  tasks: window.__tasks().n
};
