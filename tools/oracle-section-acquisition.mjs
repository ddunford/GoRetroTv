#!/usr/bin/env node
// Drive one DVB section into the independent browser oracle at a fixed guest
// instruction count. Instrumentation is served from memory; the oracle file is untouched.
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
const sectionAt = Number(arg('--section-at', '70000000'));
const stopAt = Number(arg('--stop-at', '80000000'));
const pid = Number(arg('--pid', '20'));
const hex = arg('--section-hex', '707005c67e120000');
const outPath = arg('--out', '');
const requireAcquired = args.includes('--require-acquired');
if (!Number.isSafeInteger(sectionAt) || !Number.isSafeInteger(stopAt) ||
    sectionAt < 100001 || stopAt <= sectionAt ||
    !Number.isInteger(pid) || pid < 0 || pid > 0x1fff ||
    !/^(?:[\da-fA-F]{2})+$/.test(hex)) throw new Error('invalid section argument');
const section = Array.from(Buffer.from(hex, 'hex'));
if (section.length < 3 || (((section[1] & 15) << 8) | section[2]) + 3 !== section.length)
  throw new Error('section length field differs from actual bytes');
const sections = [{pid, bytes: section}];
for (let i = 0; i < args.length; i++) {
  if (args[i] !== '--extra-section') continue;
  const spec = args[++i];
  const match = /^(?:(\d+):)?(\d+):([\da-fA-F]+)$/.exec(spec || '');
  if (!match) throw new Error('--extra-section needs [instruction:]decimal-pid:hex');
  const at = match[1] === undefined ? sectionAt : Number(match[1]);
  const extraPid = Number(match[2]);
  const bytes = Array.from(Buffer.from(match[3], 'hex'));
  if (!Number.isSafeInteger(at) || at < sectionAt || at >= stopAt ||
      extraPid > 0x1fff || match[3].length % 2 || bytes.length < 3 ||
      (((bytes[1] & 15) << 8) | bytes[2]) + 3 !== bytes.length)
    throw new Error(`invalid extra section ${spec}`);
  sections.push({at, pid: extraPid, bytes});
}
sections[0].at = sectionAt;
sections.sort((a, b) => a.at - b.at);

const htmlPath = resolve(root, 'reference/digibox-boot.html');
const original = await readFile(htmlPath);
let html = original.toString();
const needle = '  function burst(n){\n    for(var i=0;i<n && !stalled;i++){';
if (!html.includes(needle)) throw new Error('oracle burst hook moved');
const hook = `  var sectionCapture = {at: ${sectionAt}, stopAt: ${stopAt}, sections: ${JSON.stringify(sections)},
    profiled: false, next: 0, pushes: [], samples: [], done: false};
  window.__sectionCapture = sectionCapture;
  function sectionHits(){ return window.__pcHits(0x800041B4, 0x80004520, 0x800044BC, 0x8000494A,
    0x800A857A, 0x800A8594, 0x800A85D4, 0x800A8630, 0x800AF866,
    0x800A92D8, 0x800A9306, 0x800A937E, 0x800A6414, 0x800AD928); }
  function sectionRecords(){
    var out = {};
    for(var f=21;f<=24;f++){
      var b = window.__peek(0x80142D34 + f*20, 20), w = [];
      for(var j=0;j<20;j+=4) w.push(((b[j]<<24)|(b[j+1]<<16)|(b[j+2]<<8)|b[j+3])>>>0);
      out[f] = {start:w[0], end:w[1], current:w[2], last:w[3], context:w[4]};
    }
    return out;
  }
  function burst(n){
    for(var i=0;i<n && !stalled;i++){
      if(icount >= sectionCapture.stopAt){
        running = false; sectionCapture.done = true; sectionCapture.stoppedAt = icount;
        sectionCapture.after = {hits: sectionHits(), demux: window.__dispState(), records: sectionRecords(),
          demodReads: window.__siDemodCounts, siLog: window.__siLog(), tasks: window.__tasks().n,
          ids: window.__siIds(), matches: window.__siMatches()};
        return;
      }
      if(!sectionCapture.profiled && icount >= sectionCapture.at - 100000){
        window.__profile(true); sectionCapture.profiled = true;
      }
      if(sectionCapture.next < sectionCapture.sections.length &&
         icount >= sectionCapture.sections[sectionCapture.next].at){
        sectionCapture.samples.push({at:sectionCapture.sections[sectionCapture.next].at, actualAt:icount,
          hits:sectionHits(), demux:window.__dispState(),
          records:sectionRecords(), demodReads:Object.assign({},window.__siDemodCounts),
          tasks:window.__tasks().n, ids:window.__siIds(), matches:window.__siMatches()});
        while(sectionCapture.next < sectionCapture.sections.length &&
              icount >= sectionCapture.sections[sectionCapture.next].at){
          var s = sectionCapture.sections[sectionCapture.next++];
          sectionCapture.pushes.push({scheduledAt:s.at, at:icount, pid:s.pid,
            result:window.__siPush(s.pid,s.bytes)});
        }
      }`;
