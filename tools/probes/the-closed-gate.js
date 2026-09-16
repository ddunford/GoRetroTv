// THE ONE FIELD THAT STOPS THE WHOLE WIDGET TREE FROM DRAWING.
//
// Chain, decompiled and then counted on a key press:
//   apply 0x80082604  x490    if (obj[2] != 0 && gate(obj) != 0) damage(obj+0x16, win)
//   gate  0x800825F4  x48     -> 0x80085884(obj[0]):  x == 0 ? 0 : *(resolve(x) + 0x0C)
//   DAMAGE 0x80083830 x0      <- never reached
// So 442 of 490 fail `obj[2] != 0` and ALL 48 that reach the gate get zero back.
//
// (1,0xD2), the background-colour setter, calls the SAME 0x80083830 but directly, with the
// window's own clip rect and no object gate -- which is precisely why the blue screen draws and
// the widget tree does not.
//
// 0x80085894 is `lw v0,0xc(v0)`, the instruction that READS the blocking field, so a PC trace
// there captures the resolved context in $v0 -- the only way to learn the runtime address, since
// it comes out of the resolver 0x80085004 rather than from any global.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
window.__profile(true);
window.__traceCalls([
  { pc: 0x80085894, name: 'readField', args: 1 },   // $a0..; $v0 is the ctx, read below via regs
  { pc: 0x80083830, name: 'DAMAGE',    args: 2 }
]);
await new Promise(r=>setTimeout(r,80000));

// Pass 1: learn the context addresses. __traceCalls records $a0..$a3, not $v0, so the ctx is taken
// from the trace's `ra`/`sp`-independent route: break at the read instruction and read $v0 live.
window.__traceClear();
window.__breakAt([0x80085894]);
window.__key(0x7D, 0);
await new Promise(r=>setTimeout(r,3000));
// __regs() returns hex STRINGS (it formats with hex32), so a bare `>>> 0` on one yields NaN -> 0,
// which would read as "the context is null" -- the exact kind of plausible zero this project keeps
// getting caught by. Parse it.
var regs = window.__regs ? window.__regs() : null;
var ctx = (regs && typeof regs.v0 === 'string') ? (parseInt(regs.v0, 16) >>> 0)
                                                : ((regs && regs.v0 >>> 0) || 0);
if (regs && !ctx) throw new Error('breakpoint did not stall at 0x80085894: v0=' + regs.v0 + ' pc=' + regs.pc);
var snapshot = ctx ? Array.from(window.__peek(ctx, 64)) : [];
var rec = [];
for (var i = 0; i < 16; i++) rec.push(h(b32(ctx + i * 4)));
window.__breakAt([]);
if (window.__resume) window.__resume();
await new Promise(r=>setTimeout(r,2000));

// Pass 2: does ANYTHING ever write that field? A watch over the whole context record says who, and
// an empty log over a second key press says the field is never written after boot at all.
var ww = ctx ? window.__writeWatch(ctx, ctx + 64) : { watching: false };
window.__key(0x0C, 0);
await new Promise(r=>setTimeout(r,6000));
var wlog = window.__writeWatchLog();
window.__writeWatch();

return { ctx: h(ctx), field0C: h(b32(ctx + 0x0C)), record: rec,
         writeWatch: { armed: ww, writes: wlog.writes, byPc: wlog.byPc, all: wlog.all.slice(0, 12) },
         damageCalls: window.__traceLog().filter(function(e){ return e.name === 'DAMAGE'; }).length,
         blits: window.__blitLog().length, tasks: window.__tasks().n };
