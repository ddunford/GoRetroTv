// PRIME THE PERSISTENT PROFILE so every later probe can start from a box that already has its
// channel list, and skip five minutes of waiting for state it could have kept.
//
// WHAT THE WAIT ACTUALLY COSTS, measured across a day of runs on the ALL CHANNELS screen: ~135 s of
// COLD boot plus ~200 s of channel-list rebuild before a key can be pressed, on every run, six
// minutes of which nearly all is re-establishing something the box had already worked out. The
// NVRAM lives in localStorage and the probe runner keeps a persistent browser profile unless
// `--cold` is passed -- so the state survives, and the only reason it was being thrown away is that
// every probe here was written with `--cold`.
//
// AND THE OBVIOUS SHORTCUT DOES NOT WORK, which is why this is a probe rather than a flag.
// `--carousel` on a WARM box stalls the boot at 22 tasks (sky-02me.11, still open). The flag puts
// the multiplex on air DURING the boot; on a warm box that lands at a different phase and the task
// list never completes. So this boots with nothing on air, waits for the box to settle, and only
// THEN switches the carousel on by hand -- which is also the honest model, because a real box that
// has been off overnight boots from its own NVRAM and meets the broadcast afterwards.
//
// Run it once, without --cold and without --carousel:
//
//     ./ctl.sh digibox:probe scripts/digibox-probes/prime-the-nvram.js
//
// After it, a probe that also runs without --cold finds a box that already knows its twelve
// channels, and needs only enough carousel to fill the listings.
//
// IT ASSERTS THE PRIMING RATHER THAN ASSUMING IT. A run that returns without the match table
// carrying 0xA0/0xA1 and without a title PID open has not primed anything, and saying so is the
// difference between a fast probe and a fast probe measuring the wrong box.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function tables(){
  return window.__siMatches().map(function(x){
    return '0x' + (x.tableId === null ? '??' : x.tableId.toString(16))
         + (x.extension !== null ? '/' + x.extension.toString(16) : ''); }).sort().join(' ');
}
function titlePids(){
  return window.__siFilters().filter(function(f){ return f.armed; })
    .map(function(f){ return parseInt(f.pid, 16); })
    .filter(function(p){ return p >= 0x30 && p <= 0x37; });
}
function boxState(){ return document.getElementById('boxstate-t').textContent.slice(0, 44); }

var out = { wasCold: null, timeline: [] };
var t0 = Date.now(), last = '';
function note(){
  var s = boxState();
  if (s !== last) { out.timeline.push({ at: Math.round((Date.now() - t0) / 1000), state: s }); last = s; }
}

var waited = 0;
while (!/^Ready/.test(boxState()) && waited < 400) {
  note();
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
note();
if (!/^Ready/.test(boxState())) throw new Error('the box never settled in ' + waited + 's');
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
out.settledAfterSeconds = waited;
out.bootWasWarm = waited < 90;                 // a cold boot here is ~135 s, a warm one ~31 s
out.before = { tables: tables(), titlePids: titlePids().map(h) };

// THE CAROUSEL GOES ON *AFTER* THE BOOT, which is the whole trick.
if (window.__siCarousel().startedAt === undefined) window.__siCarousel(true);
for (var w = 0; w < 30; w++) { var li = window.__siListings();
  if (li.state !== 'loading' && li.state !== 'not started') break;
  await new Promise(function(r){ setTimeout(r, 1000); }); }
out.listings = window.__siListings();
if (out.listings.state !== 'loaded')
  throw new Error('listings.json did not load, so nothing would be broadcast: '
                + JSON.stringify(out.listings));

// Wait for the box to be asking for listings AND for any rebuild to have finished. On an already
// primed box this is quick; on a blank one it is the full acquisition and the 200 s rebuild, which
// is exactly the cost this probe exists to pay ONCE.
var quiet = 0, ee = window.__i2cState().eeprom.writes, secs = 0;
for (var q = 0; q < 220; q++) {
  await new Promise(function(r){ setTimeout(r, 3000); });
  secs += 3; note();
  var nw = window.__i2cState().eeprom.writes;
  quiet = (nw === ee) ? quiet + 1 : 0; ee = nw;
  // BOTH, and the order they come back in is itself a warm-boot finding: the title PID returns
  // from NVRAM almost immediately, while the 0xA0/0xA1 match unit is only programmed once the box
  // has re-run acquisition. Waiting on the PID alone exits about thirty seconds too early and then
  // fails the table assertion, which is what the first run of this probe did.
  if (/^Ready/.test(boxState()) && titlePids().length && /0xa[01]/.test(tables())
      && quiet >= 3 && secs > 30) break;
}
out.primedAfterSeconds = secs;
out.after = { tables: tables(), titlePids: titlePids().map(h), tasks: window.__tasks().n };
out.carousel = window.__siCarousel();

if (!titlePids().length)
  throw new Error('no title PID is open after ' + secs + 's, so the box is not asking for listings '
                + 'and the profile is NOT primed. Tables: ' + tables());
if (!/0xa[01]/.test(out.after.tables))
  throw new Error('the match table carries no 0xA0/0xA1 subscription after ' + secs + 's, so the '
                + 'line-up has not been absorbed and the profile is NOT primed: ' + out.after.tables);

out.headline = 'PRIMED. boot ' + (out.bootWasWarm ? 'WARM' : 'COLD') + ' in ' + waited + 's, '
  + 'ready to press after a further ' + secs + 's. Title PIDs ' + out.after.titlePids.join(' ')
  + ', tables ' + out.after.tables + '. The next probe can run WITHOUT --cold and skip all of this.';
return out;
