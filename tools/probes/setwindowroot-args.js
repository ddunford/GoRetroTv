// WHY THE APPLICATION NEVER BINDS: capture every setWindowRoot call with its arguments and caller.
//
// setWindowRoot -- 0x80085128, the native (1,0xE3) -- is the one operation that makes a widget the
// root of a plane, and it is what writes BOTH of the fields the drawing path needs:
//
//        rec = windowArray[a0];  if (a1 != rec[+0x60]) { ...; rec[+0x60] = a1;
//                                   setGateRecursive(a1, 1); setWindowRecursive(a1, a0); ... }
//
// Proven on 2026-09-14 by performing those writes with pokes over the key press's 9-node tree
// (scripts/digibox-probes/bind-the-tree.js): DAMAGE went from 0 to 48 calls, every one of them
// carrying window 1, the plane's ring advanced produce 1 -> 16, and five real fills reached the
// framebuffer. The binding is exactly and only what was missing.
//
// So the remaining question is about the APPLICATION, not the firmware, and it is sharper than
// "it never calls it" -- because it DOES. The same run counted setWindowRoot twice in the 80 s
// after boot, while both windows' +0x60 stayed 0x00000000. A call that leaves the field unchanged
// either passed the value already there (the `a1 != rec[+0x60]` guard skips) or addressed a window
// nobody is reading. Only the arguments can say which, and $ra says whether the call came from the
// o-code shim (an application decision) or from inside the firmware.
//
// The window ARRAY is read alongside, because "window 0 and window 1" is an assumption: the count
// at *0x80106F24 is what says how many there are.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }

window.__profile(true);
var base = b32(0x80105E9C), count = b32(0x80106F24), gate = b32(0x80106F20);
if (!base) throw new Error('window array pointer read as 0 -- probe starved or box not booted');
if (!count) throw new Error('window count read as 0 -- refusing to report over an empty set');

function rec(i){
  var r = base + i*100;
  return { win:i, kind:b32(r), produce:b32(r+0x50), consume:b32(r+0x54),
           bg:h(b32(r+0x5C)), rootObj:h(b32(r+0x60)) };
}
function allWindows(){ var o=[]; for (var i=0;i<count;i++) o.push(rec(i)); return o; }

// (1,0xE3)'s shim is 0x80081D30 -- an $ra there means the call came from o-code, i.e. the
// application asked for it; any other $ra means the firmware did it to itself.
var SHIM_E3 = 0x80081D30;

window.__traceCalls([
  { pc: 0x80085128, name: 'setWindowRoot',      args: 2 },
  { pc: 0x800858F4, name: 'setWindowRecursive', args: 2 },
  { pc: 0x800858B0, name: 'setGateRecursive',   args: 2 },
  { pc: 0x80085D10, name: 'addChild',           args: 2 }
]);

var windowsAtStart = allWindows();
await new Promise(function(r){ setTimeout(r, 80000); });
var idleLog = window.__traceLog();
window.__traceClear();
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 20000); });
var keyLog = window.__traceLog();

function tally(log){ var c={}; log.forEach(function(e){ c[e.name]=(c[e.name]||0)+1; }); return c; }
function detail(log, name, n){
  return log.filter(function(e){ return e.name === name; }).slice(0, n||30).map(function(e){
    return { icount: e.icount, args: e.a.join(','), ra: e.ra,
             fromOcodeShim: (parseInt(e.ra,16)>>>0) - SHIM_E3 < 0x40 &&
                            (parseInt(e.ra,16)>>>0) >= SHIM_E3, sp: e.sp };
  });
}

return {
  windowCount: count, windowGate: gate, arrayBase: h(base),
  windowsAtStart: windowsAtStart,
  idle: { counts: tally(idleLog),
          setWindowRoot:      detail(idleLog, 'setWindowRoot'),
          setWindowRecursive: detail(idleLog, 'setWindowRecursive', 12),
          setGateRecursive:   detail(idleLog, 'setGateRecursive', 12) },
  key:  { counts: tally(keyLog),
          setWindowRoot:      detail(keyLog, 'setWindowRoot'),
          setWindowRecursive: detail(keyLog, 'setWindowRecursive', 12),
          setGateRecursive:   detail(keyLog, 'setGateRecursive', 12) },
  windowsAtEnd: allWindows(),
  tasks: window.__tasks().n
};
