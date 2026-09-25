import { expect, test } from '@playwright/test';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function indexedHash(pixels: Buffer): number {
  let hash = 2166136261;
  for (const pixel of pixels) hash = Math.imul(hash ^ pixel, 16777619) >>> 0;
  return hash;
}

test('deployed HTTPS serves its own assets and no developer routes', async ({ request }) => {
  const health = await request.get('/health');
  expect(health.ok()).toBe(true);
  const identity = await health.json() as { status: string; version: string };
  expect(identity.status).toBe('ok');
  for (const [asset, file] of [['/', 'web/index.html'], [`/styles.css?v=${identity.version}`, 'web/styles.css'],
    ['/favicon.svg', 'web/favicon.svg'], [`/dist/${identity.version}/app.js`, 'web/dist/app.js'],
    [`/dist/${identity.version}/screen.js`, 'web/dist/screen.js'],
    [`/dist/${identity.version}/wire.js`, 'web/dist/wire.js'],
    [`/dist/${identity.version}/wire_generated.js`, 'web/dist/wire_generated.js']] as const) {
    const response = await request.get(asset);
    expect(response.status(), asset).toBe(200);
    const expected = readFileSync(resolve(file));
    expect(await response.body(), asset).toEqual(asset === '/'
      ? Buffer.from(expected.toString()
        .replaceAll('href="/styles.css"', `href="/styles.css?v=${identity.version}"`)
        .replaceAll('src="/dist/app.js"', `src="/dist/${identity.version}/app.js"`))
      : expected);
  }
  expect((await request.get('/dist/nonexistent.js')).status()).toBe(404);
  expect((await request.get('/dist/not-this-build/app.js')).status()).toBe(404);
  for (const path of ['/debug/pprof/', '/debug/pprof/profile', '/debug/pprof/cmdline',
    '/instruments', '/metrics', '/trace']) {
    expect((await request.get(path)).status(), path).toBe(404);
  }
});

test('deployed WSS draws the real frame and TV Guide opens the exact firmware menu', async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  const pixels = Buffer.alloc(720 * 576);
  let palette: Buffer | null = null;
  let paletteAlpha: Buffer | null = null;
  let initial: Buffer | null = null;
  const sent: string[] = [];
  let websocketURL = '';
  page.on('websocket', socket => {
    websocketURL = socket.url();
    socket.on('framesent', frame => { if (typeof frame.payload === 'string') sent.push(frame.payload); });
    socket.on('framereceived', frame => {
      if (typeof frame.payload !== 'string') return;
      const message = JSON.parse(frame.payload) as { type: string; rgb?: string; alpha?: string; pixels?: string;
        x?: number; y?: number; w?: number; h?: number };
      if (message.type === 'palette' && message.rgb && message.alpha) {
        palette = Buffer.from(message.rgb, 'base64');
        paletteAlpha = Buffer.from(message.alpha, 'base64');
      }
      if (message.type !== 'frame' || !message.pixels || message.x === undefined ||
          message.y === undefined || message.w === undefined || message.h === undefined) return;
      const patch = Buffer.from(message.pixels, 'base64');
      for (let y = 0; y < message.h; y++) {
        patch.copy(pixels, (message.y + y) * 720 + message.x, y * message.w, (y + 1) * message.w);
      }
      if (message.x === 0 && message.y === 0 && message.w === 720 && message.h === 576 && !initial) {
        initial = Buffer.from(pixels);
      }
    });
  });

  const response = await page.goto('/');
  expect(response?.status()).toBe(200);
  const expectedWebSocketURL = new URL('/ws', response!.url()).toString().replace(/^http/, 'ws');
  await expect.poll(() => websocketURL).toBe(expectedWebSocketURL);
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  const tvGuide = page.getByRole('button', { name: 'tv guide', exact: true });
  await expect(sky).toBeEnabled();
  await expect(tvGuide).toBeEnabled();
  await expect(page.locator('#box-status')).toHaveText('The box is ready. Press tv guide on the handset.');
  await expect.poll(() => initial !== null && palette !== null && paletteAlpha !== null).toBe(true);
  expect(indexedHash(initial!)).toBe(0xA6A21DC5);

  const frame = initial!;
  const colors = palette!;
  const rgba = Buffer.alloc(frame.length * 4);
  for (let i = 0; i < frame.length; i++) {
    rgba[i * 4] = colors[frame[i] * 3];
    rgba[i * 4 + 1] = colors[frame[i] * 3 + 1];
    rgba[i * 4 + 2] = colors[frame[i] * 3 + 2];
    rgba[i * 4 + 3] = paletteAlpha![frame[i]];
  }
  const expectedDigest = createHash('sha256').update(rgba).digest('hex');
  await expect.poll(() => page.locator('#screen').evaluate(async (canvas: HTMLCanvasElement) => {
    const image = canvas.getContext('2d')!.getImageData(0, 0, canvas.width, canvas.height);
    const digest = await crypto.subtle.digest('SHA-256', image.data);
    return [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('');
  })).toBe(expectedDigest);

  await tvGuide.click();
  await expect.poll(() => sent.length).toBe(1);
  expect(JSON.parse(sent[0])).toMatchObject({ type: 'key', version: 4, raw: 204, source: 0 });
  await expect.poll(() => indexedHash(pixels), { timeout: 45_000, intervals: [500, 1000] })
    .toBe(0x43779DC8);
  await expect(page.locator('#box-status')).toContainText('ready');
  expect(errors).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath('public-menu-light.png'), fullPage: true });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: testInfo.outputPath('public-menu-dark.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
  await page.screenshot({ path: testInfo.outputPath('public-menu-mobile-dark.png'), fullPage: true });
});

