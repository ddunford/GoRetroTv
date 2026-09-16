// WHO DRIVES THE favchn LOOP? -- sky-02me.18, the routine rather than the record.
//
// The loop is known byte for byte: a 2-byte offset at EEPROM 0x3C44 incremented by 8, an 8-byte zero
// record appended at 0x1BD0 + offset, and three constant records ("favchn" at 0x1FB6, a 25-byte one
// at 0x1883, a 4-byte count of 26 at 0x3C46) rewritten on every pass, for ever.
//
// A STORE TO AN EEPROM OFFSET IS INVISIBLE TO __writeWatch, because it never touches DRAM -- it goes
// out over I2C a byte at a time. So the page's EEPROM transaction log carries a stack scan like the
// demodulator log's.
//
// AND IT CANNOT NAME THE CALLER, which this probe established by trying it twice. Captured at the
// I2C STOP it reports the completion interrupt (pc=0x80006F30); captured at the arm-START it reports
// the driver's own write routine (pc=0x80006C7E); both give ra=0 and an empty chain, because the
// task that asked for the write is blocked on a semaphore and its frame is on neither stack. The
// fields are therefore named driverPc/driverRa/driverStack, and this probe is kept as the record of
// a technique that does NOT work here. To find a caller, diff the PC histogram --
// scripts/digibox-probes/what-the-dead-press-does-instead.js does, and found the path.
//
// CONTROLS: the healthy press must draw and log no EEPROM traffic, and the repro must reproduce --
// otherwise the callers below belong to something else.

function h(v){ return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0'); }
function surface(){
  var b = window.__peek(0x80584048, 720 * 576), s = 2166136261, hist = {};
  for (var i = 0; i < b.length; i++) { s = (Math.imul(s ^ b[i], 16777619)) >>> 0; hist[b[i]] = 1; }
  return { hash: h(s), colours: Object.keys(hist).length };
}
var waited = 0;
while (!/^Ready/.test(document.getElementById('boxstate-t').textContent) && waited < 240) {
  await new Promise(function(r){ setTimeout(r, 1000); }); waited++;
}
if (window.__tasks().n < 42) throw new Error('not booted: ' + window.__tasks().n + ' tasks');
window.__profile(true);
if (window.__siCarousel().running !== false) throw new Error('the carousel is RUNNING');
if (typeof window.__eeTxClear !== 'function') throw new Error('no __eeTxClear on this page');

var TRACE = [{ pc: 0x80082A6C, name: 'newWidget', args: 1 }];
async function press(){
  window.__traceCalls(TRACE); window.__traceClear();
  var s0 = surface();
  window.__key(0x7D, 0);
  await new Promise(function(r){ setTimeout(r, 9000); });
  var w = window.__traceLog().filter(function(x){ return x.name === 'newWidget'; }).length;
  window.__traceCalls([]);
  return { widgets: w, moved: surface().hash !== s0.hash };
}

var out = { settledAfterSeconds: waited };
out.healthy = await press();
if (!out.healthy.widgets) throw new Error('the healthy press built no widgets');

out.feed = { nit: [window.__siNIT(undefined,{version:61}), window.__siNIT(undefined,{version:62})].map(function(r){return r.ok?'ok':r.why;}) };
await new Promise(function(r){ setTimeout(r, 8000); });
out.feed.bat = [window.__siBAT(undefined,{version:61}), window.__siBAT(undefined,{version:62})].map(function(r){return r.ok?'ok':r.why;});
await new Promise(function(r){ setTimeout(r, 9000); });

window.__eeTxClear();
out.dead = await press();
if (out.dead.widgets) throw new Error('the repro did not reproduce: ' + out.dead.widgets + ' widgets');

var tx = window.__eeTx();
if (!tx.length) throw new Error('no EEPROM transactions logged during the dead press');
if (tx[0].driverStack === undefined)
  throw new Error('the transaction log carries no stack -- this page predates the caller capture');

// Group by the record being written, and report the callers of each. The two ADVANCING members of
// the cycle are the interesting ones: 0x3C44 (the offset) and the 0x1BD0+ record.
var byAt = {};
tx.forEach(function(x){
  var k = x.at;
  byAt[k] = byAt[k] || { at: k, n: 0, pcs: {}, ras: {}, stacks: {} };
  byAt[k].n++;
  byAt[k].pcs[x.driverPc] = 1; byAt[k].ras[x.driverRa] = 1;
  (x.driverStack || []).slice(0, 6).forEach(function(a){ byAt[k].stacks[a] = (byAt[k].stacks[a] || 0) + 1; });
});
out.transactions = tx.length;
out.records = Object.keys(byAt).map(function(k){
  var r = byAt[k];
  return { at: r.at, writes: r.n, pc: Object.keys(r.pcs),
           ra: Object.keys(r.ras),
           stackTop: Object.keys(r.stacks).sort(function(a,b){ return r.stacks[b]-r.stacks[a]; }).slice(0,8) };
}).sort(function(a,b){ return b.writes - a.writes; });

// The offset record is the one that decides whether to go round again, so call it out by name.
out.theOffsetWriter = out.records.filter(function(r){ return r.at === '0x00003C44'; })[0]
                   || 'no write to 0x3C44 in this window';
out.sampleCycle = tx.slice(0, 8).map(function(x){
  return x.at + ' n=' + x.n + ' driverPc=' + x.driverPc + '  [' + (x.driverStack||[]).slice(0,4).join(' ') + ']'; });
return out;
