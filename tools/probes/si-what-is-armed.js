// WHAT IS THIS BOX ACTUALLY ASKING FOR, AND WHEN? -- sky-eluc.12's precondition.
//
// which-descriptors.js refused to run because no section filter matched table 0x4A: the box was
// not asking for a BAT, so a pushed one would never have been delivered and every zero would have
// been about delivery. __siIds()'s own comment records the opposite -- "once acquisition starts
// this box filters 0x40 ext 0x0020, 0x42 ext 0x0020 and 0x4A ext 0x1000" -- so either acquisition
// does not start on a plain boot, or it starts later than the settle, or something else arms it.
//
// ONE OBSERVATION OF A MACHINE IS NOT A STATE, and a single reading after the settle cannot tell
// "never" from "not yet". So this samples the match table and the filter table on a curve, and
// reports the whole curve rather than a verdict. Run it BOTH WAYS -- plain and with --carousel --
// because the difference between them is the question: a box scanning a live multiplex has a
// reason to arm filters that a box on a dead one does not, and the earlier measurement was taken
// on a run that had the carousel on.
//
// THE POSITIVE CONTROL IS THE FILTERS WE KNOW EXIST. The demux is programmed for PIDs 0x10, 0x11
// and 0x14 by the end of every boot -- the gate asserts exactly that -- so if this probe sees no
// filters at all, it is the instrument that is wrong and not the box.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }

var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 180) {
  await new Promise(function(r){ setTimeout(r, 1000); });
  waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

function ic(){ return parseInt(document.getElementById('icount').textContent.replace(/,/g, ''), 10); }
function snap(){
  var m = window.__siMatches(), f = window.__siFilters();
  return {
    icountM: +(ic() / 1e6).toFixed(0),
    pids: f.map(function(x){ return x.pid + (x.armed ? '' : '(idle)'); }).join(' '),
    tables: m.map(function(x){
      return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
           + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : '')
           + (x.extensionMask !== null && x.extensionMask !== 0xFFFF
                ? '&0x' + x.extensionMask.toString(16) : '');
    }).join(' '),
    wantsBat: m.some(function(x){ return x.tableId === 0x4A; }),
    carousel: (function(){ var c = window.__siCarousel(); return c.running === false ? 'off' : ('sent ' + c.sent + ' refused ' + c.refused); })()
  };
}

// The demodulator is the thing acquisition is gated on: register 11 must answer (v & 0x3F) == 0x3F,
// and the mock is always locked. Read counts rather than assume it -- a demod nobody has addressed
// is a front end nobody is driving, which would explain a box that never scans.
var dem = window.__demod();

var curve = [snap()];
var SAMPLES = 6, EVERY = 12000;
for (var i = 0; i < SAMPLES; i++) {
  await new Promise(function(r){ setTimeout(r, EVERY); });
  curve.push(snap());
}

var demAfter = window.__demod();
var everWanted = curve.some(function(c){ return c.wantsBat; });

if (!curve[0].pids)
  throw new Error('no section filters at all -- the demux should carry 0x10, 0x11 and 0x14 by the '
                + 'end of every boot, so this is the instrument failing and not the box');

return {
  question: 'does this box ever arm a section filter for table 0x4A (the BAT), and when',
  settledAfterSeconds: waited,
  carouselFlag: 'pass --carousel to ctl.sh digibox:probe to run the other half of this comparison',
  demod: { readsAtStart: dem.reads, readsAtEnd: demAfter.reads,
           registersWritten: demAfter.writes,
           note: 'the mock is always locked; these counts say whether anyone is driving the front end' },
  everWantedBat: everWanted,
  ids: window.__siIds(),
  curve: curve
};
