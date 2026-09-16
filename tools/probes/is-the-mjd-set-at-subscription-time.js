// sky-02me.21 -- DOES THE BOX BAKE THE DATE INTO ITS LISTINGS SUBSCRIPTION?
//
// THE PREVIOUS RUN FAILED ITS PREDICTION AND THE FAILURE WAS NOT CONCLUSIVE. It fed the line-up
// first, THEN moved the clock, and the 0xA1 match unit's two mystery bytes stayed 9E 8B. But the
// same output said unitOtherwiseUnchanged: true -- the unit was never reprogrammed at all -- and
// the probe never checked that the box's clock had moved either. "__siTDT returned ok" is a claim
// about the call, not about the world. So that run tested nothing.
//
// THE HYPOTHESIS, restated so it can fail properly. In an OpenTV title section Data[8..9] is the
// MJD (jcdutton/loadepg, dave-p/openTVtoXML). 0x9E8B is 40587, the MJD of 1 Jan 1970 -- the epoch
// of a box whose clock has never been set, which is the state every probe here has measured. If the
// box programs that unit ONCE, when the line-up arrives, then the date has to be in place BEFORE
// the line-up or the reading cannot move.
//
// SO THE ORDER IS THE EXPERIMENT: carousel ON from the first instruction, which puts a 1998 TDT on
// the air before anything else, and the line-up arrives after it. If the unit then demands C6 xx
// rather than 9E 8B, the bytes are the date and the whole title-section format falls into place.
//
// AND THE CLOCK IS ASSERTED THIS TIME, not assumed. The carousel's own TDT builder is the source of
// truth for what was sent; if no TDT was sent the run says so and concludes nothing.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function hx(v){ return '0x' + (v >>> 0).toString(16); }
function mjdOf(y, m, d){
  return Math.round((Date.UTC(y, m - 1, d) - Date.UTC(1858, 10, 17)) / 86400000);
}
var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 300) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);

var out = { question: 'is the MJD baked into the listings subscription at the moment it is made',
            settledAfterSeconds: waited };

// THE CLOCK, ASSERTED. The carousel is what feeds the TDT; if it is not running, nothing below is
// a test of anything.
out.carousel = window.__siCarousel();
if (out.carousel.running === false)
  throw new Error('the carousel is NOT running, so no TDT has been broadcast and the clock is still '
                + 'at the epoch -- this run would re-measure the previous one. Use --carousel.');
out.sectionsSent = out.carousel.sent;
out.sectionsRefused = out.carousel.refused;
if (!out.sectionsSent)
  throw new Error('the carousel is running but has sent NOTHING -- no TDT, so no clock');

function units(){
  return window.__siMatches().map(function(u){
    return { tableId: hx(u.tableId), mask: hx(u.tableIdMask), ext: hx(u.extension), bytes: u.bytes };
  });
}
function a1Unit(){
  return window.__siMatches().filter(function(x){ return x.tableId === 0xA1 || x.tableId === 0xA0; })[0] || null;
}

// Give it time to acquire the whole ladder by itself -- the carousel repeats NIT, BAT+line-up, SDT
// and a running clock, so the box walks to 0xA1 unaided.
for (var w = 0; w < 30 && !a1Unit(); w++) await new Promise(function(r){ setTimeout(r, 4000); });
var u = a1Unit();
out.allUnits = units();
if (!u) { out.headline = 'the box never subscribed to 0xA0/0xA1 with the carousel running -- nothing to read'; return out; }

out.a1 = { tableId: hx(u.tableId), tableIdMask: hx(u.tableIdMask), extension: hx(u.extension),
           extensionMask: hx(u.extensionMask), bytes: u.bytes };
out.mysteryBytes = [u.bytes[6], u.bytes[7]];
var got = (parseInt(u.bytes[6], 16) << 8) | parseInt(u.bytes[7], 16);
out.mysteryAsNumber = got;
out.mysteryAsHex = hx(got);

// What the carousel's clock should read by now. Its TDT starts at 1998-01-01 12:00 and advances
// with the emulator, so the expected MJD is that day or the next one or two.
var candidates = [];
for (var dday = 0; dday <= 3; dday++) {
  var m = mjdOf(1998, 1, 1) + dday;
  candidates.push({ date: '1998-01-0' + (1 + dday), mjd: m, hex: hx(m),
                    bytes: [((m >> 8) & 0xFF).toString(16) + '/ff', (m & 0xFF).toString(16) + '/ff'] });
}
out.candidates = candidates;
out.epochMjd = { mjd: mjdOf(1970, 1, 1), hex: hx(mjdOf(1970, 1, 1)) };
out.matchesAnEraDate = candidates.some(function(c){ return c.mjd === got; });
out.stillTheEpoch = got === mjdOf(1970, 1, 1);
out.tasksAtEnd = window.__tasks().n;
out.headline = out.matchesAnEraDate
  ? ('CONFIRMED: the two bytes are the MJD and they are baked in when the subscription is made. '
     + 'Got ' + out.mysteryAsHex + ' = ' + got + ', which is '
     + candidates.filter(function(c){ return c.mjd === got; })[0].date)
  : (out.stillTheEpoch
      ? 'STILL 9E 8B (MJD 40587, 1 Jan 1970) even with a 1998 clock on the air from boot -- so the '
        + 'bytes are NOT simply today\'s date. Either the box never took the TDT, or the field is '
        + 'something else. Check the clock independently before going further.'
      : 'the bytes are ' + out.mysteryAsHex + ' = ' + got + ', which is neither the epoch nor an era '
        + 'date -- a real value that needs explaining, and the most interesting of the three outcomes');
return out;
