// Checks del índice del separador «Nuevos mensajes». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { unreadDividerIndex } from './unread.js';

test('sin no leídos → sin separador', () => {
  expect(unreadDividerIndex(0, 50)).toBeNull();
  expect(unreadDividerIndex(-1, 50)).toBeNull();
});

test('el separador va ante los N mensajes más recientes', () => {
  // 3 no leídos de 50 cargados → primer no leído en el índice 47.
  expect(unreadDividerIndex(3, 50)).toBe(47);
  expect(unreadDividerIndex(1, 50)).toBe(49);
});

test('más no leídos que cargados → se ancla arriba (todo es nuevo)', () => {
  expect(unreadDividerIndex(80, 50)).toBe(0);
  expect(unreadDividerIndex(50, 50)).toBe(0);
});

test('lista vacía → sin separador', () => {
  expect(unreadDividerIndex(5, 0)).toBeNull();
});
