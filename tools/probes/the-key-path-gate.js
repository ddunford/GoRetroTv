// THE SECOND GATE, READ AT THE MOMENT IT DECIDES.
//
// With the startup gate open (pool word 0x80054F84 poked -1 -> 0) the box keeps its root through
// the whole settle, and the FIRST thing a key press does is clear it again. The o-code, traced with
// a sliding __readWatch window and disassembled by scripts/ocode-disasm.py:
//
//   9fc730c5  push_ds ; add 0x0002E114 ; get     load DS[0x2E114]
//   9fc730cc  pop_fp_nn  (fp-8)                  keep it in a local
//   ...
//   9fc730e3  push_fp_nn (fp-8)
//   9fc730e6  91 5c   push_m_ind_fp_n            read a member through it
//   9fc730e8  36 0b   jnz_nn 0x9fc730f5          NOT TAKEN -- so the member is ZERO
//   9fc730ea  push_0 ; push_ds ; add 0x0002E118 ; get ; scall (1,0xE3)    setWindowRoot(win, 0)
//
// So this clear is NOT the (3,0x15) gate again. It is a field read through an object held in a
// global, and the field is zero. (The same function does carry more copies of the first gate
// further on -- 0x9FC73105 is another scall (3,0x15) with the same push_0/jgt_nn shape and the
// 0xDCDCDCDC background path behind it -- so do not mistake one for the other when reading the
// listing.)
//
// DS RESOLVES TO 0x8045CB64, from the already-established DS[0x0001ACB0] -> 0x80477814. That is
// arithmetic on one recorded fact, so it is checked here rather than trusted: DS[0x1ACB0] is read
// back and must still be the config word it is known to be.
//
// WHAT `push_m_ind_fp_n 0x5c` ADDRESSES IS NOT ASSUMED. The operand could be a byte offset or an
// index, so this dumps the whole head of the object and marks which words are zero, rather than
// reporting one word as "the field" and being confidently wrong about the layout.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, SET_WINDOW_ROOT = 0x80085128;
var DS = 0x8045CB64;
var OBJ_G = DS + 0x2E114, WIN_G = DS + 0x2E118, COL_G = DS + 0x2E11C;

window.__profile(true);
// THE DS CONTROL RUNS AFTER THE SETTLE, NOT HERE -- and the first version of this probe put it
// here and failed on its own control. DS[0x1ACB0] is written at icount ~149,210,673, which is
// AFTER the 42-task boot assert at ~139.5M, so at probe start it is legitimately 0 and a check
// there reads a machine that is not yet in the state being assumed. The check is still worth
// having; it just has to be asked at a moment when the answer exists.
var was = b32(POOL);
if (was !== 0xFFFFFFFF) throw new Error('pool constant is ' + h(was) + ', not 0xFFFFFFFF');
poke32(POOL, 0);
if (b32(POOL) !== 0) throw new Error('the gate poke did not land');

function globals(){
  return { objHandle: h(b32(OBJ_G)), windowId: h(b32(WIN_G)), colour: h(b32(COL_G)) };
}
function dump(obj){
  if (!obj) return null;
  var out = [];
  for (var off = 0; off <= 0x70; off += 4) out.push(h(off) + '=' + h(b32((obj + off)>>>0)));
  return out;
}

await new Promise(function(r){ setTimeout(r, 80000); });

// CONTROL, now that the application has run. Every address below is derived from the DS base, so
// two independently recorded facts are checked rather than one: DS[0x1ACB0] is the installed flag
// the box sets during boot, and DS[0x025A80] is the screen selector the application writes.
// DS[0x1ACB0] is the installed flag the box sets during boot; 1 confirms the base.
// DS[0x025A80] is the SCREEN SELECTOR and is REPORTED, not asserted -- it is written on the key
// press rather than during the settle, and a second version of this probe asserted it here and
// failed on a machine that was working perfectly. A control has to be right about WHEN as well as
// what, and an over-eager one costs a run just as an absent one does.
var cfg = b32(DS + 0x1ACB0), screen = b32(DS + 0x25A80);
if (cfg !== 1)
  throw new Error('DS base check failed: DS[0x1ACB0]=' + h(cfg) + ' (want 1) -- base 0x8045CB64 is wrong');

var atSettle = { dsInstalledFlag: h(cfg), dsScreenSelector: h(screen),
                 globals: globals(), obj: dump(b32(OBJ_G)) };

window.__breakAt([SET_WINDOW_ROOT]);
window.__key(0x7D, 0);
var caught = null, t0 = Date.now();
while (Date.now() - t0 < 60000){
  await new Promise(function(r){ setTimeout(r, 4); });
  var g = window.__regs();
  if ((parseInt(g.pc,16)>>>0) !== SET_WINDOW_ROOT) continue;
  var a1 = parseInt(g.a1,16)>>>0;
  if (a1 === 0){
    var obj = b32(OBJ_G);
    caught = { win: h(parseInt(g.a0,16)), obj: h(a1), globals: globals(),
               objHandle: h(obj), objDump: dump(obj),
               // the two readings of the operand, both shown rather than one chosen
               at_0x5C: h(b32((obj + 0x5C)>>>0)),
               at_index_0x5C_times4: h(b32((obj + 0x5C*4)>>>0)),
               ctxOfObj_0x5C: h(b32((obj - 0x2C + 0x5C)>>>0)) };
    window.__resume();
    break;
  }
  window.__resume();
}
window.__breakAt([]);
window.__resume();
if (!caught) throw new Error('never caught the key-path clear -- nothing measured');

return { atSettle: atSettle, atTheClear: caught, tasks: window.__tasks().n };
