// THE SECOND UNBIND: what clears the root on the KEY path, once the startup gate is open.
//
// ESTABLISHED (sky-eluc.28/.29). The application binds its widget root to the visible plane and
// clears it again, and the startup clear is decided by native (3,0x15) returning its default -1.
// Poking that default to 0 at its pool word 0x80054F84 DEFEATS the startup clear: across an 80 s
// settle setWindowRoot is called once and rec[1]+0x60 keeps the root. A key press then makes TWO
// more calls and leaves the root at zero, with DAMAGE at zero and no blits. So there is a second
// gate and it is on the key path.
//
// THIS PROBE REPEATS THE METHOD THAT FOUND THE FIRST ONE, which is the reason it is short:
//   1. open the startup gate, so the box reaches the key path with its root still bound
//   2. break on setWindowRoot and record every call WITH ITS ARGUMENTS -- the previous run
//      counted them and could not say what they did, which is the gap this closes
//   3. arm __readWatch over the EPG CODE chunk at the first key-path call and read it at the
//      clear, so the o-code in between can go through scripts/ocode-disasm.py
//
// CONTROLS, because three different nothings look alike here. The poke is read back. The settle
// must show exactly ONE setWindowRoot -- if it shows two, the gate did not hold and everything
// after is about a different machine. And the o-code log must be non-empty and uncapped: a
// truncated trace and a short one are indistinguishable in the output, and a listing checked
// against a truncated trace reports drift that is really missing log entries.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, SET_WINDOW_ROOT = 0x80085128;
var OCODE_LO = 0x9FC4A400, OCODE_LEN = 353188;

window.__profile(true);
var was = b32(POOL);
if (was !== 0xFFFFFFFF) throw new Error('pool constant is ' + h(was) + ', not 0xFFFFFFFF');
poke32(POOL, 0);
if (b32(POOL) !== 0) throw new Error('the gate poke did not land');

var base = b32(0x80105E9C), REC1 = base + 100;
window.__traceCalls([{ pc: SET_WINDOW_ROOT, name: 'setWindowRoot', args: 2 }]);
await new Promise(function(r){ setTimeout(r, 80000); });
var settle = window.__traceLog().map(function(e){ return e.icount + '  ' + e.a.join(','); });
if (settle.length !== 1)
  throw new Error('settle made ' + settle.length + ' setWindowRoot calls, expected 1 -- the gate did not hold: ' + settle.join(' | '));
var rootAfterSettle = b32(REC1 + 0x60);
if (!rootAfterSettle) throw new Error('root is already 0 after the settle -- nothing to lose on the key path');

window.__traceCalls([]);
// A SLIDING WINDOW, because the clear is the FIRST thing the key path does and both fixed
// strategies fail on it. Arming at the first non-clear call never arms at all -- the first call IS
// the clear. Arming at the key press caps at 4000 entries long before reaching it, and the log
// keeps the OLDEST entries, so the part that matters is exactly the part thrown away.
// So the watch is re-armed every few milliseconds, which clears its log, and the snapshot kept is
// the one taken when the breakpoint finally stalls. Two consecutive windows are retained, because
// a clear landing just after a re-arm would otherwise leave a window too short to read.
window.__breakAt([SET_WINDOW_ROOT]);
window.__key(0x7D, 0);

async function stall(limitMs){
  var t0 = Date.now();
  while (Date.now() - t0 < limitMs){
    await new Promise(function(r){ setTimeout(r, 4); });
    var g = window.__regs();
    if ((parseInt(g.pc,16)>>>0) === SET_WINDOW_ROOT) return g;
  }
  return null;
}

var calls = [], log = null, prev = null, windows = 0;
var t0 = Date.now();
window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN);
while (Date.now() - t0 < 60000){
  await new Promise(function(r){ setTimeout(r, 6); });
  var g = window.__regs();
  if ((parseInt(g.pc,16)>>>0) !== SET_WINDOW_ROOT){
    // not stalled yet: keep the previous slice and start a fresh one
    prev = window.__readWatchLog();
    window.__readWatch(OCODE_LO, OCODE_LO + OCODE_LEN);
    windows++;
    continue;
  }
  var a0 = parseInt(g.a0,16)>>>0, a1 = parseInt(g.a1,16)>>>0;
  calls.push({ win: h(a0), obj: h(a1), ra: g.ra, rootNow: h(b32(REC1+0x60)) });
  if (a1 === 0){
    log = window.__readWatchLog();
    if ((!log.all || log.all.length < 8) && prev && prev.all && prev.all.length) log = prev;
    window.__readWatch();
    window.__resume();
    break;
  }
  window.__resume();
}
window.__breakAt([]);
window.__resume();
await new Promise(function(r){ setTimeout(r, 8000); });

if (!calls.length) throw new Error('no setWindowRoot call on the key path -- nothing measured');
// THE CALL LIST IS THE PRIMARY RESULT AND IS NEVER DISCARDED. An o-code trace that came back
// empty or truncated is a fact about the trace, reported as one, rather than a reason to throw
// away the arguments this probe exists to read.
var ocodeStatus = !log || !log.all || !log.all.length ? 'EMPTY -- nothing logged'
                : log.capped ? 'CAPPED -- truncated, do not check a listing against it'
                : 'ok';
var addrs = (log && log.all ? log.all : []).map(function(e){ return parseInt(e.at,16)>>>0; });
return {
  poked: { at: h(POOL), was: h(was), now: h(b32(POOL)) },
  settleCalls: settle, rootAfterSettle: h(rootAfterSettle),
  keyPathCalls: calls, slidingWindows: windows,
  ocode: { status: ocodeStatus, entries: addrs.length,
           lo: addrs.length ? h(Math.min.apply(null, addrs)) : null,
           hi: addrs.length ? h(Math.max.apply(null, addrs)) : null },
  trace: (log && log.all ? log.all : []).map(function(e){ return e.pc + ',' + e.at + ',' + e.size + ',' + e.icount; }),
  rootAtEnd: h(b32(REC1 + 0x60)), ring: [b32(REC1+0x50), b32(REC1+0x54)],
  tasks: window.__tasks().n
};
