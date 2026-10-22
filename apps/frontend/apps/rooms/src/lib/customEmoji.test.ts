// Checks del registro de módulo de emoji personalizados (Ola 14). `bun test`.
import { expect, test } from 'bun:test';
import {
  setCustomEmoji,
  customEmojiList,
  customEmojiMap,
  customEmojiSrc,
  subscribeCustomEmoji,
} from './customEmoji.js';

test('setCustomEmoji rellena lista y mapa nombre→fileId', () => {
  setCustomEmoji([
    { name: 'parrot', fileId: 'f1' },
    { name: 'boom', fileId: 'f2' },
  ]);
  expect(customEmojiList().length).toBe(2);
  expect(customEmojiMap()).toEqual({ parrot: 'f1', boom: 'f2' });
  expect(customEmojiMap().desconocido).toBeUndefined();
});

test('customEmojiSrc apunta al proxy de adjuntos con el fileId escapado', () => {
  expect(customEmojiSrc('a b')).toBe('/api/attachment?fileId=a%20b');
});

test('subscribeCustomEmoji avisa en cada cambio y la baja lo corta', () => {
  let n = 0;
  const off = subscribeCustomEmoji(() => {
    n += 1;
  });
  setCustomEmoji([{ name: 'x', fileId: 'fx' }]);
  expect(n).toBe(1);
  off();
  setCustomEmoji([]);
  expect(n).toBe(1);
});
