// WHAT RUNS ON A HEALTHY PRESS THAT DOES NOT RUN ON A DEAD ONE? -- sky-02me.18.
//
// WHERE THIS STANDS. The minimal reproduction is NIT then BAT: a bare box subscribes to 0x40 and
// 0x73 only, the NIT opens 0x4A/ext=0x1000, and a BAT the box then actually PARSES costs it the
// ability to build any screen. The key still reaches the input layer -- keyEvents 2 on every dead
// press -- and zero widgets are built. Both of the gates skyGatesTick() answers have been ruled
// out: nothing writes the DRAM one across the BAT and re-answering it restores nothing, and the
// other is a flash byte the application never writes.
//
// SO STOP GUESSING WHICH FLAG IT IS AND ASK THE MACHINE WHERE IT DIVERGES. A healthy press and a
// dead press start from the same input event and end in different places. Whatever the healthy one
// executes and the dead one never touches contains the decision -- and finding it needs no prior
// idea of what the decision is, which is the point after two hypotheses have already been wrong.
//
// THE HISTOGRAM HAS NO CLEAR FUNCTION, so it is DIFFERENCED. __rangeHits reports a cumulative count
// per PC; snapshotting before and after a press and subtracting gives what that press ran. The
// `top` argument only slices, so asking for a very large one returns every PC in range -- which is
// what a diff needs, because a press-specific path will never appear in the top of a histogram
// dominated by the boot and the idle loop.
//
// THE DIFF IS COMPUTED IN THE PAGE and only the summary comes back. Two snapshots of tens of
// thousands of entries do not need to cross the bridge.
//
// CONTROLS:
//   * the healthy press must build widgets, asserted -- otherwise there is no "healthy" half
//   * the dead press must build none, asserted -- otherwise the reproduction did not reproduce and
//     the diff is between two healthy presses, which would be noise presented as a finding
//   * the same key is used for both, from the same screen, so the diff is about the box's state and
//     not about two different screens
//   * and a THIRD press, on the dead box, is diffed against the second: two dead presses should
//     look alike, and anything that shows up there is the instrument's own noise floor rather than
//     the divergence. Without it a busy background task would read as the answer.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
var APP_LO = 0x80000000, APP_HI = 0x80120000;

function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function snapshot(){
  var m = new Map();
  window.__rangeHits(APP_LO, APP_HI, 1000000).hottest.forEach(function(e){ m.set(e[0], e[1]); });
  return m;
}
function delta(before, after){
  var d = new Map();
  after.forEach(function(n, a){
    var was = before.get(a) || 0;
    if (n > was) d.set(a, n - was);
  });
  return d;
}

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 240) {
  await new Promise(function(r){ setTimeout(r, 1000); });
  waited++;
}
if (!/^Ready/.test(document.getElementById('boxstate-t').textContent))
  throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
if (window.__siCarousel().running !== false)
  throw new Error('the carousel is RUNNING -- this probe feeds two tables by hand');

var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
var KEYEVENT = 0x8006EA04;
function keyHits(){
  var r = window.__pcHits(KEYEVENT);
  if (r[h(KEYEVENT)] === undefined)
    throw new Error('__pcHits did not return the key it was asked for -- a harness failure, not a zero');
  return r[h(KEYEVENT)];
}
async function pressAndProfile(raw, label){
  window.__traceCalls(TRACE); window.__traceClear();
  var k0 = keyHits(), s0 = surface(), before = snapshot();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var after = snapshot();
  var log = window.__traceLog(), widgets = log.filter(function(e){ return e.name === 'newWidget'; }).length;
  window.__traceCalls([]);
  return { label: label, widgets: widgets, keyEvents: keyHits() - k0,
           moved: surface().hash !== s0.hash, ran: delta(before, after) };
}

var out = { question: 'which code a healthy press runs that a dead press never touches',
            settledAfterSeconds: waited };

// ---- the healthy press ------------------------------------------------------------------------
var healthy = await pressAndProfile(0x7D, 'sky, healthy');
if (!healthy.widgets)
  throw new Error('the healthy press built no widgets -- there is no healthy half, so a diff would '
                + 'be between two dead presses. ' + JSON.stringify({ widgets: healthy.widgets }));

// ---- break it: NIT then BAT, the minimal pair ---------------------------------------------------
out.feed = { nit: [window.__siNIT(undefined, { version: 21 }), window.__siNIT(undefined, { version: 22 })]
                    .map(function(r){ return r.ok ? 'ok' : r.why; }) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined, { version: 21 }), window.__siBAT(undefined, { version: 22 })]
                 .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 9000); });

// ---- the dead press, and a second dead press as the noise floor ---------------------------------
var dead  = await pressAndProfile(0x7D, 'sky, after NIT+BAT');
var dead2 = await pressAndProfile(0x7D, 'sky, dead again -- the noise floor');
if (dead.widgets)
  throw new Error('the press after NIT+BAT still built ' + dead.widgets + ' widgets -- the repro did '
                + 'not reproduce, and the diff would be between two healthy presses, which is noise '
                + 'presented as a finding.');

out.presses = [healthy, dead, dead2].map(function(p){
  return { label: p.label, widgets: p.widgets, keyEvents: p.keyEvents, moved: p.moved,
           distinctPCs: p.ran.size,
           instructions: (function(){ var n = 0; p.ran.forEach(function(v){ n += v; }); return n; })() };
});

