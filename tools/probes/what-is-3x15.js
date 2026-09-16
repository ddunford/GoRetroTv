// WHAT IS (3,0x15), AND WHAT DOES IT RETURN? -- the native whose answer clears the screen.
//
// From the o-code trace across the bind -> unbind gap (scripts/digibox-probes/
// ocode-at-the-unbind.js, disassembled with scripts/ocode-disasm.py):
//
//   9fc72fb8  ca 03 15   scall (3,0x15)          <- call it
//   9fc72fbb  pop_fp_minus16
//   9fc72fbd  67 f8      pop_fp_nn   (fp-8)      <- store the result
//   9fc72fbf  9e f8      push_fp_nn  (fp-8)      <- load it back
//   9fc72fc1  75         push_0
//   9fc72fc2  29 3a      jgt_nn 0x9fc72ffe       <- TAKEN, to the clear
//        ... 58 bytes skipped: the happy path, which sets the background to 0xDCDCDCDC ...
//   9fc72ffe  ... scall (1,0xE3) with 0 ...      <- setWindowRoot(win, 0)
//
// So one native's return value decides whether this box shows its interface. This resolves it
// from the RUNNING machine rather than the flash -- the dispatcher indexes a table of
// {function array, count} pairs at the word in 0x8006E71C, built at boot in DRAM, and walking
// the flash copy while reporting on the live box is the two-record trap this project already
// has a name for.
//
// The record layout is module 1's, and that is an ASSUMPTION about module 3 until it reads back
// sanely: a four-byte pointer per function, to an 8-byte {implementation, argument descriptor}.
// The implementation is a MIPS16 thunk whose real target is the word at ((lw address) & ~3) + off.
// If module 3's count is absurd or its pointers are not in the image, this says so rather than
// reporting a plausible address.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function b16(a){ var b=window.__peek(a,2); return ((b[0]<<8)|b[1])>>>0; }

window.__profile(true);
var modtab = b32(0x8006E71C);
if (!modtab) throw new Error('module table pointer read as 0 -- probe starved or box not booted');

var mods = [];
for (var m = 0; m < 16; m++) mods.push({ m: m, arr: h(b32(modtab+m*8)), n: b32(modtab+m*8+4) });

var arr3 = b32(modtab + 3*8), n3 = b32(modtab + 3*8 + 4);
if (!arr3 || n3 === 0 || n3 > 4096)
  throw new Error('module 3 table looks wrong: arr=' + h(arr3) + ' count=' + n3);
if (0x15 >= n3) throw new Error('module 3 has only ' + n3 + ' natives; 0x15 is out of range');

// Resolve the thunk the way module 1's resolver does, and SAY SO if the shape is not a thunk.
function resolve(f){
  var rec = b32(arr3 + 4*f);
  var impl = b32(rec), desc = b32(rec + 4);
  var thunk = (impl & ~1)>>>0, fn = 0, shape = 'inline (not a 16-byte thunk)';
  // addiu sp,-8 ; sw ra,4(sp) ; lw rx,off(pc) ; jalr rx
  var w = b16(thunk + 4);
  if ((w >> 11) === 0x16){                       // lw rx,off(pc)
    var off = (w & 0xFF) << 2;
    fn = b32((((thunk + 4) & ~3) + off)>>>0);
    shape = 'thunk';
  }
  // the descriptor is [return type][arg types...][0x00]
  var d = window.__peek(desc, 8), types = [];
  for (var i = 0; i < 8 && d[i] !== 0; i++) types.push(d[i]);
  return { f: h(f), rec: h(rec), impl: h(impl), shape: shape, fn: h(fn),
           ret: types[0], args: types.slice(1) };
}

var target = resolve(0x15);
if (target.shape !== 'thunk')
  throw new Error('(3,0x15) is not a thunk -- resolve by hand from impl ' + target.impl);

// Now WATCH it: break on the resolved function and read $v0 on the way out, because the o-code
// tests the RETURN VALUE and nothing else about it matters yet.
var FN = parseInt(target.fn, 16)>>>0;
window.__traceCalls([
  { pc: FN,           name: '3x15_enter', args: 4 },
  { pc: 0x80085128,   name: 'setWindowRoot', args: 2 }
]);
await new Promise(function(r){ setTimeout(r, 80000); });
var log = window.__traceLog(), counts = {};
log.forEach(function(e){ counts[e.name] = (counts[e.name]||0)+1; });

return {
  modtab: h(modtab), modules: mods.filter(function(x){ return x.n > 0; }),
  module3: { arr: h(arr3), count: n3 },
  native_3x15: target,
  neighbours: [0x13,0x14,0x15,0x16,0x17].map(resolve),
  trace: { counts: counts,
           calls: log.slice(0, 20).map(function(e){
             return { name: e.name, icount: e.icount, args: e.a.join(','), ra: e.ra }; }) },
  tasks: window.__tasks().n
};
