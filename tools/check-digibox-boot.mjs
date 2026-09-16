#!/usr/bin/env node
// THE DIGIBOX EMULATOR'S GATE: does the real firmware still boot, take a key, and accept a
// section after whatever you just changed?
//
// WHY THIS EXISTS. frontend/public/digibox-boot.html is a hardware model, and the failure
// mode of a hardware model is not a stack trace -- it is a machine that runs for ever doing
// something plausible. Twice in one session a change that looked inert stopped the boot and
// was found only by watching a task count by hand fifty seconds later:
//
//   * batching the per-instruction device pumps silently multiplied bootloaderHandoff's idle
//     threshold by sixteen, because that constant counted CALLS and not instructions. 706
//     million instructions in, five tasks, BOOTMain still polling the peripheral link -- which
//     reads exactly like a firmware hang and is not one.
//   * letting the demux register file read back its own writes -- which sounds like an
//     improvement, and was the code comment's stated intent -- stopped the RTOS starting at
//     all. The LISR spins until +0x124's bit 14 reads CLEAR, so "reads back what was written"
//     is an infinite loop.
//
// Neither was caught by anything, because nothing was watching. Both would have been caught
// here in under a minute. An instruction to be careful competes for attention; a check that
// goes red does not.
//
// WHAT IT ASSERTS, and each one is a different layer that has actually broken:
//   1. the boot completes -- 42 Nucleus tasks, the full application
//   2. it completes in a sane number of instructions, so a change that keeps the boot but
//      throws away the link-rate fix (340M instructions instead of 140M) is still a failure
//   3. the demux is programmed -- three section filters on the PIDs the box asks for, and
//      the enable mask that matches the filter records
//   4. a remote key reaches the application's input layer
//   5. a DVB section reaches the firmware's own section handler
//
// PROVE IT CAN FAIL BEFORE YOU TRUST IT: `--self-test` deliberately breaks the machine
// (it feeds a section to a filter the firmware has not armed) and requires the section check
// to go red. A gate nobody has watched fail is decoration.
//
// Usage:  node scripts/check-digibox-boot.mjs [--url URL] [--self-test] [--keep-open]
// Needs the dev server up (./ctl.sh up) and a firmware image the page can load.

// Playwright is a frontend dependency and this script is not in frontend/, so ESM's
// resolution from the script's own directory finds nothing. Resolve it from the package that
// actually declares it rather than requiring the caller to cd somewhere first.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
const { chromium } = createRequire(new URL('../frontend/package.json', import.meta.url))('playwright');

const args = process.argv.slice(2);
const opt = (name, fallback) => {
  const i = args.indexOf(name);
  return i >= 0 && args[i + 1] && !args[i + 1].startsWith('--') ? args[i + 1] : fallback;
};
// Named PAGE_URL and not URL: a module-scope `const URL` shadows the global one that the
// playwright resolution above uses, and the temporal dead zone makes that a hard error at
// line 43 rather than anything that mentions line 50.
// The page does not broadcast unless ?si=1, and this gate depends on that: checks 3 and 4 assert
// the demux carries exactly the three filters the box asks for on a bare boot, and an acquiring
// box adds more. If the broadcast ever becomes the default, this URL needs ?si=0.
// `?si=0` FOR THE SAME REASON THE PROBE RUNNER DOES IT: the page broadcasts by default now, and
// this gate's numbers -- 42 tasks, the three armed section filters, the instruction count -- were
// all measured on a box with nothing on the air. A gate whose baseline moves silently is worse than
// no gate.
function silentUnlessAsked(u) {
  if (/[?&]si=/.test(u)) return u;
  return u + (u.includes('?') ? '&' : '?') + 'si=0';
}
const PAGE_URL = silentUnlessAsked(opt('--url', 'http://localhost:5173/digibox-boot.html'));
const SELF_TEST = args.includes('--self-test');
const BOOT_TIMEOUT_S = Number(opt('--timeout', '120'));

// THE BOOT COSTS THREE TIMES AS MUCH THE FIRST TIME, and this check found that out by being
// the first thing ever to run against a fresh browser profile. The box's NVRAM is a 24C128
// persisted to localStorage, so a machine that has booted before starts from a written one:
//
//   warm (NVRAM written)   140M instructions, ~31s     20 tasks @56M -> 42 @140M
//   cold (NVRAM blank)     447M instructions, ~96s     20 tasks @56M -> 21 @284M -> 42 @447M
//
// The 228 million instructions between 20 tasks and 21 are the box doing its first-time
// initialisation, and they are real work rather than an artefact: wiping the NVRAM in a
// headed browser reproduces the headless figure to three significant figures. So the ceiling
// has to know which boot it is watching -- one number would either miss a regression on warm
// boots or cry wolf on every fresh profile.
const EXPECT_TASKS = 42;
const MAX_WARM_INSTRUCTIONS = 220e6;
const MAX_COLD_INSTRUCTIONS = 600e6;

