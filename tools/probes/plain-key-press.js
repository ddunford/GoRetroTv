// NO POKES. Boot the box, wait, press a key, look. This is what a VISITOR gets.
//
// Every earlier run that produced a menu poked two values first. This one pokes nothing: if the
// declared post-boot gate override works, the application keeps and paints its own screen with no
// help from a probe at all. If it does not, this reports a blue screen honestly rather than a
// screenshot taken with a thumb on the scale.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  var ks = Object.keys(hist).sort(function(x,y){ return hist[y]-hist[x]; }).slice(0,6);
  return { hash: h(s), distinctColours: Object.keys(hist).length,
           top: ks.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; }) };
}
window.__profile(true);
var base = b32(0x80105E9C), REC1 = base + 100;
await new Promise(function(r){ setTimeout(r, 80000); });
var before = surface();
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });
await window.__shot('plain-key-press');
var after = surface();
return {
  gates: window.__skyGates(),
  rootObj: h(b32(REC1 + 0x60)), ring: [b32(REC1+0x50), b32(REC1+0x54)],
  surfaceBefore: before, surfaceAfter: after,
  changed: before.hash !== after.hash,
  blits: window.__blitLog().slice(-12).map(function(e){ return e.note || ''; }),
  tasks: window.__tasks().n
};
