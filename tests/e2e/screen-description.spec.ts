import { expect, test, type WebSocketRoute } from '@playwright/test';

test('a changed but unrecognised firmware frame immediately loses its old description', async ({ page }) => {
  let socket: WebSocketRoute;
  await page.routeWebSocket('**/ws', ws => {
    socket = ws;
    const palette = Buffer.alloc(256 * 3);
    palette[0] = 0;
    palette[1] = 5;
    palette[2] = 69;
    ws.send(JSON.stringify({ type: 'palette', version: 4, epoch: 1, rgb: palette.toString('base64'),
      alpha: Buffer.alloc(256, 255).toString('base64') }));
    ws.send(JSON.stringify({ type: 'frame', version: 4, seq: 1, epoch: 1,
      x: 0, y: 0, w: 720, h: 576, pixels: Buffer.alloc(720 * 576).toString('base64') }));
    ws.send(JSON.stringify({ type: 'state', version: 4, phase: 'ready', reason: '' }));
  });
  await page.goto('/');
  await expect(page.locator('#screen')).toHaveAttribute('aria-label',
    'Plain dark blue Digibox screen. No menu is visible.');
  socket!.send(JSON.stringify({ type: 'frame', version: 4, seq: 2, epoch: 1,
    x: 0, y: 0, w: 1, h: 1, pixels: Buffer.from([1]).toString('base64') }));
  await expect(page.locator('#screen')).toHaveAttribute('aria-label',
    'Digibox screen changed. A text description of this firmware screen is unavailable.');
});
