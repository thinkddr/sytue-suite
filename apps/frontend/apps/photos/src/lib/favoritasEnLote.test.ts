/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { conFavoritas, planFavoritas, unLoteALaVez } from './favoritasEnLote.js';

const fotos = [
  { id: 'a', favorite: true },
  { id: 'b', favorite: false },
  { id: 'c' },
  { id: 'd', favorite: true },
];

test('si falta alguna por marcar, el lote marca y solo pide las que cambian', () => {
  expect(planFavoritas(fotos, new Set(['a', 'b', 'c']))).toEqual({ valor: true, ids: ['b', 'c'] });
});

test('si todas son ya favoritas, el lote las quita', () => {
  expect(planFavoritas(fotos, new Set(['a', 'd']))).toEqual({ valor: false, ids: ['a', 'd'] });
});

test('las marcadas que ya no están cargadas se marcan, aunque las cargadas lo sean todas', () => {
  expect(planFavoritas(fotos, new Set(['a', 'x', 'd']))).toEqual({ valor: true, ids: ['x'] });
  expect(planFavoritas([], new Set(['x', 'y']))).toEqual({ valor: true, ids: ['x', 'y'] });
});

test('en «Favoritas» quitar las saca de la lista; en la galería solo cambia su corazón', () => {
  const hechas = new Set(['a']);
  expect(conFavoritas(fotos, hechas, false, true).map((f) => f.id)).toEqual(['b', 'c', 'd']);
  expect(conFavoritas(fotos, hechas, false, false)[0]).toEqual({ id: 'a', favorite: false });
  expect(conFavoritas(fotos, new Set(['c']), true, false)[2]).toEqual({ id: 'c', favorite: true });
});

test('mientras un lote está en vuelo, otra pulsación no lanza otro; al volver, se suelta', async () => {
  const cerrojo = { current: false };
  let lanzados = 0;
  let soltar = () => {};
  const lote = () =>
    new Promise<void>((r) => {
      lanzados++;
      soltar = r;
    });
  const primero = unLoteALaVez(cerrojo, lote);
  expect(await unLoteALaVez(cerrojo, lote)).toBe(false);
  expect(lanzados).toBe(1);
  soltar();
  expect(await primero).toBe(true);
  expect(cerrojo.current).toBe(false);
  // Si el lote falla, el cerrojo se suelta igual: el corazón no se queda muerto.
  await unLoteALaVez(cerrojo, () => Promise.reject(new Error('x'))).catch(() => {});
  expect(cerrojo.current).toBe(false);
});
