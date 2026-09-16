// ARE THE OPENTV PARSERS EVER REACHED AT ALL? The control the last probe skipped.
//
// feed-0x52.js swept table ids 0xA0-0xB1 onto PID 0x52, every push accepted, and reported that NO
// parser ran for any of them -- while never establishing that the three addresses it watched
// execute in the first place. A trace armed on an address that is cold reports zero for every
// subject, and this project has already published a native as "never called" on exactly that
// confusion. So the sweep's result is currently a statement about the instrument.
//
// THE THREE ADDRESSES came from the project's own record of the OpenTV carousel -- parsers
// 0x800C95D0 and 0x800C9CA0, Huffman 0x800BECF0. This asks whether they, or anything near them,
// execute on a box with a broadcast running.
//
// __pcHits KEYS ITS RESULT WITH hex32, WHICH UPPERCASES, and a lower-case lookup returns a
// perfectly plausible zero rather than undefined. The key is built the page's way and a missing key
// THROWS, because a census that cannot find its own subject is a harness failure and not a count.
//
// AND THE POSITIVE CONTROL IS A REGION THAT MUST BE HOT: the o-code interpreter's fetch loop around
// 0x80069298 runs constantly on a box whose application is alive. If that comes back cold, the
// profiler is not recording and nothing else here means anything.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');
await new Promise(function(r){ setTimeout(r, 60000); });
window.__siCarousel(true);
await new Promise(function(r){ setTimeout(r, 60000); });

function hitsOf(addr){
  var hits = window.__pcHits(addr), k = h(addr);
  if (!(k in hits)) throw new Error('pcHits has no key ' + k + ' -- the lookup is not matching');
  return hits[k];
}

var exact = {
  otvParserA_0x800C95D0: hitsOf(0x800C95D0),
  otvParserB_0x800C9CA0: hitsOf(0x800C9CA0),
  huffman_0x800BECF0:    hitsOf(0x800BECF0)
};

// Nothing at an exact address proves nothing about the FUNCTION, so look at the neighbourhoods too.
var regions = {
  around_otvParsers_0x800C9000: window.__rangeHits(0x800C9000, 0x800CA000, 6),
  around_huffman_0x800BE000:    window.__rangeHits(0x800BE000, 0x800BF000, 6),
  whole_0x800B0000_0x800D0000:  window.__rangeHits(0x800B0000, 0x800D0000, 8),
  CONTROL_interpreter_0x80069000: window.__rangeHits(0x80069000, 0x8006A000, 3)
};
if (!regions.CONTROL_interpreter_0x80069000 || !regions.CONTROL_interpreter_0x80069000.total)
  throw new Error('the o-code interpreter region is COLD -- the profiler is not recording, so every '
                  + 'zero above is about the instrument');

return {
  exactAddresses: exact,
  anyExactHit: Object.keys(exact).some(function(k){ return exact[k] > 0; }),
  regions: regions,
  carousel: window.__siCarousel(),
  verdict: Object.keys(exact).some(function(k){ return exact[k] > 0; })
      ? 'the parsers DO execute -- feed-0x52.js measured real silence per table id'
      : (regions.around_otvParsers_0x800C9000.total || regions.around_huffman_0x800BE000.total)
      ? 'the exact addresses are cold but their NEIGHBOURHOODS are not -- the addresses are probably wrong'
      : 'that whole region of firmware never runs on this box, so feed-0x52.js was watching code that '
        + 'is not on the path and its per-id zeros mean nothing',
  tasks: window.__tasks().n
};
