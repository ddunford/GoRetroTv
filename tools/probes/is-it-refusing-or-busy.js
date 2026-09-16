// IS THE BOX REFUSING TO DRAW, OR IS IT STILL BUSY? -- sky-02me.18, and the question every
// measurement so far has quietly assumed the answer to.
//
// WHAT THE COUNTERS SAID. After a parsed BAT, a key press sends the box to the EEPROM:
//
//     press      demodReads  eeReads  eeWrites  eeSaves  i2cCycles  i2cStarts
//     healthy            84        0         0        0        454          0
//     dead 1            126      288       284       31       1517         62
//     dead 2            140      292       288       30       1590         60
//
// A healthy press does ZERO EEPROM work. A dead one does about 570 operations and thirty whole-
// device saves. The demodulator rises too, but it is polled continuously regardless and the rise is
// proportionally small; the EEPROM goes from nothing to hundreds. That is the box persisting its new
// service list -- the SI -> SVL builder -> /eeprom/svl chain sky-eluc.12 documented.
//
// AND EVERY PRESS THIS PROJECT HAS MEASURED WAITED NINE SECONDS. That is an assumption wearing the
// clothes of a measurement: "no widgets after nine seconds" has been read as "it refuses to draw",
// and "it is still writing" fits the same observation exactly. The EEPROM is a 24C128 on a modelled
// I2C bus with modelled timing, so hundreds of transactions are not free.
//
// So: press, and then WATCH. If the widgets arrive at forty seconds the bug is that our EEPROM is
// slow, the fix is on our side of the line, and nothing is refusing anything. If two minutes pass
// with the EEPROM quiet and no widgets, it is a genuine refusal and the patience question is closed
// for good.
//
// THE EEPROM COUNTERS ARE SAMPLED ALONGSIDE, because "still busy" and "gave up" are distinguishable
// only by whether the traffic is still moving. A flat counter with no widgets is a refusal; a
// climbing counter with no widgets is patience.
//
// CONTROLS: a healthy press first, timed the same way, so the normal arrival time of the widgets is
// measured rather than assumed -- and the repro is asserted before the long wait, or the wait is of
// a box that was never broken.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
function ee(){ var i = window.__i2cState();
               return { reads: i.eeprom.reads, writes: i.eeprom.writes, saves: i.eeprom.saves,
                        cycles: i.cycles }; }
function icount(){ return parseInt(document.getElementById('icount').textContent.replace(/,/g, ''), 10); }

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

// Press, then sample on a curve until the widgets arrive or the patience runs out. The curve is the
// measurement: a flat EEPROM with no widgets is a refusal, a climbing one is patience.
async function pressAndWatch(raw, label, seconds){
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface(), e0 = ee(), t0 = Date.now(), ic0 = icount();
  window.__key(raw, 0);
  var samples = [], drewAt = null;
  for (var t = 0; t < seconds; t += 5) {
    await new Promise(function(r){ setTimeout(r, 5000); });
    var log = window.__traceLog();
    var w = log.filter(function(x){ return x.name === 'newWidget'; }).length;
    var e = ee(), s = surface();
    samples.push({ atSeconds: +((Date.now() - t0) / 1000).toFixed(0), widgets: w,
                   eeReads: e.reads - e0.reads, eeWrites: e.writes - e0.writes,
                   eeSaves: e.saves - e0.saves,
                   moved: s.hash !== s0.hash,
                   millions: +((icount() - ic0) / 1e6).toFixed(0) });
    if (w > 0 && drewAt === null) drewAt = samples[samples.length - 1].atSeconds;
    if (w > 0 && s.hash !== s0.hash) break;
  }
  window.__traceCalls([]);
  var last = samples[samples.length - 1];
  return { label: label, drewAtSeconds: drewAt, widgets: last.widgets, moved: last.moved,
           eeTotal: { reads: last.eeReads, writes: last.eeWrites, saves: last.eeSaves },
           // Was the EEPROM still moving at the end? That is what separates busy from refusing.
           eeStillMoving: samples.length > 1
             && (last.eeReads > samples[samples.length - 2].eeReads
              || last.eeWrites > samples[samples.length - 2].eeWrites),
           samples: samples };
}

var out = { question: 'is the box refusing to draw, or still writing its service list',
            settledAfterSeconds: waited };

// The healthy press, timed the same way, so the normal arrival time is measured and not assumed.
out.healthy = await pressAndWatch(0x7D, 'sky, healthy', 30);
if (!out.healthy.widgets)
  throw new Error('the healthy press built no widgets in 30s -- there is no healthy half');

out.feed = { nit: [window.__siNIT(undefined, { version: 41 }), window.__siNIT(undefined, { version: 42 })]
                    .map(function(r){ return r.ok ? 'ok' : r.why; }) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined, { version: 41 }), window.__siBAT(undefined, { version: 42 })]
                 .map(function(r){ return r.ok ? 'ok' : r.why; });
await new Promise(function(r){ setTimeout(r, 9000); });

// TWO MINUTES. Thirteen times the wait every previous measurement used.
out.afterFeed = await pressAndWatch(0x7D, 'sky, after NIT+BAT -- watched for two minutes', 120);

out.verdict = out.afterFeed.drewAtSeconds !== null
  ? 'IT WAS BUSY, NOT REFUSING. The widgets arrived at ' + out.afterFeed.drewAtSeconds + 's, against '
    + out.healthy.drewAtSeconds + 's on a healthy box. Every earlier measurement waited nine seconds '
    + 'and read "no widgets yet" as "it will not draw". sky-02me.18 is a SPEED problem in our EEPROM '
    + 'model, not a logic fault in the firmware, and the fix is on our side.'
  : (out.afterFeed.eeStillMoving
      ? 'STILL WRITING AFTER TWO MINUTES and no widgets. The EEPROM counters were still climbing at '
        + 'the end, so it has not given up -- it is slower than two minutes. Either the model is far '
        + 'too slow or something is making it rewrite the same thing for ever; the save count is '
        + 'where to look (' + out.afterFeed.eeTotal.saves + ' whole-device saves).'
      : 'A GENUINE REFUSAL. Two minutes, no widgets, and the EEPROM had STOPPED moving by the end -- '
        + 'so the box finished whatever it was doing and still will not draw. The patience question '
        + 'is closed and the refusal is real.');
out.tasksAtEnd = window.__tasks().n;
return out;
