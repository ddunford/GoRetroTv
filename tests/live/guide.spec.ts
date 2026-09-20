import { expect, test } from '@playwright/test';
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

// TC-6.9, through the browser.
//
// The Go tests drive box.CSI.Key directly and hash the composed framebuffer.
// What they cannot show is that the WHOLE PATH works -- a click in the page,
// over the WebSocket, into the CSI link, and the drawn frame back out again --
// and that path is what a visitor actually uses.
//
// The frames are reconstructed here from the socket rather than read off the
// canvas, so the assertions are about the pixels the box sent, not about what
// a screenshot of a canvas happened to capture.

const WIDTH = 720;
const HEIGHT = 576;

// The now-and-next banner sits across the bottom of the picture. Measured off
// .artifacts renderings: the channel line, the NOW line, the next line and the
// colour-key strip all fall inside this band.
const BANNER_TOP = 400;
const BANNER_HEIGHT = 176;

type Frame = { pixels: Buffer; complete: boolean };

function bannerDigest(pixels: Buffer): string {
  const band = pixels.subarray(BANNER_TOP * WIDTH, (BANNER_TOP + BANNER_HEIGHT) * WIDTH);
  return createHash('sha256').update(band).digest('hex');
}

function isUniform(pixels: Buffer): boolean {
  const band = pixels.subarray(BANNER_TOP * WIDTH, (BANNER_TOP + BANNER_HEIGHT) * WIDTH);
  return band.every(pixel => pixel === band[0]);
}

test('pressing tv guide in the browser draws now and next, and it follows the schedule', async ({ page }, testInfo) => {
  test.setTimeout(240_000);
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));

  const live: Frame = { pixels: Buffer.alloc(WIDTH * HEIGHT), complete: false };
  page.on('websocket', socket => socket.on('framereceived', frame => {
    if (typeof frame.payload !== 'string') return;
    const message = JSON.parse(frame.payload) as { type: string; pixels?: string;
      x?: number; y?: number; w?: number; h?: number };
    if (message.type !== 'frame' || !message.pixels) return;
    const { x, y, w, h } = message;
    if (x === undefined || y === undefined || w === undefined || h === undefined) return;
    const patch = Buffer.from(message.pixels, 'base64');
    for (let row = 0; row < h; row++) {
      patch.copy(live.pixels, (y + row) * WIDTH + x, row * w, (row + 1) * w);
    }
    live.complete = true;
  }));

  await page.goto('/');
  await expect(page.locator('#box-status')).toHaveText('The box is ready. Press tv guide on the handset.');
  await expect.poll(() => live.complete, { timeout: 60_000 }).toBe(true);

  // Wait for a banner that has been DRAWN and has stopped changing, ignoring
  // whatever was on screen before the press. A digest taken at a fixed moment
  // catches the box mid-redraw; one taken after the screen settles catches the
  // blank picture the banner returns to when it times out.
  const pressAndRead = async (label: string): Promise<string> => {
    const before = bannerDigest(live.pixels);
    await page.getByRole('button', { name: 'tv guide', exact: true }).click();
    let candidate = '';
    let steady = 0;
    await expect.poll(async () => {
      const now = bannerDigest(live.pixels);
      if (now === before) { candidate = ''; steady = 0; return false; }
      if (now === candidate) steady++; else { candidate = now; steady = 0; }
      return steady >= 3;
    }, { timeout: 90_000, intervals: [250] }).toBe(true);
    testInfo.attach(`banner-${label}`, { body: Buffer.from(candidate), contentType: 'text/plain' });
    return candidate;
  };

  // The guide has to have something to show, so give the carousel time to put
  // the line-up and the titles on air before asking for them.
  await expect.poll(async () => {
    const digest = await pressAndRead('probe').catch(() => '');
    return digest !== '';
  }, { timeout: 120_000, intervals: [2_000] }).toBe(true);

  const first = await pressAndRead('first');
  expect(isUniform(live.pixels), 'the banner band is a flat colour, so nothing was drawn there').toBe(false);

  // TC-6.9's second half: the drawn banner follows the SCHEDULE. The live box
  // broadcasts from a copy, so this edits that copy and lets TASK-6.8's reload
  // carry it on air -- no restart, exactly as a person editing listings would.
  const schedule = resolve('.artifacts/live-listings/1998-12-24.json');
  const original = readFileSync(schedule, 'utf8');
  const edited = original.replace('"Dream Team"', '"Edited While Watching"');
  expect(edited, 'the fixture no longer contains the programme this test edits').not.toBe(original);
  writeFileSync(schedule, edited);

  let afterEdit = '';
  await expect.poll(async () => {
    afterEdit = await pressAndRead('after-edit');
    return afterEdit !== first;
  }, { timeout: 120_000, intervals: [3_000] }).toBe(true);

  writeFileSync(schedule, original);
  await expect.poll(async () => await pressAndRead('restored') === first,
    { timeout: 120_000, intervals: [3_000] }).toBe(true);

  expect(errors, 'the page reported script errors').toEqual([]);
});
