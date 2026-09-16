// DOES THE BOX COME BACK? -- sky-02me.18, and the question that decides whether listings are
// reachable today or not at all.
//
// THE ASSUMPTION WORTH CHALLENGING. Everything so far has treated the post-BAT state as permanent:
// the box stops drawing and that is that. But the slot pool DRAINS -- 121 of 160 bits set, falling
// one per allocation, 39 already gone when first read -- so the loop consumes a finite resource and
// must end. At roughly one slot per iteration that is about 121 more iterations.
//
// AND THE TWO-MINUTE WATCH STOPPED JUST SHORT. It ran to 440 EEPROM saves and ~88 cycles and was
// still climbing when it gave up. If a cycle costs a slot, exhaustion is nearer three minutes than
// two -- so "it never recovers" may be an artefact of where the previous measurement stopped, which
// is the same mistake as reading nine seconds of silence as a refusal.
//
// IF IT RECOVERS, the route to listings opens immediately: the box then has a line-up AND a working
// interface, which is the combination this whole epic needs and has never had at once.
// IF IT DOES NOT, that is worth knowing precisely too, and the pool hitting zero is the moment the
// caller has to do something different -- which is the best chance of naming it.
//
// WATCHED, NOT ASSUMED: widgets per probe press, the bitmap's set-bit count, and the EEPROM
// counters, sampled together for six minutes. The bitmap is the clock; the widgets are the answer.
//
// CONTROLS: a healthy press first, asserted, so "it draws again" can be compared against what a
// working press looks like. The repro asserted, so the wait is of a broken box. And the pool is
// found by BREAKING at the allocator rather than by assuming the address from a previous run -- it
// is a heap pointer and there is no reason it should be stable across boots.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function u32(a){ var b = window.__peek(a >>> 0, 4); return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0; }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function ee(){ var i = window.__i2cState(); return { writes: i.eeprom.writes, saves: i.eeprom.saves }; }

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 240) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');

var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
async function press(raw, seconds){
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, (seconds || 9) * 1000); });
  var w = window.__traceLog().filter(function(x){ return x.name === 'newWidget'; }).length;
  window.__traceCalls([]);
  return { widgets: w, moved: surface().hash !== s0.hash, surface: surface() };
}

var out = { settledAfterSeconds: waited };
out.healthy = await press(0x7D);
if (!out.healthy.widgets) throw new Error('the healthy press built no widgets -- no healthy half');

out.feed = { nit: [window.__siNIT(undefined,{version:81}), window.__siNIT(undefined,{version:82})].map(function(r){return r.ok?'ok':r.why;}) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined,{version:81}), window.__siBAT(undefined,{version:82})].map(function(r){return r.ok?'ok':r.why;});
await new Promise(function(r){ setTimeout(r, 9000); });
out.dead = await press(0x7D);
if (out.dead.widgets) throw new Error('the repro did not reproduce: ' + out.dead.widgets + ' widgets');

// ---- find the pool by breaking once, not by assuming last run's address ------------------------
var ALLOC = 0x800B4798, pool = 0;
window.__breakAt([ALLOC, ALLOC | 1]);
for (var i = 0; i < 80 && !pool; i++) {
  await new Promise(function(r){ setTimeout(r, 250); });
  var rg = window.__regs();
  if (rg.pc === h(ALLOC) || rg.pc === h(ALLOC | 1)) pool = parseInt(rg.a0, 16) >>> 0;
}
window.__breakAt([]); window.__resume();
out.pool = pool ? h(pool) : 'the break was never taken -- the bitmap cannot be watched';
var bmAddr = 0, slots = 0;
if (pool) {
  var size = u32(pool + 4), entry = u32(pool + 8);
  slots = entry ? Math.floor(size / entry) : 0;
  bmAddr = u32(pool + 0x40) >>> 0;
  out.poolShape = { size: size, entry: entry, slots: slots, bitmap: h(bmAddr) };
}
function bitsSet(){
  if (!bmAddr || !slots) return -1;
  var b = window.__peek(bmAddr, Math.ceil(slots / 8)), n = 0;
  for (var k = 0; k < b.length; k++) for (var bit = 0; bit < 8; bit++) if (b[k] & (1 << bit)) n++;
  return n;
}

// ---- six minutes, watching the clock and the answer --------------------------------------------
out.curve = [];
var recoveredAt = null, e0 = ee(), t0 = Date.now();
for (var t = 0; t < 24 && recoveredAt === null; t++) {
  var p = await press(0x7D, 14);                  // a real press each round: the answer, not a guess
  var e = ee();
  var row = { atSeconds: Math.round((Date.now() - t0) / 1000), widgets: p.widgets,
              moved: p.moved, colours: p.surface.colours,
              bitsSet: bitsSet(), eeWrites: e.writes - e0.writes, eeSaves: e.saves - e0.saves };
  out.curve.push(row);
  if (p.widgets > 0) recoveredAt = row.atSeconds;
}
out.recoveredAtSeconds = recoveredAt;

if (recoveredAt !== null) {
  // IT CAME BACK. Prove it is really working rather than twitching once: go to the guide, which is
  // a different screen and a bigger build (249 widgets on a healthy box), and check the line-up is
  // still there.
  out.afterRecovery = { guide: await press(0x80, 14) };
  out.afterRecovery.tables = window.__siMatches().map(function(x){
    return '0x' + x.tableId.toString(16) + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
  out.verdict = 'IT RECOVERS. The box drew again after ' + recoveredAt + 's, so the post-BAT state is '
              + 'TRANSIENT and not a permanent refusal -- every earlier measurement simply stopped '
              + 'before the loop had consumed its pool. A box with a line-up AND a working interface '
              + 'is now reachable, which is the combination this epic has never had at once.';
} else {
  out.verdict = 'NO RECOVERY in ' + Math.round((Date.now() - t0) / 1000) + 's. bitsSet went from '
              + out.curve[0].bitsSet + ' to ' + out.curve[out.curve.length - 1].bitsSet
              + (out.curve[out.curve.length - 1].bitsSet === 0
                  ? ' -- THE POOL IS EXHAUSTED and it still does not draw, so exhaustion is not the '
                  + 'end of the loop. What the caller does with a failed allocation is the subject.'
                  : ' -- the pool is NOT yet empty, so this watch is still too short to settle it. '
                  + 'The drain rate over this window says how much longer is needed.');
}
out.tasksAtEnd = window.__tasks().n;
return out;
