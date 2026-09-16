// WHICH MEMBER DOES THE KEY-PATH GATE READ? Ask the interpreter, not the operand.
//
// The clear at o-code 0x9FC730F2 is gated on `91 5c` -- PUSH_M_IND_FP_N with operand 0x5C -- read
// through DS[0x2E114], which is the widget tree root 0x80430A14. The object's live fields stop at
// +0x34 and everything from +0x34 to +0x70 is zero, so EVERY candidate reading of that operand
// lands in the zero region and the branch is consistent with all of them. Choosing one and
// reporting it would be a guess dressed as a finding.
//
// So this does not decode the operand. It watches the handler compute the address.
// scripts/ocode-disasm.py --handlers resolves opcode 0x91 through the interpreter's own 209-entry
// dispatch at 0x800692E0 to 0x8006C050, and the o-code trace independently shows PC 0x8006C052
// reading the operand byte at 0x9FC730E7 -- two instruments agreeing on the handler before
// anything is built on it.
//
// __readWatch takes a MEMORY range and a fromPc filter, so pointing the range at the object and
// the filter at the handler makes the log's `at` field literally the member address. No decoding,
// no assumption about scaling, no assumption about whether the operand is an offset or an index.
//
// CONTROLS. The gate poke is read back. DS is checked through DS[0x1ACB0] AFTER the settle, since
// that word is written at ~149.2M and reads 0 on a healthy box before then. And the watch must
// come back non-empty: a PC filter that is too narrow logs nothing, which looks exactly like a
// handler that never ran.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, SET_WINDOW_ROOT = 0x80085128;
var DS = 0x8045CB64, OBJ_G = DS + 0x2E114;
var H91_LO = 0x8006C050, H91_HI = 0x8006C090;

window.__profile(true);
var was = b32(POOL);
if (was !== 0xFFFFFFFF) throw new Error('pool constant is ' + h(was) + ', not 0xFFFFFFFF');
poke32(POOL, 0);
if (b32(POOL) !== 0) throw new Error('the gate poke did not land');

await new Promise(function(r){ setTimeout(r, 80000); });
var cfg = b32(DS + 0x1ACB0);
if (cfg !== 1) throw new Error('DS[0x1ACB0] = ' + h(cfg) + ', not 1 -- the DS base is wrong');
var obj = b32(OBJ_G);
if (!obj) throw new Error('DS[0x2E114] is 0 after the settle -- no root object to watch');

// NO PC FILTER, and that is a correction rather than a preference. Filtering to the handler's
// own address range 0x8006C050..0x8006C090 logged nothing: PC 0x8006C052 is where the handler
// reads its OPERAND BYTE out of the o-code chunk, which the trace already showed, and that is not
// where it reads the member. Rather than guess the handler's extent or which helper it calls, the
// watch is pointed at the OBJECT and every reader is reported with its PC -- so the answer comes
// back as "this PC read this offset" instead of resting on an assumption about the interpreter's
// internal layout.
var LO = (obj - 0x40)>>>0, HI = (obj + 0x200)>>>0;
window.__readWatch(LO, HI);
window.__breakAt([SET_WINDOW_ROOT]);
window.__key(0x7D, 0);

var caught = null, t0 = Date.now();
while (Date.now() - t0 < 60000){
  await new Promise(function(r){ setTimeout(r, 4); });
  var g = window.__regs();
  if ((parseInt(g.pc,16)>>>0) !== SET_WINDOW_ROOT) continue;
  if ((parseInt(g.a1,16)>>>0) === 0){ caught = { win: h(parseInt(g.a0,16)) }; window.__resume(); break; }
  window.__resume();
}
var lg = window.__readWatchLog();
window.__readWatch();
window.__breakAt([]);
window.__resume();

if (!caught) throw new Error('never caught the key-path clear -- nothing measured');
if (!lg.all || !lg.all.length)
  throw new Error('the object watch logged NOTHING across a whole key press -- harness failure');

// The LAST read before the clear is the one the branch tested.
var rows = lg.all.map(function(e){
  var at = parseInt(e.at,16)>>>0;
  return { pc: e.pc, at: e.at, offsetFromObj: h((at - obj)>>>0), size: e.size,
           value: h(b32(at)), icount: e.icount };
});
var offsets = {}, byPc = {};
rows.forEach(function(r){
  offsets[r.offsetFromObj] = (offsets[r.offsetFromObj]||0) + 1;
  byPc[r.pc + ' -> ' + r.offsetFromObj] = (byPc[r.pc + ' -> ' + r.offsetFromObj]||0) + 1;
});
// The handler's own reads, if any reached the object at all.
var fromHandler = rows.filter(function(r){
  var p = parseInt(r.pc,16)>>>0; return p >= H91_LO && p < H91_HI; });

return {
  rootObject: h(obj), reads: lg.reads, entries: rows.length, capped: !!lg.capped,
  distinctOffsets: offsets, readersByPcAndOffset: byPc,
  readsFromThe0x91Handler: fromHandler.slice(-8),
  lastTenBeforeClear: rows.slice(-10),
  objAtClear: (function(){ var o=[]; for (var x=0; x<=0x70; x+=4) o.push(h(x)+'='+h(b32((obj+x)>>>0))); return o; })(),
  tasks: window.__tasks().n
};