test('firmware selection exposes only the guide-configured programme without a browser colour key', async ({ page }, testInfo) => {
  test.setTimeout(270_000);
  let videoFrames = 0;
  let audioChunks = 0;
  const pixels = Buffer.alloc(720 * 576);
  page.on('websocket', socket => socket.on('framereceived', frame => {
    if (typeof frame.payload === 'string') {
      const message = JSON.parse(frame.payload) as { type: string; pixels?: string;
        x?: number; y?: number; w?: number; h?: number };
      if (message.type === 'frame' && message.pixels && message.x !== undefined && message.y !== undefined &&
          message.w !== undefined && message.h !== undefined) {
        const patch = Buffer.from(message.pixels, 'base64');
        for (let y = 0; y < message.h; y++) {
          patch.copy(pixels, (message.y + y) * 720 + message.x, y * message.w, (y + 1) * message.w);
        }
      }
      return;
    }
    const payload = Buffer.from(frame.payload);
    if (payload.subarray(0, 4).toString() !== 'GRTV' || payload[4] !== 2) return;
    if (payload[5] === 1) videoFrames++;
    if (payload[5] === 2) audioChunks++;
  }));
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Reset the box' })).toBeEnabled({ timeout: 15_000 });
  await page.getByRole('button', { name: 'Reset the box' }).click();
  await expect.poll(() => indexedHash(pixels), { timeout: 30_000, intervals: [250, 500] })
    .toBe(0xA6A21DC5);
  await expect(page.getByRole('button', { name: 'Reset the box' })).toBeEnabled({ timeout: 15_000 });
  await page.reload();
  await expect(page.getByRole('button', { name: 'box office', exact: true })).toBeEnabled({ timeout: 30_000 });
  await expect(page.locator('#reset-feedback')).toBeEmpty();

  const press = async (name: string, expectedHash?: number) => {
    await page.getByRole('button', { name, exact: true }).click();
    if (expectedHash !== undefined) {
      await expect.poll(() => indexedHash(pixels), { timeout: 20_000, intervals: [250, 500] })
        .toBe(expectedHash);
    } else {
      await page.waitForTimeout(1_000);
    }
  };
  await press('box office', 0xFE8D1CCC);
  await press('Left');
  await page.waitForTimeout(12_000);
  await press('select');
  // The grid can hold a plausible, half-painted frame for several seconds while listings fill.
  // A SELECT in that interval is swallowed by the real firmware; wait through the measured paint
  // tail rather than treating the first stable-looking canvas as an operable row.
  await page.waitForTimeout(25_000);
  await press('select');

  await expect(page.locator('body')).toHaveAttribute('data-media', 'active', { timeout: 60_000 });
  await expect(page.locator('#box-status')).toHaveText('BBC One — Listings not yet reconstructed is playing.');
  await expect(page.locator('#programme')).toHaveCSS('visibility', 'visible');
  await expect.poll(() => videoFrames, { timeout: 60_000 }).toBeGreaterThan(1);
  await expect.poll(() => audioChunks, { timeout: 60_000 }).toBeGreaterThan(1);
  const firstVideo = await page.locator('#programme').evaluate((canvas: HTMLCanvasElement) =>
    canvas.toDataURL());
  await expect.poll(() => page.locator('#programme').evaluate((canvas: HTMLCanvasElement) =>
    canvas.toDataURL()), { timeout: 10_000 }).not.toBe(firstVideo);
  await page.waitForTimeout(12_000);
  await expect(page.getByRole('button', { name: 'box office', exact: true })).toBeEnabled();
  await page.screenshot({ path: testInfo.outputPath('programme-configured-light.png'), fullPage: true });

  // Re-enter the firmware-owned guide, move from BBC One to ITV (two rows down), and tune it.
  // ITV has listings but no media source. The browser is only observing the resulting service;
  // it neither names ITV nor chooses the source itself.
  await press('box office', 0xFE8D1CCC);
  await press('Left');
  await page.waitForTimeout(12_000);
  await press('select');
  await page.waitForTimeout(25_000);
  await press('Down');
  await press('Down');
  await press('select');
  await expect(page.locator('body')).toHaveAttribute('data-media', 'inactive', { timeout: 60_000 });
  await expect(page.locator('#programme')).toHaveCSS('visibility', 'hidden');
  await expect(page.locator('#box-status'))
    .toHaveText('The selected channel has no configured programme source.');

  await page.screenshot({ path: testInfo.outputPath('programme-unconfigured-light.png'), fullPage: true });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: testInfo.outputPath('programme-unconfigured-dark.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
  await page.screenshot({ path: testInfo.outputPath('programme-unconfigured-mobile-dark.png'), fullPage: true });
});
