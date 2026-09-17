import { expect, test } from '@playwright/test';

test('real firmware sends its screen, accepts Sky, and draws the Box Office menu', async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto('/');
  await expect(page.locator('#box-status')).toHaveText('The box is ready. Press sky on the handset.');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await expect(sky).toBeEnabled();
  const screen = page.locator('#screen');
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
  await expect(page.locator('#key-feedback')).toContainText('sky sent to the box');
  await expect.poll(countColours,
  { timeout: 45_000, intervals: [500, 1000] }).toBeGreaterThan(10);
  await expect(page.locator('#box-status')).toContainText('ready');
  expect(errors).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath('real-firmware-menu-light.png'), fullPage: true });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: testInfo.outputPath('real-firmware-menu-dark.png'), fullPage: true });
});