const results = [];
const check = (name, ok, detail) => { results.push({ name, ok, detail }); return ok; };

const browser = await chromium.launch();
const page = await browser.newPage();
const consoleErrors = [];
page.on('console', m => { if (m.type() === 'error') consoleErrors.push(m.text()); });
page.on('pageerror', e => consoleErrors.push('pageerror: ' + e.message));

let exitCode = 0;
try {
  await page.goto(PAGE_URL, { waitUntil: 'domcontentloaded' });
  // WAIT FOR THE MACHINE TO EXIST BEFORE ASKING IT ANYTHING. domcontentloaded is not ready:
  // the page builds its NVRAM array empty and only fills it from localStorage (or with 0xFF
  // for a fresh one) in its own init. Reading it in that window reports 16,384 written bytes
  // on a blank device, which labelled a cold boot warm and failed the instruction ceiling
  // with a number that was correct for the boot it actually was.
  // The signal is the NVRAM array itself, because that is the thing being read. eeLoad()
  // fills it with 0xFF -- blank, as a 24C128 leaves the fab -- and then overlays anything
  // saved, so an array that is still entirely 0x00 is a Uint8Array nobody has touched.
  // (The Run button is NOT the signal: it stays disabled while the page loads its own image
  // and starts, so waiting on it times out on a machine that is booting perfectly well.)
  await page.waitForFunction(
    () => typeof window.__eeprom === 'function' && window.__eeprom().some(b => b !== 0x00),
    { timeout: 30000 }
  ).catch(() => { throw new Error('the page never initialised its NVRAM within 30s — is the firmware image being served?'); });

  // ---- 1 and 2: the boot ---------------------------------------------------
  const boot = await page.evaluate(async (limitS) => {
    if (typeof window.__tasks !== 'function') return { fatal: '__tasks() is missing — the page did not load its debug API' };
    // Read the NVRAM BEFORE booting: the box writes it during the boot, so asking afterwards
    // would report every boot as warm.
    var ee = window.__eeprom();
    var written = 0;
    for (var i = 0; i < ee.length; i++) if (ee[i] !== 0xFF) written++;
    const cold = written < 64;
    const t0 = performance.now();
    document.getElementById('sp-max').click();
    let t = null;
    // Milestones as well as the total, because "the boot costs more instructions than it
    // used to" and "the boot is stuck somewhere new" look identical in a single final number,
    // and the first thing anyone will want to know is WHERE the extra went.
    const marks = [];
    let seen = 0;
    const ic = () => parseInt(document.getElementById('icount').textContent.replace(/,/g, ''), 10);
    for (let i = 0; i < limitS * 4; i++) {
      await new Promise(r => setTimeout(r, 250));
      t = window.__tasks();
      const n = t.n || 0;
      if (n > seen) { marks.push(n + '@' + (ic() / 1e6).toFixed(0) + 'M'); seen = n; }
      if (n >= 42) break;
    }
    return {
      tasks: t && t.n ? t.n : 0,
      error: t && t.error ? t.error : null,
      seconds: +((performance.now() - t0) / 1000).toFixed(1),
      icount: ic(),
      marks: marks.slice(-8).join(' '),
      cold: cold,
      nvramBytes: written,
    };
  }, BOOT_TIMEOUT_S);

  if (boot.fatal) {
    check('page loads its debug API', false, boot.fatal);
  } else {
    check('boot reaches the full task list', boot.tasks >= EXPECT_TASKS,
          `${boot.tasks} tasks in ${boot.seconds}s (want >= ${EXPECT_TASKS})`
          + (boot.error ? ` — ${boot.error}` : ''));
    const ceiling = boot.cold ? MAX_COLD_INSTRUCTIONS : MAX_WARM_INSTRUCTIONS;
    check(`boot costs a sane number of instructions (${boot.cold ? 'cold' : 'warm'})`,
          boot.icount > 0 && boot.icount < ceiling,
          `${(boot.icount / 1e6).toFixed(1)}M (want < ${ceiling / 1e6}M, NVRAM ${boot.nvramBytes} bytes written)`
          + `  [${boot.marks}]`);
  }

  // Everything below needs a booted machine; running it on a half-booted one reports on a
  // state nobody chose, which is its own kind of wrong answer.
  if (boot.tasks >= EXPECT_TASKS) {
    // ---- 3: the demux is programmed ---------------------------------------
    const dmx = await page.evaluate(() => {
      const d = window.__dispState();
      return { pids: d.pids, enable: d.enable[2] };
    });
    const wantPids = { 22: '0x14', 23: '0x11', 24: '0x10' };
    const got = Object.fromEntries(dmx.pids.map(p => [p.ch, p.pid]));
    const pidsOk = Object.entries(wantPids).every(([ch, pid]) => got[ch] === pid);
    check('the demux programs its section filters', pidsOk,
          `filters ${JSON.stringify(got)} (want ${JSON.stringify(wantPids)} — TDT, SDT, NIT)`);
    check('the section filters are armed', dmx.enable === '0xFFC00000',
          `enable ${dmx.enable} (want 0xFFC00000 — filters 22..31)`);

    // ---- 3b: let the box settle before asking it anything about input -----
    // MAPPING THE SECOND FLASH CHIP GAVE THE BOX REAL WORK TO DO AFTER THE BOOT. The resident
    // content manager validates the partition in bank 1 by CRC-ing 1.5 MB of it, which a real
    // box does in a quarter of a second and this emulator does in about twenty-five, and while
    // it runs the machine is saturated and key frames on the CSI link are dropped. Measured:
    // three presses inside that window produced one key event between them, and eight presses
    // outside it produced two events each, every time.
    //
    // So a key check that fires as soon as 42 tasks exist is a coin toss. Waiting for BGLOAD
    // (TASK20) to stop being scheduled is the signal, because that IS the task doing the work,
    // and it is reported rather than assumed: a run that never settles says so instead of
    // quietly testing a saturated box.
    //
    // AND THE STILLNESS TEST HAS TO WAIT FOR BGLOAD TO HAVE RUN, which the first version of
    // this did not: measured on a cold boot, TASK20.runs reads 1 for about two seconds after
    // the 42nd task appears, climbs to 2589 over the next eighteen, and then never moves
    // again. "The count did not change twice running" is satisfied by the plateau BEFORE the
    // work as readily as by the one after it, so this reported "BGLOAD stopped being scheduled
    // after 2s" on a box that had not started -- the exact saturated state the wait exists to
    // avoid, reported as the absence of it. The key check below passed anyway, because a key
    // reaching the input layer is a weaker claim than the application redrawing; it was a coin
    // toss dressed as a measurement. So: track the PEAK, and only count stillness once the peak
    // has risen above where it started.
    const settle = await page.evaluate(async () => {
      const runs = () => { let r = -1; window.__tasks().tasks.forEach(t => { if (t.name === 'TASK20') r = t.runs; }); return r; };
      const first = runs();
      let peak = first, still = 0;
      for (let i = 0; i < 90; i++) {
        await new Promise(r => setTimeout(r, 1000));
        const now = runs();
        if (now > peak) { peak = now; still = 0; }
        else if (peak > first) still++;
        if (still >= 3) return { settled: true, seconds: i + 1, ran: peak - first };
        // A box where BGLOAD never runs must say so rather than wait out the loop in silence.
        if (peak === first && i >= 60) return { settled: false, seconds: i + 1, ran: 0 };
      }
      return { settled: false, seconds: 90, ran: peak - first };
    });
    check('the box settles after the boot', settle.settled,
          settle.settled
            ? `BGLOAD ran ${settle.ran} times and then stopped, after ${settle.seconds}s`
            : (settle.ran
                 ? `BGLOAD was still being scheduled after ${settle.seconds}s — the checks below would be of a saturated box`
                 : `BGLOAD never ran in ${settle.seconds}s — this check cannot tell a settled box from one that has not started`));

    // ---- 4: a key reaches the application ---------------------------------
    const key = await page.evaluate(async () => {
      window.__profile(true);
      await new Promise(r => setTimeout(r, 500));
      const PCS = [0x800297B0, 0x8006EA04];
      const before = window.__pcHits.apply(null, PCS);
      window.__key(0x21, 0);                      // Sky / logical 0x500, on the handset
      await new Promise(r => setTimeout(r, 2500));
      const after = window.__pcHits.apply(null, PCS);
      return { dispatcher: after['0x800297B0'] - before['0x800297B0'],
               keyEvent: after['0x8006EA04'] - before['0x8006EA04'] };
    });
    check('a remote key reaches the input layer', key.dispatcher > 0 && key.keyEvent > 0,
          `dispatcher +${key.dispatcher}, key events +${key.keyEvent} (want both > 0)`);

    // ---- 5: a section reaches the firmware --------------------------------
    // --self-test aims the same push at a filter the firmware has NOT armed, which must be
    // refused and must therefore make this check go red. That is the proof the check has
    // teeth; without it a green line here means nothing.
    const sect = await page.evaluate(async (selfTest) => {
      const PCS = [0x800041B4, 0x80004520];
      const before = window.__pcHits.apply(null, PCS);
      const push = selfTest
        ? window.__siPushFilter(5, [0x70, 0x70, 0x05, 0xC6, 0x9E, 0x12, 0x00, 0x00])
        : window.__siTDT(1998, 1, 1, 12, 0, 0);
      await new Promise(r => setTimeout(r, 2500));
      const after = window.__pcHits.apply(null, PCS);
      return { push, lisr: after['0x800041B4'] - before['0x800041B4'],
               task: after['0x80004520'] - before['0x80004520'] };
    }, SELF_TEST);
    check('a DVB section reaches the firmware', sect.lisr > 0 && sect.task > 0,
          `push ${sect.push.ok ? 'accepted' : 'refused: ' + sect.push.why}`
          + `, LISR +${sect.lisr}, section task +${sect.task} (want both > 0)`);
  }

  check('no uncaught page errors', consoleErrors.filter(e => !/favicon/.test(e)).length === 0,
        consoleErrors.filter(e => !/favicon/.test(e)).join(' | ') || 'none');

  // EVERY window.__x() THE PAGE CALLS IS ALSO DEFINED BY THE PAGE.
  //
  // This exists because a patch deleted window.__siGuideSlot while leaving both of its call sites,
  // and NOTHING caught it: the file still parsed, because a deleted assignment is not a syntax
  // error; this gate still passed, because it runs with ?si=0 and the caller is on the broadcast
  // path, minutes into a run that never happens here; and the demo host served it for an hour. The
  // box halted with "window.__siGuideSlot is not a function" the moment it first opened a title
  // PID.
  //
  // Driving the carousel here would catch it honestly and costs four minutes; this is structural
  // and costs milliseconds, which is the right trade for a gate that runs after every edit. It is a
  // STRUCTURAL check -- does a name that is called also exist -- not a semantic one, which is the
  // only kind a regex is allowed to make.
  //
  // AND IT ASSERTS ITS OWN SUBJECT COUNT, because a pattern that matches nothing reports a clean
  // page exactly like one that found no faults.
  const pageSrc = readFileSync(new URL('../frontend/public/digibox-boot.html', import.meta.url), 'utf8');
  const defined = new Set([...pageSrc.matchAll(/window\.(__\w+)\s*=/g)].map(m => m[1]));
  const called = new Set([...pageSrc.matchAll(/window\.(__\w+)\s*\(/g)].map(m => m[1]));
  const missing = [...called].filter(n => !defined.has(n));
  check('every window.__x() the page calls is defined by the page',
        missing.length === 0 && defined.size > 20 && called.size > 5,
        missing.length ? ('UNDEFINED: ' + missing.join(' '))
                       : `${called.size} called, ${defined.size} defined`);
} finally {
  if (!args.includes('--keep-open')) await browser.close();
}

const pad = Math.max(...results.map(r => r.name.length));
for (const r of results) {
  console.log(`${r.ok ? 'ok  ' : 'FAIL'}  ${r.name.padEnd(pad)}  ${r.detail}`);
  if (!r.ok) exitCode = 1;
}

if (SELF_TEST) {
  // Inverted on purpose: the run is correct when the section check FAILED.
  const sectionCheck = results.find(r => r.name === 'a DVB section reaches the firmware');
  const proved = sectionCheck && !sectionCheck.ok;
  console.log(proved
    ? '\nself-test: the section check went red on an unarmed filter, as it must — the gate has teeth'
    : '\nself-test: THE SECTION CHECK DID NOT FAIL on an unarmed filter. It cannot detect the thing it claims to.');
  process.exit(proved ? 0 : 1);
}

console.log(exitCode === 0 ? '\ndigibox gate: ok' : '\ndigibox gate: FAILED');
process.exit(exitCode);
