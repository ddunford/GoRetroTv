// HOW WIDE IS THE SECTION-FILTER INDEX IN A MATCH COMMAND? Read the raw writes.
//
// THE SUSPICION, and it is about our own decoder rather than the firmware. dispWrite() decodes a
// match programming command at +0x144 as 0xC0 in the high byte and (byteIndex << 4) | filter in the
// low byte -- FOUR BITS of filter index. But this demux has 32 section filters and the box arms
// channel 21 for PID 0x52, which does not fit in four bits. If the real field is wider, every
// filter from 16 up is being recorded under an index 16 lower, and __siMatches() has been reporting
// filters 1, 2, 3 and 5 for writes that were really 17, 18, 19 and 21.
//
// THAT WOULD MATTER A LOT RIGHT NOW: filter 21 is the one carrying PID 0x52, the filter the box
// programmed for itself after acquisition and the only one nothing feeds. If the "filter 5 wants
// table 0x73" row is really filter 21, then 0x73 is the answer to what PID 0x52 wants -- and if it
// is not, then 0x52's match has never been decoded at all and the guide is waiting for something
// nobody has seen.
//
// EITHER WAY THE CURRENT JOIN IS UNSOUND. __siMatches() is indexed one way and __siFilters()/
// __dispState() another, so pairing them by index -- which the previous probe did -- produced
// "nothing feeds 0x52" as an ARTEFACT rather than a finding. This reads the raw register writes so
// the field width is measured instead of assumed.
//
// CONTROL: the writes must exist at all. A demux log with no 0x144 commands means the box never
// programmed a match and every conclusion below would be about an empty set.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase(); }

window.__profile(true);
if (window.__tasks().n < 42) throw new Error('not booted');
await new Promise(function(r){ setTimeout(r, 80000); });
window.__siCarousel(true);
await new Promise(function(r){ setTimeout(r, 30000); });

// __dmxLog() now returns { dropped, kept, entries, raw } -- a ring that keeps the NEWEST writes and
// says how many it threw away. It used to keep the oldest 4000, which the boot filled, so the
// acquisition-time commands this probe exists to read were never in it.
var log = window.__dmxLog ? window.__dmxLog() : null;
if (!log || !log.raw) throw new Error('no demux register log -- nothing to read');
if (!log.raw.length) throw new Error('the demux log is empty after a boot -- harness failure');

// Pair each +0x144 command with the +0x148 value that preceded it, which is how the hardware is
// driven: value first, then the command that files it.
var cmds = [], lastValue = null;
log.raw.forEach(function(e){
  var reg = e[0], val = e[1] >>> 0, ic = e[2];
  if (reg === 0x148) lastValue = val;
  if (reg === 0x144){
    cmds.push({ command: h(val), high: h((val >>> 8) & 0xFF),
                low: h(val & 0xFF),
                asFourBit:  { filter: val & 0x0F,  byteIndex: (val >>> 4) & 0x0F },
                asFiveBit:  { filter: val & 0x1F,  byteIndex: (val >>> 5) & 0x07 },
                value: lastValue === null ? null : h(lastValue),
                matchMask: lastValue === null ? null
                           : h((lastValue >>> 8) & 0xFF) + '/' + h(lastValue & 0xFF),
                icount: ic });
  }
});
if (!cmds.length) throw new Error('no +0x144 match commands in the log -- the box programmed none');

// THE FIRST DISCRIMINATOR WAS THE WRONG ONE. "Every programmed filter should be a channel the box
// armed" fails under BOTH readings, because the firmware sweeps every filter at init -- so the set
// of indices seen says nothing about the field width.
//
// THE BYTE INDEX DOES. Whatever the split is, the low byte holds both fields, and __siMatches()
// keeps a TEN-byte match array per filter. Under a four-bit filter the byte index has four bits and
// can reach 9; under a five-bit filter it has only three and cannot exceed 7. So a single observed
// byte index of 8 or 9 settles it, and a maximum of 7 leaves it open. That is a property of the
// data rather than a preference between two readings.
var armed = window.__dispState().pids.map(function(p){ return p.ch; });
function distinct(f){ var s = {}; cmds.forEach(function(c){ s[f(c)] = 1; }); return Object.keys(s).map(Number).sort(function(a,b){return a-b;}); }
var four = distinct(function(c){ return c.asFourBit.filter; });
var five = distinct(function(c){ return c.asFiveBit.filter; });
var fourBytes = distinct(function(c){ return c.asFourBit.byteIndex; });
var fiveBytes = distinct(function(c){ return c.asFiveBit.byteIndex; });
var maxFourByte = Math.max.apply(null, fourBytes);
function subsetOfArmed(list){ return list.every(function(x){ return armed.indexOf(x) >= 0; }); }

return {
  logDropped: log.dropped, logKept: log.kept,
  commands: cmds.length,
  armedChannels: armed,
  fourBitReading: { filters: four, byteIndexes: fourBytes, allArmed: subsetOfArmed(four) },
  fiveBitReading: { filters: five, byteIndexes: fiveBytes, allArmed: subsetOfArmed(five) },
  maxByteIndexUnderFourBit: maxFourByte,
  verdict: maxFourByte > 7
      ? 'FOUR BITS: a byte index of ' + maxFourByte + ' was observed, and a five-bit filter field ' +
        'leaves only three bits for it, which cannot exceed 7. The existing decoder is right and ' +
        'the match filters are 0..15 -- a SMALLER space than the 32 PID channels.'
      : 'OPEN: no byte index above 7 was seen, so both splits remain possible',
  sample: cmds.slice(0, 16),
  tasks: window.__tasks().n
};
