import { expect, test } from '@playwright/test';
import { copyFile, mkdir } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';

function indexedHash(pixels: Buffer): number {
  let hash = 2166136261;
  for (const pixel of pixels) hash = Math.imul(hash ^ pixel, 16777619) >>> 0;
  return hash;
}

test('record the firmware-owned guide tuning a configured channel', async ({ page }) => {
  const pixels = Buffer.alloc(720 * 576);
  let videoFrames = 0;
  page.on('websocket', socket => socket.on('framereceived', frame => {
    if (typeof frame.payload !== 'string') {
      const payload = Buffer.from(frame.payload);
      if (payload.subarray(0, 4).toString() === 'GRTV' && payload[4] === 2 && payload[5] === 1) {
        videoFrames++;
      }
      return;
    }
    const message = JSON.parse(frame.payload) as { type: string; pixels?: string;
      x?: number; y?: number; w?: number; h?: number };
    if (message.type !== 'frame' || !message.pixels || message.x === undefined || message.y === undefined ||
        message.w === undefined || message.h === undefined) return;
    const patch = Buffer.from(message.pixels, 'base64');
    for (let y = 0; y < message.h; y++) {
      patch.copy(pixels, (message.y + y) * 720 + message.x, y * message.w, (y + 1) * message.w);
    }
  }));

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'box office', exact: true })).toBeEnabled();

  const press = async (name: string, expectedHash?: number) => {
    await page.getByRole('button', { name, exact: true }).click();
    if (expectedHash !== undefined) {
      await expect.poll(() => indexedHash(pixels), { timeout: 20_000, intervals: [250, 500] })
        .toBe(expectedHash);
    } else {
      await page.waitForTimeout(1_000);
    }
  };

  await page.waitForTimeout(2_000);
  await press('box office', 0xFE8D1CCC);
  await page.waitForTimeout(1_500);
  await press('Left');
  await page.waitForTimeout(12_000);
  await press('select');
  await page.waitForTimeout(25_000);
  await press('select');
  await expect(page.locator('body')).toHaveAttribute('data-media', 'active', { timeout: 60_000 });
  await expect(page.locator('#programme')).toHaveCSS('visibility', 'visible');
  await expect.poll(() => videoFrames, { timeout: 60_000 }).toBeGreaterThan(1);
  await page.waitForTimeout(8_000);

  const video = page.video();
  expect(video).not.toBeNull();
  await page.close();
  const captured = await video!.path();
  const destination = resolve('.artifacts/goretrotv-linkedin-demo.webm');
  await mkdir(dirname(destination), { recursive: true });
  await copyFile(captured, destination);
});
