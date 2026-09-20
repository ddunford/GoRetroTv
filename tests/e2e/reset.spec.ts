import { expect, test, type WebSocketRoute } from '@playwright/test';
import { readFileSync } from 'node:fs';

const capturedWire = readFileSync('tests/fixtures/wire.jsonl', 'utf8').trim().split('\n');

function sendFullScreen(ws: WebSocketRoute): void {
  const pixels = Buffer.alloc(720 * 576);
  ws.send(capturedWire[0]);
  ws.send(JSON.stringify({ type: 'frame', version: 1, seq: 1, epoch: 1,
    x: 0, y: 0, w: 720, h: 576, pixels: pixels.toString('base64') }));
}

function sendState(ws: WebSocketRoute, phase: string, reason: string): void {
  ws.send(JSON.stringify({ type: 'state', version: 1, phase, reason }));
}

const resetButton = 'button#reset-box';

test('reset sends one request, refuses a second, and announces the outcome', async ({ page }) => {
  const sent: string[] = [];
  let socket: WebSocketRoute | null = null;
  await page.routeWebSocket('**/ws', ws => {
    socket = ws;
    ws.onMessage(message => sent.push(String(message)));
    sendFullScreen(ws);
    sendState(ws, 'ready', 'The box is ready. Press sky on the handset.');
  });
  await page.goto('/');
  const reset = page.locator(resetButton);
  await expect(reset).toBeEnabled();

  await reset.click();
  expect(sent).toEqual([JSON.stringify({ type: 'reset', version: 1 })]);
  // No double-submit: the control is held for the host's own minimum gap, so a
  // second press cannot become a request the host silently folds away.
  await expect(reset).toBeDisabled();
  await expect(page.locator('#reset-feedback')).toHaveText('Resetting the box…');

  sendState(socket!, 'ready', 'The box was reset and restored to its startup state. Press sky on the handset.');
  await expect(page.locator('#reset-feedback')).toHaveText('The box was reset.');
  await expect(page.locator('#box-status'))
    .toHaveText('The box was reset and restored to its startup state. Press sky on the handset.');
  expect(sent).toHaveLength(1);

  // It comes back by itself once the host would accept another request.
  await expect(reset).toBeEnabled({ timeout: 6000 });
});

test('reset stays available on a halted box, where the handset does not', async ({ page }) => {
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    ws.onMessage(message => sent.push(String(message)));
    sendFullScreen(ws);
    sendState(ws, 'halted', 'guest instruction fault');
  });
  await page.goto('/');
  await expect(page.locator('#box-status')).toContainText('The box stopped');
  // This is the whole point of the control. A reset that went dark exactly
  // when the box did would be decoration.
  await expect(page.locator(resetButton)).toBeEnabled();
  await expect(page.getByRole('button', { name: 'sky', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Standby' })).toBeDisabled();

  await page.locator(resetButton).click();
  expect(sent).toEqual([JSON.stringify({ type: 'reset', version: 1 })]);
});

test('reset is disabled while disconnected and sends nothing', async ({ page }) => {
  const sockets: WebSocketRoute[] = [];
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    sockets.push(ws);
    ws.onMessage(message => sent.push(String(message)));
    if (sockets.length === 1) {
      sendFullScreen(ws);
      sendState(ws, 'ready', 'The box is ready. Press sky on the handset.');
    }
  });
  await page.goto('/');
  const reset = page.locator(resetButton);
  await expect(reset).toBeEnabled();
  await sockets[0].close();
  await expect(reset).toBeDisabled();
  await expect(page.locator('#box-status')).toContainText('connection to the box was lost');
  expect(sent).toEqual([]);
});

test('reset is operable by keyboard and shows its states @motion', async ({ page }, testInfo) => {
  const sent: string[] = [];
  await page.routeWebSocket('**/ws', ws => {
    ws.onMessage(message => sent.push(String(message)));
    sendFullScreen(ws);
    sendState(ws, 'ready', 'The box is ready. Press sky on the handset.');
  });
  await page.goto('/');
  const reset = page.locator(resetButton);
  await expect(reset).toBeEnabled();

  await reset.focus();
  await expect(reset).toBeFocused();
  const outline = await reset.evaluate(element => getComputedStyle(element).outlineWidth);
  expect(outline).not.toBe('0px');
  await page.screenshot({ path: testInfo.outputPath('reset-focused.png'), fullPage: true, animations: 'disabled' });

  await page.keyboard.press('Enter');
  expect(sent).toEqual([JSON.stringify({ type: 'reset', version: 1 })]);
  await expect(reset).toBeDisabled();

  // The transition is token-driven, so reduced motion shortens it rather than
  // leaving a second animation path nobody checks.
  const duration = await reset.evaluate(element => getComputedStyle(element).transitionDuration);
  const reduced = testInfo.project.name === 'reduced-motion';
  expect(duration.startsWith(reduced ? '0.001s' : '0.11s')).toBe(true);
});

test('the reset explains itself and is not mistaken for the handset standby key', async ({ page }) => {
  await page.routeWebSocket('**/ws', ws => {
    sendFullScreen(ws);
    sendState(ws, 'ready', 'The box is ready. Press sky on the handset.');
  });
  await page.goto('/');
  const reset = page.locator(resetButton);
  // The accessible description has to distinguish a HOST restart from the
  // guest key next to it, because both read as "power" to a first-time viewer.
  const describedBy = await reset.getAttribute('aria-describedby');
  expect(describedBy).toBe('reset-note');
  await expect(page.locator('#reset-note')).toContainText('standby key');
  await expect(page.locator('#reset-note')).toContainText('everyone watching');
  // And it lives outside the handset, not among the guest's own keys.
  expect(await reset.evaluate(element => element.closest('#handset') === null)).toBe(true);
});
