// WHY IS EVERY FILL THE SAME COLOUR? Compare the blitter's registers across the fills.
//
// With both gates answered the box paints its own menu, and all eleven fills log
// `value 0x0055C288` while the boot's four log `value 0x000000DC`. The emulator's fill decode is:
//
//     if (bpp === 16)            v = r[14] >>> 16;
//     else if (w0 & 0x1000000)   v = r[14] & 0xFF;
//     else                       v = r[1] & 0xFFFFFF;
//
// so the menu fills are taking the ELSE arm and reading a TWENTY-FOUR BIT value out of word 1,
// while an 8 bpp fill wants a palette index. blitPut then keeps the low byte, 0x88, which is the
// light blue on screen. Two readings fit that and they lead opposite ways:
//
//   (a) the application really does fill everything with one colour, and the menu rows are meant
//       to be distinguished by something other than a fill -- text, borders, a later pass
//   (b) our model reads the colour from the wrong register for this op, so genuinely different
//       colours are arriving and being flattened to one
//
// THE DISCRIMINATOR IS WHETHER ANYTHING ELSE VARIES. If word 1 is constant across all eleven fills
// and every other word is constant too, the application is sending one colour and (a) holds. If
// some OTHER word varies from fill to fill while word 1 does not, that word is the colour and (b)
// holds -- and the varying word names its own register.
//
// This asserts nothing about which. It dumps all fifteen words per fill, marks which vary, and
// includes the boot fills as the contrast case, because they demonstrably carry a different value
// through the same code.
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
if (b32((obj + 0x14)>>>0) !== 1) throw new Error('the member poke did not land');

var bootBlits = window.__blitLog().slice();          // by value: the log is a live array
if (!bootBlits.length) throw new Error('no boot blits recorded -- nothing to contrast against');

window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });
var all = window.__blitLog().slice();
var menu = all.slice(bootBlits.length);
if (!menu.length) throw new Error('the key press produced NO blits -- the gates did not open');

function varying(list){
  var out = {};
  for (var i = 0; i < 15; i++){
    var seen = {};
    list.forEach(function(e){ seen[e.words[i]] = 1; });
    var vals = Object.keys(seen);
    if (vals.length > 1) out['word' + i] = vals;
  }
  return out;
}
function rows(list){
  return list.map(function(e){
    return { op: e.op, note: e.note,
             w1: e.words[1], w13: e.words[13], w14: e.words[14],
             w7: e.words[7], w8: e.words[8], w9: e.words[9] };
  });
}

return {
  bootFills:  { n: bootBlits.length, varyingWords: varying(bootBlits), rows: rows(bootBlits) },
  menuFills:  { n: menu.length,      varyingWords: varying(menu),      rows: rows(menu).slice(0, 12) },
  allWordsOfFirstMenuFill: menu[0].words,
  allWordsOfFirstBootFill: bootBlits[0].words,
  // the CLUT is the other place a single on-screen colour could come from
  windowBackground: h(b32(b32(0x80105E9C) + 100 + 0x5C)),
  tasks: window.__tasks().n
};
