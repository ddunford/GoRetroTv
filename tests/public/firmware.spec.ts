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
  expect((await health.json()).status).toBe('ok');
  for (const asset of ['/', '/styles.css', '/favicon.svg', '/dist/app.js',
    '/dist/wire.js', '/dist/wire_generated.js']) {
    const response = await request.get(asset);
    expect(response.status(), asset).toBe(200);
    const file = asset === '/' ? 'web/index.html' : `web${asset}`;
    expect(await response.body(), asset).toEqual(readFileSync(resolve(file)));
  }
  expect((await request.get('/dist/nonexistent.js')).status()).toBe(404);
  for (const path of ['/debug/pprof/', '/debug/pprof/profile', '/debug/pprof/cmdline',
    '/instruments', '/metrics', '/trace']) {
    expect((await request.get(path)).status(), path).toBe(404);
  }
});

test('deployed WSS draws the real frame and Sky opens the exact firmware menu', async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  const pixels = Buffer.alloc(720 * 576);
  let palette: Buffer | null = null;
  let initial: Buffer | null = null;
  const sent: string[] = [];
  let websocketURL = '';
  page.on('websocket', socket => {
    websocketURL = socket.url();
    socket.on('framesent', frame => { if (typeof frame.payload === 'string') sent.push(frame.payload); });
    socket.on('framereceived', frame => {
      if (typeof frame.payload !== 'string') return;
      const message = JSON.parse(frame.payload) as { type: string; rgb?: string; pixels?: string;
        x?: number; y?: number; w?: number; h?: number };
      if (message.type === 'palette' && message.rgb) palette = Buffer.from(message.rgb, 'base64');
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
  await expect.poll(() => websocketURL).toBe('wss://goretrotv.demosrv.uk/ws');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await expect(sky).toBeEnabled();
  await expect(page.locator('#box-status')).toHaveText('The box is ready. Press sky on the handset.');
  await expect.poll(() => initial !== null && palette !== null).toBe(true);
  expect(indexedHash(initial!)).toBe(0xA6A21DC5);

  const frame = initial!;
  const colors = palette!;
  const rgba = Buffer.alloc(frame.length * 4);
  for (let i = 0; i < frame.length; i++) {
    rgba[i * 4] = colors[frame[i] * 3];
    rgba[i * 4 + 1] = colors[frame[i] * 3 + 1];
    rgba[i * 4 + 2] = colors[frame[i] * 3 + 2];
    rgba[i * 4 + 3] = 255;
  }
  const expectedDigest = createHash('sha256').update(rgba).digest('hex');
  await expect.poll(() => page.locator('#screen').evaluate(async (canvas: HTMLCanvasElement) => {
    const image = canvas.getContext('2d')!.getImageData(0, 0, canvas.width, canvas.height);
    const digest = await crypto.subtle.digest('SHA-256', image.data);
    return [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('');
  })).toBe(expectedDigest);

  await sky.click();
  await expect.poll(() => sent.length).toBe(1);
  expect(JSON.parse(sent[0])).toMatchObject({ type: 'key', version: 1, raw: 125, source: 0 });
  await expect.poll(() => indexedHash(pixels), { timeout: 45_000, intervals: [500, 1000] })
    .toBe(0xFE8D1CCC);
  await expect(page.locator('#box-status')).toContainText('ready');
  expect(errors).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath('public-menu-light.png'), fullPage: true });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: testInfo.outputPath('public-menu-dark.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
  await page.screenshot({ path: testInfo.outputPath('public-menu-mobile-dark.png'), fullPage: true });
});
