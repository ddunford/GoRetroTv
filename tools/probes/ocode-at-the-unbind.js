// THE O-CODE THAT CLEARS THE ROOT. Trace the interpreter across the bind -> unbind gap.
//
// Established (see docs/reference/digibox-emulation.md): the application binds its widget root to
// the visible plane and clears it 2727 instructions later, both calls from its own o-code through
// the (1,0xE3) shim, and it calls NO other native and prints nothing in between. Refusing the
// unbind makes the box paint its menu. So the decision is in the bytecode, and this is the trace
// that contains it.
//
// HOW THE TRACE WORKS. __readWatch over the EPG module's CODE chunk logs every read of the
// bytecode with the PC that made it. The interpreter's main fetch site 0x80069298 reads OPCODE
// bytes and every other site reads OPERANDS, so walking the log in icount order recovers the
// instruction boundaries -- which is what scripts/ocode-disasm.py consumes, and what its --check
// mode validates a static listing against.
//
// THE WINDOW IS THE GAP, on purpose. Arming the watch at probe start would cover the ~25 million
// instructions between the boot assert and the bind and blow the 4000-entry cap long before
// reaching it. So the watch is armed AT the bind, from inside the breakpoint, and read at the
// unbind. Whatever chose to clear the root executed in between.
//
// CAPPING IS A HARNESS FAILURE HERE, NOT A RESULT. A truncated trace and a short one look
// identical in the output, and a listing checked against a truncated trace reports missing
// boundaries that are really missing log entries. The log's own `capped` flag is returned and
// asserted on.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
var OCODE_LO = 0x9FC4A400, OCODE_LEN = 353188;
var SET_WINDOW_ROOT = 0x80085128;

window.__profile(true);
async function stall(limitMs){
  var t0 = Date.now();
  while (Date.now() - t0 < limitMs){
    await new Promise(function(r){ setTimeout(r, 4); });
    var g = window.__regs();
    if ((parseInt(g.pc,16)>>>0) === SET_WINDOW_ROOT) return g;
  }
  return null;
}

window.__breakAt([SET_WINDOW_ROOT]);
var g1 = await stall(90000);
if (!g1) throw new Error('never stalled at the bind -- nothing traced');
var bind = [parseInt(g1.a0,16)>>>0, parseInt(g1.a1,16)>>>0];
if (bind[1] === 0) throw new Error('first stall is the CLEAR, not the bind -- window is wrong');

window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN);
window.__resume();

var g2 = await stall(90000);
if (!g2) throw new Error('never stalled at the unbind -- the gap was not captured');
var unbind = [parseInt(g2.a0,16)>>>0, parseInt(g2.a1,16)>>>0];
var lg = window.__readWatchLog();
window.__readWatch();
window.__breakAt([]);
window.__resume();

if (unbind[1] !== 0) throw new Error('second stall is not the clear (a1=' + h(unbind[1]) + ')');
if (!lg.all || !lg.all.length) throw new Error('the o-code watch logged NOTHING across the gap');
if (lg.capped) throw new Error('the o-code trace CAPPED -- it is truncated, not short');

// The span of bytecode the gap touched, which is the window to disassemble statically.
var addrs = lg.all.map(function(e){ return parseInt(e.at, 16)>>>0; });
var lo = Math.min.apply(null, addrs), hi = Math.max.apply(null, addrs);

return {
  bind: bind.map(h).join(','), unbind: unbind.map(h).join(','),
  reads: lg.reads, entries: lg.all.length, capped: !!lg.capped,
  ocodeSpan: { lo: h(lo), hi: h(hi), bytes: hi - lo + 1 },
  fetchSites: (function(){
    var c = {}; lg.all.forEach(function(e){ c[e.pc] = (c[e.pc]||0)+1; }); return c; })(),
  trace: lg.all.map(function(e){ return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }),
  tasks: window.__tasks().n
};
