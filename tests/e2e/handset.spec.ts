import { expect, test, type WebSocketRoute } from '@playwright/test';
import { readFileSync } from 'node:fs';

const capturedWire = readFileSync('tests/fixtures/wire.jsonl', 'utf8').trim().split('\n');

function sendFullScreen(ws: WebSocketRoute, litPixel: number): void {
  const pixels = Buffer.alloc(720 * 576);
  pixels[litPixel] = 1;
  ws.send(capturedWire[0]);
  ws.send(JSON.stringify({ type: 'frame', version: 1, seq: 1, epoch: 1,
    x: 0, y: 0, w: 720, h: 576, pixels: pixels.toString('base64') }));
}

function sendReady(ws: WebSocketRoute): void {
  sendFullScreen(ws, 7 * 720 + 5);
  ws.send(JSON.stringify({ type: 'state', version: 1, phase: 'ready', reason: 'The box is ready. Press sky.' }));
}

test('canvas paints pixels from a captured Go WebSocket frame', async ({ page }) => {
  await page.routeWebSocket('**/ws', ws => {
    sendFullScreen(ws, 0);
    for (const message of capturedWire) ws.send(message);
  });
  await page.goto('/');
  await expect.poll(async () => page.locator('#screen').evaluate((element: HTMLCanvasElement) => {
    return [...element.getContext('2d')!.getImageData(5, 7, 1, 1).data];
  })).toEqual([255, 255, 255, 255]);
});

test('handset preserves the screen, refuses input during a socket loss, and resumes after reconnect', async ({ page }, testInfo) => {
  const sockets: WebSocketRoute[] = [];
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    sockets.push(ws);
    ws.onMessage(message => sent.push(String(message)));
    if (sockets.length === 1) sendReady(ws);
  });
  await page.goto('/');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await expect(sky).toBeEnabled();
  const pixel = () => page.locator('#screen').evaluate((element: HTMLCanvasElement) =>
    [...element.getContext('2d')!.getImageData(5, 7, 1, 1).data]);
  await expect.poll(pixel).toEqual([255, 255, 255, 255]);
  await sockets[0].close();
  await expect(page.locator('#box-status')).toContainText('connection to the box was lost');
  await expect(sky).toBeDisabled();
  await expect(page.locator('#key-feedback')).toContainText('unavailable while disconnected');
  await page.screenshot({ path: testInfo.outputPath('disconnected-light.png'), fullPage: true, animations: 'disabled' });
  await sky.evaluate((element: HTMLButtonElement) => element.click());
  expect(sent).toHaveLength(0);
  await expect.poll(pixel).toEqual([255, 255, 255, 255]);
  await expect.poll(() => sockets.length).toBe(2);
  await expect(sky).toBeDisabled();
  sendFullScreen(sockets[1], 7 * 720 + 6);
  sockets[1].send(JSON.stringify({ type: 'state', version: 1, phase: 'ready', reason: '' }));
  await expect(sky).toBeEnabled();
  await expect.poll(pixel).toEqual([0, 0, 0, 255]);
  await expect.poll(() => page.locator('#screen').evaluate((element: HTMLCanvasElement) =>
    [...element.getContext('2d')!.getImageData(6, 7, 1, 1).data])).toEqual([255, 255, 255, 255]);
  await sky.click();
  await expect.poll(() => sent.length).toBe(1);
  const secondClosedAt = Date.now();
  await sockets[1].close();
  await expect.poll(() => sockets.length).toBe(3);
  expect(Date.now() - secondClosedAt).toBeGreaterThanOrEqual(450);
});

