// FORCE THE MEMBER AND LET THE BOX RUN: the end-to-end test for the whole chain.
//
// THE CHAIN, every link measured (sky-eluc.28 -> .31):
//   startup   (3,0x15) returns -1 -> o-code jgt at 0x9FC72FC2 -> setWindowRoot(win, 0)
//   key path  obj+0x14 reads 0    -> o-code jnz at 0x9FC730E8 -> setWindowRoot(win, 0)
// where obj is DS[0x2E114], the widget tree root, and the application ZEROES +0x14 three
// instructions before it binds (0x9FC72FA2), so the member is a "has anything filled this in yet?"
// flag that nothing fills.
//
// Poking the (3,0x15) pool constant already defeats the first clear. This adds the second: after
// the settle, with the root bound and DS[0x2E114] pointing at it, write 1 into obj+0x14 and press
// a key. Nothing else is touched -- no patched branch, no forced bind, no neutered clear. The
// application runs its own code and answers its own two questions with the two values changed.
//
// WHY 1 IS SAFE TO WRITE. The arm the jnz skips to does not dereference +0x14: 0x9FC730F5 pushes
// *(obj) -- the object's first word -- and passes THAT to (1,0x5C). +0x14 is tested for non-zero
// and not otherwise used on this path, so a 1 exercises the branch without being followed as a
// pointer. If that turns out to be wrong the symptom will be a wedged machine, and the task count
// at the end is what says so.
//
// OUTCOMES, chosen before the run:
//   a menu on screen              -> the chain is complete and both gates are the whole of it
//   damage and fills, flat colour -> same as refuse-the-unbind.js: gates done, painting is next
//   still cleared                 -> a THIRD gate, and the o-code trace says where
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,8);
  return { hash: h(s), distinctColours: Object.keys(hist).length,
           top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}

var POOL = 0x80054F84, DS = 0x8045CB64, OBJ_G = DS + 0x2E114;
window.__profile(true);
var was = b32(POOL);
if (was !== 0xFFFFFFFF) throw new Error('pool constant is ' + h(was) + ', not 0xFFFFFFFF');
poke32(POOL, 0);
if (b32(POOL) !== 0) throw new Error('the gate poke did not land');

var base = b32(0x80105E9C), REC1 = base + 100;
await new Promise(function(r){ setTimeout(r, 80000); });
if (b32(DS + 0x1ACB0) !== 1) throw new Error('DS base check failed');
var obj = b32(OBJ_G);
if (!obj) throw new Error('DS[0x2E114] is 0 after the settle');
if (b32(REC1 + 0x60) !== obj)
  throw new Error('the window root is ' + h(b32(REC1+0x60)) + ' but the global says ' + h(obj));

poke32((obj + 0x14)>>>0, 1);
if (b32((obj + 0x14)>>>0) !== 1) throw new Error('the member poke did not land');

window.__traceCalls([
  { pc: 0x80085128, name: 'setWindowRoot', args: 2 },
  { pc: 0x80083830, name: 'DAMAGE',        args: 2 },
  { pc: 0x80082604, name: 'apply',         args: 1 },
  { pc: 0x80082A6C, name: 'newWidget',     args: 1 }
]);
var before = { blits: window.__blitLog().length, surface: surface(),
               ring: [b32(REC1+0x50), b32(REC1+0x54)] };
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });
var log = window.__traceLog(), counts = {};
log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });
await window.__shot('plus14-forced');

var newBlits = window.__blitLog().slice(before.blits);
var values = {};
newBlits.forEach(function(e){
  var m = /value (0x[0-9a-fA-F]+)/.exec(e.note || ''); if (m) values[m[1]] = (values[m[1]]||0)+1; });

return {
  poked: { gate: h(b32(POOL)), member: h(b32((obj+0x14)>>>0)), obj: h(obj) },
  press: { counts: counts,
           setWindowRoot: log.filter(function(e){ return e.name==='setWindowRoot'; })
                             .map(function(e){ return e.a.join(','); }),
           damageWindows: [...new Set(log.filter(function(e){return e.name==='DAMAGE';})
                                         .map(function(e){ return e.a[1]; }))] },
  rootAtEnd: h(b32(REC1 + 0x60)), ring: [b32(REC1+0x50), b32(REC1+0x54)],
  blits: { before: before.blits, new: newBlits.length, distinctFillValues: values,
           notes: newBlits.slice(0, 26).map(function(e){ return e.note || ''; }) },
  surfaceBefore: before.surface, surfaceAfter: surface(),
  tasks: window.__tasks().n
};
