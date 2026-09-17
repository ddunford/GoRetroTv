#!/usr/bin/env node
// Capture the independent browser oracle after a warm boot and a handset Sky press.
// The temporary instrumentation is served from memory; reference/digibox-boot.html is untouched.
// Run: node tools/oracle-warm-surface.mjs --nvram .artifacts/menu-cold-nvram.img \
//        --key-at 200000000 --stop-at 230000000 --out .artifacts/oracle-warm-surface.json
// Requires Playwright installed locally, or PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.js.
import { createRequire } from 'node:module';
import { createServer } from 'node:http';
import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const args = process.argv.slice(2);
function arg(name, fallback) {
  const i = args.indexOf(name);
  if (i < 0) return fallback;
  if (i + 1 >= args.length) throw new Error(`${name} needs a value`);
  return args[i + 1];
}
const nvramPath = arg('--nvram', resolve(root, '.artifacts/menu-cold-nvram.img'));
const keyAt = Number(arg('--key-at', '200000000'));
const stopAt = Number(arg('--stop-at', '230000000'));
const outPath = arg('--out', '');
const shotPath = arg('--shot', '');
if (!Number.isSafeInteger(keyAt) || !Number.isSafeInteger(stopAt) || keyAt < 1 || stopAt <= keyAt)
  throw new Error('instruction counts must be safe integers with 0 < key-at < stop-at');

const nvram = await readFile(nvramPath);
if (nvram.length !== 16384) throw new Error(`NVRAM must be 16384 bytes, got ${nvram.length}`);
const htmlPath = resolve(root, 'reference/digibox-boot.html');
let html = await readFile(htmlPath, 'utf8');
const needle = '  function burst(n){\n    for(var i=0;i<n && !stalled;i++){';
if (!html.includes(needle)) throw new Error('oracle burst hook changed; refusing an uninstrumented run');
const hook = `  var warmCapture = {keyAt: ${keyAt}, stopAt: ${stopAt}, keyIcount: null, done: false, before: null};
  function warmSurface(){
    var b = window.__peek(0x80584048, 720*576), hash = 2166136261, hist = {};
    for(var j=0;j<b.length;j++){ hash = Math.imul(hash ^ b[j], 16777619)>>>0; hist[b[j]] = (hist[b[j]]||0)+1; }
    var top = Object.keys(hist).sort(function(a,b){ return hist[b]-hist[a]; }).slice(0,6);
    return {hash: '0x'+hash.toString(16).toUpperCase().padStart(8,'0'),
            distinctColours: Object.keys(hist).length,
            top: top.map(function(k){ return '0x'+(+k).toString(16)+':'+hist[k]; })};
  }
  window.__warmCapture = warmCapture;
  function burst(n){
    for(var i=0;i<n && !stalled;i++){
      if(icount >= warmCapture.stopAt){
        running = false; warmCapture.done = true; warmCapture.stopIcount = icount;
        warmCapture.after = warmSurface(); warmCapture.tasks = window.__tasks();
        warmCapture.gates = window.__skyGates(); warmCapture.blits = window.__blitLog().length;
        warmCapture.keyBlits = warmCapture.blits - warmCapture.beforeBlits;
        warmCapture.keyLog = window.__keyLog().slice(-2);
        return;
      }
      if(warmCapture.keyIcount === null && icount >= warmCapture.keyAt){
        warmCapture.before = warmSurface();
        warmCapture.beforeBlits = window.__blitLog().length;
        warmCapture.keyIcount = icount;
        remoteKeyRaw(0x7D, 0);
      }`;
html = html.replace(needle, hook);
const contents = new Map([
  ['/digibox-boot.html', [Buffer.from(html), 'text/html; charset=utf-8']],
  ['/mips16.js', [await readFile(resolve(root, 'reference/mips16.js')), 'text/javascript']],
  ['/FLASH_U202.bin', [await readFile(resolve(root, 'firmware/FLASH_U202.bin')), 'application/octet-stream']],
  ['/FLASH_U203.bin', [await readFile(resolve(root, 'firmware/FLASH_U203.bin')), 'application/octet-stream']],
]);
const server = createServer((req, res) => {
  const path = new URL(req.url, 'http://localhost').pathname;
  const content = contents.get(path);
  if (!content) { res.writeHead(404); res.end(); return; }
  res.writeHead(200, {'content-type': content[1], 'cache-control': 'no-store'});
  res.end(content[0]);
});
await new Promise((resolveReady, reject) => {
  server.once('error', reject);
  server.listen(0, '127.0.0.1', resolveReady);
});

const require = createRequire(import.meta.url);
const playwrightModule = process.env.PLAYWRIGHT_MODULE || 'playwright';
let chromium;
try { ({ chromium } = require(playwrightModule)); }
catch (error) {
  throw new Error(`Cannot load ${playwrightModule}; install Playwright locally or set PLAYWRIGHT_MODULE to its index.js: ${error.message}`);
}
let browser;
try {
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.addInitScript(b64 => localStorage.setItem('retrotv.digibox.eeprom', b64), nvram.toString('base64'));
  const url = `http://127.0.0.1:${server.address().port}/digibox-boot.html?si=0`;
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => window.__warmCapture?.done === true ||
    document.getElementById('done')?.dataset.show === '1', null, {timeout: 300000});
  const result = await page.evaluate(() => ({...window.__warmCapture,
    stop: document.getElementById('d-stall')?.textContent}));
  if (errors.length) throw new Error(`browser errors: ${errors.join('; ')}`);
  if (!result.done || result.keyIcount === null || result.tasks?.n !== 42)
    throw new Error(`oracle did not reach a 42-task surface: ${JSON.stringify(result)}`);
  const report = {
    oracleSha256: createHash('sha256').update(await readFile(htmlPath)).digest('hex'),
    nvramSha256: createHash('sha256').update(nvram).digest('hex'),
    keyRequestedAt: keyAt, keyIcount: result.keyIcount,
    stopRequestedAt: stopAt, stopIcount: result.stopIcount,
    tasks: result.tasks.n, gates: result.gates, blits: result.blits,
    beforeBlits: result.beforeBlits, keyBlits: result.keyBlits,
    before: result.before, after: result.after, keyLog: result.keyLog,
  };
  if (shotPath) {
    await mkdir(dirname(shotPath), {recursive: true});
    await page.locator('#fb').screenshot({path: shotPath});
    report.shot = shotPath;
  }
  if (outPath) {
    await mkdir(dirname(outPath), {recursive: true});
    await writeFile(outPath, JSON.stringify(report, null, 2) + '\n');
  }
  console.log(JSON.stringify(report, null, 2));
} finally {
  if (browser) await browser.close();
  await new Promise(resolveClose => server.close(resolveClose));
}