test('a guest halt states its reason and refuses handset input', async ({ page }, testInfo) => {
  await page.emulateMedia({ colorScheme: 'dark' });
  let socket: WebSocketRoute;
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    socket = ws;
    ws.onMessage(message => sent.push(String(message)));
    sendReady(ws);
  });
  await page.goto('/');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await expect(sky).toBeEnabled();
  socket!.send(JSON.stringify({ type: 'state', version: 1, phase: 'halted', reason: 'invalid guest instruction at 0x80001234' }));
  await expect(page.locator('#box-status')).toContainText('invalid guest instruction at 0x80001234');
  await expect(sky).toBeDisabled();
  await expect(page.locator('#key-feedback')).toContainText('unavailable while the box is stopped');
  await page.screenshot({ path: testInfo.outputPath('halted-dark.png'), fullPage: true, animations: 'disabled' });
  await sky.evaluate((element: HTMLButtonElement) => element.click());
  expect(sent).toHaveLength(0);
});

test('handset has visible keyboard, pointer, acknowledgement and reduced-motion states @motion', async ({ page }, testInfo) => {
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    ws.onMessage(message => sent.push(String(message)));
    sendReady(ws);
  });
  await page.goto('/');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await expect(sky).toBeEnabled();
  await expect(page.locator('#box-status')).toHaveText('The box is ready. Press sky.');
  await page.screenshot({ path: testInfo.outputPath('handset-ready.png'), fullPage: true, animations: 'disabled' });

  // The reset control sits before the handset, so Tab order enters the keys
  // from there. Seeding focus keeps this about the KEY's focus ring rather
  // than about how many controls happen to precede it.
  await page.locator('#reset-box').focus();
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
    sendReady(ws);
  });
  await page.goto('/');
  const keys = page.locator('#handset button[data-raw]');
  const count = await keys.count();
  expect(count).toBeGreaterThan(20);
  await expect(keys.first()).toBeEnabled();
  await keys.first().focus();
  for (let index = 0; index < count; index++) {
    if (index > 0) await page.keyboard.press('Tab');
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

test('mobile handset stays within the viewport', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto('/');
  const widths = await page.evaluate(() => ({ viewport: innerWidth, page: document.documentElement.scrollWidth }));
  expect(widths.page).toBe(widths.viewport);
  await expect(page.locator('.remote')).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('mobile-dark.png'), fullPage: true, animations: 'disabled' });
});

test('phone keeps the screen visible while handset keys receive focus', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 320, height: 700 });
  await page.routeWebSocket('**/ws', ws => sendReady(ws));
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'sky', exact: true })).toBeEnabled();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(320);
  const keys = page.locator('#handset button[data-raw]');
  await keys.first().focus();
  for (let index = 0; index < await keys.count(); index++) {
    if (index > 0) await page.keyboard.press('Tab');
    await expect(keys.nth(index)).toBeFocused();
    const positions = await page.evaluate(() => {
      const television = document.querySelector('.television')!.getBoundingClientRect();
      const screen = document.querySelector('#screen')!.getBoundingClientRect();
      const key = document.activeElement!.getBoundingClientRect();
      return { televisionTop: television.top, televisionBottom: television.bottom,
        screenTop: screen.top, screenBottom: screen.bottom, keyTop: key.top, viewport: innerHeight };
    });
    expect(positions.screenTop).toBeGreaterThanOrEqual(0);
    expect(positions.screenBottom).toBeLessThan(positions.viewport);
    expect(positions.keyTop).toBeGreaterThanOrEqual(positions.televisionBottom);
  }
  await page.screenshot({ path: testInfo.outputPath('phone-screen-and-handset.png'), animations: 'disabled' });

  await page.setViewportSize({ width: 760, height: 700 });
  await page.getByRole('button', { name: 'select', exact: true }).scrollIntoViewIfNeeded();
  const tablet = await page.evaluate(() => {
    const screen = document.querySelector('#screen')!.getBoundingClientRect();
    const select = [...document.querySelectorAll('button')].find(button => button.textContent?.trim() === 'select')!.getBoundingClientRect();
    return { screenTop: screen.top, screenBottom: screen.bottom, selectTop: select.top, viewport: innerHeight };
  });
  expect(tablet.screenTop).toBeGreaterThanOrEqual(0);
  expect(tablet.screenBottom).toBeLessThan(tablet.selectTop);
  expect(tablet.selectTop).toBeLessThan(tablet.viewport);

  await page.setViewportSize({ width: 320, height: 500 });
  await page.getByRole('button', { name: 'select', exact: true }).scrollIntoViewIfNeeded();
  const compact = await page.evaluate(() => {
    const screen = document.querySelector('#screen')!.getBoundingClientRect();
    const select = [...document.querySelectorAll('button')].find(button => button.textContent?.trim() === 'select')!.getBoundingClientRect();
    return { screenTop: screen.top, screenBottom: screen.bottom, selectTop: select.top, selectBottom: select.bottom };
  });
  expect(compact.screenTop).toBeGreaterThanOrEqual(0);
  expect(compact.screenBottom).toBeLessThan(compact.selectTop);
  expect(compact.selectBottom).toBeLessThan(500);
});

