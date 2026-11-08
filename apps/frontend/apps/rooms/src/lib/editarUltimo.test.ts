// Checks de «↑ edita tu último mensaje». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { ChatMessage } from './chat.js';
import { abreEdicionUltimo, editandoYa, ultimoEditable } from './editarUltimo.js';

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

test('el último propio, aunque haya ajenos detrás', () => {
  const ms = [msg('a', { mine: true }), msg('b', { mine: true }), msg('c')];
  expect(ultimoEditable(ms)?.id).toBe('b');
});

test('se salta lo que no se puede editar: en vuelo, fallido, llamada y tarjeta', () => {
  const ms = [
    msg('bueno', { mine: true }),
    msg('llamada', { mine: true, type: 'call' }),
    msg('encuesta', { mine: true, card: { kind: 'poll' } as NonNullable<ChatMessage['card']> }),
    msg('fallido', { mine: true, failed: true }),
    msg('enviando', { mine: true, pending: true }),
  ];
  expect(ultimoEditable(ms)?.id).toBe('bueno');
});

test('sin mensajes propios no hay nada que editar', () => {
  expect(ultimoEditable([msg('a'), msg('b')])).toBeNull();
  expect(ultimoEditable([])).toBeNull();
});

const flecha = { key: 'ArrowUp', shiftKey: false, altKey: false, ctrlKey: false, metaKey: false };

test('↑ a secas con la caja vacía abre la edición', () => {
  expect(abreEdicionUltimo(flecha, '', 0)).toBe(true);
});

test('con texto, adjuntos, modificadores u otra tecla la flecha es la de siempre', () => {
  expect(abreEdicionUltimo(flecha, 'hola', 0)).toBe(false);
  expect(abreEdicionUltimo(flecha, '', 1)).toBe(false);
  expect(abreEdicionUltimo({ ...flecha, shiftKey: true }, '', 0)).toBe(false);
  expect(abreEdicionUltimo({ ...flecha, metaKey: true }, '', 0)).toBe(false);
  expect(abreEdicionUltimo({ ...flecha, key: 'ArrowDown' }, '', 0)).toBe(false);
});

test('↑ sobre el mensaje que ya se edita solo devuelve el foco, no lo reabre', () => {
  expect(editandoYa('m1', 'm1')).toBe(true);
  expect(editandoYa('m1', 'm2')).toBe(false);
  expect(editandoYa(null, 'm1')).toBe(false);
});
