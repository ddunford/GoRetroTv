// WHAT DOES THE BOX WANT ON PID 0x52? Ask the hardware filter it programmed itself.
//
// sky-02me.3 established that with a broadcast running the box acquires, takes every NIT, SDT and
// BAT offered, and CONCLUDES its guide search -- the "Searching for listings" panel disappears and
// only "Further schedule information is not available" remains. So the plumbing works and what is
// missing is schedule content.
//
// The box has already said where it expects that content: having acquired, it registered a subtable
// for network 0x20 and programmed its OWN section filter on PID 0x52, which nothing in this project
// feeds. Rather than guess a table id and push bytes at it until something sticks -- a decoder run
// over data does not fail, it produces plausible output -- this reads the FILTER'S OWN MATCH
// CRITERIA out of the demux registers the firmware wrote.
//
// __siMatches() decodes those writes into table id, table-id mask, extension and extension mask. A
// filter with a zero mask on its table-id byte matches nothing and is reported inactive rather than
// as "table 0", which is the distinction that matters here: an inactive filter and a filter wanting
// table 0 look identical in a raw dump.
//
// THE MASK MATTERS AS MUCH AS THE MATCH, and this is where a feed would silently miss. An extension
// of 0x1000 under mask 0xFFF0 accepts 0x1000-0x100F; under 0xFFFF it accepts only 0x1000. Sky's
// real bouquet ids are 0x1001-0x1004, so whether an authentic broadcast is seen AT ALL turns on the
// mask, not the match.
//
// CONTROLS: 42 tasks, and the carousel must actually be accepted (sections sent, none refused)
// before any statement about what is still missing means anything.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase(); }

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');
await new Promise(function(r){ setTimeout(r, 80000); });
window.__siCarousel(true);
await new Promise(function(r){ setTimeout(r, 30000); });

var car = window.__siCarousel();
if (!car || !car.sent) throw new Error('the carousel sent nothing -- no statement below is safe');

// Marry each filter's PID to what it is matching, so "PID 0x52 wants table X" is one row rather
// than two lists a reader has to join by hand and can join wrongly.
var pids = {}, filters = window.__siFilters();
filters.forEach(function(f){ pids[f.filter] = f; });
var wants = window.__siMatches().map(function(m){
  var f = pids[m.filter];
  return { filter: m.filter, pid: f ? f.pid : '(no PID programmed)', armed: f ? f.armed : false,
           tableId: h(m.tableId), tableIdMask: h(m.tableIdMask),
           extension: m.extension === null ? null : h(m.extension),
           extensionMask: m.extensionMask === null ? null : h(m.extensionMask),
           raw: m.bytes };
});

return {
  carousel: { sent: car.sent, refused: car.refused },
  ids: window.__siIds(),
  whatEachFilterWants: wants,
  theOneNothingFeeds: wants.filter(function(w){ return w.pid === '0x52'; }),
  demuxPids: window.__dispState().pids,
  siLogTableIds: (function(){
    var seen = {}; window.__siLog().forEach(function(e){ seen[e.tableId] = (seen[e.tableId]||0)+1; });
    return seen; })(),
  tasks: window.__tasks().n
};
