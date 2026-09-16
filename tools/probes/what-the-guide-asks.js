// sky-02me.21 -- WHICH NATIVE DOES THE GUIDE ASK FOR ITS ROWS, AND WHAT MODULE IS IT?
//
// THE DEDUCTION THAT MAKES THIS THE RIGHT QUESTION. The guide build is 37,610 o-code instructions
// and it is BYTE-IDENTICAL on a box holding 240 of our programmes and on one holding none. If the
// build were reading the store and finding it empty, a loop over 240 events and a loop over zero
// could not produce the same instruction stream. It never reaches the store at all -- so there is a
// gate BEFORE it, and the gate is in the guide's own bytecode.
//
// AND THE STORE IS NOT IN THE DATA SEGMENT, which rules out the obvious next move. FUN_800c95d0
// links its blocks from *0x801071D0 and *0x801071D4 -- NATIVE pointer tables in DRAM, not DS -- so
// the guide cannot read them with a `push_ds; add; get`. It has to ASK, and an ask is an `scall`.
//
// The guide build's scall census is dominated by module 1, the UI library: (1,0x57) x248,
// (1,0x75) x201, (1,0x56) x42, (1,0x5D) x35. One call stands out by being neither module 1 nor
// module 0: **(12,0x23), exactly twice**. Twice is the shape of "ask the database", not of a
// drawing primitive.
//
// SO THIS RESOLVES THE MODULE TABLE FROM THE RUNNING MACHINE. The dispatcher indexes a table of
// {function array, count} pairs at the word in 0x8006E71C, built at boot in DRAM -- reading the
// flash copy instead and reporting on the running box is exactly the two-record trap this project
// has already fallen into once. Each entry is a pointer to an 8-byte {implementation, argument
// descriptor}, and the implementation is a MIPS16 thunk whose real target is the pool word its
// `lw rx,off(pc)` resolves to.
//
// A COUNT OF ZERO FOR A MODULE IS A FINDING, NOT A GAP, so every module the table declares is
// listed with its count rather than only the ones asked about -- a table read that silently
// returned an empty row would otherwise look like "module 12 does not exist".

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function b32(a){ var b = window.__peek(a, 4); return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0; }
function b16(a){ var b = window.__peek(a, 2); return ((b[0] << 8) | b[1]) & 0xFFFF; }
function inFlash(a){ return a >= 0x9FC00000 && a < 0x9FE00000; }
function inRam(a){ return a >= 0x80000000 && a < 0x82000000; }

// The thunk rule, from scripts/opentv-natives.py: 210 of module 1's 236 entries are a 16-byte
// shim whose `lw rx,off(pc)` names the real function; the other 26 do the work inline and ARE the
// function. MIPS16 LWPC is opcode 0b10110 with the base at the instruction address & ~3.
function resolveThunk(impl){
  var base = impl & ~1;
  for (var k = 0; k < 8; k++) {
    var hw = b16(base + k * 2);
    if ((hw >>> 11) === 0x16) {
      var off = (hw & 0xFF) * 4;
      var pool = ((base + k * 2) & ~3) + off;
      return { via: 'thunk', lwAt: h(base + k * 2), pool: h(pool), fn: h(b32(pool)) };
    }
  }
  return { via: 'inline -- no lw rx,off(pc) in the first eight halfwords', fn: h(base) };
}
function argDescriptor(a){
  var b = window.__peek(a, 12), out = [];
  for (var i = 0; i < b.length; i++) { if (b[i] === 0) break; out.push(b[i]); }
  return { ret: out.length ? out[0] : null, args: out.slice(1), argCount: Math.max(0, out.length - 1) };
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

var out = { question: 'which native does the guide ask for its rows', settledAfterSeconds: waited };
var modtab = b32(0x8006E71C);
out.moduleTable = h(modtab);
if (!modtab || modtab < 0x80000000 || modtab >= 0x82000000)
  throw new Error('the module table pointer at 0x8006E71C reads ' + h(modtab) + ', which is not a '
                + 'DRAM address -- every row below would be read out of nothing');

out.modules = [];
for (var m = 0; m < 24; m++) {
  var arr = b32(modtab + m * 8), n = b32(modtab + m * 8 + 4);
  // A MODULE'S ARRAY MAY BE IN FLASH OR IN DRAM, and the first version of this guard only allowed
  // DRAM -- so module 12, whose array is at 0x9FC2BEA4 with a perfectly good count of 51, was
  // rejected as unusable. The guard was wrong, not the data. __peek goes through the same load()
  // the CPU uses, so both regions read.
  out.modules.push({ module: m, array: h(arr), count: n, where: inFlash(arr) ? 'flash' : (inRam(arr) ? 'dram' : 'neither'),
                     plausible: (inFlash(arr) || inRam(arr)) && n > 0 && n < 4096 });
}
var mod12 = out.modules[12];
if (!mod12.plausible)
  throw new Error('module 12 declares array ' + mod12.array + ' count ' + mod12.count + ' in '
                + mod12.where + ', which is not a usable row -- (12,0x23) is a call the guide makes, '
                + 'so an unusable row here is a harness failure rather than evidence that the module '
                + 'is absent');
if (0x23 >= mod12.count)
  throw new Error('module 12 declares only ' + mod12.count + ' functions, so 0x23 is out of range '
                + 'and the (12,0x23) reading from the scall census is about a different table');

function entry(m, f){
  var arr = b32(modtab + m * 8), cnt = b32(modtab + m * 8 + 4);
  if (f >= cnt) return { call: '(' + m + ',0x' + f.toString(16) + ')', error: 'index ' + f + ' is beyond the module\'s count of ' + cnt };
  var rec = b32(arr + 4 * f);
  var impl = b32(rec), desc = b32(rec + 4);
  return { call: '(' + m + ',0x' + f.toString(16) + ')', record: h(rec), impl: h(impl),
           descriptor: h(desc), sig: argDescriptor(desc), target: resolveThunk(impl) };
}

// (12,0x23) is the subject. Its neighbours come too, because one function in isolation says nothing
// about what the module IS, and a module whose neighbours are all schedule-shaped is a different
// finding from one where 0x23 stands alone.
out.subject = entry(12, 0x23);
out.module12Neighbours = [];
for (var f = 0x1E; f <= 0x2A; f++) out.module12Neighbours.push(entry(12, f));

// And the module-1 natives the guide leans on hardest, so the drawing half can be named too.
out.module1Hot = [0x57, 0x75, 0x56, 0x5D, 0x62, 0x5B].map(function(f){ return entry(1, f); });

// THE CONTROL. Module 1 resolves to the array this project has already verified against the flash;
// if that has moved, the table reading is wrong and module 12's rows are wrong with it.
out.module1ArrayMatchesFlash = out.modules[1].array === h(0x9FC29F04);
out.note = out.module1ArrayMatchesFlash
  ? 'module 1 still resolves to the array verified against the flash, so the table walk is sound'
  : 'MODULE 1 DOES NOT RESOLVE TO 0x9FC29F04 -- the live table has moved and every row here is '
  + 'suspect, including module 12\'s';
return out;
