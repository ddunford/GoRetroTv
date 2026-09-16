// DOES ANY PATH EVER WRITE obj+0x14? Watch the exact word across every key the handset has.
//
// The static side has gone as far as it usefully can. A write to +0x14 encodes as `5b 5c`; the
// EPG code chunk holds 37 such byte pairs, 13 preceded by push_0 (zeroing) and 24 pushing a real
// value. Disassembling around the six nearest confirms four as genuine instruction boundaries,
// and two of those read as tiny SETTER FUNCTIONS -- `push_fp_nn <local> ; 5b 5c ; ret` at
// 0x9FC704F8 and 0x9FC705D6. So the writer is a setter called from somewhere, and which somewhere
// is a question about a CALLER, which a byte scan cannot answer.
//
// A write watch can. It is pointed at the four bytes themselves, so anything that writes them --
// setter, memcpy, o-code, firmware -- is caught with its PC, and no assumption about how the
// object is reached survives to affect the result.
//
// THE CONTROL IS A DELIBERATE WRITE. A watch that logs nothing and a watch that was never armed
// look identical, and this project has already reported a native as never called on exactly that
// confusion. So the probe pokes the word itself at the end and requires its own write to appear in
// the log. If it does not, the run is a harness failure and says so rather than reporting that
// nothing writes the member.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, DS = 0x8045CB64, OBJ_G = DS + 0x2E114;
window.__profile(true);
var was = b32(POOL);
if (was !== 0xFFFFFFFF) throw new Error('pool constant is ' + h(was) + ', not 0xFFFFFFFF');
poke32(POOL, 0);                       // open the startup gate so the root survives the settle
if (b32(POOL) !== 0) throw new Error('the gate poke did not land');

await new Promise(function(r){ setTimeout(r, 80000); });
if (b32(DS + 0x1ACB0) !== 1) throw new Error('DS base check failed');
var obj = b32(OBJ_G);
if (!obj) throw new Error('DS[0x2E114] is 0 after the settle');
var TARGET = (obj + 0x14)>>>0;

window.__writeWatch(TARGET, (TARGET + 4)>>>0);
var KEYS = [[0x7D,'guide'],[0x0C,'backup'],[0x83,'services'],[0x5C,'select'],[0x3C,'up'],
            [0x3D,'down'],[0x21,'sky'],[0x00,'zero']];
var pressed = [];
for (var i = 0; i < KEYS.length; i++){
  pressed.push(KEYS[i][1] + '=' + window.__key(KEYS[i][0], 0));
  await new Promise(function(r){ setTimeout(r, 7000); });
}
// SNAPSHOT BY VALUE. __writeWatchLog() returns the live wwLog array BY REFERENCE -- unlike
// __traceLog, which slices -- so holding the object and comparing it to a later call compares an
// array with itself and can never show a difference. The first version of this probe did exactly
// that and failed its own control on a perfectly working watch.
var duringKeys = window.__writeWatchLog();
var nDuringKeys = duringKeys.all.length;
var firmwareWrites = duringKeys.all.slice().map(function(e){
  return { pc: e.pc, at: e.at, size: e.size, val: e.val, icount: e.icount }; });
var pcsDuringKeys = duringKeys.byPc.slice();

// CONTROL: write it ourselves and require the watch to see that.
poke32(TARGET, 0x5A5A5A5A);
var sawControl = window.__writeWatchLog().all.length > nDuringKeys;
poke32(TARGET, 0);
window.__writeWatch(0, 0);
if (!sawControl)
  throw new Error('the write watch did not even record our own poke -- harness failure, so ' +
                  '"nothing wrote the member" would be a statement about the instrument');

return {
  object: h(obj), watched: h(TARGET), keysPressed: pressed,
  writesFromTheFirmware: nDuringKeys,
  byPc: pcsDuringKeys,
  entries: firmwareWrites.slice(0, 20),
  controlWriteSeen: sawControl,
  verdict: nDuringKeys === 0
      ? 'NOTHING in the firmware writes obj+0x14 across eight different keys -- the setter exists but no path reaches it'
      : 'something DOES write it; the PCs above are the writers',
  memberAtEnd: h(b32(TARGET)), tasks: window.__tasks().n
};
