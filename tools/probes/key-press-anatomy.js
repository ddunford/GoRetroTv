// WHAT THE KEY PRESS ACTUALLY DOES TO THE PLANE, function by function.
//
// Derived soundly (not by proximity): the natives whose code READS the window-record array pointer
// *0x80105E9C are the plane family, and of that family a key press reaches exactly two --
// (1,0xCB) x26 and (1,0xE4) x2. Everything else it runs is the widget toolkit.
//
//   (1,0xCB) 0x80083528  if classOf(obj) == 1: set an indexed attribute, and if it CHANGED,
//                        apply it via 0x80082604. classOf is 0x80082AD8, the same helper the
//                        layout engine 0x80082BCC uses to pick a vtable.
//   (1,0xE4) 0x800837A0  drain window N's command ring
//   (1,0x57) 0x80082A6C  new widget(class) -- its argument IS the class index
//
// THREE THINGS THIS SETTLES, each of which would be the answer on its own:
//   1. Which classes the press instantiates. If nothing it builds is class 1, all 26 (1,0xCB)
//      calls are no-ops and the attribute never reaches a plane.
//   2. Whether (1,0xCB)'s gate passes -- 0x80082604 running at all is the test, and it is a
//      separate PC so "called 26 times" and "did something 26 times" cannot be confused.
//   3. WHICH WINDOW the drain targets. If the press drains a window the widgets were never
//      pointed at, the ring being empty is a routing fault rather than a missing producer.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function b32(a){ var b=window.__peek(a,4); return ((b[0]<<24)|(b[1]<<16)|(b[2]<<8)|b[3])>>>0; }
window.__profile(true);
var base = b32(0x80105E9C), count = b32(0x80106F24);
function rec(i){ var r = base+i*100; return { win:i, kind:b32(r), depth:(b32(r+0x40)>>>24)&0xFF,
  produce:b32(r+0x50), consume:b32(r+0x54), bg:h(b32(r+0x5c)) }; }
function rings(){ var o=[]; for (var i=0;i<Math.min(count,6);i++) o.push(rec(i)); return o; }

window.__traceCalls([
  { pc: 0x80082A6C, name: 'new(1,0x57)',   args: 1 },   // $a0 = class index
  { pc: 0x80083528, name: 'attr(1,0xCB)',  args: 2 },
  { pc: 0x80082604, name: 'APPLY',         args: 1 },   // only runs if 0xCB's gate passed
  { pc: 0x80082AD8, name: 'classOf',       args: 1 },
  { pc: 0x800837A0, name: 'drain(1,0xE4)', args: 1 },   // $a0 = which window
  { pc: 0x80084388, name: 'bg(1,0xD2)',    args: 2 }
]);
// Wait past the blue fill, so the press is measured on a settled box rather than across the boot's
// own draw -- the fills happen about a minute in and would otherwise land in the same window.
await new Promise(r=>setTimeout(r,80000));
window.__traceClear();
var before = { rings: rings(), blits: window.__blitLog().length };
window.__key(0x7D, 0);
await new Promise(r=>setTimeout(r,8000));
var log = window.__traceLog();

var counts = {}, classes = {}, drained = {}, attrs = 0;
log.forEach(function(e){
  counts[e.name] = (counts[e.name]||0)+1;
  if (e.name === 'new(1,0x57)')   classes[e.a[0]] = (classes[e.a[0]]||0)+1;
  if (e.name === 'drain(1,0xE4)') drained[e.a[0]] = (drained[e.a[0]]||0)+1;
  if (e.name === 'attr(1,0xCB)')  attrs++;
});
return { counts: counts,
         widgetClassesCreated: classes,
         windowsDrained: drained,
         cbCalls: attrs, applies: counts['APPLY'] || 0,
         before: before,
         after: { rings: rings(), blits: window.__blitLog().length },
         firstAttrArgs: log.filter(function(e){return e.name==='attr(1,0xCB)';})
                           .slice(0,6).map(function(e){ return e.a.join(','); }),
         traceN: log.length, tasks: window.__tasks().n };
