// WHAT HARDWARE IS THIS EMULATOR MISSING? Ask the box, rather than reasoning about the board.
//
// Every read that reaches the bottom of load() -- past flash, DRAM, the smartcard link, the UART,
// DMA, the blitter, per-register read-back and MMIO_MODEL -- is answered with a default ZERO and
// counted. That counter is the list of registers the firmware asked about and nothing answered,
// and it is the hardware-shaped version of this project's oldest problem: a zero is
// indistinguishable from a device reporting "nothing to report", so the firmware never complains
// about a single one of them. Nothing throws, nothing logs, and the box goes on looking healthy.
//
// TWO CENSUSES WITH DIFFERENT SPANS, and saying which is which is the point:
//   __mmioUnanswered()  accumulates from the FIRST INSTRUCTION of the boot, because the counter is
//                       armed for the life of the page. It covers everything.
//   __taggedReads()     only from the moment __tagReads(lo,hi) is called, which here is after the
//                       boot has been asserted. It covers the settled box and the key press.
// So an address with a big count and no tag was read during the boot and not since -- which is a
// finding about WHEN it matters, not a gap in the instrument.
//
// WHAT THE COUNTS MEAN. A high count is a POLL: the firmware is waiting for a bit and getting zero,
// which is the failure mode that costs whole days here. A single read is usually a probe whose zero
// was a perfectly good answer. Rank by count, then attribute by PC, then read the code around the
// PC to find out what bit it wanted -- that order, because the count says how badly something is
// wanted and only the code says what would satisfy it.
//
// AND DO NOT ANSWER ONE WITHOUT BOOTING AFTERWARDS. Recording what the firmware writes is inert;
// answering a read it used to get zero for is not. Making the demux register file read back its
// own writes stopped the RTOS starting, and blanket read-back hung the bootloader's display init:
// a poll that used to see 0 and exit saw its own write and spun for ever. Every entry in
// MMIO_MODEL is a deliberate fiction with a stated reason, and each one cost a boot to prove.
function h(v){ return '0x'+(v>>>0).toString(16).toUpperCase().padStart(8,'0'); }

window.__profile(true);
if (typeof window.__mmioUnanswered !== 'function')
  throw new Error('__mmioUnanswered missing -- this probe needs the accessor, not a plausible zero');

var boot = window.__mmioUnanswered();
if (!boot.length)
  throw new Error('the unanswered-read census is EMPTY, which cannot be true of a box that booted');

// Tag from here on, across the whole application image, so the settled box and the key press get
// an owner for each address.
window.__tagReads(0x80000000, 0x80200000);
await new Promise(function(r){ setTimeout(r, 80000); });
var settled = window.__mmioUnanswered();
window.__key(0x7D, 0);
await new Promise(function(r){ setTimeout(r, 20000); });
var after = window.__mmioUnanswered();

function asMap(rows){ var m = {}; rows.forEach(function(r){ m[r[0]] = r[1]; }); return m; }
var b = asMap(boot), a = asMap(after);
var live = Object.keys(a).filter(function(k){ return (a[k] - (b[k]||0)) > 0; })
                         .map(function(k){ return { addr: k, sinceBoot: a[k] - (b[k]||0), total: a[k] }; })
                         .sort(function(x, y){ return y.sinceBoot - x.sinceBoot; });

// The ASIC window is the one with no public manual; everything else is either the VR4111's own
// on-chip units or an address nobody has placed.
function region(addr){
  var v = parseInt(addr, 16)>>>0;
  if (v >= 0xB0000000 && v < 0xB1000000) return 'ASIC (external bus, undocumented)';
  if (v >= 0xAB000000 && v < 0xAC000000) return 'VR4111 on-chip';
  return 'unplaced';
}
var byRegion = {};
after.forEach(function(r){ var k = region(r[0]); byRegion[k] = (byRegion[k]||0) + r[1]; });

return {
  distinctAddresses: after.length,
  totalUnansweredReads: after.reduce(function(n, r){ return n + r[1]; }, 0),
  byRegion: byRegion,
  topOverall: after.slice(0, 25).map(function(r){ return { addr: r[0], reads: r[1], region: region(r[0]) }; }),
  stillPollingAfterBoot: live.slice(0, 20).map(function(r){
    return { addr: r.addr, sinceBootAsserted: r.sinceBoot, total: r.total, region: region(r.addr) }; }),
  whoIsWaiting: window.__taggedReads().slice(0, 30),
  duringSettleOnly: settled.length,
  unknownOpcodeSites: window.emuTrace().skipped,
  tasks: window.__tasks().n
};
