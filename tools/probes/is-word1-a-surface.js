// IS WORD 1 A COLOUR OR A SOURCE SURFACE? Look at what is actually at that address.
//
// THE COMPETING READINGS. Our decode says bit 23 = fill and bit 24 = "pattern from word 14, else
// the value is word 1". Under it the menu commands (0x00AC0000) are fills whose colour is word 1,
// 0x0055C288, of which the low byte 0x88 reaches an 8 bpp plane.
//
// The alternative is that bit TWENTY-FOUR is the fill bit and bit 23 is something else:
//   boot 0x01AC0000  bit24 set   -> FILL, colour from word 14 = 0xDC   (matches the blue screen)
//   menu 0x00AC0000  bit24 clear -> COPY, source word 1 = 0x8055C288 -> the framebuffer
// which would mean the menu is drawn by COPYING from a prepared surface -- which is how text and
// graphics get onto a screen, and would explain a menu that lays out perfectly and paints blank.
//
// EVIDENCE FOR: 0x8055C288 sits among this machine's other plane buffers (0x80575798, 0x805A81A8,
// 0x805B6A58 -- and that last one is this very command's destination field-1), so it is exactly
// where a surface would live. Word 14 on the menu commands reads 0x801D6160, a STACK pointer,
// which is not a colour and is consistent with "word 14 unused here".
// EVIDENCE AGAINST: the source-side geometry is nonsense. Word 5 would be the source pitch and
// reads 0x8042F354, giving 62293 pixels; words 2-4 and 6 are code and heap pointers.
//
// SO NEITHER READING FITS, and the cheap discriminator is not more decoding -- it is to LOOK AT
// THE MEMORY. A source surface holds picture: varied bytes, structure, non-zero. An address that
// is really a colour constant points at whatever happens to be there, most likely nothing.
//
// This only reads. It changes no decode and asserts no conclusion; it reports what is at the
// address and lets that decide which way the next step goes.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function poke32(a,v){ window.__poke(a, [(v>>>24)&255,(v>>>16)&255,(v>>>8)&255,v&255]); }

function describe(addr, bytes){
  var b = window.__peek(addr, bytes), hist = {}, nz = 0;
  for (var i = 0; i < b.length; i++){ hist[b[i]] = (hist[b[i]]||0)+1; if (b[i]) nz++; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; });
  return { at: h(addr), bytes: bytes, distinctByteValues: ks.length,
           nonZeroFraction: +(nz / b.length).toFixed(4),
           top: ks.slice(0, 8).map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }),
           firstRow: Array.from(b.slice(0, 32)).map(function(v){ return ('0'+v.toString(16)).slice(-2); }).join(' ') };
}

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

var W1 = 0x8055C288;
return {
  // the candidate source, one 480-pixel row and a little more
  word1AsSurface: describe(W1, 480 * 4),
  // CONTROLS: a surface we KNOW holds picture, and one we know is background
  framebuffer_knownPicture: describe(0x80584048 + 148*720 + 120, 480 * 4),
  planeBuffer_0x805B6A58:   describe(0x805B6A58, 480),
  planeBuffer_0x80575798:   describe(0x80575798, 480),
  blitsSeen: window.__blitLog().length, tasks: window.__tasks().n
};
