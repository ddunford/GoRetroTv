// WHICH DEVICE DOES A DEAD PRESS TALK TO INSTEAD OF DRAWING? -- sky-02me.18.
//
// WHERE THIS STANDS. The minimal reproduction is NIT then BAT. Diffing the PC histogram of a healthy
// press against two dead ones -- 14,868 PCs that only the healthy press ran, against a noise floor
// of 59, so the signal is not in question -- put the "only in dead" set squarely in the I2C driver:
// 0x800069DA, 0x80006A1E, 0x80006A78 and their neighbourhoods. Decompiling 0x80006A78 confirms what
// it is rather than trusting a comment: a per-bus structure at DAT_80006b10 + bus * 0x48, a
// semaphore, a transfer set up and kicked off. An I2C transaction start.
//
// SO AFTER A PARSED BAT, A KEY PRESS SENDS THE BOX TO THE I2C BUS instead of to the widget builder.
// THIS BOX HAS TWO SLAVES ON IT and they mean completely different things:
//
//   the EEPROM at 0xA0    the box is writing its new service list to /eeprom/svl, and the press is
//                         queued behind a store -- a slow path, not a refusal
//   the DEMODULATOR 0x18  the box is RE-TUNING, because a service list is the first time it knows
//                         which service it is supposed to be on. A press that lands during an
//                         acquisition it will never finish would never draw, and that is a very
//                         different fault with a very different fix
//
// Naming which is the whole of this run, and the page reports both without any new instrumentation:
// __demod() counts reads and register writes, __i2cState() counts EEPROM reads, writes and saves.
//
// CONTROLS:
//   * a healthy press FIRST, with the same counters, so "the dead press did I2C" is a difference
//     rather than an absolute -- an idle box polls the demodulator anyway
//   * two dead presses, so a one-off is not read as the pattern
//   * and the repro is asserted: if the press after NIT+BAT still builds widgets, there is no dead
//     press and the counters below are about nothing

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function devices(){
  var d = window.__demod(), i = window.__i2cState();
  return { demodReads: d.reads, demodRegsWritten: d.writes,
           eeReads: i.eeprom.reads, eeWrites: i.eeprom.writes, eeSaves: i.eeprom.saves,
           eeBytesNotFF: i.eeprom.bytesNotFF, i2cCycles: i.cycles };
}
function diffDevices(a, b){
  var o = {};
  Object.keys(b).forEach(function(k){ o[k] = b[k] - a[k]; });
  return o;
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
// The I2C transaction start named above, plus the driver's read and write entry points from the
// page's own notes -- counted so the traffic can be attributed to a routine and not only to a bus.
var I2C_START = 0x80006A78, I2C_READ = 0x80006961, I2C_WRITE = 0x80006BD5;
function i2cHits(){
  var r = window.__pcHits(I2C_START, I2C_READ, I2C_WRITE);
  [I2C_START, I2C_READ, I2C_WRITE].forEach(function(a){
    if (r[h(a)] === undefined)
      throw new Error('__pcHits did not return ' + h(a) + ' -- a harness failure, not a zero');
  });
  return { start: r[h(I2C_START)], read: r[h(I2C_READ)], write: r[h(I2C_WRITE)] };
}

async function press(raw, label){
  window.__traceCalls(TRACE); window.__traceClear();
  var d0 = devices(), i0 = i2cHits(), s0 = surface();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var log = window.__traceLog();
  window.__traceCalls([]);
  var d1 = devices(), i1 = i2cHits();
  return { label: label,
           widgets: log.filter(function(e){ return e.name === 'newWidget'; }).length,
           moved: surface().hash !== s0.hash,
           devices: diffDevices(d0, d1),
           i2cRoutines: { start: i1.start - i0.start, read: i1.read - i0.read, write: i1.write - i0.write } };
}

var out = { question: 'does a dead press go to the EEPROM or to the demodulator',
            settledAfterSeconds: waited, devicesAtRest: devices() };

out.healthy = await press(0x7D, 'sky, healthy');
if (!out.healthy.widgets)
  throw new Error('the healthy press built no widgets -- there is no healthy half to compare against');

out.feed = { nit: [window.__siNIT(undefined, { version: 31 }), window.__siNIT(undefined, { version: 32 })]
                    .map(function(r){ return r.ok ? 'ok' : r.why; }) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined, { version: 31 }), window.__siBAT(undefined, { version: 32 })]
                 .map(function(r){ return r.ok ? 'ok' : r.why; });
// What the FEED itself did to the devices, separately from any press: a service list being written
// to the EEPROM would show here rather than on the press.
var dFeed0 = out.devicesAtRest;
await new Promise(function(r){ setTimeout(r, 9000); });
out.feedDidToDevices = diffDevices(dFeed0, devices());

out.dead = [ await press(0x7D, 'sky, after NIT+BAT'), await press(0x7D, 'sky, dead again') ];
if (out.dead[0].widgets)
  throw new Error('the press after NIT+BAT still built ' + out.dead[0].widgets + ' widgets -- the '
                + 'repro did not reproduce and the counters are about nothing');

// ---- attribute it ---------------------------------------------------------------------------
var h0 = out.healthy.devices, d0 = out.dead[0].devices, d1 = out.dead[1].devices;
var demodUp = (d0.demodReads > h0.demodReads) && (d1.demodReads > h0.demodReads);
var eeUp    = (d0.eeReads + d0.eeWrites > h0.eeReads + h0.eeWrites)
           && (d1.eeReads + d1.eeWrites > h0.eeReads + h0.eeWrites);
out.verdict =
    demodUp && !eeUp ? 'THE DEMODULATOR. A dead press drives the tuner and not the EEPROM, so the '
      + 'box is RE-TUNING -- a service list is the first time it knows which service it should be '
      + 'on, and a press landing inside an acquisition it never finishes would never draw. That is '
      + 'a different fault from a slow store, and the next subject is what it is tuning to and why '
      + 'that never completes.'
  : eeUp && !demodUp ? 'THE EEPROM. A dead press drives the store and not the tuner, so the press is '
      + 'queued behind the service list being written to /eeprom/svl. That is a slow path rather '
      + 'than a refusal, and the next question is why it never finishes.'
  : demodUp && eeUp ? 'BOTH went up on both dead presses -- the bus is busier and this cannot '
      + 'separate the two slaves. Count transactions per SLAVE ADDRESS rather than per device API.'
  : 'NEITHER went up on both dead presses. The I2C addresses in the histogram diff are real, so '
      + 'either the traffic is to a slave neither counter watches, or it happens on the FEED rather '
      + 'than on the press -- feedDidToDevices is where to look.';
out.tasksAtEnd = window.__tasks().n;
return out;
