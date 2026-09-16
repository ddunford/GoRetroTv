// WHICH TASK IS STUCK AFTER THE LOOP FINISHES? -- sky-02me.18.
//
// THE LOOP IS NOT INFINITE, and that correction matters more than anything else found today.
// Watched for six minutes with a real press every fourteen seconds:
//
//     t(s)   widgets  bitsSet  eeWrites  eeSaves
//       14         0      117       632       57
//       57         0       88      3342      234
//      113         0       56      6891      461
//      127         0       56      6895      462     <- everything freezes here
//      340         0       56      6895      462     <- and stays frozen for 227 more seconds
//
// It runs for about 113 seconds, consumes 104 of the 160 pool slots, writes 6,895 EEPROM bytes
// across 462 saves, AND THEN STOPS. The pool is not exhausted -- 56 slots remain. After that the box
// is quiescent and still will not build a single widget, with the surface parked on the 37-colour
// menu the healthy press drew.
//
// SO IT IS NOT A BUSY LOOP STARVING THE DRAWING. Something completes, and leaves the box in a state
// where a key press builds nothing. That is a blocked or waiting task, not a spin -- and this
// project's own note says reading a blocked task's stack for return addresses is the fastest
// diagnosis available here.
//
// SO: census the task list healthy, break the box, wait for the freeze, census again, and diff. A
// task whose STATUS changed is the answer; its stack says what it is waiting for.
// Nucleus status: 0 ready, 2 sleep, 3 mailbox, 4 queue, 5 pipe, 6 semaphore, 7 event.
//
// THE FREEZE IS DETECTED, NOT TIMED. Waiting a fixed three minutes would be the same mistake as
// waiting nine seconds: the census must be taken once the EEPROM counters have genuinely stopped,
// and the probe says how long that took rather than assuming it.
//
// CONTROLS: the healthy press must draw; the repro must reproduce; and the census is taken TWICE
// after the freeze, because a task that is merely slow will differ between the two while a blocked
// one will not.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function u32(a){ var b = window.__peek(a >>> 0, 4); return ((b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3]) >>> 0; }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function ee(){ var i = window.__i2cState(); return i.eeprom.writes + i.eeprom.saves; }
function census(){
  var t = window.__tasks();
  if (t.error) return { error: t.error };
  var by = {};
  t.tasks.forEach(function(x){ by[x.name] = { status: x.status, st: x.st, runs: x.runs, sp: x.sp, freeMin: x.freeMin }; });
  return by;
}
// The call chain of a task that is not running: scan its saved stack for return addresses. Taken
// from the saved stack pointer upward, unfiltered, because a reader checks them against the code.
function chain(sp){
  var a = parseInt(sp, 16) >>> 0, out = [];
  if (a < 0x80000000 || a >= 0x82000000) return ['sp not in DRAM: ' + sp];
  for (var o = 0; o < 320 && out.length < 14; o += 4) {
    var w = u32(a + o);
    if (w >= 0x80000000 && w < 0x80120000 && (w & 1)) out.push(h(w));   // odd = a MIPS16 return
  }
  return out;
}

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
  return { widgets: w, moved: surface().hash !== s0.hash, colours: surface().colours };
}

var out = { settledAfterSeconds: waited };
out.healthy = await press(0x7D);
if (!out.healthy.widgets) throw new Error('the healthy press built no widgets -- no healthy half');
out.censusHealthy = census();

out.feed = { nit: [window.__siNIT(undefined,{version:91}), window.__siNIT(undefined,{version:92})].map(function(r){return r.ok?'ok':r.why;}) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined,{version:91}), window.__siBAT(undefined,{version:92})].map(function(r){return r.ok?'ok':r.why;});
await new Promise(function(r){ setTimeout(r, 9000); });
out.dead = await press(0x7D);
if (out.dead.widgets) throw new Error('the repro did not reproduce: ' + out.dead.widgets + ' widgets');

// ---- wait for the EEPROM work to genuinely stop, rather than for a guessed number of seconds ----
var t0 = Date.now(), last = ee(), still = 0, froze = null;
for (var i = 0; i < 90; i++) {
  await new Promise(function(r){ setTimeout(r, 5000); });
  var now = ee();
  still = (now === last) ? still + 1 : 0;
  last = now;
  if (still >= 3) { froze = Math.round((Date.now() - t0) / 1000); break; }
}
out.frozeAfterSeconds = froze;
out.eeTotalAtFreeze = last;
if (froze === null) {
  out.verdict = 'the EEPROM work never stopped within 450s, so the freeze seen at 113s in the '
              + 'previous run did not happen here -- the census below would be of a box still '
              + 'working and is not taken.';
  return out;
}

out.pressAfterFreeze = await press(0x7D);
out.censusStuck = census();
await new Promise(function(r){ setTimeout(r, 8000); });
out.censusStuckAgain = census();

// ---- the diff -----------------------------------------------------------------------------------
var moved = [], changedStatus = [];
Object.keys(out.censusStuck).forEach(function(name){
  var a = out.censusHealthy[name], b = out.censusStuck[name], c = out.censusStuckAgain[name];
  if (!a || !b) return;
  if (a.st !== b.st) changedStatus.push({ task: name, was: a.status, now: b.status,
                                          runs: a.runs + ' -> ' + b.runs,
                                          stillRunning: c && c.runs !== b.runs });
  if (c && c.runs !== b.runs) moved.push(name);
});
out.tasksWhoseStatusChanged = changedStatus;
out.tasksStillBeingScheduled = moved;

// For every task that changed status and is NOT still being scheduled, read its stack. That is the
// blocked one, and the chain says what it is waiting on.
out.stacks = changedStatus.filter(function(x){ return !x.stillRunning; }).map(function(x){
  return { task: x.task, waitingIn: x.now, sp: out.censusStuck[x.task].sp,
           returnAddresses: chain(out.censusStuck[x.task].sp) };
});

out.verdict = !changedStatus.length
  ? 'NO TASK CHANGED STATUS between healthy and stuck. The refusal is not a blocked task at all, '
  + 'which rules out the whole family and points back at the application choosing not to build.'
  : out.stacks.length
    ? 'STUCK TASK(S): ' + out.stacks.map(function(s){ return s.task + ' in ' + s.waitingIn; }).join(', ')
    + '. The return addresses are the call chain that got there -- check them against the code and '
    + 'the top one is where it is waiting.'
    : 'tasks changed status but all of them are still being scheduled, so nothing is BLOCKED -- they '
    + 'are cycling through a wait rather than stuck in one, and the chain below is a snapshot rather '
    + 'than a resting place.';
out.tasksAtEnd = window.__tasks().n;
return out;
