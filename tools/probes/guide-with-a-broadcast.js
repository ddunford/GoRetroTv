// DOES THE GUIDE FILL WHEN THE BOX IS ACTUALLY BEING BROADCAST TO? -- sky-02me.3.
//
// Everything the interface shows says the same thing: "Searching for listings / Please wait" on the
// TV Guide, on MOVIES A-Z and on Interactive. The UI is fully navigable (sky-02me.2 and .10), so
// what is missing is DATA -- and this box already has both halves of the machinery for that. The
// demodulator answers register 11 as 0x3F, which is what makes it acquire at all, and __siCarousel
// repeats NIT, BAT, SDT and TDT at DVB intervals into whatever filters the FIRMWARE has armed.
//
// So this is the cheapest possible question before any new mock is built: run the box with the
// carousel on air and walk to the guide. If the guide fills, sky-02me.3 is mostly done and the work
// is content rather than plumbing. If it still says "Searching", the plumbing is where to look, and
// the SI state captured here says which half.
//
// WHAT IS REPORTED RATHER THAN ASSUMED. A carousel that is running is not a carousel that is being
// ACCEPTED: __siCarousel only pushes to a PID the firmware has armed, so `sent` and `refused` and
// the armed filter list are all captured. A run where the box never asked for a PID is a finding,
// not a silence to paper over -- and it looks identical to a broadcast the box ignored unless the
// numbers are on the page.
//
// CONTROLS. 42 tasks before and after. 0x7D must produce the menu (37 colours) or the walk is
// happening somewhere unknown and nothing below means anything.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function surface(){
  var b = window.__peek(0x80584048, 720*576), s = 2166136261, hist = {};
  for (var i=0;i<b.length;i++){ s = (Math.imul(s ^ b[i], 16777619))>>>0; hist[b[i]] = (hist[b[i]]||0)+1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');

// THE CAROUSEL STARTS AFTER THE BOOT, NOT BEFORE IT. With the runner's --carousel flag ALONE the
// box stalled at 22 TASKS and burned 2.0 BILLION instructions without finishing, against 42 tasks
// and 140M warm with no broadcast.
//
// THAT IS NOT A CONTRADICTION OF THE RECORDED CLAIM, and saying so is the point. The emulation doc
// records the box reaching 42 tasks with the carousel on air -- measured with --ack-all as well,
// where every peripheral command is answered. This run had no --ack-all. Two different machines,
// so the observation stands on its own conditions and does not overturn the other one. Whether
// --carousel needs --ack-all to boot is a real question and it is filed rather than assumed here.
//
// Starting it after the boot is what this question needs anyway: the guide is reached by a person
// pressing a button on a running box, not during its boot.
var siAtStart = window.__siCarousel();          // READ, never start -- the accessor is read-only
await new Promise(function(r){ setTimeout(r, 80000); });
if (window.__tasks().n < 42) throw new Error('lost tasks during the settle');
var started = window.__siCarousel(true);
await new Promise(function(r){ setTimeout(r, 20000); });   // let it repeat a few rounds

async function press(raw, name, waitMs){
  var before = surface();
  window.__key(raw, 0);
  await new Promise(function(r){ setTimeout(r, waitMs || 8000); });
  var after = surface();
  await window.__shot(name);
  return { pressed: h(raw).slice(-2), name: name, moved: before.hash !== after.hash,
           colours: after.colours, hash: after.hash };
}

var menu = await press(0x7D, 'bcast-01-menu');
if (menu.colours < 30)
  throw new Error('0x7D did not produce the menu (' + menu.colours + ' colours)');

// Give the SI a long run at the guide: a scan waiting for the network to settle will not conclude
// on one section, which is exactly why the carousel repeats.
var guide = await press(0x80, 'bcast-02-guide', 20000);
var guideAgain = { colours: surface().colours, hash: surface().hash };
await new Promise(function(r){ setTimeout(r, 25000); });
var guideSettled = surface();
await window.__shot('bcast-03-guide-settled');

return {
  carouselAtStart: siAtStart, carouselStarted: started,
  carouselNow: window.__siCarousel(),
  ids: (window.__siIds ? window.__siIds() : null),
  armedFilters: (window.__siFilters ? window.__siFilters() : null),
  demuxPids: (window.__dispState ? window.__dispState().pids : null),
  menu: menu,
  guide: guide,
  guideAfter25sMore: { hash: guideSettled.hash, colours: guideSettled.colours,
                       changedWhileWaiting: guideSettled.hash !== guideAgain.hash },
  siLogTail: (window.__siLog ? window.__siLog().slice(-12) : null),
  tasks: window.__tasks().n
};
