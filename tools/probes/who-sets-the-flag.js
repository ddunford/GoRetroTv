// DOES ANYTHING EVER TRY TO SET THE FLAG? -- the last link in sky-eluc.29.
//
// The box clears its own screen because *(char*)0x80161D3C is zero (sky-eluc.28). A store scan
// over the image (scripts/mips16-xrefs.py --stores) finds NINE instructions that write that byte,
// and the decompiled C shows it is not a boolean but a small STATE MACHINE taking 0, 1, 2 and 3.
// Two functions drive it:
//
//   FUN_80055D84  a message handler, dispatching on *(byte*)(param+0x21) against 0xF4/0xF5/0xF6 to
//                 three handlers (0x800592D8, 0x8005936C, 0x80059B94). Sets the flag to 2, and to
//                 1 further in, behind a condition on the flag's own current value and on
//                 *0x80161D40 != *0x8010161C.
//   FUN_8005803C  sets the flag to 2 if it is currently 0 and a second word is 0 -- an "if idle,
//                 begin" shape.
//
// So there are exactly two ways this box could ever raise the flag, and this asks whether either
// is reached. The three outcomes are different findings and must not be collapsed:
//   neither runs                  -> the question moves UP to their callers
//   one runs, flag stays 0        -> the question is its internal condition, and the state that
//                                    feeds it is read here so the next step needs no second run
//   one runs and the flag moves   -> something clears it again, and that is a third question
//
// CONTROL. The probe asserts it saw the two setWindowRoot calls, because a trace that caught
// nothing and a box that did nothing look identical -- and this project has already reported a
// native as never called when the trace was armed on an address no PC can equal.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function b8(a){ return window.__peek(a,1)[0]; }

var FLAG = 0x80161D3C, VALUE = 0x80161D40, REF = 0x8010161C, SECOND = 0x80101618;
function state(){
  return { flag: b8(FLAG), value: b32(VALUE)|0, ref: b32(REF)|0,
           word0x80101610: h(b32(0x80101610)), word0x80101618: h(b32(SECOND)) };
}

window.__profile(true);
var atStart = state();

window.__traceCalls([
  { pc: 0x80055D84, name: 'msgHandler_setsFlag',  args: 1 },
  { pc: 0x8005803C, name: 'ifIdleBegin_setsFlag', args: 1 },
  { pc: 0x800592D8, name: 'handler_F4', args: 2 },
  { pc: 0x8005936C, name: 'handler_F5', args: 2 },
  { pc: 0x80059B94, name: 'handler_F6', args: 2 },
  { pc: 0x80085128, name: 'setWindowRoot', args: 2 }
]);

await new Promise(function(r){ setTimeout(r, 80000); });
var idle = window.__traceLog(), afterIdle = state();
window.__traceClear();
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 20000); });
var key = window.__traceLog();

function tally(log){ var c={}; log.forEach(function(e){ c[e.name]=(c[e.name]||0)+1; }); return c; }
var idleCounts = tally(idle);
if (!idleCounts.setWindowRoot)
  throw new Error('the control did not fire: setWindowRoot was not seen, so no count here means anything');

function detail(log, name, n){
  return log.filter(function(e){ return e.name === name; }).slice(0, n||10).map(function(e){
    return { icount: e.icount, args: e.a.join(','), ra: e.ra }; });
}

return {
  stateAtStart: atStart, stateAfterIdle: afterIdle, stateAtEnd: state(),
  idle: { counts: idleCounts,
          msgHandler: detail(idle, 'msgHandler_setsFlag'),
          ifIdleBegin: detail(idle, 'ifIdleBegin_setsFlag') },
  key:  { counts: tally(key),
          msgHandler: detail(key, 'msgHandler_setsFlag'),
          ifIdleBegin: detail(key, 'ifIdleBegin_setsFlag') },
  tasks: window.__tasks().n
};