test('touch activates a handset key at mobile width', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 320, height: 700 }, hasTouch: true, isMobile: true });
  const page = await context.newPage();
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    ws.onMessage(message => sent.push(String(message)));
    sendReady(ws);
  });
  await page.goto('http://127.0.0.1:8766/');
  const sky = page.getByRole('button', { name: 'sky', exact: true });
  await sky.tap();
  await expect.poll(() => sent.length).toBe(1);
  expect(JSON.parse(sent[0])).toMatchObject({ type: 'key', raw: 125, source: 0 });
  await expect(page.locator('#key-feedback')).toContainText('sky sent to the box');
  await page.getByRole('button', { name: 'select', exact: true }).tap();
  await expect.poll(() => sent.length).toBe(2);
  const screen = await page.locator('#screen').boundingBox();
  const select = await page.getByRole('button', { name: 'select', exact: true }).boundingBox();
  expect(screen).not.toBeNull();
  expect(select).not.toBeNull();
  expect(screen!.y).toBeGreaterThanOrEqual(0);
  expect(screen!.y + screen!.height).toBeLessThan(select!.y);
  await context.close();
});

test('short landscape keeps screen beside usable handset', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 568, height: 320 });
  await page.routeWebSocket('**/ws', ws => sendReady(ws));
  await page.goto('/');
  const select = page.getByRole('button', { name: 'select', exact: true });
  await expect(select).toBeEnabled();
  await select.scrollIntoViewIfNeeded();
  const bounds = await page.evaluate(() => {
    const screen = document.querySelector('#screen')!.getBoundingClientRect();
    const key = [...document.querySelectorAll('button')].find(button => button.textContent?.trim() === 'select')!.getBoundingClientRect();
    return { screenTop: screen.top, screenBottom: screen.bottom, screenRight: screen.right,
      keyLeft: key.left, keyTop: key.top, keyBottom: key.bottom,
      scrollWidth: document.documentElement.scrollWidth, viewportWidth: innerWidth, viewportHeight: innerHeight };
  });
  expect(bounds.scrollWidth).toBe(bounds.viewportWidth);
  expect(bounds.screenTop).toBeGreaterThanOrEqual(0);
  expect(bounds.screenBottom).toBeLessThan(bounds.viewportHeight);
  expect(bounds.screenRight).toBeLessThan(bounds.keyLeft);
  expect(bounds.keyTop).toBeGreaterThanOrEqual(0);
  expect(bounds.keyBottom).toBeLessThan(bounds.viewportHeight);
  await page.screenshot({ path: testInfo.outputPath('landscape-screen-and-handset.png'), animations: 'disabled' });
});

