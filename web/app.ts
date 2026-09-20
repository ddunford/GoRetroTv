import { decodeServerMessage, encodeKeyMessage } from './wire.js';
import { describeScreen, unknownScreen } from './screen.js';

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
let machineReady = false;
let awaitingFullFrame = true;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
let haltReason = '';
let screenRevision = 0;

function setKeysEnabled(enabled: boolean): void {
  for (const key of keys) key.disabled = !enabled;
}

function showStatus(state: string, message: string): void {
  document.body.dataset.state = state;
  statusLine.textContent = message;
}

function describeCurrentFrame(): void {
  const revision = ++screenRevision;
  canvas.setAttribute('aria-label', unknownScreen);
  void describeScreen(framebuffer, palette).then(description => {
    if (revision === screenRevision) canvas.setAttribute('aria-label', description);
  }).catch(() => {
    if (revision === screenRevision) canvas.setAttribute('aria-label', unknownScreen);
  });
}

function updateKeys(): void {
  ready = connected && machineReady && !awaitingFullFrame;
  setKeysEnabled(ready);
}

function reconnectDelay(): number {
  const delay = Math.min(5000, 250 * 2 ** Math.min(reconnectAttempts, 5));
  reconnectAttempts++;
  return delay;
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
    if (message.epoch !== paletteEpoch) {
      awaitingFullFrame = true;
      updateKeys();
      if (machineReady) keyFeedback.textContent = 'Waiting for the box to send its screen.';
    }
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
    if (awaitingFullFrame && (message.x !== 0 || message.y !== 0 || message.w !== width || message.h !== height)) {
      throw new Error('The box did not send a complete screen after connecting');
    }
    for (let row = 0; row < message.h; row++) {
      framebuffer.set(message.pixels.subarray(row * message.w, (row + 1) * message.w),
        (message.y + row) * width + message.x);
    }
    paint(message.x, message.y, message.w, message.h);
    describeCurrentFrame();
    if (awaitingFullFrame) {
      awaitingFullFrame = false;
      updateKeys();
      if (ready) keyFeedback.textContent = 'The handset is ready.';
    }
    return;
  }
  if (message.phase === 'halted') {
    machineReady = false;
    updateKeys();
    haltReason = message.reason || 'the firmware halted';
    showStatus('halted', `The box stopped: ${haltReason}.`);
    keyFeedback.textContent = 'The handset is unavailable while the box is stopped.';
    return;
  }
  const phaseText: Record<string, string> = {
    booting: 'The box is starting its firmware…',
    'flash-check': 'The box is checking its flash memory…',
    'channel-list': 'The box is rebuilding its channel list…',
    ready: 'The box is ready. Press sky on the handset.',
  };
  haltReason = '';
  machineReady = message.phase === 'ready';
  updateKeys();
  showStatus(message.phase, message.reason || phaseText[message.phase] || 'The box is working…');
  keyFeedback.textContent = ready ? 'The handset is ready.' :
    machineReady ? 'Waiting for the box to send its screen.' : 'The handset will wake when the box is ready.';
}

function connect(): void {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const next = new WebSocket(`${scheme}//${window.location.host}/ws`);
  let openedAt = 0;
  socket = next;
  connected = false;
  machineReady = false;
  awaitingFullFrame = true;
  paletteEpoch = -1;
  updateKeys();
  showStatus('connecting', reconnectAttempts ? 'Reconnecting to the Digibox…' : 'Connecting to the Digibox…');
  keyFeedback.textContent = 'The handset is unavailable while disconnected.';
  next.addEventListener('open', () => {
    if (socket !== next) return;
    connected = true;
    openedAt = Date.now();
    showStatus('booting', 'Connected. Waiting for the box to report its state…');
  });
  next.addEventListener('message', (event: MessageEvent<string>) => {
    if (socket !== next) return;
    try {
      handleMessage(event.data);
    } catch (error) {
      machineReady = false;
      updateKeys();
      haltReason = error instanceof Error ? error.message : 'The display data could not be read';
      showStatus('halted', `The box stopped: ${haltReason}.`);
      next.close();
    }
  });
  next.addEventListener('close', () => {
    if (socket !== next) return;
    socket = null;
    connected = false;
    machineReady = false;
    updateKeys();
    if (openedAt && Date.now() - openedAt >= 10_000) reconnectAttempts = 0;
    const delay = reconnectDelay();
    if (haltReason) showStatus('halted', `The box stopped: ${haltReason}. Reconnecting…`);
    else showStatus('disconnected', `The connection to the box was lost. Reconnecting in ${Math.ceil(delay / 1000)} second${delay > 1000 ? 's' : ''}…`);
    keyFeedback.textContent = 'The handset is unavailable while disconnected.';
    reconnectTimer = setTimeout(connect, delay);
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
