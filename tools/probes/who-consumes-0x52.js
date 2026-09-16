// WHO READS FILTER 21's SECTION RING? Let the firmware name its own consumer.
//
// THE ROUTE HERE MATTERS. feed-0x52.js pushed every table id from 0xA0 to 0xB1 onto PID 0x52, saw
// all of them accepted, and reported that no parser ran -- while watching three addresses the
// project's notes name as the OpenTV carousel parsers. are-the-parsers-cold.js then checked whether
// those addresses execute AT ALL: 0x800C95D0, 0x800C9CA0 and 0x800BECF0 are stone cold, and so are
// their whole neighbourhoods, while the surrounding 0x800B0000-0x800D0000 region takes 42.8 MILLION
// hits. So the sweep was watching code that is not on the path and its per-id zeros meant nothing.
//
// GUESSING A BETTER ADDRESS WOULD REPEAT THE MISTAKE. Instead this asks the machine: arm a read
// watch over filter 21's own section ring and push a section into it. Whatever PC reads those bytes
// IS the consumer, named by the firmware rather than by a note. Section RAM is thirty-two 12 KB
// rings at 0xA07A0000 + f*0x3000, so filter 21's is 0xA07DF000 -- and __siFilters() independently
// reports that same address for it, which is the agreement that makes it safe to watch.
//
// CONTROLS:
//   PID 0x52 must be armed, or this is watching a ring nothing delivers to.
//   The push must be accepted.
//   The watch must log SOMETHING. A read watch that fires on nothing and a ring nobody reads look
//   identical, so an empty log is raised as a harness failure and not reported as "nobody reads it".
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }
function u16(v){ return [(v >>> 8) & 0xFF, v & 0xFF]; }
var CRC_TAB = (function(){
  var t = new Int32Array(256);
  for (var i = 0; i < 256; i++){
    var c = i << 24;
    for (var j = 0; j < 8; j++) c = (c & 0x80000000) ? ((c << 1) ^ 0x04C11DB7) : (c << 1);
    t[i] = c;
  }
  return t;
})();
function crc32(b){ var c = -1; for (var i=0;i<b.length;i++) c = (c << 8) ^ CRC_TAB[((c>>>24)^b[i])&0xFF]; return c>>>0; }
function section(tableId, ext, payload){
  var len = 5 + payload.length + 4;
  var body = [tableId & 0xFF, 0xB0 | ((len >>> 8) & 0x0F), len & 0xFF]
               .concat(u16(ext), [0xC1, 0x00, 0x00], payload);
  var c = crc32(body);
  return body.concat([(c>>>24)&0xFF, (c>>>16)&0xFF, (c>>>8)&0xFF, c&0xFF]);
}

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');
await new Promise(function(r){ setTimeout(r, 80000); });
window.__siCarousel(true);
await new Promise(function(r){ setTimeout(r, 30000); });

var filters = window.__siFilters();
var f52 = filters.filter(function(f){ return f.pid === '0x52'; })[0];
if (!f52) throw new Error('no filter carries PID 0x52 -- the box has not acquired');
var RING = parseInt(f52.ring, 16) >>> 0;
if (RING !== ((0xA07A0000 + f52.filter * 0x3000) >>> 0))
  throw new Error('filter ' + f52.filter + ' reports ring ' + h(RING) + ' but the layout says ' +
                  h(0xA07A0000 + f52.filter * 0x3000) + ' -- two readings disagree, so neither is safe');

// Watch the ring, then deliver into it.
window.__readWatch(RING, (RING + 0x3000) >>> 0);
var pushed = window.__siPush(0x52, section(0xA0, 0x0020, [0x00,0x00,0x00,0x00]));
if (!pushed || !pushed.ok) { window.__readWatch(); throw new Error('push refused: ' + (pushed && pushed.why)); }
await new Promise(function(r){ setTimeout(r, 12000); });
var lg = window.__readWatchLog();
window.__readWatch();

if (!lg.all || !lg.all.length)
  throw new Error('the ring watch logged NOTHING across 12 s after an accepted push -- harness ' +
                  'failure, not evidence that nobody reads filter ' + f52.filter);

var byPc = {};
lg.all.forEach(function(e){ byPc[e.pc] = (byPc[e.pc] || 0) + 1; });
var readers = Object.keys(byPc).sort(function(a, b){ return byPc[b] - byPc[a]; })
                    .map(function(pc){ return { pc: pc, reads: byPc[pc] }; });

return {
  filter: f52.filter, ring: h(RING), pushAccepted: true,
  reads: lg.reads, capped: !!lg.capped,
  readersOfTheRing: readers.slice(0, 12),
  firstReads: lg.all.slice(0, 10).map(function(e){
    return { pc: e.pc, at: e.at, size: e.size, icount: e.icount }; }),
  note: 'these PCs are the firmware naming its own consumer for PID 0x52 -- no address was guessed',
  tasks: window.__tasks().n
};
