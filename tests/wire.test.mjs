import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { decodeServerMessage, encodeKeyMessage } from '../web/dist/wire.js';

const [paletteWire, frameWire] = readFileSync(new URL('./fixtures/wire.jsonl', import.meta.url), 'utf8').trimEnd().split('\n');

test('decodes palette and dirty pixels captured from the Go WebSocket server', () => {
  const palette = decodeServerMessage(paletteWire);
  const frame = decodeServerMessage(frameWire);
  assert.equal(palette.type, 'palette');
  assert.equal(palette.epoch, 1);
  assert.equal(palette.rgb.length, 768);
  assert.deepEqual([...palette.rgb.slice(0, 6)], [0, 0, 0, 255, 255, 255]);
  assert.equal(frame.type, 'frame');
  assert.deepEqual([frame.seq, frame.x, frame.y, frame.w, frame.h, frame.epoch], [1, 5, 7, 1, 1, 1]);
  assert.deepEqual([...frame.pixels], [1]);
});

test('rejects protocol version, impossible rectangle, and truncated bytes', () => {
  const original = JSON.parse(frameWire);
  assert.throws(() => decodeServerMessage(JSON.stringify({ ...original, version: 2 })), /version/);
  assert.throws(() => decodeServerMessage(JSON.stringify({ ...original, x: 720 })), /frame x/);
  assert.throws(() => decodeServerMessage(JSON.stringify({ ...original, pixels: '' })), /length/);
  assert.throws(() => decodeServerMessage(JSON.stringify({ ...original, pixels: '*' })), /base64/);
});

test('encodes key and validates raw/source bytes', () => {
  assert.deepEqual(JSON.parse(encodeKeyMessage(0x11, 2)), { type: 'key', version: 1, raw: 0x11, source: 2 });
  assert.throws(() => encodeKeyMessage(256, 1), /raw/);
  assert.throws(() => encodeKeyMessage(1, -1), /source/);
});
