import { decodeServerMessage, encodeKeyMessage, encodeResetMessage } from './wire.js';
import { describeScreen, unknownScreen } from './screen.js';

const canvasNode = document.querySelector<HTMLCanvasElement>('#screen');
const programmeNode = document.querySelector<HTMLCanvasElement>('#programme');
const statusNode = document.querySelector<HTMLElement>('#box-status');
const feedbackNode = document.querySelector<HTMLElement>('#key-feedback');
const resetNode = document.querySelector<HTMLButtonElement>('#reset-box');
const resetFeedbackNode = document.querySelector<HTMLElement>('#reset-feedback');
const soundToggleNode = document.querySelector<HTMLButtonElement>('#sound-toggle');
const soundStatusNode = document.querySelector<HTMLElement>('#sound-status');
const keys = Array.from(document.querySelectorAll<HTMLButtonElement>('#handset button[data-raw]'));
if (!canvasNode || !programmeNode || !statusNode || !feedbackNode || !resetNode || !resetFeedbackNode || !soundToggleNode || !soundStatusNode) {
  throw new Error('Digibox page is missing its screen, status, handset feedback, or reset control');
}
const canvas: HTMLCanvasElement = canvasNode;
const programme: HTMLCanvasElement = programmeNode;
const statusLine: HTMLElement = statusNode;
const keyFeedback: HTMLElement = feedbackNode;
const resetButton: HTMLButtonElement = resetNode;
const resetFeedback: HTMLElement = resetFeedbackNode;
const soundToggle: HTMLButtonElement = soundToggleNode;
const soundStatus: HTMLElement = soundStatusNode;

// Mirrors the server's own minimum gap between restores, so the page never
// sends a reset the host would silently fold into the previous one.
const resetCooldown = 3000;
const drawingContext = canvas.getContext('2d');
if (!drawingContext) {
  throw new Error('This browser cannot display the Digibox framebuffer');
}
const context: CanvasRenderingContext2D = drawingContext;
const programmeDrawingContext = programme.getContext('2d');
if (!programmeDrawingContext) throw new Error('This browser cannot display programme video');
const programmeContext: CanvasRenderingContext2D = programmeDrawingContext;

const width = canvas.width;
const height = canvas.height;
const framebuffer = new Uint8Array(width * height);
let palette = new Uint8Array(256 * 3);
let paletteAlpha = new Uint8Array(256).fill(255);
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
let resetPending = false;
let resetTimer: ReturnType<typeof setTimeout> | null = null;
let mediaActive = false;
let mediaService = '';
let mediaProgramme = '';
let audioContext: AudioContext | null = null;
let nextAudioTime = 0;
let lastAudioInstruction = -1;
let muted = true;
const scheduledAudio = new Set<AudioBufferSourceNode>();

function showAudioState(state: 'locked' | 'unlocked' | 'muted', message: string): void {
  document.body.dataset.audio = state;
  soundToggle.textContent = state === 'muted' ? 'Unmute sound' : state === 'unlocked' ? 'Mute sound' : 'Enable sound';
  soundToggle.setAttribute('aria-pressed', state === 'unlocked' ? 'true' : 'false');
  soundStatus.textContent = message;
}

function stopAudio(): void {
  for (const source of scheduledAudio) {
    try { source.stop(); } catch { /* A source which already ended needs no further action. */ }
  }
  scheduledAudio.clear();
  nextAudioTime = 0;
  lastAudioInstruction = -1;
}

async function unlockAudio(): Promise<boolean> {
  if (!audioContext) {
    audioContext = new AudioContext();
    audioContext.addEventListener('statechange', () => {
      if (audioContext?.state !== 'running' && !muted) showAudioState('locked', 'Your browser blocked sound. Press Enable sound to try again.');
    });
  }
  try {
    await audioContext.resume();
  } catch {
    showAudioState('locked', 'Your browser blocked sound. Press Enable sound to try again.');
    return false;
  }
  if (audioContext.state !== 'running') {
    showAudioState('locked', 'Your browser blocked sound. Press Enable sound to try again.');
    return false;
  }
  muted = false;
  showAudioState('unlocked', 'Sound is on.');
  return true;
}

const videoBuffer = document.createElement('canvas');
videoBuffer.width = 352;
videoBuffer.height = 288;
const videoDrawingContext = videoBuffer.getContext('2d');
if (!videoDrawingContext) throw new Error('This browser cannot display decoded programme video');
const videoContext: CanvasRenderingContext2D = videoDrawingContext;

function handleBinary(data: ArrayBuffer): void {
  const bytes = new Uint8Array(data);
  if (bytes.length < 16 || new TextDecoder().decode(bytes.subarray(0, 4)) !== 'GRTV' || bytes[4] !== 2) {
    throw new Error('The box sent an unsupported programme stream');
  }
  const payload = bytes.subarray(16);
  if (bytes[5] === 1) {
    if (payload.length !== 352 * 288 * 4) throw new Error('The programme video frame has the wrong size');
    videoContext.putImageData(new ImageData(new Uint8ClampedArray(payload), 352, 288), 0, 0);
    programmeContext.drawImage(videoBuffer, 0, 0, programme.width, programme.height);
    return;
  }
  if (bytes[5] !== 2 || payload.length % 4 !== 0 || muted || !audioContext || audioContext.state !== 'running') return;
  const instruction = Number(new DataView(bytes.buffer, bytes.byteOffset, 16).getBigUint64(8));
  if (!Number.isSafeInteger(instruction) || instruction <= lastAudioInstruction) return;
  lastAudioInstruction = instruction;
  const samples = payload.length / 4;
  const buffer = audioContext.createBuffer(2, samples, 48_000);
  const view = new DataView(payload.buffer, payload.byteOffset, payload.byteLength);
  for (let sample = 0; sample < samples; sample++) {
    buffer.getChannelData(0)[sample] = view.getInt16(sample * 4, true) / 32768;
    buffer.getChannelData(1)[sample] = view.getInt16(sample * 4 + 2, true) / 32768;
  }
  const source = audioContext.createBufferSource();
  source.buffer = buffer;
  source.connect(audioContext.destination);
  scheduledAudio.add(source);
  source.addEventListener('ended', () => scheduledAudio.delete(source));
  nextAudioTime = Math.max(nextAudioTime, audioContext.currentTime + 0.04);
  source.start(nextAudioTime);
  nextAudioTime += buffer.duration;
}

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

