// WHERE IS THE FILL COMMAND REALLY BUILT? Follow the copy back to its source.
//
// The DMA buffer at physical 0x0079FF88 is not where the command is constructed. A write watch
// over it during a key press logs 165 writes and every single one comes from ONE pc, 0x800D01A0 --
// a copy loop, not fifteen field stores. So the command is assembled somewhere else and memcpy'd
// into the buffer the DMA reads, and the word we are arguing about (word 1 = 0x0055C288) was
// written by a copy that knows nothing about its meaning.
//
// This breaks in that loop and dumps every register, then looks for one that points at memory
// holding the command's own first words. THAT is the control: a register is the source pointer
// because the bytes under it ARE the command, not because it looks like an address. Several
// registers will look like addresses; only one reads back 0x00AC0000.
//
// Once the source is known, the writers of the REAL command block are one more write watch away,
// and that is the code that decides the colour.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, DS = 0x8045CB64, OBJ_G = DS + 0x2E114, COPY = 0x800D01A0;
window.__profile(true);
if (b32(POOL) !== 0xFFFFFFFF) throw new Error('pool constant is not 0xFFFFFFFF');
poke32(POOL, 0);
await new Promise(function(r){ setTimeout(r, 80000); });
if (b32(DS + 0x1ACB0) !== 1) throw new Error('DS base check failed');
var obj = b32(OBJ_G);
if (!obj) throw new Error('DS[0x2E114] is 0 after the settle');
poke32((obj + 0x14)>>>0, 1);

window.__breakAt([COPY]);
window.__key(0x7D, 0);

var found = null, hits = 0, t0 = Date.now();
while (Date.now() - t0 < 60000 && !found){
  await new Promise(function(r){ setTimeout(r, 3); });
  var g = window.__regs();
  if ((parseInt(g.pc,16)>>>0) !== COPY){ continue; }
  hits++;
  // Which register points at bytes that ARE the command? Scan a window each side, because the
  // loop has advanced and the pointer no longer sits exactly on word 0.
  var cands = [];
  Object.keys(g).forEach(function(k){
    if (k === 'pc' || k === 'isa') return;
    var v = parseInt(g[k], 16)>>>0;
    if ((v & 0xE0000000) === 0 || v < 0x80000000) return;   // must be a mapped DRAM pointer
    for (var back = 0; back <= 64; back += 4){
      var a = (v - back)>>>0;
      if (b32(a) === 0x00AC0000 && b32((a+4)>>>0) === 0x0055C288){
        cands.push({ reg: k, value: h(v), commandAt: h(a), backBy: back });
        break;
      }
    }
  });
  if (cands.length) found = { regs: g, candidates: cands };
  window.__resume();
  if (hits > 400) break;
}
window.__breakAt([]);
window.__resume();
await new Promise(function(r){ setTimeout(r, 5000); });

if (!hits) throw new Error('never stalled in the copy loop at ' + h(COPY) + ' -- nothing measured');
if (!found)
  throw new Error('stalled ' + hits + ' times in the copy loop and no register pointed at a block ' +
                  'whose first two words are the command -- so the source was not found, rather ' +
                  'than the command not being copied');

var src = parseInt(found.candidates[0].commandAt, 16)>>>0;
var words = [];
for (var i = 0; i < 15; i++) words.push(h(b32((src + 4*i)>>>0)));
return {
  stallsInCopyLoop: hits,
  sourcePointerCandidates: found.candidates,
  theRealCommandBlock: h(src),
  words: words,
  registersAtTheStall: found.regs,
  tasks: window.__tasks().n
};
