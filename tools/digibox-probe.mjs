#!/usr/bin/env node
// Boot the Digibox emulator page in a real browser, then run one or more probe files inside it.
//
// WHY THIS EXISTS. Every structural fact this project has settled about the firmware was
// measured by driving the running machine, not by reading it: the o-code interpreter's blocking
// object, the application registry's field layout, the permission gate. Each of those took a
// boot, a poke and a trace, and doing that by hand through a browser console is how a session's
// worth of findings gets lost. A probe file is the unit of that work and is worth keeping.
//
// A probe file is the BODY of an async function evaluated in the page; whatever it returns is
// printed as JSON. The page's whole debug API is in scope (__tasks, __peek, __poke, __traceCalls,
// __siPush, __call, …) and `await window.__shot('name')` captures the OSD panel.
//
// Flags:
//   --cold          a fresh browser profile, so the NVRAM is blank: 447M instructions, ~96s
//                   (a kept profile is a warm boot, 140M / ~31s -- ALWAYS say which you measured)
//   --dbg           re-poke the firmware's debug verbosity mask throughout the boot
//   --trace-boot    capture the firmware's own printf from the first instruction
//   --profile-boot  arm the PC profiler before the boot, so __pcHits/__rangeHits cover it --
//                   without this a boot-time routine reads 0 because nothing was watching
//   --carousel      put the SI carousel on air before the boot completes
//   --ack-all       answer every peripheral command
//   --allow-partial do not abort when the boot stops short (for watching WHERE it stops)
//   --url URL       drive a different host, e.g. the demo host
// Each probe file is the BODY of an async function evaluated in the page; whatever it
// returns is printed as JSON. Boot is asserted (42 tasks) before any probe runs, because a
// measurement read from a half-booted machine reports on a state nobody chose.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
const { chromium } = createRequire(new URL('/opt/workspaces/development/skytv.demosrv.uk/frontend/package.json', import.meta.url))('playwright');

const urlIx = process.argv.indexOf('--url');
// `--url VALUE` takes an argument, so VALUE is not a probe file. Filtering only on a leading `--`
// swept the URL into the file list and the runner then tried to open it, reporting
// "ENOENT: no such file or directory, open 'https://...'" AFTER a successful run had already
// printed its result -- a harness error trailing a green result, which is the shape somebody
// dismisses as noise.
const files = process.argv.slice(2)
  .filter((a, i) => !a.startsWith('--') && !(urlIx >= 0 && i === urlIx - 1));
// The page does not broadcast unless ?si=1, so a probe measures a silent box unless --carousel
// switches one on explicitly. That is what keeps the two states distinguishable.
// THE PAGE NOW BROADCASTS BY DEFAULT, so `?si=0` is appended unless the caller has said otherwise.
// Every probe in scripts/digibox-probes/ was written against a SILENT box -- "on a plain boot the
// match table is 0x40 and 0x73 only" is a statement about silence -- and letting the page's new
// default reach them would move all of those baselines at once. --carousel remains the explicit way
// to put a multiplex on the air, so a probe still says which state it measured.
function silentUnlessAsked(u) {
  if (/[?&]si=/.test(u)) return u;                       // the caller chose; leave it alone
  return u + (u.includes('?') ? '&' : '?') + 'si=0';
}
const PAGE_URL = silentUnlessAsked(
  urlIx >= 0 ? process.argv[urlIx + 1] : 'http://localhost:5173/digibox-boot.html');

// A PERSISTENT PROFILE, because the NVRAM lives in localStorage: a fresh profile is a COLD
// boot (447M instructions, ~107s measured here) and a kept one is warm (~31s). Every probe
// run after the first is therefore of a box that has booted before -- which is the state the
// gate and every earlier measurement used, so they are comparable.
// THE PROFILE LIVES OUTSIDE THE REPO. It is a browser profile, not source, and its only
// content that matters is the box's NVRAM in localStorage -- which is exactly what makes a
// boot warm rather than cold. Keeping it in the working tree put an untracked directory under
// scripts/ on the first run. `--cold` takes a fresh one, so the NVRAM is blank.
import { tmpdir } from 'node:os';
import { join } from 'node:path';
const PROFILE = process.argv.includes('--cold')
  ? join(tmpdir(), 'digibox-profile-cold-' + Date.now())
  : join(tmpdir(), 'digibox-profile');
const browser = await chromium.launchPersistentContext(PROFILE, { headless: true });
const page = await browser.newPage();
const logs = [];
page.on('console', m => logs.push(m.type() + ': ' + m.text()));
page.on('pageerror', e => logs.push('pageerror: ' + e.message));
// Let a probe ask for a picture of the OSD at a moment it chooses -- a hardware model's
// most important output is the screen, and a hash of it says only "different", not "what".
let shotN = 0;
await page.exposeFunction('__shot', async (name) => {
  const f = `shots/${String(++shotN).padStart(2,'0')}-${name}.png`;
  const el = await page.$('#fb');
  if (el) await el.screenshot({ path: new URL('./' + f, import.meta.url).pathname });
  console.log('SHOT ' + f);
  return f;
});