// The reset is deliberately NOT gated on the box being ready. A halted or
// wedged box is the one it exists for, so it needs only a socket to send on.
function updateReset(): void {
  resetButton.disabled = !connected || resetPending;
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
      image.data[target + 3] = paletteAlpha[index];
    }
  }
  context.putImageData(image, x, y);
}

function handleMessage(payload: string): void {
  const message = decodeServerMessage(payload);
  if (message.type === 'media') {
    mediaActive = message.active === 1;
    mediaService = message.service;
    mediaProgramme = message.programme;
    document.body.dataset.media = mediaActive ? 'active' : 'inactive';
    if (mediaActive) {
      paint(0, 0, width, height);
      showStatus('ready', `${message.service} — ${message.programme} is playing.`);
    } else if (message.requested === 1 && machineReady) {
      showStatus('ready', 'The selected channel has no configured programme source.');
    }
    return;
  }
  if (message.type === 'palette') {
    if (message.epoch !== paletteEpoch) {
      awaitingFullFrame = true;
      updateKeys();
      if (machineReady) keyFeedback.textContent = 'Waiting for the box to send its screen.';
    }
    palette = new Uint8Array(message.rgb);
    paletteAlpha = new Uint8Array(message.alpha);
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
  acknowledgeReset();
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
    ready: 'The box is ready. Press tv guide on the handset.',
  };
  haltReason = '';
  machineReady = message.phase === 'ready';
  updateKeys();
  showStatus(message.phase, message.reason || phaseText[message.phase] || 'The box is working…');
  if (mediaActive) showStatus('ready', `${mediaService} — ${mediaProgramme} is playing.`);
  keyFeedback.textContent = ready ? 'The handset is ready.' :
    machineReady ? 'Waiting for the box to send its screen.' : 'The handset will wake when the box is ready.';
}

// A state message is the host's answer to a reset: it is pushed with a reason
// naming the intervention, so it arrives even when the phase is unchanged.
function acknowledgeReset(): void {
  if (!resetPending) return;
  resetFeedback.textContent = 'The box was reset.';
}

function connect(): void {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const next = new WebSocket(`${scheme}//${window.location.host}/ws`);
  next.binaryType = 'arraybuffer';
  let openedAt = 0;
  socket = next;
  connected = false;
  stopAudio();
  machineReady = false;
  awaitingFullFrame = true;
  paletteEpoch = -1;
  updateKeys();
  updateReset();
  showStatus('connecting', reconnectAttempts ? 'Reconnecting to the Digibox…' : 'Connecting to the Digibox…');
  keyFeedback.textContent = 'The handset is unavailable while disconnected.';
  next.addEventListener('open', () => {
    if (socket !== next) return;
    connected = true;
    openedAt = Date.now();
    updateReset();
    if (!resetPending) resetFeedback.textContent = '';
    showStatus('booting', 'Connected. Waiting for the box to report its state…');
  });
  next.addEventListener('message', (event: MessageEvent<string | ArrayBuffer>) => {
    if (socket !== next) return;
    try {
      if (typeof event.data === 'string') handleMessage(event.data);
      else handleBinary(event.data);
    } catch (error) {
      machineReady = false;
      updateKeys();
      updateReset();
      haltReason = error instanceof Error ? error.message : 'The display data could not be read';
      showStatus('halted', `The box stopped: ${haltReason}.`);
      next.close();
    }
  });
  next.addEventListener('close', () => {
    if (socket !== next) return;
    socket = null;
    connected = false;
    stopAudio();
    machineReady = false;
    updateKeys();
    updateReset();
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
    if (muted) void unlockAudio();
    keyFeedback.textContent = `${key.getAttribute('aria-label') || key.textContent?.trim() || 'Key'} sent to the box.`;
  });
}

soundToggle.addEventListener('click', () => {
  if (!muted) {
    muted = true;
    stopAudio();
    showAudioState('muted', 'Sound is muted.');
    return;
  }
  void unlockAudio();
});

showAudioState('locked', 'Sound is locked until you enable it or press a handset key.');

resetButton.addEventListener('pointerdown', () => {
  if (!resetButton.disabled) resetButton.dataset.pressed = 'true';
});
for (const event of ['pointerup', 'pointercancel', 'pointerleave', 'blur']) {
  resetButton.addEventListener(event, () => delete resetButton.dataset.pressed);
}
resetButton.addEventListener('click', () => {
  if (resetButton.disabled || socket?.readyState !== WebSocket.OPEN) {
    resetFeedback.textContent = 'The box cannot be reset while the page is disconnected.';
    return;
  }
  socket.send(encodeResetMessage());
  resetPending = true;
  updateReset();
  resetFeedback.textContent = 'Resetting the box…';
  if (resetTimer !== null) clearTimeout(resetTimer);
  // Held for the full cooldown even when the box answers at once: releasing
  // early would let a second press be folded by the host and look swallowed.
  resetTimer = setTimeout(() => {
    resetTimer = null;
    resetPending = false;
    updateReset();
  }, resetCooldown);
});

connect();
