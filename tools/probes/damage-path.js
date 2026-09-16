// WHERE THE KEY PRESS'S DRAWING INTENT DIES, decomposed into the three gates it has to pass.
//
// Measured already: a press builds 62 widgets (class 1 x40, class 0 x18, class 3 x4), sets 26
// attributes through (1,0xCB), runs the apply 490 times, and drains WINDOW 1 twice -- the correct
// live 8bpp plane holding the blue. And window 1's produce index never moves off 1. So the intent
// dies somewhere between "attribute changed" and "command in the ring".
//
// THE CHAIN, from the decompilation:
//   0x80082604  apply(obj):  if (obj[2] != 0 && gate(obj) != 0) {
//                                win = ctxOf(obj[0])[0x28];
//                                damage(obj+0x16, win);            <- 0x80083830
//                                if (rec[win][4] == 2 && rec[win][0x38] != -1)
//                                    damage(obj+0x16, rec[win][0x38]);
//                            }
//   0x800825F4  gate(obj) -> 0x80085884(obj[0]) -> tail call
//   0x80083830  damage(rect, win): intersect against rec[win]+0x24, align to the bit depth, enqueue
//
// The && SHORT-CIRCUITS, so the counts decompose it with no ambiguity:
//   gate calls  <  apply calls   =>  obj[2] == 0 for the difference
//   damage      <  gate calls    =>  the gate returned 0
//   damage > 0 and produce still 0  =>  the enqueue itself is declining, and the lead moves inside
//                                       0x80083830 (its rect intersection is the first thing to
//                                       suspect: an empty intersection damages nothing)
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
window.__profile(true);
var base = b32(0x80105E9C);
function ring(i){ var r=base+i*100; return { win:i, produce:b32(r+0x50), consume:b32(r+0x54),
                                             clipAt:h(b32(r+0x24)), kind:b32(r), link:h(b32(r+0x38)) }; }

window.__traceCalls([
  { pc: 0x80082604, name: 'apply',      args: 1 },
  { pc: 0x800825F4, name: 'gate',       args: 1 },
  { pc: 0x80085884, name: 'gateInner',  args: 1 },
  { pc: 0x80083830, name: 'DAMAGE',     args: 2 },
  { pc: 0x80083644, name: 'execOneCmd', args: 1 },
  { pc: 0x800837A0, name: 'drain',      args: 1 }
]);
// Past the blue fill, so the press is measured on a settled box.
await new Promise(r=>setTimeout(r,80000));
window.__traceClear();
var before = [ring(0), ring(1)];
window.__key(0x7D, 0);
await new Promise(r=>setTimeout(r,8000));

var log = window.__traceLog(), counts = {};
log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });
return { counts: counts,
         damageArgs: log.filter(function(e){return e.name==='DAMAGE';}).slice(0,8)
                        .map(function(e){ return e.a.join(','); }),
         before: before, after: [ring(0), ring(1)],
         blits: window.__blitLog().length, traceN: log.length, tasks: window.__tasks().n };
