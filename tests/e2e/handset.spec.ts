import { expect, test } from '@playwright/test';
import { readFileSync } from 'node:fs';

const capturedWire = readFileSync('tests/fixtures/wire.jsonl', 'utf8').trim().split('\n');

test('canvas paints pixels from a captured Go WebSocket frame', async ({ page }) => {
  await page.routeWebSocket('**/ws', ws => {
    for (const message of capturedWire) ws.send(message);
  });
  await page.goto('/');
  await expect.poll(async () => page.locator('#screen').evaluate((element: HTMLCanvasElement) => {
    return [...element.getContext('2d')!.getImageData(5, 7, 1, 1).data];
  })).toEqual([255, 255, 255, 255]);
});

test('handset explains disconnection and refuses input', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#box-status')).toContainText('connection to the box was lost');
  await expect(page.getByRole('button', { name: 'sky', exact: true })).toBeDisabled();
  await expect(page.locator('#key-feedback')).toContainText('unavailable while disconnected');
});

test('handset has visible keyboard, pointer, acknowledgement and reduced-motion states @motion', async ({ page }, testInfo) => {
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    ws.onMessage(message => sent.push(String(message)));
    ws.send(JSON.stringify({ type: 'state', version: 1, phase: 'ready', reason: 'The box is ready. Press sky.' }));
  });
  await page.goto('/');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await expect(sky).toBeEnabled();
  await expect(page.locator('#box-status')).toHaveText('The box is ready. Press sky.');
  await page.screenshot({ path: testInfo.outputPath('handset-ready.png'), fullPage: true, animations: 'disabled' });

  await page.keyboard.press('Tab');
  await expect(page.getByRole('button', { name: 'Standby' })).toBeFocused();
  const focus = await page.getByRole('button', { name: 'Standby' }).evaluate(element => {
    const style = getComputedStyle(element);
    return { color: style.outlineColor, width: style.outlineWidth, style: style.outlineStyle };
  });
  expect(focus.style).toBe('solid');
  expect(focus.width).toBe('3px');
  expect(focus.color).toBe('rgb(255, 204, 82)');

  await page.keyboard.press('Tab');
  await expect(sky).toBeFocused();
  await page.keyboard.down('Space');
  await expect(sky).toHaveAttribute('data-pressed', 'true');
  await page.keyboard.up('Space');
  await expect(sky).not.toHaveAttribute('data-pressed', 'true');
  await expect.poll(() => sent.length).toBe(1);
  expect(JSON.parse(sent[0])).toMatchObject({ type: 'key', version: 1, raw: 125, source: 0 });
  await expect(page.locator('#key-feedback')).toContainText('sky sent to the box');

  const box = await sky.boundingBox();
  if (!box) throw new Error('Sky key is not laid out');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await expect(sky).toHaveAttribute('data-pressed', 'true');
  await page.mouse.up();
  await expect(sky).not.toHaveAttribute('data-pressed', 'true');
  await expect.poll(() => sent.length).toBe(2);

  await page.emulateMedia({ reducedMotion: 'reduce' });
  const duration = await sky.evaluate(element => getComputedStyle(element).transitionDuration);
  expect(duration).toBe('0.001s, 0.001s');
  await sky.focus();
  await page.keyboard.down('Enter');
  await expect(sky).toHaveAttribute('data-pressed', 'true');
  await page.keyboard.up('Enter');
  await expect(sky).not.toHaveAttribute('data-pressed', 'true');
});

test('every handset key has a visible Tab focus and a usable touch target', async ({ page }) => {
  await page.routeWebSocket('**/ws', ws => {
    ws.send(JSON.stringify({ type: 'state', version: 1, phase: 'ready', reason: '' }));
  });
  await page.goto('/');
  const keys = page.locator('#handset button[data-raw]');
  const count = await keys.count();
  expect(count).toBeGreaterThan(20);
  await expect(keys.first()).toBeEnabled();
  for (let index = 0; index < count; index++) {
    await page.keyboard.press('Tab');
    const key = keys.nth(index);
    await expect(key).toBeFocused();
    const appearance = await key.evaluate(element => {
      const style = getComputedStyle(element);
      const rect = element.getBoundingClientRect();
      return { color: style.outlineColor, width: style.outlineWidth, targetWidth: rect.width, targetHeight: rect.height };
    });
    expect(appearance.color).toBe('rgb(255, 204, 82)');
    expect(appearance.width).toBe('3px');
    expect(appearance.targetWidth).toBeGreaterThanOrEqual(44);
    expect(appearance.targetHeight).toBeGreaterThanOrEqual(44);
  }
});

test('mobile handset stays within the viewport', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto('/');
  const widths = await page.evaluate(() => ({ viewport: innerWidth, page: document.documentElement.scrollWidth }));
  expect(widths.page).toBe(widths.viewport);
  await expect(page.locator('.remote')).toBeVisible();
});

test('touch activates a handset key at mobile width', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 375, height: 812 }, hasTouch: true, isMobile: true });
  const page = await context.newPage();
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    ws.onMessage(message => sent.push(String(message)));
    ws.send(JSON.stringify({ type: 'state', version: 1, phase: 'ready', reason: '' }));
  });
  await page.goto('http://127.0.0.1:8766/');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await sky.tap();
  await expect.poll(() => sent.length).toBe(1);
  expect(JSON.parse(sent[0])).toMatchObject({ type: 'key', raw: 125, source: 0 });
  await expect(page.locator('#key-feedback')).toContainText('sky sent to the box');
  await context.close();
});
