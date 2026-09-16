// WHAT IS THE EEPROM LOOP ACTUALLY WRITING? -- sky-02me.18, the last question before a fix.
//
// ESTABLISHED. After a parsed BAT, a key press puts the box into a non-terminating read-modify-write
// loop on the EEPROM. Watched for two minutes:
//
//     t(s)   eeReads  eeWrites  saves     rate
//        5       161       167     18     ~33/s
//       62      2592      2590    214     ~42/s
//      124      6179      6175    440     ~50/s   still climbing, perfectly linear
//
// Reads track writes to within half a percent at every sample, 440 whole-device saves, 299 million
// instructions burned, zero widgets. A healthy press does ZERO EEPROM work and draws in five
// seconds. So it is not refusing and it is not merely slow: it writes, reads back, and goes round
// again.
//
// THAT SHAPE HAS ONE OBVIOUS CAUSE AND THIS ASKS WHETHER IT IS THE RIGHT ONE: the driver writes a
// record, reads it back to verify, gets something other than what it wrote, and retries for ever.
// If so the bug is in OUR 24C128 model -- our side of the line, and a much more tractable fault
// than an application that will not draw.
//
// __eeTx() RECORDS EVERY TRANSACTION, logged at the I2C STOP with its start address, its length and
// its bytes -- AND IT CAPS AT 400, which the boot alone fills. The first run of this probe read it
// after the press and got `loggedDuringThisPress: 0` against a full log: the FIRST 400 transactions
// of the run, the boot's, presented as a record of what had just happened. It said so only because
// it compared the length before and after, which is the difference between a measurement and a
// plausible story. __eeTxClear() was added to the page for exactly this, and is called immediately
// before the press so the window is explicit.
//
// WHAT THE ANSWER LOOKS LIKE, so that the reading is not invented afterwards:
//   the same address written over and over with the same bytes   -> the read-back does not match,
//                                                                   and the model's write is wrong
//   the same address written with DIFFERENT bytes each time      -> two writers fighting, or a
//                                                                   pointer that never advances
//   addresses marching upwards and wrapping                      -> the driver is filling the device
//                                                                   and our size or wrap is wrong
//
// CONTROLS: the healthy press must draw and must log no EEPROM traffic at all, and the repro must
// reproduce -- otherwise the log is of something else entirely.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
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
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');

var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
function eeCounts(){ var i = window.__i2cState();
                     return { reads: i.eeprom.reads, writes: i.eeprom.writes, saves: i.eeprom.saves }; }

var out = { question: 'what the non-terminating EEPROM loop is writing', settledAfterSeconds: waited };

// ---- the healthy control ---------------------------------------------------------------------
window.__traceCalls(TRACE); window.__traceClear();
var e0 = eeCounts(), s0 = surface();
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 9000); });
out.healthy = { widgets: window.__traceLog().filter(function(x){ return x.name === 'newWidget'; }).length,
                moved: surface().hash !== s0.hash,
                ee: (function(){ var e = eeCounts();
                      return { reads: e.reads - e0.reads, writes: e.writes - e0.writes,
                               saves: e.saves - e0.saves }; })() };
window.__traceCalls([]);
if (!out.healthy.widgets) throw new Error('the healthy press built no widgets -- no healthy half');

// ---- break it ----------------------------------------------------------------------------------
out.feed = { nit: [window.__siNIT(undefined, { version: 51 }), window.__siNIT(undefined, { version: 52 })]
                    .map(function(r){ return r.ok ? 'ok' : r.why; }) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined, { version: 51 }), window.__siBAT(undefined, { version: 52 })]
                 .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 9000); });

// ---- press, let the loop run, then read the wire ------------------------------------------------
window.__traceCalls(TRACE); window.__traceClear();
if (typeof window.__eeTxClear !== 'function')
  throw new Error('this page has no __eeTxClear -- the transaction log caps at 400 and the boot '
                + 'fills it, so the log read afterwards would be the boot\'s and not the loop\'s');
var cleared = window.__eeTxClear();
var e1 = eeCounts(), s1 = surface(), txBefore = 0;
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 25000); });
var e2 = eeCounts();
out.deadPress = { widgets: window.__traceLog().filter(function(x){ return x.name === 'newWidget'; }).length,
                  moved: surface().hash !== s1.hash,
                  ee: { reads: e2.reads - e1.reads, writes: e2.writes - e1.writes,
                        saves: e2.saves - e1.saves } };
window.__traceCalls([]);
if (out.deadPress.widgets)
  throw new Error('the press after NIT+BAT built ' + out.deadPress.widgets + ' widgets -- the repro '
                + 'did not reproduce, and the log below is of a healthy box');

// THE WIRE ITSELF. __eeTx logs at the I2C STOP: {icount, at, n, bytes}.
var tx = window.__eeTx();
out.transactions = { clearedBeforePress: cleared, totalLogged: tx.length,
                     loggedDuringThisPress: tx.length - txBefore,
                     capped: tx.length >= 400 ? 'THE LOG IS FULL (400) even after clearing -- these '
                             + 'are the first 400 transactions OF THE LOOP, which is what was '
                             + 'wanted, but it did not run to completion within the window' : false };
var recent = tx.slice(-24);
out.lastTransactions = recent.map(function(x){ return x.at + ' n=' + x.n + '  ' + x.bytes; });

// Which addresses does it keep going back to, and is it writing the SAME bytes each time?
var byAddr = {};
tx.slice(-200).forEach(function(x){
  var k = x.at;
  byAddr[k] = byAddr[k] || { count: 0, distinctPayloads: {} };
  byAddr[k].count++;
  byAddr[k].distinctPayloads[x.bytes] = 1;
});
out.hotAddresses = Object.keys(byAddr).map(function(k){
  return { at: k, times: byAddr[k].count, distinctPayloads: Object.keys(byAddr[k].distinctPayloads).length };
}).sort(function(a, b){ return b.times - a.times; }).slice(0, 12);

var top = out.hotAddresses[0];
out.reading = !top ? 'no EEPROM transactions were logged at all, which contradicts the counters'
  : top.times < 3 ? 'no address is revisited -- the writes are marching through the device rather '
      + 'than retrying one record. Check the size and the page wrap in our 24C128 model.'
  : top.distinctPayloads === 1 ? 'ONE ADDRESS, ONE PAYLOAD, written ' + top.times + ' times: the '
      + 'driver writes the same bytes over and over, which is a read-back that never matches. The '
      + 'bug is in what our model RETURNS on the verifying read.'
  : 'ONE ADDRESS, ' + top.distinctPayloads + ' DIFFERENT payloads over ' + top.times + ' writes: two '
      + 'writers are fighting over the same record, or a pointer never advances. Not a read-back '
      + 'mismatch.';
out.tasksAtEnd = window.__tasks().n;
return out;
