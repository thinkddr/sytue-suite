import { expect, test } from 'bun:test';
import { enHilos } from './comentarios.js';
import type { Comentario } from './tipos.js';

const c = (id: string, reply_to?: string, deleted = false): Comentario => ({
  id,
  author: { sub: 's', name: 'Ana' },
  body: id,
  created_at: '2026-09-27T10:00:00Z',
  can: { delete: false },
  ...(reply_to ? { reply_to } : {}),
  ...(deleted ? { deleted } : {}),
});

test('un nivel: la respuesta a una respuesta cuelga del raíz', () => {
  const h = enHilos([c('a'), c('b', 'a'), c('c', 'b'), c('d')]);
  expect(h.map((x) => x.raiz.id)).toEqual(['a', 'd']);
  expect(h[0]!.respuestas.map((x) => x.id)).toEqual(['b', 'c']);
});

test('una respuesta huérfana (su padre está en otra página) se enseña, no se pierde', () => {
  expect(enHilos([c('x', 'nolohay')]).map((x) => x.raiz.id)).toEqual(['x']);
});

test('un borrado sin respuestas desaparece; con respuestas deja el hueco', () => {
  expect(enHilos([c('a', undefined, true)])).toEqual([]);
  const h = enHilos([c('a', undefined, true), c('b', 'a')]);
  expect(h).toHaveLength(1);
  expect(h[0]!.raiz.deleted).toBe(true);
});
