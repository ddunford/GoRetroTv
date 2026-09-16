// WHAT BINDS A WIDGET TO A PLANE -- and does this box ever do it?
//
// Static reading (Ghidra, scripts/mips16-xrefs.py) says the binding is ONE operation and names it:
//
//   0x80085128  (1,0xE3)  setWindowRoot(windowId, obj)
//       rec = windowArray[windowId]            // *0x80105E9C, 100-byte records
//       if (obj != rec[+0x60]) {               // +0x60 IS THE WINDOW'S ROOT OBJECT
//           applySubtree(rec[+0x60]);
//           setGateRecursive  (rec[+0x60], 0);         // old root: gate CLOSED
//           setWindowRecursive(rec[+0x60], sentinel);  // old root: window = no-window
//           rec[+0x60] = obj;
//           setGateRecursive  (obj, 1);                // new root: gate OPEN   -> ctx+0x0C = 1
//           setWindowRecursive(obj, windowId);         // new root: window      -> ctx+0x28 = id
//           applySubtree(obj);
//       }
//   0x800858B0  setGateRecursive(root, v)    walks the subtree, writes ctx+0x0C = v on every node
//   0x800858F4  setWindowRecursive(root, v)  walks the subtree, writes ctx+0x28 = v on every node
//   0x80085D10  (1,0x4F) addChild(parent,child)  links, then INHERITS the parent's gate and window
//   0x80085D84           addChild variant, same inheritance
//
// That closes the loop on the two unset fields exactly: +0x0C and +0x28 are set together, by one
// call, on a whole subtree -- and a child added later inherits both from its parent. So a widget
// tree whose root was never made a window's root has BOTH fields at their memset zero / sentinel,
// which is precisely the state measured on 2026-09-14.
//
// The xref scan is sound rather than by proximity: only four instructions in the whole image load
// a pool word holding 0x800858F5, and two of them are inside (1,0x4F) -- the known inheritance
// path, which is this scan's positive control.
//
// THIS PROBE ASKS THE MACHINE. Reading says what the operation is; only running says whether the
// application performs it, on which window, and with which object. Three questions:
//   1. does setWindowRoot run at all -- idle, and on a key press -- and with what arguments?
//   2. what is each window's root object (+0x60) after a boot?
//   3. is the key press's widget tree anywhere under that root?
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }

window.__profile(true);
var base = b32(0x80105E9C);
var gate = b32(0x80106F20), count = b32(0x80106F24);
if (!base) throw new Error('window array pointer read as 0 -- probe starved or box not booted');

function rec(i){
  var r = base + i*100;
  return { win:i, addr:h(r), kind:b32(r), linked:h(b32(r+0x38)), depth:b32(r+0x40),
           produce:b32(r+0x50), consume:b32(r+0x54),
           bgSet:b32(r+0x58), bg:h(b32(r+0x5C)), ROOT_OBJ:h(b32(r+0x60)),
           clip:[b32(r+0x24),b32(r+0x28),b32(r+0x2C),b32(r+0x30)].map(h) };
}

var TRACE = [
  { pc: 0x80085128, name: 'setWindowRoot',      args: 2 },
  { pc: 0x800858F4, name: 'setWindowRecursive', args: 2 },
  { pc: 0x800858B0, name: 'setGateRecursive',   args: 2 },
  { pc: 0x80085D10, name: 'addChild',           args: 2 },
  { pc: 0x80085D84, name: 'addChildVariant',    args: 2 },
  { pc: 0x80083830, name: 'DAMAGE',             args: 2 }
];
function tally(log){
  var c = {}; log.forEach(function(e){ c[e.name] = (c[e.name]||0)+1; }); return c;
}
function sample(log, name, n){
  return log.filter(function(e){ return e.name === name; }).slice(0, n||8)
            .map(function(e){ return e.a.map(function(x){ return h(x); }).join(','); });
}

// BASELINE: an untouched box. Module-1 activity here is bursty and periodic, so a count from a
// key-press window means nothing without the same count from a window with no key in it.
window.__traceCalls(TRACE);
await new Promise(function(r){ setTimeout(r, 25000); });
var idleLog = window.__traceLog();
var idle = { counts: tally(idleLog),
             setWindowRoot: sample(idleLog, 'setWindowRoot', 20),
             setWindowRecursive: sample(idleLog, 'setWindowRecursive', 20),
             windows: [rec(0), rec(1)] };

// THE KEY PRESS.
window.__traceClear();
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 15000); });
var keyLog = window.__traceLog();

return {
  windowCount: count, windowGate: gate, arrayBase: h(base),
  idle: idle,
  key: { counts: tally(keyLog),
         setWindowRoot: sample(keyLog, 'setWindowRoot', 20),
         setWindowRecursive: sample(keyLog, 'setWindowRecursive', 20),
         setGateRecursive: sample(keyLog, 'setGateRecursive', 20),
         addChild: sample(keyLog, 'addChild', 12),
         addChildVariant: sample(keyLog, 'addChildVariant', 12),
         windows: [rec(0), rec(1)] },
  tasks: window.__tasks().n
};
