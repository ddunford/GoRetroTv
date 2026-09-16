// sky-02me.5 -- TWELVE CHANNELS, EACH WITH ITS OWN 24-HOUR DAY, SCROLLED ON THE GUIDE.
//
// WHAT IS BEING ASKED. The page now broadcasts a twelve-channel line-up in which the NIT, the SDT
// and the BAT all declare the same services -- built from one DEMO_LINEUP rather than three
// hardcoded lists -- and the carousel sends each channel its OWN day of twelve programmes. This
// walks the guide down the line-up and photographs every channel, which is the only way to tell
// "the listings arrived" from "one channel's listings arrived and the rest are the same bytes".
//
// EACH CHANNEL MUST SHOW DIFFERENT PROGRAMMES, and that is asserted rather than admired: the
// surface hash is taken at every step and a scroll that produces a hash already seen is reported.
// Twelve channels drawn from twelve distinct days cannot collide, so a repeat means either the
// scroll did nothing or the channel is reading someone else's records -- and those are different
// faults with the same screenshot.
//
// THE GUIDE REGISTERS ITS NOTIFICATION SLOT WHEN IT IS OPENED, so the first press subscribes and
// the next carousel wave fills it. That is why this opens the guide, waits, and only then starts
// scrolling -- pressing down immediately measures a guide that has not been told anything yet.
//
// DOWN IS RAW 0x59 AND UP IS 0x58, read off the page's own handset rather than assumed.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
async function press(raw, label, settleMs){
  window.__traceCalls(TRACE); window.__traceClear();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, settleMs === undefined ? 9000 : settleMs); });
  var lg = window.__traceLog(), n = {};
  lg.forEach(function(e){ n[e.name] = (n[e.name] || 0) + 1; });
  window.__traceCalls([]);
  return { key: label, widgets: n.newWidget || 0, surface: surface() };
}

