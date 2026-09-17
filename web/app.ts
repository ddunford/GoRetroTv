import { decodeServerMessage, encodeKeyMessage } from './wire.js';

const canvasNode = document.querySelector<HTMLCanvasElement>('#screen');
const statusNode = document.querySelector<HTMLElement>('#box-status');
const feedbackNode = document.querySelector<HTMLElement>('#key-feedback');
const keys = Array.from(document.querySelectorAll<HTMLButtonElement>('#handset button[data-raw]'));
if (!canvasNode || !statusNode || !feedbackNode) {
  throw new Error('Digibox page is missing its screen, status, or handset feedback');
}
const canvas: HTMLCanvasElement = canvasNode;
const statusLine: HTMLElement = statusNode;
const keyFeedback: HTMLElement = feedbackNode;
const drawingContext = canvas.getContext('2d', { alpha: false });
if (!drawingContext) {
  throw new Error('This browser cannot display the Digibox framebuffer');
}
const context: CanvasRenderingContext2D = drawingContext;

const width = canvas.width;
const height = canvas.height;
const framebuffer = new Uint8Array(width * height);
let palette = new Uint8Array(256 * 3);
let paletteEpoch = 0;
let socket: WebSocket | null = null;
let connected = false;
let ready = false;

function setKeysEnabled(enabled: boolean): void {
  for (const key of keys) key.disabled = !enabled;
}

function showStatus(state: string, message: string): void {
  document.body.dataset.state = state;
  statusLine.textContent = message;
  canvas.setAttribute('aria-label', `Sky Digibox screen. ${message}`);
}

function paint(x: number, y: number, w: number, h: number): void {
  const image = context.createImageData(w, h);
  for (let row = 0; row < h; row++) {
    for (let col = 0; col < w; col++) {
      const index = framebuffer[(y + row) * width + x + col];
      const source = index * 3;
      const target = (row * w + col) * 4;
      image.data[target] = palette[source];
      image.data[target + 1] = palette[source + 1];
      image.data[target + 2] = palette[source + 2];
      image.data[target + 3] = 255;
    }
  }
  context.putImageData(image, x, y);
}

function handleMessage(payload: string): void {
  const message = decodeServerMessage(payload);
  if (message.type === 'palette') {
    palette = new Uint8Array(message.rgb);
    paletteEpoch = message.epoch;
    return;
  }
  if (message.type === 'frame') {
    if (message.epoch !== paletteEpoch || message.x < 0 || message.y < 0 ||
        message.x + message.w > width || message.y + message.h > height ||
        message.pixels.length !== message.w * message.h) {
      throw new Error('The box sent a frame that does not match its screen or palette');
    }
    for (let row = 0; row < message.h; row++) {
      framebuffer.set(message.pixels.subarray(row * message.w, (row + 1) * message.w),
        (message.y + row) * width + message.x);
    }
    paint(message.x, message.y, message.w, message.h);
    return;
  }
  if (message.phase === 'halted') {
    ready = false;
    setKeysEnabled(false);
    showStatus('halted', `The box stopped: ${message.reason || 'the firmware halted.'}`);
    keyFeedback.textContent = 'The handset is unavailable while the box is stopped.';
    return;
  }
  const phaseText: Record<string, string> = {
    booting: 'The box is starting its firmware…',
    'flash-check': 'The box is checking its flash memory…',
    'channel-list': 'The box is rebuilding its channel list…',
    ready: 'The box is ready. Press sky on the handset.',
  };
  ready = message.phase === 'ready';
  setKeysEnabled(connected && ready);
  showStatus(message.phase, message.reason || phaseText[message.phase] || 'The box is working…');
  keyFeedback.textContent = ready ? 'The handset is ready.' : 'The handset will wake when the box is ready.';
}

function connect(): void {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const next = new WebSocket(`${scheme}//${window.location.host}/ws`);
  socket = next;
  showStatus('connecting', 'Connecting to the Digibox…');
  setKeysEnabled(false);
  next.addEventListener('open', () => {
    connected = true;
    showStatus('booting', 'Connected. Waiting for the box to report its state…');
  });
  next.addEventListener('message', (event: MessageEvent<string>) => {
    try {
      handleMessage(event.data);
    } catch (error) {
      ready = false;
      setKeysEnabled(false);
      showStatus('halted', error instanceof Error ? error.message : 'The display data could not be read.');
      next.close();
    }
  });
  next.addEventListener('close', () => {
    if (socket !== next) return;
    connected = false;
    ready = false;
    setKeysEnabled(false);
    if (document.body.dataset.state !== 'halted') showStatus('disconnected', 'The connection to the box was lost.');
    keyFeedback.textContent = 'The handset is unavailable while disconnected.';
  });
}

for (const key of keys) {
  const setPressed = (pressed: boolean): void => {
    if (pressed && key.disabled) return;
    if (pressed) key.dataset.pressed = 'true';
    else delete key.dataset.pressed;
  };
  key.addEventListener('pointerdown', () => setPressed(true));
  key.addEventListener('pointerup', () => setPressed(false));
  key.addEventListener('pointercancel', () => setPressed(false));
  key.addEventListener('pointerleave', () => setPressed(false));
  key.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' || event.key === ' ') setPressed(true);
  });
  key.addEventListener('keyup', (event) => {
    if (event.key === 'Enter' || event.key === ' ') setPressed(false);
  });
  key.addEventListener('blur', () => setPressed(false));
  key.addEventListener('click', () => {
    if (key.disabled || !connected || !ready || socket?.readyState !== WebSocket.OPEN) {
      keyFeedback.textContent = 'The box is not ready for handset input.';
      return;
    }
    const raw = Number(key.dataset.raw);
    socket.send(encodeKeyMessage(raw, 0));
    keyFeedback.textContent = `${key.getAttribute('aria-label') || key.textContent?.trim() || 'Key'} sent to the box.`;
  });
}

connect();