try {
  await page.goto(PAGE_URL, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => typeof window.__eeprom === 'function' && window.__eeprom().some(b => b !== 0x00), { timeout: 30000 });
  const boot = await page.evaluate(async (opts) => {
    if (opts.traceBoot) window.__debugPrintf(true);
    if (opts.traceWaits) window.__traceCalls([{ pc: 0x800CDAB8, name: 'wait', args: 2 }]);
    if (opts.traceApp) window.__traceCalls([
      { pc: 0x8007D06C, name: 'dbg', fmt: true },
      { pc: 0x80036D0C, name: 'appStart', args: 2 },
      { pc: 0x8008E7C8, name: 'getCodeModule', args: 1 },
      { pc: 0x8008C8A8, name: 'appById', args: 1 },
      { pc: 0x8007BCC0, name: 'registryInsert', args: 2 },
      { pc: 0x800511D0, name: 'carouselAcquire', args: 2 }
    ]);
    if (opts.traceReg) window.__traceCalls([
      { pc: 0x8007BB3C, name: 'lookup', args: 1 },
      { pc: 0x8007BCC0, name: 'insert', args: 3 },
      { pc: 0x8007BD3C, name: 'reg_8007BD3C', args: 2 },
      { pc: 0x8007BD64, name: 'reg_8007BD64', args: 2 },
      { pc: 0x8008C8A8, name: 'appById', args: 1 },
      { pc: 0x8008E7C8, name: 'getCodeModule', args: 1 }
    ]);
    if (opts.traceStart) window.__traceCalls([
      { pc: 0x8007D06C, name: 'dbg', fmt: true },
      { pc: 0x80036B54, name: 'appDescriptor', args: 3 },
      { pc: 0x8006E7F4, name: 'f_8006E7F4', args: 3 },
      { pc: 0x8006E808, name: 'f_8006E808', args: 3 },
      { pc: 0x8006EBAC, name: 'f_8006EBAC', args: 3 },
      { pc: 0x80051300, name: 'intprt_80051300', args: 3 }
    ]);
    if (opts.traceSig) window.__traceCalls([
      { pc: 0x8007D06C, name: 'dbg', fmt: true },
      { pc: 0x8003A124, name: 'sigVerify', args: 4 },
      { pc: 0x800DCD58, name: 'sigParse', args: 4 },
      { pc: 0x800E40EC, name: 'ndsVerify', args: 4 },
      { pc: 0x8008E7C8, name: 'getCodeModule', args: 2 }
    ]);
    // ACKING EVERY PERIPHERAL COMMAND stalls the boot at 19 tasks with TASK0 inside the NDS CA
    // init waiting for SI sections -- which was a real thing to wait for, at a time when
    // nothing could broadcast any. It can now. The two halves have never been on the machine
    // at once, so this pairs them rather than repeating either.
    if (opts.ackAll) window.__ackSet('all');
    // PROFILING FROM THE FIRST INSTRUCTION. __pcHits accumulates only while profiling is on,
    // and a probe body runs AFTER the boot is asserted -- so anything that happens during the
    // boot is invisible to it, and a count of 0 for a boot-time routine means "I was not
    // watching", not "it did not run". That cost two readings before it was noticed. This flag
    // arms the profiler in the pre-boot window, where --trace-boot already arms the printf.
    if (opts.profileBoot) window.__profile(true);
    const ee = window.__eeprom(); let written = 0;
    for (let i = 0; i < ee.length; i++) if (ee[i] !== 0xFF) written++;
    const cold = written < 64;
    const t0 = performance.now();
    document.getElementById('sp-max').click();
    let t = null;
    for (let i = 0; i < 1600; i++) {
      await new Promise(r => setTimeout(r, 250));
      // THE DEBUG MASK IS BSS AND THE APPLICATION CLEARS IT WHEN IT RELOCATES, so setting it
      // once before the boot sets nothing. Re-poking it each poll costs nothing and means the
      // firmware's own commentary is on for every phase of the boot rather than the tail.
      if (opts.dbg) window.__poke(0x80105D14, [0xFF,0xFF,0xFF,0xFF]);
      // A BROADCAST THAT IS ALREADY ON THE AIR WHEN THE BOX STARTS. Switching the carousel on
      // after the boot answers a different question from switching it on before -- a first
      // install has nothing to scan if the multiplex only appears once it has given up.
      if (opts.carousel) window.__siCarousel(true);
      t = window.__tasks();
      if ((t.n || 0) >= 42) break;
    }
    return { tasks: t && t.n, cold, seconds: +((performance.now() - t0) / 1000).toFixed(1),
             icount: parseInt(document.getElementById('icount').textContent.replace(/,/g,''), 10) };
  }, { traceBoot: process.argv.includes('--trace-boot'), dbg: process.argv.includes('--dbg'),
       profileBoot: process.argv.includes('--profile-boot'),
       carousel: process.argv.includes('--carousel'),
       ackAll: process.argv.includes('--ack-all'),
       traceWaits: process.argv.includes('--trace-waits'),
       traceSig: process.argv.includes('--trace-sig'),
       traceStart: process.argv.includes('--trace-start'),
       traceReg: process.argv.includes('--trace-reg'),
       traceApp: process.argv.includes('--trace-app') });
  console.log('BOOT ' + JSON.stringify(boot));
  if (boot.tasks < 42 && !process.argv.includes('--allow-partial'))
    throw new Error('boot did not complete — every reading below would be of the wrong machine');
  for (const f of files) {
    const body = readFileSync(f, 'utf8');
    const out = await page.evaluate(new Function('return (async () => {' + body + '})()'));
    console.log('--- ' + f + '\n' + JSON.stringify(out, null, 1));
  }
} catch (e) {
  console.log('HARNESS ERROR: ' + e.message);
  process.exitCode = 2;
} finally {
  if (logs.length) console.log('--- page log\n' + logs.slice(-40).join('\n'));
  await browser.close();
}