// ---- the diff ------------------------------------------------------------------------------------
// ONLY IN HEALTHY: ran during the healthy press and never during EITHER dead press. Requiring it to
// be absent from both is what keeps a one-off background task out of the answer.
var onlyHealthy = [];
healthy.ran.forEach(function(n, a){
  if (!dead.ran.has(a) && !dead2.ran.has(a)) onlyHealthy.push([a, n]);
});
onlyHealthy.sort(function(x, y){ return parseInt(x[0], 16) - parseInt(y[0], 16); });

// ONLY IN DEAD: ran on BOTH dead presses and never on the healthy one -- what it does instead.
var onlyDead = [];
dead.ran.forEach(function(n, a){
  if (dead2.ran.has(a) && !healthy.ran.has(a)) onlyDead.push([a, n]);
});
onlyDead.sort(function(x, y){ return parseInt(x[0], 16) - parseInt(y[0], 16); });

// The noise floor: present in one dead press and not the other. If this is large the diff above is
// not trustworthy, and saying so is the difference between a measurement and a story.
var deadOnlyOnce = 0;
dead.ran.forEach(function(n, a){ if (!dead2.ran.has(a)) deadOnlyOnce++; });

out.diff = {
  onlyInHealthy: onlyHealthy.length,
  onlyInDead: onlyDead.length,
  noiseFloor: deadOnlyOnce,
  note: 'noiseFloor is how many PCs ran in one dead press and not the other. If it is comparable to '
      + 'onlyInHealthy then the diff is noise and nothing below is a finding.',
  // Address-ordered, because the FIRST divergence is what matters and a count sort hides it.
  onlyInHealthyByAddress: onlyHealthy.slice(0, 60).map(function(e){ return e[0] + ' x' + e[1]; }),
  onlyInHealthyHottest: onlyHealthy.slice().sort(function(x, y){ return y[1] - x[1]; })
                          .slice(0, 20).map(function(e){ return e[0] + ' x' + e[1]; }),
  onlyInDeadByAddress: onlyDead.slice(0, 40).map(function(e){ return e[0] + ' x' + e[1]; }),
  // THE I2C DRIVER IS KNOWN AND IS NOT THE ANSWER. 0x80006000-0x80007000 is the driver whose
  // transactions are already characterised; what is wanted is whoever CALLS it round the loop. Two
  // attempts to get that from the transaction log failed -- the STOP runs in the completion
  // interrupt and the arm-START inside the driver, and both gave ra=0 with an unreadable stack,
  // because the task that asked for the write is blocked on a semaphore and its frame is not
  // reachable from either. The histogram does not care: code that runs ONLY when the box is dead,
  // and is not the driver, is the loop.
  onlyInDeadOutsideTheI2cDriver: onlyDead.filter(function(e){
      var a = parseInt(e[0], 16) >>> 0;
      return !(a >= 0x80006000 && a < 0x80007000);
    }).sort(function(x, y){ return y[1] - x[1]; }).slice(0, 30)
      .map(function(e){ return e[0] + ' x' + e[1]; }),
  onlyInDeadOutsideDriverCount: onlyDead.filter(function(e){
      var a = parseInt(e[0], 16) >>> 0;
      return !(a >= 0x80006000 && a < 0x80007000);
    }).length,
  // GROUPED BY 0x100, because a flat address-ordered list of 1,961 PCs is one function repeated and
  // says nothing about the CHAIN. A region with a few dozen distinct PCs and a high total is a
  // routine that ran; the set of them is the call path the dead press takes instead of drawing.
  onlyInDeadByRegion: (function(){
    var reg = {};
    onlyDead.forEach(function(e){
      var a = parseInt(e[0], 16) >>> 0;
      if (a >= 0x80006000 && a < 0x80007000) return;     // the known I2C driver
      var k = h(a & 0xFFFFFF00);
      reg[k] = reg[k] || { hits: 0, pcs: 0 };
      reg[k].hits += e[1]; reg[k].pcs++;
    });
    return Object.keys(reg).map(function(k){ return { at: k, hits: reg[k].hits, distinctPCs: reg[k].pcs }; })
      .sort(function(x, y){ return y.hits - x.hits; }).slice(0, 25);
  })(),
  // And the same for the healthy side, so the two paths can be read against each other.
  onlyInHealthyByRegion: (function(){
    var reg = {};
    onlyHealthy.forEach(function(e){
      var a = parseInt(e[0], 16) >>> 0;
      var k = h(a & 0xFFFFFF00);
      reg[k] = reg[k] || { hits: 0, pcs: 0 };
      reg[k].hits += e[1]; reg[k].pcs++;
    });
    return Object.keys(reg).map(function(k){ return { at: k, hits: reg[k].hits, distinctPCs: reg[k].pcs }; })
      .sort(function(x, y){ return y.hits - x.hits; }).slice(0, 15);
  })()
};
out.verdict = out.diff.noiseFloor >= out.diff.onlyInHealthy
  ? 'THE NOISE FLOOR IS AS BIG AS THE SIGNAL -- two dead presses differ from each other about as '
  + 'much as healthy differs from dead, so this diff cannot separate them. A longer settle or a '
  + 'quieter box is needed before the lists mean anything.'
  : 'the divergence is ' + out.diff.onlyInHealthy + ' PCs that the healthy press ran and neither '
  + 'dead press touched, against a noise floor of ' + out.diff.noiseFloor
  + '. The lowest addresses in onlyInHealthyByAddress are where to look first.';
out.tasksAtEnd = window.__tasks().n;
return out;