html = html.replace(needle, hook);
const readNeedle = '    var v = demodAnswer(dmReg); dmReads++;';
if (!html.includes(readNeedle)) throw new Error('oracle demod read hook moved');
html = html.replace(readNeedle,
  '    window.__siDemodCounts[dmReg] = (window.__siDemodCounts[dmReg] || 0) + 1;\n' + readNeedle);
html = html.replace('  function demodReadByte(){',
  '  window.__siDemodCounts = {};\n  function demodReadByte(){');

const contents = new Map([
  ['/digibox-boot.html', [Buffer.from(html), 'text/html; charset=utf-8']],
  ['/mips16.js', [await readFile(resolve(root, 'reference/mips16.js')), 'text/javascript']],
  ['/FLASH_U202.bin', [await readFile(resolve(root, 'firmware/FLASH_U202.bin')), 'application/octet-stream']],
  ['/FLASH_U203.bin', [await readFile(resolve(root, 'firmware/FLASH_U203.bin')), 'application/octet-stream']],
]);
const server = createServer((req, res) => {
  const content = contents.get(new URL(req.url, 'http://localhost').pathname);
  if (!content) { res.writeHead(404); res.end(); return; }
  res.writeHead(200, {'content-type': content[1], 'cache-control': 'no-store'});
  res.end(content[0]);
});
await new Promise((ready, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', ready); });

const require = createRequire(import.meta.url);
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
let browser;
try {
  browser = await chromium.launch({headless: true, executablePath: process.env.PLAYWRIGHT_CHROME || undefined});
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(`http://127.0.0.1:${server.address().port}/digibox-boot.html?si=0`,
    {waitUntil: 'domcontentloaded'});
  await page.waitForFunction(() => window.__sectionCapture?.done === true ||
    document.getElementById('done')?.dataset.show === '1', null, {timeout: 600000});
  const capture = await page.evaluate(() => window.__sectionCapture);
  if (errors.length) throw new Error(`browser errors: ${errors.join('; ')}`);
  if (!capture.done || capture.pushes.length !== sections.length ||
      !capture.pushes.every((push, i) => push.result.ok &&
        push.scheduledAt === sections[i].at && push.at >= push.scheduledAt &&
        push.at <= push.scheduledAt + 1) ||
      capture.stoppedAt !== stopAt ||
      capture.after.siLog.length !== sections.length)
    throw new Error(`section acquisition controls failed: ${JSON.stringify(capture)}`);
  if (requireAcquired &&
      (!capture.after.demux.pids.some(entry => entry.pid === '0x52') ||
       ![0x40, 0x42, 0x4a, 0x73].every(table =>
         capture.after.matches.some(match => match.tableId === table)) ||
       !(capture.after.hits['0x800A6414'] > 0) ||
       !(capture.after.demodReads['11'] > 0)))
    throw new Error(`guest SI acquisition subject missing: ${JSON.stringify(capture.after)}`);
  const report = {oracleSha256: createHash('sha256').update(original).digest('hex'),
    urlParameters: 'si=0', ...capture};
  if (outPath) {
    await mkdir(dirname(outPath), {recursive: true});
    await writeFile(outPath, JSON.stringify(report, null, 2) + '\n');
  }
  console.log(JSON.stringify(report, null, 2));
} finally {
  if (browser) await browser.close();
  await new Promise(done => server.close(done));
}