// The handset has to be usable without scrolling away from the picture. Above
// the handset's floor that means the whole page fits the window; below it the
// page scrolls and the pinned stage keeps the picture in view while you reach
// the keys. Both regimes are asserted, because the fix for one is not the fix
// for the other and a single size would hide whichever broke.
const geometry = (page: import('@playwright/test').Page) => page.evaluate(() => {
  const box = (selector: string) => document.querySelector(selector)!.getBoundingClientRect();
  const visible = (selector: string) => {
    const rect = box(selector);
    return rect.top >= 0 && rect.bottom <= innerHeight;
  };
  const keys = [...document.querySelectorAll<HTMLElement>('#handset button[data-raw]')];
  return {
    scrollY: Math.round(scrollY),
    maxScroll: Math.round(document.body.scrollHeight - innerHeight),
    screen: visible('#screen'),
    status: visible('.status-panel'),
    reset: visible('#reset-box'),
    sky: visible('#handset button[data-raw="0x7D"]'),
    zeroKey: visible('#handset button[data-raw="0x00"]'),
    numberPad: visible('.number-pad'),
    screenTop: Math.round(box('#screen').top),
    // Nothing in the pinned stage may be covered by it.
    resetOnTop: document.elementFromPoint(
      box('#reset-box').left + 4, box('#reset-box').top + 4)?.closest('#reset-box') !== null,
    smallestKey: Math.round(Math.min(...keys.map(key => Math.min(
      key.getBoundingClientRect().width, key.getBoundingClientRect().height)))),
    scrollWidth: document.documentElement.scrollWidth,
  };
});

for (const [width, height] of [[1366, 768], [1440, 900], [1920, 1080]] as const) {
  test(`the box and the whole handset fit ${width}x${height} without scrolling`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height });
    await page.routeWebSocket('**/ws', ws => sendReady(ws));
    await page.goto('/');
    await expect(page.getByRole('button', { name: 'sky', exact: true })).toBeEnabled();

    const fitted = await geometry(page);
    // The point of the whole layout: no scrolling at all.
    expect(fitted.maxScroll).toBeLessThanOrEqual(0);
    expect(fitted.scrollWidth).toBe(width);
    // Both ends of the handset at once, which is what scrolling used to cost.
    expect(fitted.sky).toBe(true);
    expect(fitted.zeroKey).toBe(true);
    expect(fitted.numberPad).toBe(true);
    // And the box it drives, with its status and its recovery control.
    expect(fitted.screen).toBe(true);
    expect(fitted.status).toBe(true);
    expect(fitted.reset).toBe(true);
    expect(fitted.resetOnTop).toBe(true);
    // The handset shrank by giving up spacing, never target size.
    expect(fitted.smallestKey).toBeGreaterThanOrEqual(44);
    await page.screenshot({ path: testInfo.outputPath(`fits-${width}x${height}.png`), animations: 'disabled' });
  });
}

test('a window too short to fit the handset scrolls with the picture pinned', async ({ page }, testInfo) => {
  // Two columns of 44px keys beside a picture do not fit 620px of height, so
  // this is the honest fallback rather than a layout failure.
  await page.setViewportSize({ width: 1280, height: 620 });
  await page.routeWebSocket('**/ws', ws => sendReady(ws));
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'sky', exact: true })).toBeEnabled();

  const top = await geometry(page);
  expect(top.maxScroll).toBeGreaterThan(0); // otherwise this proves nothing

  await page.evaluate(() => window.scrollTo(0, Math.round(document.body.scrollHeight / 2)));
  expect((await geometry(page)).screen).toBe(true);

  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  const bottom = await geometry(page);
  expect(bottom.scrollY).toBe(bottom.maxScroll);
  expect(bottom.screen).toBe(true);
  expect(bottom.numberPad).toBe(true);
  expect(bottom.status).toBe(true);
  expect(bottom.reset).toBe(true);
  expect(bottom.resetOnTop).toBe(true);
  expect(bottom.scrollWidth).toBe(1280);
  // Pinned, not merely still on screen. Capping the stage's height alone
  // shortens the page enough that everything happens to fit at the bottom,
  // which is why removing the sticky positioning left an earlier version of
  // this test passing until it asserted the position.
  expect(bottom.screenTop).toBeGreaterThanOrEqual(16);
  expect(bottom.smallestKey).toBeGreaterThanOrEqual(44);
  await page.screenshot({ path: testInfo.outputPath('short-window-scrolled.png'), animations: 'disabled' });
});