// THE TIMELINE IS PART OF THE MEASUREMENT, not scaffolding. With the carousel on air the box goes
// Ready, then parses a BAT, then rebuilds its channel list once per service -- so it is Ready, busy
// and Ready again, and a settle loop that stops at the first Ready presses into the second. The
// transitions are recorded because how long a twelve-channel line-up takes to absorb is a fact a
// visitor experiences, and the first run of this probe threw "never settled in 400s" without saying
// what the box had been doing for those 400 seconds.
var stateLog = [], lastState = '', waited = 0;
function boxState(){ return document.getElementById('boxstate-t').textContent.slice(0, 48); }
while (waited < 900) {
  var st = boxState();
  if (st !== lastState) { stateLog.push({ atSeconds: waited, state: st }); lastState = st; }
  if (/^Ready/.test(st) && waited >= 30) break;
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (!/^Ready/.test(boxState()))
  throw new Error('the box never settled in ' + waited + 's. Timeline: ' + JSON.stringify(stateLog));
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
var car = window.__siCarousel();
if (car.running === false || car.startedAt === undefined)
  throw new Error('the carousel is not running (' + JSON.stringify(car) + ') -- this probe measures '
                + 'the page own broadcast. Pass --carousel or ?si=1.');
window.__profile(true);
var out = { settledAfterSeconds: waited, stateLog: stateLog, channels: [] };

// Wait for the box to open a title PID of its own accord, then for the channel-list rebuild.
for (var w = 0; w < 30; w++) {
  await new Promise(function(r){ setTimeout(r, 5000); });
  var f = window.__siFilters().filter(function(x){ return x.armed; }).map(function(x){ return parseInt(x.pid, 16); });
  if (f.some(function(p){ return p >= 0x30 && p <= 0x37; })) break;
}
out.filters = window.__siFilters().filter(function(x){ return x.armed; }).map(function(x){ return x.pid; });
out.carousel = window.__siCarousel();
out.tables = window.__siMatches().map(function(x){
  return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
       + (x.extension !== null ? '/ext=0x' + x.extension.toString(16) : ''); }).sort().join(' ');
if (!out.filters.some(function(p){ var v = parseInt(p, 16); return v >= 0x30 && v <= 0x37; }))
  throw new Error('the box never opened a title PID, so the carousel had nowhere to send listings: '
                + out.filters.join(' ') + '; tables ' + out.tables + '; carousel '
                + JSON.stringify(out.carousel) + '; timeline ' + JSON.stringify(stateLog));
// WAIT ON THE BOX'S OWN STATUS LINE, NOT ON A TIMER. A parsed BAT sends the box into a channel-list
// rebuild during which its o-code never returns to its event loop, so a key press is never LOOKED
// at and the screen stays blank -- which reads exactly like a dead emulator. The page says
// "Rebuilding the channel list" in words for the duration, and that is the signal the project's own
// notes say to use.
//
// AND TWELVE CHANNELS IS THREE TIMES THE REBUILD OF FOUR. The loop runs once per service, so a wait
// calibrated on the four-channel line-up (74-140 s) is far too short here -- the first run of this
// probe pressed straight through it and reported "the box is not drawing", which was true and was
// about the wait rather than about the line-up. Both signals are required and the elapsed time is
// reported, because how long this takes is itself something a visitor experiences.
var rebuildSec = 0, eeL = window.__i2cState().eeprom.writes, eeS = 0;
for (var q = 0; q < 300; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  rebuildSec += 3;
  var nw = window.__i2cState().eeprom.writes;
  eeS = (nw === eeL) ? eeS + 1 : 0; eeL = nw;
  var ready = /^Ready/.test(document.getElementById('boxstate-t').textContent);
  if (ready && eeS >= 3 && rebuildSec >= 30) break;
  if (rebuildSec % 60 === 0) stateLog.push({ atSeconds: waited + rebuildSec, state: 'rebuild: ' + boxState() });
}
out.rebuildSeconds = rebuildSec;
out.boxStateAfterRebuild = document.getElementById('boxstate-t').textContent.slice(0, 60);
if (!/^Ready/.test(out.boxStateAfterRebuild))
  throw new Error('the box never came back to Ready in ' + rebuildSec + 's -- it says "'
                + out.boxStateAfterRebuild + '". Every press below would be into a rebuild.');

out.sky = await press(0x7D, 'sky');
if (!out.sky.widgets) throw new Error('the sky press built no widgets; the box is not drawing');
out.guideFirstOpen = await press(0x80, 'tv guide');
if (!out.guideFirstOpen.widgets) throw new Error('the guide press built no widgets');
// The slot exists now. Give the carousel a wave or two to fill it for every channel.
await new Promise(function(r){ setTimeout(r, 45000); });
out.guideFilled = { surface: surface() };
await window.__shot('guide-channel-00');
out.channels.push({ step: 0, key: 'first channel', surface: out.guideFilled.surface });

// AND NOW WALK IT. One press of DOWN per channel, a screenshot each.
var seen = {};
seen[out.guideFilled.surface.hash] = 0;
out.repeats = [];
for (var c = 1; c <= 12; c++) {
  var p = await press(0x59, 'down #' + c, 7000);
  var shot = 'guide-channel-' + (c < 10 ? '0' + c : c);
  await window.__shot(shot);
  out.channels.push({ step: c, key: p.key, widgets: p.widgets, surface: p.surface });
  if (seen[p.surface.hash] !== undefined)
    out.repeats.push({ step: c, sameAs: seen[p.surface.hash], hash: p.surface.hash });
  seen[p.surface.hash] = c;
}
out.distinctScreens = Object.keys(seen).length;
out.verdict = out.distinctScreens + ' distinct screens across ' + out.channels.length
  + ' channel positions'
  + (out.repeats.length ? ('; REPEATS at ' + JSON.stringify(out.repeats)
     + ' -- either the scroll did nothing there or two channels are reading the same records')
     : '; every position drew something different, which twelve distinct days should');
return out;
