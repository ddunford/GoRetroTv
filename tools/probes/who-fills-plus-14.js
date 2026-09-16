// WHO WRITES THE MEMBER AT +0x14, AND WHAT GETS WRITTEN INSTEAD?
//
// MEASURED (sky-eluc.31): the key-path clear is gated on PUSH_M_IND_FP_N reading offset +0x14 of
// the widget tree root, and the read returns zero. The offset is not decoded from the 0x5C
// operand -- it is the address the INTERPRETER computed, caught by watching the object and
// reporting the reader's PC:
//
//     pc 0x8006C0EE   at 0x80430A28 = obj+0x14   size 4   value 0x00000000
//
// Three things agree that this is the gate's read: it is the LAST read of the object before the
// clear (153k instructions after the previous one), its PC lies inside the 0x91 handler resolved
// through the interpreter's own dispatch table, and its value is zero, which is what makes the
// `jnz` fall through. (The first attempt at this filtered reads to 0x8006C050..0x8006C090 and
// logged nothing, because that range covers where the handler reads its OPERAND BYTE out of the
// o-code, not where it reads the member. The filter was the error, not the handler.)
//
// SO THE QUESTION IS NOW A WRITE. This watches the object's head across a key press and reports
// every write with the PC that made it. Two outcomes, and they lead different places:
//   +0x14 is never written      -> find the code that populates its NEIGHBOURS and look for the
//                                  store to +0x14 it does not reach
//   +0x14 is written then zeroed -> something clears it, and that is the thing to find
//
// A WATCH THAT LOGS NOTHING PROVES NOTHING, so the control is that writes to the object must be
// seen at all. The object is created before the settle ends, so creation-time writes are outside
// this window by construction -- that is stated rather than discovered later.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, SET_WINDOW_ROOT = 0x80085128;
var DS = 0x8045CB64, OBJ_G = DS + 0x2E114;

window.__profile(true);
var was = b32(POOL);
if (was !== 0xFFFFFFFF) throw new Error('pool constant is ' + h(was) + ', not 0xFFFFFFFF');
poke32(POOL, 0);
if (b32(POOL) !== 0) throw new Error('the gate poke did not land');

await new Promise(function(r){ setTimeout(r, 80000); });
if (b32(DS + 0x1ACB0) !== 1) throw new Error('DS base check failed');
var obj = b32(OBJ_G);
if (!obj) throw new Error('DS[0x2E114] is 0 after the settle');

function dump(){ var o=[]; for (var x=0; x<=0x40; x+=4) o.push(h(x)+'='+h(b32((obj+x)>>>0))); return o; }
var before = dump();

window.__writeWatch(obj, (obj + 0x40)>>>0);
window.__breakAt([SET_WINDOW_ROOT]);
window.__key(0x7D, 0);

var caught = null, t0 = Date.now();
while (Date.now() - t0 < 60000){
  await new Promise(function(r){ setTimeout(r, 4); });
  var g = window.__regs();
  if ((parseInt(g.pc,16)>>>0) !== SET_WINDOW_ROOT) continue;
  if ((parseInt(g.a1,16)>>>0) === 0){ caught = true; window.__resume(); break; }
  window.__resume();
}
var wl = window.__writeWatchLog();
window.__writeWatch(0, 0);
window.__breakAt([]);
window.__resume();

if (!caught) throw new Error('never caught the key-path clear');
if (!wl.all || !wl.all.length)
  throw new Error('NO write to the object head across a whole key press -- watch armed wrong, or ' +
                  'this object is written only at creation, before this window opens');

var rows = wl.all.map(function(e){
  var at = parseInt(e.at,16)>>>0;
  return { pc: e.pc, off: h((at - obj)>>>0), size: e.size, icount: e.icount };
});
var byOff = {};
rows.forEach(function(r){ byOff[r.off] = (byOff[r.off]||0) + 1; });

return {
  rootObject: h(obj),
  objBeforePress: before, objAtClear: dump(),
  writes: wl.writes, capped: wl.all.length >= 4000,
  writesByOffset: byOff,
  plus14Written: !!byOff[h(0x14)],
  writersOfNeighbours: rows.filter(function(r){
      return ['0x00000010','0x00000014','0x00000018','0x0000001C'].indexOf(r.off) >= 0; }).slice(0, 20),
  firstTwenty: rows.slice(0, 20),
  byPc: wl.byPc,
  tasks: window.__tasks().n
};
