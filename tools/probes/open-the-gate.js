// OPEN THE GATE AT ITS SOURCE AND LET THE BOX RUN ITSELF.
//
// THE CHAIN, all measured (sky-eluc.28 / .29):
//   The application asks (3,0x15) whether it may keep its screen. That native is four lines and
//   its DEFAULT return is the word in its own literal pool at 0x80054F84, which is 0xFFFFFFFF:
//
//       int f(void) {
//           int r = *(int*)0x80054F84;                    // -1
//           if (*(char*)0x80161D3C != 0 && *(int*)0x80161D40 != *(int*)0x8010161C)
//               r = *(int*)0x80161D40;
//           return r;
//       }
//
//   The o-code branch at 0x9FC72FC2 is "jump if 0 > r" -- measured, not read off the mnemonic --
//   so -1 clears the screen and any r >= 0 keeps it.
//
// WHY THE DEFAULT IS WHAT THE BOX GETS, and it is worse than a flag simply being unset. The flag
// at 0x80161D3C IS eventually raised, to 1 -- but at icount 165434409, which is 592,817
// instructions AFTER the decision was taken at 164841592. The box asks before the answer exists.
// And even afterwards the conditional still fails, because it also needs
// *0x80161D40 != *0x8010161C and both settle at 0. So the native returns -1 for the life of the
// machine and the screen can never survive.
//
// SO THIS POKES ONE WORD: the pool constant, from 0xFFFFFFFF to 0. It does not patch the branch,
// does not touch the widget tree, does not force a bind and does not skip the unbind -- the
// application runs its own code and makes its own decision, with the one value it reads changed
// from "no" to "yes". If the box then keeps its screen unaided, the whole chain is confirmed and
// this is a working lever. If it does not, something else also gates the draw and the chain is
// incomplete.
//
// THE CONTROL IS THE POKE ITSELF. The word is read back after writing, because a poke that did
// not land and a gate that did not matter produce the same blue screen.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,6);
  return { hash: h(s), distinctColours: Object.keys(hist).length,
           top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}

var POOL = 0x80054F84;
window.__profile(true);
var was = b32(POOL);
if (was !== 0xFFFFFFFF)
  throw new Error('the pool constant is ' + h(was) + ', not 0xFFFFFFFF -- wrong address or wrong build');
poke32(POOL, 0);
var now = b32(POOL);
if (now !== 0) throw new Error('the poke did not land: the word still reads ' + h(now));

var base = b32(0x80105E9C), REC1 = base + 100;
window.__traceCalls([
  { pc: 0x80085128, name: 'setWindowRoot', args: 2 },
  { pc: 0x80083830, name: 'DAMAGE',        args: 2 },
  { pc: 0x80082604, name: 'apply',         args: 1 },
  { pc: 0x80082A6C, name: 'newWidget',     args: 1 }
]);

var before = { blits: window.__blitLog().length, surface: surface(),
               ring: [b32(REC1+0x50), b32(REC1+0x54)], rootObj: h(b32(REC1+0x60)) };
await new Promise(function(r){ setTimeout(r, 80000); });
var settleLog = window.__traceLog(), settleCounts = {};
settleLog.forEach(function(e){ settleCounts[e.name] = (settleCounts[e.name]||0)+1; });
var afterSettle = { blits: window.__blitLog().length, surface: surface(),
                    ring: [b32(REC1+0x50), b32(REC1+0x54)], rootObj: h(b32(REC1+0x60)) };
await window.__shot('gate-open-no-key');

window.__traceClear();
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });
var keyLog = window.__traceLog(), keyCounts = {};
keyLog.forEach(function(e){ keyCounts[e.name] = (keyCounts[e.name]||0)+1; });
await window.__shot('gate-open-after-key');

var newBlits = window.__blitLog().slice(afterSettle.blits);
return {
  poked: { at: h(POOL), was: h(was), now: h(now) },
  settle: { counts: settleCounts,
            setWindowRoot: settleLog.filter(function(e){ return e.name==='setWindowRoot'; })
                                    .map(function(e){ return e.icount + '  ' + e.a.join(','); }),
            state: afterSettle },
  press:  { counts: keyCounts,
            damageWindows: [...new Set(keyLog.filter(function(e){return e.name==='DAMAGE';})
                                             .map(function(e){ return e.a[1]; }))],
            ring: [b32(REC1+0x50), b32(REC1+0x54)], rootObj: h(b32(REC1+0x60)),
            newBlits: newBlits.length,
            blitNotes: newBlits.slice(0, 24).map(function(e){ return e.note || ''; }) },
  surfaceBefore: before.surface, surfaceAfter: surface(),
  tasks: window.__tasks().n
};
