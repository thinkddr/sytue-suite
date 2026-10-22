/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Buscar» con una hoja (sheets/search.ts): el orden es el de leer la hoja, se busca
 * en el texto crudo, «reemplazar todo» junta los cambios por celda y la siguiente
 * coincidencia es la primera detrás de lo escrito. Los mismos casos que
 * `apps/fyne/calcus/buscar_test.go`.
 * Ejecutar: cd apps/frontend && bun test.
 */
import { expect, test } from 'bun:test';
import { rawText } from '../src/editor/sheets/format';
import {
  hitCells,
  nextFrom,
  replaceAllPlan,
  sameMatch,
  sheetMatches,
  withText,
} from '../src/editor/sheets/search';

const cells: Record<string, string> = {
  A2: 'hola',
  B1: '<b>Hola hola</b>',
  C3: '=LEN("hola")',
  A30: 'x',
};
const texto = (ref: string): string => rawText(cells[ref] ?? '');

test('sheetMatches: fila a fila (B1 antes que A2) y en el texto crudo, fórmula incluida', () => {
  const ms = sheetMatches(cells, 'hola', {}, texto, () => false);
  expect(ms.map((m) => `${m.c},${m.r},${m.start}`)).toEqual(['1,0,0', '1,0,5', '0,1,0', '2,2,6']);
  expect(sheetMatches(cells, 'Hola', { caseSensitive: true }, texto, () => false)).toHaveLength(1);
  expect(sheetMatches(cells, 'hola', { wholeWord: true }, texto, () => false)).toHaveLength(4);
  expect(sheetMatches(cells, '', {}, texto, () => false)).toHaveLength(0);
});

test('sheetMatches: lo que no se puede llevar a la vista no se encuentra', () => {
  const ms = sheetMatches(cells, 'hola', {}, texto, (_c, r) => r === 0);
  expect(ms.map((m) => `${m.c},${m.r}`)).toEqual(['0,1', '2,2']);
});

test('withText conserva el envoltorio de la celda y escapa el texto', () => {
  expect(withText('<b>gato y gato</b>', 'perro & gato')).toBe('<b>perro &amp; gato</b>');
  expect(withText('<b>x</b>', '')).toBe('');
});

test('replaceAllPlan: todas las de una celda en un solo texto, en orden de lectura', () => {
  const c = { A1: 'gato y gato', A2: 'gato' };
  const ms = sheetMatches(
    c,
    'gato',
    {},
    (ref) => c[ref as keyof typeof c] ?? '',
    () => false,
  );
  const plan = replaceAllPlan(ms, (m) => (m.r === 0 ? c.A1 : c.A2), 'pez');
  expect(plan).toEqual([
    { c: 0, r: 0, text: 'pez y pez' },
    { c: 0, r: 1, text: 'pez' },
  ]);
});

test('nextFrom: la primera en la posición o detrás; si no hay, vuelta a la primera', () => {
  const ms = [
    { c: 0, r: 0, start: 8, end: 12 },
    { c: 0, r: 1, start: 0, end: 4 },
  ];
  expect(nextFrom(ms, 0, 0, 5)).toBe(0);
  expect(nextFrom(ms, 0, 0, 9)).toBe(1);
  expect(nextFrom(ms, 5, 5, 0)).toBe(0);
  expect(sameMatch(ms[0], { c: 0, r: 0, start: 8, end: 12 })).toBe(true);
  expect(sameMatch(ms[0], ms[1])).toBe(false);
  expect(sameMatch(undefined, ms[1])).toBe(false);
});

test('hitCells: una clave por celda aunque case varias veces, para pintarlas de amarillo', () => {
  const ms = sheetMatches(cells, 'hola', {}, texto, () => false);
  expect([...hitCells(ms)].sort()).toEqual(['0,1', '1,0', '2,2']);
});
