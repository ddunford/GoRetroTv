#!/usr/bin/env node
// Capture an unpatched, SI-silent cold boot from the independent browser oracle.
// Instrumentation exists only in the served copy; the checked-in oracle is never edited.
import { createRequire } from 'node:module';
import { createServer } from 'node:http';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const args = process.argv.slice(2);
function option(name, fallback) {
  const index = args.indexOf(name);
  if (index < 0) return fallback;
  if (index + 1 >= args.length) throw new Error(`${name} needs a value`);
  return args[index + 1];
}
const stopAt = Number(option('--stop-at', '470000000'));
const interval = Number(option('--interval', '1000'));
const injectAt = Number(option('--inject-at', '-1'));
const traceFrom = Number(option('--trace-from', '-1'));
const traceTo = Number(option('--trace-to', '-1'));
const outPath = resolve(option('--out', '.artifacts/oracle-cold-boot.stream'));
const tracePath = option('--trace-out', '');
if (![stopAt, interval, injectAt, traceFrom, traceTo].every(Number.isSafeInteger) ||
    stopAt <= 0 || interval <= 0 || injectAt >= stopAt ||
    (traceFrom < 0) !== (traceTo < 0) || (traceFrom >= 0 && (traceTo <= traceFrom || traceTo > stopAt))) {
  throw new Error('invalid instruction count, interval, injection, or trace window');
}
if (traceFrom >= 0 && !tracePath) throw new Error('--trace-out is required with a trace window');

const htmlPath = resolve(root, 'reference/digibox-boot.html');
const original = await readFile(htmlPath);
let html = original.toString('utf8');
const gateNeedle = 'var skyGates = true, skyGatesDone = false, skyGatesNext = 0;';
const burstNeedle = '  function burst(n){\n    for(var i=0;i<n && !stalled;i++){';
if (!html.includes(gateNeedle) || !html.includes(burstNeedle))
  throw new Error('oracle instrumentation anchors changed; refusing an unmeasured capture');
html = html.replace(gateNeedle, gateNeedle.replace('= true', '= false'));
const hook = `  var coldCapture = {stopAt: ${stopAt}, injectAt: ${injectAt}, traceFrom: ${traceFrom}, traceTo: ${traceTo},
    injected: false, done: false, trace: []};
  window.__coldCapture = coldCapture;
  function coldTrace(){
    return {at: icount, pc: pc>>>0, isa: isa, hi: hi>>>0, lo: lo>>>0,
      reg: Array.from(reg, function(v){return v>>>0;}),
      cp0: Array.from(cp0, function(v){return v>>>0;}), hash: cpHash()>>>0};
  }
  function burst(n){
    for(var i=0;i<n && !stalled;i++){
      if(icount >= coldCapture.stopAt){
        running = false; coldCapture.done = true; coldCapture.stopIcount = icount;
        coldCapture.tasks = window.__tasks().n; coldCapture.cp = window.__cpState();
        return;
      }
      if(!coldCapture.injected && coldCapture.injectAt >= 0 && icount >= coldCapture.injectAt){
        cp0[12] ^= 0x10000000; coldCapture.injected = true; coldCapture.injectedAt = icount;
      }
      if(icount >= coldCapture.traceFrom && icount < coldCapture.traceTo)
        coldCapture.trace.push(coldTrace());`;
html = html.replace(burstNeedle, hook);
const contents = new Map([
  ['/digibox-boot.html', [Buffer.from(html), 'text/html; charset=utf-8']],
  ['/mips16.js', [await readFile(resolve(root, 'reference/mips16.js')), 'text/javascript']],
  ['/FLASH_U202.bin', [await readFile(resolve(root, 'firmware/FLASH_U202.bin')), 'application/octet-stream']],
  ['/FLASH_U203.bin', [await readFile(resolve(root, 'firmware/FLASH_U203.bin')), 'application/octet-stream']],
]);
const server = createServer((request, response) => {
  const path = new URL(request.url, 'http://localhost').pathname;
  const content = contents.get(path);
  if (!content) { response.writeHead(404); response.end(); return; }
  response.writeHead(200, {'content-type': content[1], 'cache-control': 'no-store'});
  response.end(content[0]);
});
await new Promise((ready, reject) => {
  server.once('error', reject);
  server.listen(0, '127.0.0.1', ready);
});

const require = createRequire(import.meta.url);
const playwrightModule = process.env.PLAYWRIGHT_MODULE || 'playwright';
let chromium;
try { ({chromium} = require(playwrightModule)); }
catch (error) { throw new Error(`Cannot load ${playwrightModule}: ${error.message}`); }
let browser;
try {
  browser = await chromium.launch({headless: true,
    ...(process.env.PLAYWRIGHT_CHROME ? {executablePath: process.env.PLAYWRIGHT_CHROME} : {})});
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const url = `http://127.0.0.1:${server.address().port}/digibox-boot.html?cp=${interval}&si=0`;
  await page.goto(url, {waitUntil: 'domcontentloaded'});
  await page.waitForFunction(() => window.__coldCapture?.done === true ||
    document.getElementById('done')?.dataset.show === '1', null, {timeout: 900000});
  const capture = await page.evaluate(() => ({...window.__coldCapture,
    stream: window.__checkpoints(), stall: document.getElementById('d-stall')?.textContent}));
  if (errors.length) throw new Error(`browser errors: ${errors.join('; ')}`);
  if (!capture.done || capture.stopIcount !== stopAt || capture.cp.truncated ||
      capture.cp.interval !== interval || capture.cp.last < stopAt - interval ||
      (stopAt >= 470000000 && capture.tasks !== 42) ||
      capture.injected !== (injectAt >= 0))
    throw new Error(`oracle capture incomplete: ${JSON.stringify({...capture, stream: undefined, trace: undefined})}`);
  await mkdir(dirname(outPath), {recursive: true});
  await writeFile(outPath, capture.stream);
  if (tracePath) {
    await mkdir(dirname(resolve(tracePath)), {recursive: true});
    await writeFile(tracePath, capture.trace.map(row => JSON.stringify(row)).join('\n') + '\n');
  }
  console.log(JSON.stringify({oracleSha256: createHash('sha256').update(original).digest('hex'),
    oracleCopySha256: createHash('sha256').update(html).digest('hex'),
    urlParameters: `cp=${interval}&si=0`, stopIcount: capture.stopIcount,
    checkpoints: capture.cp.count, lastCheckpoint: capture.cp.last,
    tasks: capture.tasks, injected: capture.injected, injectedAt: capture.injectedAt,
    traceRows: capture.trace.length, outPath, tracePath: tracePath || undefined}));
} finally {
  if (browser) await browser.close();
  await new Promise(done => server.close(done));
}
