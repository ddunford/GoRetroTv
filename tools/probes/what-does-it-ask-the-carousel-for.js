// WHAT DOES THE BOX ASK THE OBJECT CAROUSEL FOR, AND WHERE DOES IT EXPECT IT?
//
// THE PREMISE, and it is the right one: everything this box shows that is not video -- the menus,
// the guide's screens, the music, Box Office, Sky Active -- arrives over the wire as OpenTV
// applications and their resources, in a DSM-CC object carousel. We have never sent one. Every
// section this project has ever broadcast is SI: NIT, BAT, SDT, TDT/TOT and the Sky title records
// the now/next banner draws from.
//
// AND THE ALL CHANNELS GRID FITS THAT SHAPE EXACTLY. It registers no SI notification slot, reads
// none of the structures our SI fills, and gives up before touching any of it -- while
// **0x800BECF0, the Huffman decompressor, has never executed once in this project's history.** A
// decompressor that has never run is the single cleanest marker for "no compressed module has ever
// arrived and been accepted".
//
// SO THIS DOES NOT GUESS A CAROUSEL FORMAT. Guessing a fifth section format is what cost this
// project a week on the listings, and the thing that finally worked was letting the box name what
// it wanted. The same method applies here and it is cheaper: watch the functions that would run IF
// the box were trying to load something, and capture their ARGUMENTS, which say what it is asking
// for and which PID it expects it on.
//
//     0x800511D0  carouselAcquire     the box asking for a carousel at all
//     0x8008E7C8  getCodeModule       asking for a module by id
//     0x8008C8A8  appById             resolving an application
//     0x80036D0C  appStart            starting one
//     0x800BECF0  huffman             a compressed resource being expanded
//     0x800C9CA0  carousel parser     the neighbourhood this project has long suspected
//
// AND THE UNFED PIDS ARE THE OTHER HALF OF THE ANSWER. The box arms PID channels for what it wants;
// anything armed that we never send to is, by definition, something it is waiting for. That list is
// the "where it expects it to come from" question answered by the box rather than by me.
//
// NOTHING IS ASSERTED ABOUT THE OUTCOME. This run reports; it does not conclude. A census showing
// every one of those cold is as useful as one showing them hot -- it would mean the box is not
// asking for a carousel in this state, and the next question would be what makes it ask.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
var WATCH = [
  { a: 0x800511D0, n: 'carouselAcquire' },
  { a: 0x8008E7C8, n: 'getCodeModule' },
  { a: 0x8008C8A8, n: 'appById' },
  { a: 0x80036D0C, n: 'appStart' },
  { a: 0x8007BCC0, n: 'registryInsert' },
  { a: 0x800BECF0, n: 'huffman' },
  { a: 0x800C9CA0, n: 'carouselParser_0x800C9CA0' },
  { a: 0x800C95D0, n: 'titleSectionParser (known good, a live control)' }
];
WATCH.forEach(function(w){ if (w.a & 1) throw new Error(w.n + ' is odd; __pcHits normalises upward only'); });
function census(){
  var r = window.__pcHits.apply(null, WATCH.map(function(w){ return w.a; })), o = {};
  WATCH.forEach(function(w){
    if (r[h(w.a)] === undefined) throw new Error('__pcHits could not find ' + w.n + ' -- harness failure');
    o[w.n] = r[h(w.a)];
  });
  return o;
}
function diff(a, b){ var o = {}; Object.keys(b).forEach(function(k){ if (b[k] !== a[k]) o[k] = b[k] - a[k]; }); return o; }
function armedPids(){
  return window.__siFilters().filter(function(f){ return f.armed; }).map(function(f){ return parseInt(f.pid, 16); });
}
function tables(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/' + x.extension.toString(16) : ''); }).sort().join(' ');
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 400) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
if (window.__siCarousel().startedAt === undefined) window.__siCarousel(true);
for (var w2 = 0; w2 < 30; w2++) { var li = window.__siListings();
  if (li.state !== 'loading' && li.state !== 'not started') break;
  await new Promise(function(r){ setTimeout(r, 1000); }); }
var out = { settledAfterSeconds: waited, listings: window.__siListings(), warmBoot: waited < 90 };

// Let it acquire, however long that takes on this profile.
for (var q = 0; q < 220; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  if (/^Ready/.test(document.getElementById('boxstate-t').textContent)
      && armedPids().some(function(p){ return p >= 0x30 && p <= 0x37; }) && q > 8) break;
}
out.acquiredAfterSeconds = 3 * q;
out.tables = tables();

// WHAT IS ARMED THAT WE NEVER FEED. The carousel sends to 0x0010, 0x0011, 0x0014 and whichever
// title PID the box opened; anything else armed is the box waiting for something nobody sends.
var FED = [0x0010, 0x0011, 0x0014];
var armed = armedPids();
out.armedPids = armed.map(h);
out.unfedPids = armed.filter(function(p){
  return FED.indexOf(p) < 0 && !(p >= 0x30 && p <= 0x37); }).map(function(p){ return '0x' + p.toString(16); });

var TRACE = [
  { pc: 0x800511D0, name: 'carouselAcquire', args: 2 },
  { pc: 0x8008E7C8, name: 'getCodeModule', args: 1 },
  { pc: 0x8008C8A8, name: 'appById', args: 1 },
  { pc: 0x80036D0C, name: 'appStart', args: 2 }
];
var WIDGETS = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
async function step(raw, label, ms){
  window.__traceCalls(TRACE.concat(WIDGETS)); window.__traceClear();
  var c0 = census();
  if (raw !== null) window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, ms || 10000); });
  var lg = window.__traceLog(), n = {};
  lg.forEach(function(e){ n[e.name] = (n[e.name] || 0) + 1; });
  window.__traceCalls([]);
  return { key: label, widgets: n.newWidget || 0, calls: n,
           args: lg.filter(function(e){ return e.name !== 'newWidget'; }).slice(0, 12)
                   .map(function(e){ return e.name + '(' + (e.a || []).map(h).join(', ') + ')'; }),
           census: diff(c0, census()) };
}

out.atRest = await step(null, 'at rest, no key', 8000);
out.sky = await step(0x7D, 'sky menu');
out.tab = await step(0x5A, 'TV GUIDE tab');
out.grid = await step(0x5C, 'ALL CHANNELS', 14000);
await window.__shot('carousel-question');
out.totals = census();

var hot = Object.keys(out.totals).filter(function(k){ return out.totals[k] > 0; });
var cold = Object.keys(out.totals).filter(function(k){ return out.totals[k] === 0; });
out.headline = 'HOT: ' + (hot.join(', ') || 'nothing') + '  ||  COLD: ' + (cold.join(', ') || 'nothing')
  + '  ||  armed PIDs ' + out.armedPids.join(' ') + ', of which NOBODY FEEDS: '
  + (out.unfedPids.join(' ') || 'none') + '  ||  tables ' + out.tables;
return out;
