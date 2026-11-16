/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { idsDelRango, marcarConTramo } from './seleccion';

const fotos = Array.from({ length: 12 }, (_, i) => ({ id: `f${i + 1}` }));

test('el tramo va del ancla al pulsado, en cualquier orden y sin los que no tienen id', () => {
  expect(idsDelRango(fotos, 2, 4)).toEqual(['f3', 'f4', 'f5']);
  expect(idsDelRango(fotos, 4, 2)).toEqual(['f3', 'f4', 'f5']);
  expect(idsDelRango([{ id: 'a' }, {}, { id: 'c' }], 0, 2)).toEqual(['a', 'c']);
  // Un ancla que se ha quedado fuera de la lista no inventa posiciones.
  expect(idsDelRango(fotos, 10, 40)).toEqual(['f11', 'f12']);
});

test('marcar la 3 y Mayús + clic en la 10 deja marcadas de la 3 a la 10', () => {
  const uno = marcarConTramo(new Set(), fotos, 2, -1, false);
  expect([...uno.marcadas]).toEqual(['f3']);
  expect(uno.ancla).toBe(2);
  const dos = marcarConTramo(uno.marcadas, fotos, 9, uno.ancla, true);
  expect([...dos.marcadas].sort((a, b) => Number(a.slice(1)) - Number(b.slice(1)))).toEqual([
    'f3',
    'f4',
    'f5',
    'f6',
    'f7',
    'f8',
    'f9',
    'f10',
  ]);
  // El ancla no se mueve: un segundo Mayús + clic sigue contando desde la 3.
  expect(dos.ancla).toBe(2);
});

test('Mayús + clic añade sin desmarcar, y sin ancla es un clic normal', () => {
  const r = marcarConTramo(new Set(['f12']), fotos, 1, 3, true);
  expect(r.marcadas.has('f12')).toBe(true);
  expect(r.marcadas.has('f2') && r.marcadas.has('f4')).toBe(true);
  const s = marcarConTramo(new Set(['f5']), fotos, 4, -1, true);
  expect(s.marcadas.has('f5')).toBe(false);
  expect(s.ancla).toBe(4);
});
