// IS THE RASTER PATH WILLING, OR IS IT THE APPLICATION THAT NEVER ASKS?
//
// The window record (100 bytes, array at *0x80105E9C, count at *0x80106F24) carries a command ring:
// +0x50 is the PRODUCE index, +0x54 the CONSUME index, +0x40 the bit depth, +0x58/+0x5C the
// background-colour flag and its replicated value. Window 1 is the visible plane and reads
// 0xDCDCDCDC -- exactly the palette index the four 720x144 fills wrote, which is what identifies
// (1,0xD2) at 0x80084388 as the call that drew the blue screen.
//
// (1,0xE4) at 0x800837A0 is the drain: `while (produce != consume) executeOne()`. The boot runs
// (1,0xD2) then (1,0xE4) 204k instructions later and four fills come out -- see who-drew-the-blue.js.
//
// So drive the pair by hand: produce a different background colour with (1,0xD2), then drain with
// (1,0xE4). What it establishes is that the producer is willing and that NOTHING consumes the ring
// on its own -- the produce index advances and stays ahead of consume until a drain runs. Both
// natives are MIPS16, so wantIsa must be 1.
//
// WINDOW 0 IS A TRAP AND IT COST A RUN. The validator errors when the window id is 0 while the gate
// at *0x80106F20 reads 0, and the error handler does not return -- the call burns its whole
// instruction budget and leaves the machine wedged, which then reads as "the drain did nothing".
// Only window 1 is driven here.
function h(v){ return '0x'+(v>>>0).toString(16).padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,3);
  return { hash: h(s), top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}
window.__profile(true);
var base = b32(0x80105E9C);
function rec(i){ var r = base+i*100; return {
  win:i, kind:h(b32(r)), depth:(b32(r+0x40)>>>24)&0xFF,
  produce:b32(r+0x50), consume:b32(r+0x54), bgSet:b32(r+0x58), bg:h(b32(r+0x5c)) }; }

// The blue fill only happens about a minute after boot, so measuring before it would compare
// against the pre-blue screen and call the difference a result.
await new Promise(r=>setTimeout(r,75000));
var steps = [{ at:'before', surface:surface(), blits:window.__blitLog().length, rec:rec(1) }];

var setCol = window.__call(0x80084388, 1, 1, 0x2A, 0, 0);     // (1,0xD2) produce: bg of window 1
await new Promise(r=>setTimeout(r,4000));
steps.push({ at:'after (1,0xD2) bg=0x2A', call:{ok:setCol.ok,why:setCol.why,v0:setCol.v0},
             surface:surface(), blits:window.__blitLog().length, rec:rec(1) });

var drain = window.__call(0x800837A0, 1, 1, 0, 0, 0);          // (1,0xE4) consume
await new Promise(r=>setTimeout(r,4000));
steps.push({ at:'after (1,0xE4) drain', call:{ok:drain.ok,why:drain.why,v0:drain.v0},
             surface:surface(), blits:window.__blitLog().length, rec:rec(1) });

await window.__shot('after-forced-draw');
return { base:h(base), steps:steps,
         newBlits: window.__blitLog().slice(steps[0].blits).map(function(e){ return e.note || JSON.stringify(e); }),
         tasks: window.__tasks().n };
