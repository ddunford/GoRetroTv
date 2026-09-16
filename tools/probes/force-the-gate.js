// FALSIFY THE DIAGNOSIS: force the closed gate open and see whether the widget tree draws.
//
// The claim to test: the ONLY thing stopping a key press from rasterising is that
// `*(resolve(obj[0]) + 0x0C)` reads zero, so apply() never calls damage() and the ring stays empty.
//
//   apply 0x80082604  x490   if (obj[2] != 0 && gate(obj) != 0) damage(obj+0x16, win)
//   gate  0x800825F4  x48    -> 0x80085884: x == 0 ? 0 : *(resolve(x) + 0x0C)
//   0x80085894 is `lw v0,0xc(v0)` -- the instruction that reads it, with the context still in $v0.
//
// So: break on that instruction, write 1 into [$v0 + 0x0C] before it executes, resume, repeat. If
// DAMAGE starts firing and window 1's produce index moves, the diagnosis is proven and the next
// question is what SHOULD set that field. If it changes nothing, the diagnosis is wrong and this
// file's own rule applies -- go and look again rather than trying a bigger version of it.
//
// This is a post-boot poke and therefore CANNOT test boot-time state; it tests one thing only,
// namely whether this gate is what stands between the widget tree and the plane.
//
// RESULT (2026-09-14): 320 gate reads forced across 20 distinct contexts -> DAMAGE fired 204 times,
// having been at ZERO. So the gate is real and forcing it does open the path. And still no pixels:
// produce stayed 1, blits stayed 8, the surface hash did not move. The damage ARGUMENTS say why --
// the window id apply() passes is `ctxOf(obj[0])[0x28]`, and it comes out as
//     0xFFFFFFFF  (the "no window" sentinel, the same value apply tests rec[win][0x38] against)
//     0x00000000  (window 0 -- the 4bpp window, not the visible plane)
// and NEVER 1. The objects are not bound to the visible plane at all, so the second unset field in
// the same record, +0x28, matters as much as +0x0C. Both keep their memset zero/sentinel.
//
// PACING FLAW, kept as a warning: driving 4000 breakpoint stalls from a JS loop that awaits 5ms per
// iteration shares a thread with the emulator, so this ran ~15 minutes rather than the ~20 seconds
// the arithmetic suggests, and 3680 of 4000 iterations were spins that found no stall. If this is
// re-run, drive far fewer iterations or find a lever that does not stall per read.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,4);
  return { hash: h(s), top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}
window.__profile(true);
var base = b32(0x80105E9C);
function ring(i){ var r=base+i*100; return { win:i, produce:b32(r+0x50), consume:b32(r+0x54) }; }

window.__traceCalls([
  { pc: 0x80083830, name: 'DAMAGE', args: 2 },
  { pc: 0x80082604, name: 'apply',  args: 1 }
]);
await new Promise(r=>setTimeout(r,80000));
var before = { rings: [ring(0), ring(1)], blits: window.__blitLog().length, surface: surface() };

window.__traceClear();
window.__breakAt([0x80085894]);
window.__key(0x7D, 0);

// Drive the breakpoint by hand: each stall is one gate read, and the poke happens BEFORE the
// instruction that reads the field executes.
var forced = 0, spins = 0, ctxs = {};
for (var i = 0; i < 4000 && forced < 400; i++){
  await new Promise(r=>setTimeout(r,5));
  var r = window.__regs();
  if (parseInt(r.pc, 16) >>> 0 !== 0x80085894){ spins++; continue; }
  var ctx = parseInt(r.v0, 16) >>> 0;
  if (ctx){ ctxs[h(ctx)] = (ctxs[h(ctx)] || 0) + 1; poke32(ctx + 0x0C, 1); forced++; }
  window.__resume();
}
window.__breakAt([]);
window.__resume();
await new Promise(r=>setTimeout(r,6000));

var log = window.__traceLog(), counts = {};
log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });
return { forcedGateReads: forced, distinctContexts: Object.keys(ctxs).length, spins: spins,
         counts: counts,
         damageArgs: log.filter(function(e){return e.name==='DAMAGE';}).slice(0,10)
                        .map(function(e){ return e.a.join(','); }),
         before: before,
         after: { rings: [ring(0), ring(1)], blits: window.__blitLog().length, surface: surface() },
         newBlits: window.__blitLog().slice(before.blits).map(function(e){ return e.note || ''; }),
         tasks: window.__tasks().n };
