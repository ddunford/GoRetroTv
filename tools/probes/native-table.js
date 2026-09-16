// RESOLVE MODULE 1's NATIVES FROM THE RUNNING MACHINE, not from the flash.
// The flash holds a candidate array at 0x9FC29F04; the dispatcher (0x8006A2E8 -> 0x8006E6A8)
// indexes a table of {function array, count} pairs held at the word in 0x8006E71C and built at
// boot in DRAM. Walking the flash copy and reporting on the running box is exactly the
// two-record trap, so this reads the live one and says whether they agree.
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function h(v){ return '0x'+(v>>>0).toString(16).padStart(8,'0'); }

window.__profile(true);
var modtab = b32(0x8006E71C);
var mods = [];
for (var m=0;m<16;m++) mods.push({ m:m, arr:h(b32(modtab+m*8)), n:b32(modtab+m*8+4) });
var arr1 = b32(modtab+1*8), n1 = b32(modtab+1*8+4);
function entry(f){
  var rec = b32(arr1+4*f);
  return { f:h(f), rec:h(rec), impl:h(b32(rec)), desc:h(b32(rec+4)) };
}
var of_interest = [0x26,0x2D,0x3D,0x56,0x57,0x5D,0x62,0x7D,0xC7,0xD5,0xE4,0xE8].map(entry);

// CONTROLS FIRST. (1,0x26) is the event wait and fires constantly; if IT does not appear in the
// trace then the trace is not armed and every "never called" below is a statement about the
// instrument. 0x80081C58 is the (1,0xE4) shim, 0x800837A0 the function it jumps to.
window.__traceCalls([
  { pc: 0x80084050, name: 'CONTROL_eventWait_1x26', args: 1 },
  { pc: 0x80081CE8, name: 'CONTROL_shim_1x26',      args: 1 },
  { pc: 0x80081C58, name: 'shim_1xE4',              args: 1 },
  { pc: 0x800837A0, name: 'paint_1xE4',             args: 1 },
  { pc: 0x80083644, name: 'execOneCmd',             args: 1 }
]);
window.__traceClear();
var keys = [];
keys.push(window.__key(0x7D, 0));
await new Promise(r=>setTimeout(r,4000));
var mid = window.__traceLog().length;
keys.push(window.__key(0x0C, 0));
await new Promise(r=>setTimeout(r,4000));

var log = window.__traceLog();
var counts = {};
log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });
return { modtab:h(modtab), mods:mods, module1:{ arr:h(arr1), count:n1 },
         flashArrayMatches: h(arr1) === h(0x9FC29F04),
         natives: of_interest,
         keys: keys, midCount: mid, traceCounts: counts, traceN: log.length,
         firstPaintArgs: log.filter(function(e){return e.name==='paint_1xE4';})
                            .slice(0,6).map(function(e){return e.a.map(h).join(',');}),
         blits: window.__blitLog().length, tasks: window.__tasks().n };
