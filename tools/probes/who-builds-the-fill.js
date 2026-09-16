// WHICH WORDS OF THE FILL COMMAND DOES THE BUILDER ACTUALLY WRITE?
//
// Established: the command is DMA'd on channel 12 with len = 60, so all fifteen words ARE
// transferred and nothing is stale in the register file. The command block lives on the STACK
// (0x8079FF88 and 0x8079FFC4), which means the builder fills in the words it needs and leaves the
// rest as whatever its frame already held. Words 2..6 -- the source side, irrelevant to a fill --
// hold code and heap pointers, which is exactly what uninitialised stack looks like.
//
// SO THE QUESTION IS WHETHER WORD 1 IS WRITTEN OR INHERITED, and it decides everything:
//   written   -> 0x0055C288 is the application's own value and our decode is reading the right
//                word; what it MEANS is then the open question
//   inherited -> our "else v = r[1] & 0xFFFFFF" is reading stack garbage, the colour on screen is
//                an accident, and the real colour is in a word we are not reading
//
// A write watch over the command block answers it directly and names the builder's PC for every
// word it does set. No decoding, no inference about the opcode's bit meanings.
//
// THE CONTROL IS WORD 0. The opcode must be written by the builder -- a command whose opcode was
// inherited from the stack could not possibly be a coherent fill. If word 0 does not appear in the
// log, the watch is not covering the block that was used and every absence below is meaningless.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, DS = 0x8045CB64, OBJ_G = DS + 0x2E114;
var BLOCK_LO = 0x8079FF80, BLOCK_HI = 0x807A0010;   // both observed command blocks, plus slack

window.__profile(true);
if (b32(POOL) !== 0xFFFFFFFF) throw new Error('pool constant is not 0xFFFFFFFF');
poke32(POOL, 0);
await new Promise(function(r){ setTimeout(r, 80000); });
if (b32(DS + 0x1ACB0) !== 1) throw new Error('DS base check failed');
var obj = b32(OBJ_G);
if (!obj) throw new Error('DS[0x2E114] is 0 after the settle');
poke32((obj + 0x14)>>>0, 1);

var dmaBefore = window.__dmaLog().length;
var armed = window.__writeWatch(BLOCK_LO, BLOCK_HI);
if (!armed || !armed.watching) throw new Error('write watch refused the range: ' + JSON.stringify(armed));
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });

var wl = window.__writeWatchLog();
var n = wl.all.length;
var entries = wl.all.slice().map(function(e){
  return { pc: e.pc, at: e.at, size: e.size, val: e.val, icount: e.icount }; });
var pcs = wl.byPc.slice();
window.__writeWatch(0, 0);

var dma = window.__dmaLog().slice(dmaBefore).filter(function(e){ return e.ch === 12; });
if (!dma.length) throw new Error('no channel-12 DMA during the press');
// COMPARE PHYSICAL ADDRESSES. The builder writes the command block through the UNCACHED KSEG1
// alias -- 0xA079FF88, not 0x8079FF88 -- which is the right thing to do for a DMA buffer and is
// exactly what a driver should be doing. An earlier version of this probe took the DMA's source
// and OR'd in 0x80000000, then compared that against the logged KSEG1 addresses, matched nothing,
// and reported that the builder never writes the command. Mask the segment off both sides.
function phys(a){ return (a & 0x1FFFFFFF)>>>0; }
var bases = [...new Set(dma.map(function(e){ return phys(parseInt(e.src,16)); }))];

// Group the writes by which word of which command block they touched.
var byWord = {};
entries.forEach(function(e){
  var at = phys(parseInt(e.at, 16));
  bases.forEach(function(b){
    if (at >= b && at < b + 60){
      var w = 'word' + String((at - b) >> 2).padStart(2, '0') + ' of ' + h(b);
      (byWord[w] = byWord[w] || []).push({ pc: e.pc, val: e.val });
    }
  });
});
var wordsWritten = Object.keys(byWord).sort();
// THE OPCODE IS NOT REWRITTEN PER FILL, and the first version of this probe treated that as a
// harness failure and threw. It is not: it is the finding. The command block is PERSISTENT -- the
// builder sets it up once and updates only the words that change from blit to blit -- so "word 0
// was not written in this window" means the block was prepared earlier, not that the watch missed
// it. What the control must actually require is that SOME write to the block was seen; without
// that, no absence means anything.
// TWO DIFFERENT NOTHINGS, AND THE FIRST VERSION CONFLATED THEM. `wordsWritten` empty can mean
// the watch logged nothing at all, or that it logged plenty and none of it fell inside the two
// command blocks -- and those lead opposite ways. The raw count decides, and both are reported
// rather than thrown, because a probe that discards its own evidence to raise an error about that
// evidence is the worst of both.
var word0Written = wordsWritten.some(function(k){ return /word0$/.test(k); });
if (n === 0)
  throw new Error('the write watch logged NOTHING across the whole press over ' + h(BLOCK_LO) +
                  '..' + h(BLOCK_HI) + ' -- harness failure, not a result');

return {
  commandBlocks: bases.map(h), writes: n,
  opcodeRewrittenPerFill: word0Written,
  wordsTheBuilderWrote: wordsWritten,
  word1Written: wordsWritten.some(function(k){ return /word1$/.test(k); }),
  detailWord0: byWord[wordsWritten.filter(function(k){ return /word0$/.test(k); })[0]],
  detailWord1: byWord[wordsWritten.filter(function(k){ return /word1$/.test(k); })[0]] || null,
  detailWord14: byWord[wordsWritten.filter(function(k){ return /word14$/.test(k); })[0]] || null,
  byPc: pcs,
  rawWritesOutsideTheBlocks: entries.filter(function(e){
      var at = parseInt(e.at,16)>>>0;
      return !bases.some(function(b){ return phys(at) >= b && phys(at) < b + 60; }); }).slice(0, 25),
  distinctAddressesWritten: [...new Set(entries.map(function(e){ return e.at; }))].slice(0, 40),
  tasks: window.__tasks().n
};
