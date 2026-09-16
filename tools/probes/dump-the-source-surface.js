// DUMP THE SOURCE SURFACE SO ITS PITCH CAN BE MEASURED RATHER THAN GUESSED.
//
// sky-eluc.34 proved word 1 of the menu commands (0x0055C288) is a real surface: three palette
// indices in runs over 1920 bytes, 100% non-zero. What is NOT known is its geometry. The current
// side layout puts the source pitch in word 5, which reads 0x8042F354 and would mean 62,293
// pixels, so blitSide(r,1) is not the source descriptor for this opcode.
//
// A WRONG PITCH DOES NOT THROW. It reads plausible rubbish out of DRAM and paints a convincing
// screen of noise, which is this project's entire genre of failure -- so the pitch is measured off
// the DATA before any decode is changed. This probe only dumps; the analysis is offline, where the
// true pitch is the one that maximises row-to-row agreement. A bitmap's rows resemble their
// neighbours and a wrong stride destroys that, so the measurement has a sharp maximum rather than
// a judgement call.
//
// The dump covers 480x32 at one byte per pixel (15,360 bytes) plus slack, which is the size of one
// menu row at the destination geometry the command itself carries -- 0x001F01DF is (h-1)=31,
// (w-1)=479.
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
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });

// Take the source address from the LAST command the blitter actually executed, rather than from a
// literal typed in here -- a hardcoded address is the kind that silently stops matching.
var blits = window.__blitLog();
var last = null;
for (var i = blits.length - 1; i >= 0; i--){
  if ((blits[i].note || '').indexOf('fill 8bpp') === 0){ last = blits[i]; break; }
}
if (!last) throw new Error('no 8bpp command in the blit log -- nothing to take a source from');
var src = (parseInt(last.words[1], 16) | 0x80000000)>>>0;
var b = window.__peek(src, 0x6000);
var out = [];
for (var j = 0; j < b.length; j++) out.push(('0' + b[j].toString(16)).slice(-2));

return {
  takenFrom: { op: last.op, note: last.note, word1: last.words[1] },
  sourceAddress: h(src), bytes: b.length,
  destGeometry: { word11: last.words[11], word12: last.words[12], word13: last.words[13] },
  hex: out.join(''),
  tasks: window.__tasks().n
};
