// WHAT DOES (3,0x15) ACTUALLY RETURN, AND WHAT STATE PRODUCES IT?
//
// The o-code that clears this box's screen tests one native's return value:
//
//   9fc72fb8  scall (3,0x15)
//   9fc72fbf  push_fp_nn (fp-8)      the result
//   9fc72fc1  push_0
//   9fc72fc2  jgt_nn 0x9fc72ffe      TAKEN -> setWindowRoot(win, 0)
//
// and the native, at 0x80054DDC, is four lines:
//
//   int f(void) {
//       int r = -1;                                              // pool word 0x80054F84
//       if (*(char*)0x80161D3C != 0 && *(int*)0x80161D40 != *(int*)0x8010161C)
//           r = *(int*)0x80161D40;
//       return r;
//   }
//
// TWO THINGS ARE UNMEASURED AND ONE OF THEM IS A TRAP. `push X; push 0; jgt` could be "X > 0" or
// "0 > X", and the default return is -1, so the two readings put the box on OPPOSITE branches: on
// one, something set a positive value and diverted it; on the other, the DEFAULT is the clear and
// the happy path needs a value that was never written. Reading the disassembler's mnemonic as if
// it settled the operand order is exactly the static-reading error this firmware has produced
// twice before. So this reads $v0 at the `jr ra` -- the actual returned word -- and the three
// memory locations that produce it, at the moment it is produced.
//
// The instrument bug this replaces: the previous probe armed a trace on 0x80054DDD, the ODD
// MIPS16 form, which no PC can ever equal, and reported the native as never called. A count of
// zero from a subject that cannot match is a harness failure, so the control here is explicit --
// if the native is not seen at all, this raises rather than reporting state.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function s32(a){ return b32(a) | 0; }
function b8(a){ return window.__peek(a,1)[0]; }

var FN_3x15 = 0x80054DDC, RET_3x15 = 0x80054DF2, SET_WINDOW_ROOT = 0x80085128;
var FLAG = 0x80161D3C, VALUE = 0x80161D40, REF = 0x8010161C;

function state(){
  return { flagByte: b8(FLAG), value: s32(VALUE), valueHex: h(b32(VALUE)),
           ref: s32(REF), refHex: h(b32(REF)) };
}

window.__profile(true);
var atBoot = state();
window.__breakAt([RET_3x15, SET_WINDOW_ROOT]);

var returns = [], roots = [], spins = 0, sawUnbind = false;
for (var i = 0; i < 6000 && !sawUnbind; i++){
  await new Promise(function(r){ setTimeout(r, 3); });
  var g = window.__regs(), p = parseInt(g.pc, 16)>>>0;
  if (p === RET_3x15){
    if (returns.length < 40)
      returns.push({ icount: g.icount, v0: h(parseInt(g.v0,16)), v0signed: parseInt(g.v0,16)|0,
                     ra: g.ra, state: state() });
  } else if (p === SET_WINDOW_ROOT){
    var a0 = parseInt(g.a0,16)>>>0, a1 = parseInt(g.a1,16)>>>0;
    roots.push({ win: h(a0), obj: h(a1), lastReturnBefore: returns.length });
    if (a1 === 0) sawUnbind = true;
  } else { spins++; continue; }
  window.__resume();
}
window.__breakAt([]);
window.__resume();

if (!returns.length) throw new Error('(3,0x15) never stalled -- the break did not take, nothing measured');
if (!sawUnbind) throw new Error('never reached the unbind -- the window closed before the decision');

// The decisive pair: the last value returned before the clear, and what the branch did with it.
var lastBeforeClear = returns[Math.min(returns.length, roots[roots.length-1].lastReturnBefore) - 1];
return {
  stateAtProbeStart: atBoot,
  calls: returns.length, spins: spins,
  setWindowRootCalls: roots,
  returnsSeen: returns.slice(0, 12),
  THE_DECIDING_RETURN: lastBeforeClear,
  branchReading: lastBeforeClear
      ? (lastBeforeClear.v0signed > 0
           ? 'returned > 0, so jgt means "X > 0" and something WROTE a positive value'
           : 'returned <= 0 (' + lastBeforeClear.v0signed + '), so jgt means "0 > X" and the DEFAULT is the clear')
      : 'no return captured before the clear',
  tasks: window.__tasks().n
};
