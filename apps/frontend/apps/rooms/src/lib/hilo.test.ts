import { expect, test } from 'bun:test';
import type { ChatMessage } from './chat.js';
import { applyReactionDelta, destinosDeReaccion, mergeEdit, toggleMyReaction } from './hilo.js';

const ctx = { yo: 'yo', canalActivo: 'c1', canalDelHilo: 'c2' };

test('una reacción ajena a una respuesta llega al hilo abierto aunque el canal activo sea otro', () => {
  expect(destinosDeReaccion({ channelId: 'c2', subject: 'ana' }, ctx)).toEqual({
    canal: false,
    hilo: true,
  });
});

test('con el hilo del canal activo abierto, la reacción va a las dos vistas', () => {
  expect(
    destinosDeReaccion({ channelId: 'c1', subject: 'ana' }, { ...ctx, canalDelHilo: 'c1' }),
  ).toEqual({ canal: true, hilo: true });
});

test('el eco de la reacción propia no se aplica (ya se pintó optimista)', () => {
  expect(destinosDeReaccion({ channelId: 'c1', subject: 'yo' }, ctx)).toEqual({
    canal: false,
    hilo: false,
  });
});

test('sin hilo abierto, solo cuenta el canal activo', () => {
  expect(
    destinosDeReaccion({ channelId: 'c2', subject: 'ana' }, { ...ctx, canalDelHilo: null }),
  ).toEqual({ canal: false, hilo: false });
});

const base: ChatMessage = {
  id: 'm',
  channelId: 'c1',
  author: { id: 'yo', name: 'Yo' },
  body: 'hola',
  createdAt: '2026-11-05T10:00:00Z',
  mine: true,
};

test('alternar la reacción propia es su propio inverso', () => {
  const con = toggleMyReaction(base, '👍', 'yo');
  expect(con.reactions).toEqual([{ emoji: '👍', count: 1, mine: true, subs: ['yo'] }]);
  expect(toggleMyReaction(con, '👍', 'yo').reactions).toEqual([]);
});

test('el delta ajeno suma y resta sin tocar «mine»', () => {
  const con = applyReactionDelta(base, '🎉', true, 'ana');
  expect(con.reactions).toEqual([{ emoji: '🎉', count: 1, mine: false, subs: ['ana'] }]);
  expect(applyReactionDelta(con, '🎉', false, 'ana').reactions).toEqual([]);
});

test('una edición conserva reacciones y contador de respuestas', () => {
  const m = { ...base, replyCount: 3, reactions: [{ emoji: '👍', count: 1, mine: false }] };
  const editado = mergeEdit(m, { ...base, body: 'adiós', editedAt: '2026-11-05T10:05:00Z' });
  expect(editado).toMatchObject({ body: 'adiós', replyCount: 3, editedAt: '2026-11-05T10:05:00Z' });
  expect(editado.reactions).toHaveLength(1);
});
