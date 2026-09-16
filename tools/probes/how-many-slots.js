// HOW MANY SLOTS DOES THE POOL HAVE? -- sky-02me.18, and the arithmetic that either ends it or
// rules out the lead.
//
// FUN_800b4798 is the hottest routine that runs ONLY on a dead press (10,788 hits, 74 distinct PCs,
// against a noise floor of 189). Decompiled it is a slot allocator:
//
//     n   = pool[4] / pool[8];                    // size / entry size
//     idx = (random() * n) >> 8 & 0xFF;
//     for (i = 0; i < n; i++) {
//         if (bitmap[idx >> 3] & (1 << (7 - (idx & 7)))) return idx;
//         idx = (idx + 1) & 0xFF;                 // <-- masked to 256
//         if (idx >= n) idx = 0;
//     }
//     return pool[0x20];
//
// THE FIRST HYPOTHESIS WAS THE & 0xFF MASK -- if n > 256 the search could never reach the slots
// above 255 and a caller that allocates until it succeeds would spin. IT IS REFUTED: n is 160 in
// every sample (size 10,240, entry 64), so the mask is never reached and the shape was a red
// herring. Recorded here rather than deleted, because the next reader will see the mask too.
//
// WHAT THE SAME NUMBERS POINT AT INSTEAD: the loop returns a slot ONLY where a bitmap bit is SET,
// and falls out after n iterations to the fallback at pool[0x20]. If no bit is ever set the search
// fails every time, all 160 iterations, and a caller that retries spins -- the same end, by a
// different route, and one that does not need n to be large at all. So this now reads THE BITMAP.
//
// $a0 IS THE POOL. FUN_800b4798(param_1) takes it as the single argument, so the break is taken at
// the function's first instruction, before the prologue has moved anything.
//
// THE BREAK ADDRESS IS TRIED BOTH WAYS. This is MIPS16 code and a PC there carries the ISA bit --
// __pcHits already compensates by summing a and a|1 -- so both are armed rather than guessing which
// the comparison uses. If neither is ever hit that is a harness failure and must be reported as one,
// not as "the routine did not run", because the histogram says it runs ten thousand times.
//
// CONTROLS: the healthy press must draw first; the repro must reproduce; and the pool is sampled
// SEVERAL times, because one reading of one call cannot tell a pool that is always oversized from
// one that is oversized once.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function u32(a){ var b = window.__peek(a >>> 0, 4); return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0; }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 240) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');

var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
async function press(){
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface();
  window.__key(0x7D, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var w = window.__traceLog().filter(function(x){ return x.name === 'newWidget'; }).length;
  window.__traceCalls([]);
  return { widgets: w, moved: surface().hash !== s0.hash };
}

var out = { settledAfterSeconds: waited };
out.healthy = await press();
if (!out.healthy.widgets) throw new Error('the healthy press built no widgets -- no healthy half');

out.feed = { nit: [window.__siNIT(undefined,{version:71}), window.__siNIT(undefined,{version:72})].map(function(r){return r.ok?'ok':r.why;}) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined,{version:71}), window.__siBAT(undefined,{version:72})].map(function(r){return r.ok?'ok':r.why;});
await new Promise(function(r){ setTimeout(r, 9000); });
out.dead = await press();
if (out.dead.widgets) throw new Error('the repro did not reproduce: ' + out.dead.widgets + ' widgets');

// ---- break at the allocator and read the pool ---------------------------------------------------
var ALLOC = 0x800B4798;
out.armed = window.__breakAt([ALLOC, ALLOC | 1]);
out.samples = [];
for (var s = 0; s < 6; s++) {
  window.__key(0x7D, 0);                       // keep the loop fed so the allocator is reached
  var hit = null;
  for (var i = 0; i < 80; i++) {
    await new Promise(function(r){ setTimeout(r, 250); });
    var rg = window.__regs();
    if (rg.pc === h(ALLOC) || rg.pc === h(ALLOC | 1)) { hit = rg; break; }
  }
  if (!hit) { out.samples.push({ error: 'the break was never taken in 20s' }); break; }
  var pool = parseInt(hit.a0, 16) >>> 0;
  var size = u32(pool + 4), entry = u32(pool + 8);
  out.samples.push({
    pool: h(pool), 'pool[4] size': size, 'pool[8] entrySize': entry,
    n: entry ? Math.floor(size / entry) : 'DIVIDE BY ZERO -- the firmware traps on this',
    'pool[0x20] fallback': h(u32(pool + 0x20)), 'pool[0x40] bitmap': h(u32(pool + 0x40)),
    exceeds256: entry ? (Math.floor(size / entry) > 256) : null,
    // THE BITMAP ITSELF: one bit per slot, so n bits rounded up to a byte. A search that can never
    // succeed is a search over an all-zero bitmap, and that is a 20-byte read rather than a theory.
    bitmap: (function(){
      var bm = u32(pool + 0x40) >>> 0;
      if (bm < 0x80000000 || bm >= 0x82000000) return 'not a DRAM address: ' + h(bm);
      var nbytes = Math.ceil((entry ? Math.floor(size / entry) : 0) / 8) || 20;
      var b = window.__peek(bm, nbytes), set = 0, hex = [];
      for (var k = 0; k < b.length; k++) {
        hex.push(('0' + b[k].toString(16)).slice(-2));
        for (var bit = 0; bit < 8; bit++) if (b[k] & (1 << bit)) set++;
      }
      return { bytes: hex.join(' '), bitsSet: set, ofSlots: entry ? Math.floor(size / entry) : 0 };
    })()
  });
  window.__resume();
  await new Promise(function(r){ setTimeout(r, 400); });
}
window.__breakAt([]);
window.__resume();

var good = out.samples.filter(function(x){ return typeof x.n === 'number'; });
var over = good.filter(function(x){ return x.exceeds256; });
out.verdict = !good.length
  ? 'THE BREAK WAS NEVER TAKEN -- the histogram says this routine runs ten thousand times per dead '
  + 'press, so this is a harness failure (probably the ISA bit or the break comparison) and NOT a '
  + 'statement about the firmware.'
  : over.length === good.length
    ? 'CONFIRMED: every sample has n = ' + good[0].n + ', which EXCEEDS 256. The allocator masks its '
    + 'index to & 0xFF, so it can never reach the slots above 255 and a caller that allocates until '
    + 'it succeeds spins for ever. That is the favchn loop. Find what sized this pool.'
    : over.length
      ? 'MIXED: ' + over.length + ' of ' + good.length + ' samples exceed 256. More than one pool is '
      + 'in play and only some are oversized -- report both and find which one the loop uses.'
      : (function(){
          var bm = good[0].bitmap;
          var allZero = bm && typeof bm === 'object' && bm.bitsSet === 0;
          return 'the & 0xFF mask is REFUTED -- n = ' + good[0].n + ', within 256. '
            + (allZero
                ? 'AND THE BITMAP IS ALL ZEROS (' + bm.ofSlots + ' slots, 0 bits set), so the search '
                + 'cannot succeed on any slot and falls through all ' + good[0].n + ' iterations to '
                + 'the fallback EVERY TIME. A caller that retries would spin -- same end, different '
                + 'route. The next question is who should have set those bits.'
                : (bm && typeof bm === 'object'
                    ? 'The bitmap has ' + bm.bitsSet + ' of ' + bm.ofSlots + ' bits set, so the '
                    + 'search CAN succeed and the allocator is not the thing spinning. Its CALLER is '
                    + 'the next subject.'
                    : 'The bitmap could not be read: ' + bm));
        })();
out.tasksAtEnd = window.__tasks().n;
return out;
