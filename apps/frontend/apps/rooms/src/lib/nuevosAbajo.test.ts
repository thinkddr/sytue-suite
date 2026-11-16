// Checks de «↓ N mensajes nuevos». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { ChatMessage } from './chat.js';
import { llegadosAbajo } from './nuevosAbajo.js';

function msg(id: string, extra: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id,
    channelId: 'c1',
    author: { id: extra.mine ? 'yo' : 'ana', name: '' },
    body: id,
    createdAt: '2026-11-05T10:00:00Z',
    ...extra,
  };
}

test('cuenta lo ajeno que llega detrás del último que había', () => {
  const ms = [msg('a'), msg('b'), msg('c'), msg('d')];
  expect(llegadosAbajo(ms, ['a', 'b'])).toEqual(['c', 'd']);
});

test('lo propio y lo que va de camino no cuentan', () => {
  const ms = [
    msg('a'),
    msg('b', { mine: true }),
    msg('tmp-1', { mine: true, pending: true }),
    msg('c'),
  ];
  expect(llegadosAbajo(ms, ['a'])).toEqual(['c']);
});

test('sin cambios al final (o un prepend de historial) no hay nada nuevo', () => {
  expect(llegadosAbajo([msg('a'), msg('b')], ['a', 'b'])).toEqual([]);
  expect(llegadosAbajo([msg('z'), msg('a'), msg('b')], ['a', 'b'])).toEqual([]);
});

test('la lista capada pierde por arriba y sigue contando lo de abajo', () => {
  expect(llegadosAbajo([msg('b'), msg('c'), msg('d')], ['a', 'b', 'c'])).toEqual(['d']);
});

test('otra lista (otro canal, otra ventana) reinicia la cuenta', () => {
  expect(llegadosAbajo([msg('x'), msg('y')], ['a', 'b'])).toBeNull();
});

test('sin lista anterior no hay nada que contar', () => {
  expect(llegadosAbajo([msg('a')], [])).toEqual([]);
});

test('borrar el último no tira la cuenta: el ancla es el último que sigue', () => {
  expect(llegadosAbajo([msg('a'), msg('c')], ['a', 'c', 'd'])).toEqual([]);
});

test('confirmar el optimista propio del final no tira la cuenta ni suma', () => {
  const antes = ['a', 'c', 'pending-1'];
  expect(llegadosAbajo([msg('a'), msg('c'), msg('r1', { mine: true })], antes)).toEqual([]);
  expect(llegadosAbajo([msg('a'), msg('c'), msg('r1', { mine: true }), msg('e')], antes)).toEqual([
    'e',
  ]);
});
