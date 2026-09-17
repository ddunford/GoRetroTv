import {
  FRAME_HEIGHT,
  FRAME_WIDTH,
  STATE_PHASES,
  WIRE_VERSION,
  type FrameMessage,
  type KeyMessage,
  type PaletteMessage,
  type StateMessage,
} from './wire_generated.js';

export type { FrameMessage, KeyMessage, PaletteMessage, StateMessage } from './wire_generated.js';
export { FRAME_HEIGHT, FRAME_WIDTH, WIRE_VERSION } from './wire_generated.js';

export type ServerMessage =
  | (Omit<PaletteMessage, 'rgb'> & { rgb: Uint8Array })
  | (Omit<FrameMessage, 'pixels'> & { pixels: Uint8Array })
  | StateMessage;

function object(value: unknown): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error('wire message must be an object');
  }
  return value as Record<string, unknown>;
}

function integer(value: unknown, name: string, max = Number.MAX_SAFE_INTEGER): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0 || value > max) {
    throw new Error(`invalid ${name}`);
  }
  return value;
}

function bytes(value: unknown, name: string, expected: number): Uint8Array {
  if (typeof value !== 'string' || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) {
    throw new Error(`invalid ${name} base64`);
  }
  const decoded = atob(value);
  if (decoded.length !== expected) throw new Error(`invalid ${name} length`);
  return Uint8Array.from(decoded, (char) => char.charCodeAt(0));
}

export function decodeServerMessage(json: string): ServerMessage {
  const value = object(JSON.parse(json) as unknown);
  if (value.version !== WIRE_VERSION) throw new Error('unsupported wire version');
  switch (value.type) {
    case 'palette': {
      const epoch = integer(value.epoch, 'palette epoch');
      return { type: 'palette', version: WIRE_VERSION, epoch, rgb: bytes(value.rgb, 'palette rgb', 256 * 3) };
    }
    case 'frame': {
      const seq = integer(value.seq, 'frame sequence');
      const epoch = integer(value.epoch, 'frame epoch');
      const x = integer(value.x, 'frame x', FRAME_WIDTH - 1);
      const y = integer(value.y, 'frame y', FRAME_HEIGHT - 1);
      const w = integer(value.w, 'frame width', FRAME_WIDTH);
      const h = integer(value.h, 'frame height', FRAME_HEIGHT);
      if (w === 0 || h === 0 || x + w > FRAME_WIDTH || y + h > FRAME_HEIGHT) {
        throw new Error('frame rectangle outside raster');
      }
      return { type: 'frame', version: WIRE_VERSION, seq, epoch, x, y, w, h, pixels: bytes(value.pixels, 'frame pixels', w * h) };
    }
    case 'state': {
      const phase = value.phase;
      if (typeof phase !== 'string' || !STATE_PHASES.some((item) => item === phase)) {
        throw new Error('invalid machine phase');
      }
      if (typeof value.reason !== 'string') throw new Error('invalid state reason');
      return { type: 'state', version: WIRE_VERSION, phase: phase as StateMessage['phase'], reason: value.reason };
    }
    default:
      throw new Error('unknown server message type');
  }
}

export function encodeKeyMessage(raw: number, source: number): string {
  integer(raw, 'key raw code', 255);
  integer(source, 'key source', 255);
  const message: KeyMessage = { type: 'key', version: WIRE_VERSION, raw, source };
  return JSON.stringify(message);
}
