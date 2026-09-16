// WHERE DOES THE FILL COLOUR REALLY COME FROM? Read the command the DMA actually delivers.
//
// A blit command is not written register by register. It is DMA'd on channel 12 from a block in
// RAM, and the emulator loads `Math.min(len, 60) >>> 2` words from it -- so IF len IS SHORT, ONLY
// THE FIRST FEW WORDS COME FROM THE COMMAND and every later register keeps whatever the previous
// blit left in it. That single line reframes everything observed so far: the menu fills' words
// 2..6 hold code and heap pointers (0x8008502F, 0x8043071C, 0x80085311, 0x800859B7), which reads
// as a misbuilt command, and would instead be a perfectly ordinary consequence of a short
// transfer.
//
// So before deciding what bit 24 selects, establish how much of the command is even real:
//   len >= 60  -> all fifteen words are the builder's, w1 genuinely holds 0x0055C288, and the
//                 question is what the builder means by it
//   len small  -> only len/4 words are the builder's and the rest are STALE, in which case our
//                 "value = word 1" is reading a register the command never set, and the reported
//                 colour is an artefact of the previous blit rather than anything the application
//                 chose
//
// The two cases call for opposite next moves, which is exactly why this is measured before either
// is assumed. The command block at `src` is dumped alongside, so the builder's own bytes can be
// compared against the registers the emulator ended up with.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

var POOL = 0x80054F84, DS = 0x8045CB64, OBJ_G = DS + 0x2E114;
window.__profile(true);
if (b32(POOL) !== 0xFFFFFFFF) throw new Error('pool constant is not 0xFFFFFFFF');
poke32(POOL, 0);
await new Promise(function(r){ setTimeout(r, 80000); });
if (b32(DS + 0x1ACB0) !== 1) throw new Error('DS base check failed');
var obj = b32(OBJ_G);
if (!obj) throw new Error('DS[0x2E114] is 0 after the settle');
poke32((obj + 0x14)>>>0, 1);

var dmaBefore = window.__dmaLog().length;
var blitBefore = window.__blitLog().length;
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });

var dma = window.__dmaLog().slice(dmaBefore).filter(function(e){ return e.ch === 12; });
var blits = window.__blitLog().slice(blitBefore);
if (!blits.length) throw new Error('the press produced no blits -- the gates did not open');
if (!dma.length)
  throw new Error('blits happened but NO channel-12 DMA was logged -- the command reached the ' +
                  'registers by some other route, and this probe is measuring the wrong thing');

// The command block as the BUILDER wrote it, read back from RAM.
function block(srcHex, words){
  var sa = (parseInt(srcHex, 16) | 0x80000000)>>>0, out = [];
  for (var i = 0; i < words; i++) out.push(h(b32((sa + 4*i)>>>0)));
  return out;
}

var lens = {};
dma.forEach(function(e){ lens[e.len] = (lens[e.len]||0) + 1; });

return {
  channel12Transfers: dma.length, blits: blits.length,
  lengthsSeen: lens,
  wordsActuallyLoaded: Object.keys(lens).map(function(L){
    return L + ' bytes -> ' + Math.min(+L, 60) / 4 + ' of 15 words from the command, ' +
           (15 - Math.min(+L, 60)/4) + ' STALE'; }),
  first: dma.length ? { ch: dma[0].ch, src: dma[0].src, ctl: dma[0].ctl, len: dma[0].len,
                        commandInRam: block(dma[0].src, 15), note: dma[0].blit } : null,
  last:  dma.length ? { src: dma[dma.length-1].src, len: dma[dma.length-1].len,
                        commandInRam: block(dma[dma.length-1].src, 15),
                        note: dma[dma.length-1].blit } : null,
  registersTheEmulatorUsed: blits[blits.length-1].words,
  allSources: [...new Set(dma.map(function(e){ return e.src; }))],
  tasks: window.__tasks().n
};
