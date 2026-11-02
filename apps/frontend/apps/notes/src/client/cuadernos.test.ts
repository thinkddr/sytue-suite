// SPDX-License-Identifier: Apache-2.0
import { expect, test } from 'bun:test';
import type { Notebook } from '@sytue/api-clients';
import { colocarCuaderno } from './cuadernos.js';

const cuaderno = (id: string, position: number, extra: Partial<Notebook> = {}): Notebook => ({
  id,
  position,
  owner_sub: 'yo',
  parent_id: null,
  title: id,
  emoji: '',
  color: '#146a47',
  layout: 'columns',
  visibility: 'private',
  indexable: false,
  encrypted: false,
  room_id: 'sala',
  note_count: 0,
  created_at: '2026-11-02T00:00:00Z',
  updated_at: '',
  ...extra,
});

test('mover un cuaderno conserva formato, visibilidad, índice y material de cifrado', () => {
  const crypto = {
    v: 1 as const,
    kdf: 'PBKDF2-SHA256' as const,
    salt: 'salt',
    iterations: 210000,
    check: 'check',
  };
  const original = cuaderno('privado', 0, { encrypted: true, crypto });
  const plan = colocarCuaderno(
    [original, cuaderno('destino', 1)],
    original.id,
    'destino',
    'inside',
  )!;
  expect(plan).toHaveLength(1);
  expect(plan[0]!.input).toEqual({
    title: 'privado',
    parent_id: 'destino',
    position: 1024,
    emoji: '',
    color: '#146a47',
    layout: 'columns',
    visibility: 'private',
    indexable: false,
    encrypted: true,
    crypto,
    room_id: 'sala',
  });
  expect(original.parent_id).toBeNull();
});

test('antes y después dan un orden real sin empates, también cuando no queda un entero entre vecinos', () => {
  const inicial = [cuaderno('a', 0), cuaderno('b', 1), cuaderno('c', 2)];
  const aplicar = (id: string, destino: string, lado: 'before' | 'after') => {
    const plan = colocarCuaderno(inicial, id, destino, lado)!;
    return inicial
      .map((nb) => {
        const cambio = plan.find((c) => c.notebook.id === nb.id);
        return cambio ? { ...nb, position: cambio.input.position! } : nb;
      })
      .sort((a, b) => a.position - b.position);
  };
  const despues = aplicar('a', 'b', 'after');
  expect(despues.map((n) => n.id)).toEqual(['b', 'a', 'c']);
  expect(new Set(despues.map((n) => n.position)).size).toBe(3);
  expect(aplicar('c', 'a', 'before').map((n) => n.id)).toEqual(['c', 'a', 'b']);
});

test('no permite ciclos, destinos inexistentes ni cuadernos de solo lectura', () => {
  const todos = [
    cuaderno('padre', 0),
    cuaderno('hijo', 1, { parent_id: 'padre' }),
    cuaderno('solo-lectura', 2, { read_only: true }),
  ];
  expect(colocarCuaderno(todos, 'padre', 'hijo', 'inside')).toBeNull();
  expect(colocarCuaderno(todos, 'padre', 'hijo', 'before')).toBeNull();
  expect(colocarCuaderno(todos, 'padre', 'padre', 'inside')).toBeNull();
  expect(colocarCuaderno(todos, 'hijo', 'solo-lectura', 'inside')).toBeNull();
  expect(colocarCuaderno(todos, 'no-existe', 'padre', 'after')).toBeNull();
});
