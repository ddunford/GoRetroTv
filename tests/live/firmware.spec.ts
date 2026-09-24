import { expect, test } from '@playwright/test';
import { createHash } from 'node:crypto';
import { describeScreen, unknownScreen } from '../../web/dist/screen.js';

function indexedHash(pixels: Buffer): number {
  let hash = 2166136261;
  for (const pixel of pixels) hash = Math.imul(hash ^ pixel, 16777619) >>> 0;
  return hash;
}

test('real firmware sends its screen, accepts Sky, and draws the Box Office menu', async ({ page }, testInfo) => {
  test.setTimeout(180_000);
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  let palette: Buffer | null = null;
  let paletteAlpha: Buffer | null = null;
  let initialFrame: Buffer | null = null;
  let frameMessages = 0;
  const livePixels = Buffer.alloc(720 * 576);
  page.on('websocket', socket => socket.on('framereceived', frame => {
    if (typeof frame.payload !== 'string') return;
    const message = JSON.parse(frame.payload) as { type: string; rgb?: string; alpha?: string; pixels?: string;
      x?: number; y?: number; w?: number; h?: number };
    if (message.type === 'palette' && message.rgb && message.alpha) {
      palette = Buffer.from(message.rgb, 'base64');
      paletteAlpha = Buffer.from(message.alpha, 'base64');
    }
    if (message.type === 'frame' && message.pixels && message.x !== undefined &&
        message.y !== undefined && message.w !== undefined && message.h !== undefined) {
      frameMessages++;
      const patch = Buffer.from(message.pixels, 'base64');
      for (let y = 0; y < message.h; y++) {
        patch.copy(livePixels, (message.y + y) * 720 + message.x, y * message.w, (y + 1) * message.w);
      }
      if (message.x === 0 && message.y === 0 && message.w === 720 && message.h === 576 && initialFrame === null) {
        initialFrame = Buffer.from(livePixels);
      }
    }
  }));
  await page.goto('/');
  await expect(page.locator('#box-status')).toHaveText('The box is ready. Press tv guide on the handset.');
  const sky = page.getByRole('button', { name: 'box office', exact: true });
  await expect(sky).toBeEnabled();
  const screen = page.locator('#screen');
  await expect.poll(() => initialFrame !== null && palette !== null && paletteAlpha !== null).toBe(true);
  const frameBytes = initialFrame!;
  const paletteBytes = palette!;
  const rgba = Buffer.alloc(720 * 576 * 4);
  for (let i = 0; i < frameBytes.length; i++) {
    const index = frameBytes[i];
    rgba[i * 4] = paletteBytes[index * 3];
    rgba[i * 4 + 1] = paletteBytes[index * 3 + 1];
    rgba[i * 4 + 2] = paletteBytes[index * 3 + 2];
    rgba[i * 4 + 3] = paletteAlpha![index];
  }
  expect(indexedHash(frameBytes)).toBe(0xA6A21DC5); // Pinned by board's real snapshot Compose test.
  await expect(screen).toHaveAttribute('aria-label', 'Plain dark blue Digibox screen. No menu is visible.');
  const expectedCanvasSHA = createHash('sha256').update(rgba).digest('hex');
  const browserCanvasSHA = await screen.evaluate(async (canvas: HTMLCanvasElement) => {
    const bytes = canvas.getContext('2d')!.getImageData(0, 0, canvas.width, canvas.height).data;
    const digest = await crypto.subtle.digest('SHA-256', bytes);
    return [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('');
  });
  expect(browserCanvasSHA).toBe(expectedCanvasSHA);
  const pixel = await screen.evaluate((canvas: HTMLCanvasElement) =>
    [...canvas.getContext('2d')!.getImageData(0, 0, 1, 1).data]);
  expect(pixel).toEqual([0, 5, 69, 255]);
  const countColours = () => screen.evaluate((canvas: HTMLCanvasElement) => {
    const pixels = canvas.getContext('2d')!.getImageData(0, 0, canvas.width, canvas.height).data;
    const seen = new Set<number>();
    for (let offset = 0; offset < pixels.length; offset += 4) {
      seen.add((pixels[offset] << 16) | (pixels[offset + 1] << 8) | pixels[offset + 2]);
    }
    return seen.size;
  });
  const before = await countColours();
  expect(before).toBe(1);

  await sky.click();
  await expect(page.locator('#key-feedback')).toContainText('box office sent to the box');
  try {
    await expect.poll(countColours,
    { timeout: 45_000, intervals: [500, 1000] }).toBeGreaterThan(10);
  } catch (error) {
    throw new Error(`Sky menu did not reach the canvas: WebSocket frames=${frameMessages}, ` +
      `latest indexed hash=${indexedHash(livePixels).toString(16)}, page errors=${errors.join('; ')}`, { cause: error });
  }
  await expect.poll(() => indexedHash(livePixels),
  { timeout: 45_000, intervals: [500, 1000] }).toBe(0xFE8D1CCC); // Pinned by board's post-Sky Compose test.
  await expect(page.locator('#box-status')).toContainText('ready');
  // The four sections must be named, not just the six rows: the tab bar is on
  // screen and a description that omits it hides three quarters of the menu.
  await expect(screen).toHaveAttribute('aria-label',
    /Sky menu\. Four sections: TV Guide, Box Office, Services, Interactive; Box Office shown\..*Selected: Movies by Start Time\./);
  const changedChrome = Buffer.from(livePixels);
  changedChrome[0] ^= 1;
  expect(await describeScreen(changedChrome, palette!)).toBe(unknownScreen);
  const changedOption = Buffer.from(livePixels);
  changedOption[164 * 720 + 260] ^= 1;
  expect(await describeScreen(changedOption, palette!)).toBe(unknownScreen);
  const changedPalette = Buffer.from(palette!);
  changedPalette[0] ^= 1;
  expect(await describeScreen(livePixels, changedPalette)).toBe(unknownScreen);
  for (let row = 2; row <= 6; row++) {
    let moved = false;
    for (let attempt = 0; attempt < 5 && !moved; attempt++) {
      await page.waitForTimeout(1500);
      await page.getByRole('button', { name: 'Down' }).click();
      try {
        await expect.poll(() => livePixels[(164 + (row - 1) * 32) * 720 + 125], { timeout: 4_000 }).toBe(97);
        moved = true;
      } catch { /* The firmware can ignore a key while its menu event is in flight. */ }
    }
    expect(moved, `row ${row} was never reached`).toBe(true);
    await expect(screen).toHaveAttribute('aria-label', new RegExp(`Selected: ${[
      'Movies by Start Time', 'Movies A–Z', 'New Movies', 'Sports & Events', 'Specialist', 'Free Previews',
    ][row - 1]}\\.`));
  }
  expect(errors).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath('real-firmware-menu-light.png'), fullPage: true });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: testInfo.outputPath('real-firmware-menu-dark.png'), fullPage: true });
});
