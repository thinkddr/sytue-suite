/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las ayudas PURAS de las notas al final (endnotes):
 *  - romanNumeral: entero → romano en minúscula, con clamp ≥ 1.
 *  - renumberEndnotes: renumera/recopila en orden de documento (secuencia +
 *    etiqueta romana + contenido por id), con notas faltantes, huérfanas y
 *    marcadores duplicados.
 * Sin DOM (bun corre sin navegador). El pegamento con el folio vive en
 * ribbon/tabs/Referencias.tsx. Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/endnotes.test.ts
 */
import { expect, test } from 'bun:test';
import { renumberEndnotes, romanNumeral } from '../src/editor/endnotes';

test('romanNumeral: enteros a romano en minúscula', () => {
  expect(romanNumeral(1)).toBe('i');
  expect(romanNumeral(3)).toBe('iii');
  expect(romanNumeral(4)).toBe('iv');
  expect(romanNumeral(9)).toBe('ix');
  expect(romanNumeral(14)).toBe('xiv');
  expect(romanNumeral(40)).toBe('xl');
  expect(romanNumeral(2026)).toBe('mmxxvi');
});

test('romanNumeral: clamp a ≥ 1 (índice inválido → «i»)', () => {
  expect(romanNumeral(0)).toBe('i');
  expect(romanNumeral(-5)).toBe('i');
  expect(romanNumeral(Number.NaN)).toBe('i');
  expect(romanNumeral(2.9)).toBe('ii'); // se trunca a 2
});

test('renumberEndnotes: secuencia + romano + contenido en orden de documento', () => {
  // Los marcadores aparecen en el cuerpo en el orden 3, 1, 2 (ids estables).
  const content = new Map([
    ['1', 'Primera'],
    ['2', 'Segunda'],
    ['3', 'Tercera'],
  ]);
  expect(renumberEndnotes(['3', '1', '2'], content)).toEqual([
    { seq: 1, label: 'i', content: 'Tercera' },
    { seq: 2, label: 'ii', content: 'Primera' },
    { seq: 3, label: 'iii', content: 'Segunda' },
  ]);
});

test('renumberEndnotes: marcador sin nota → contenido vacío', () => {
  const content = new Map([['1', 'Solo esta']]);
  expect(renumberEndnotes(['1', '9'], content)).toEqual([
    { seq: 1, label: 'i', content: 'Solo esta' },
    { seq: 2, label: 'ii', content: '' },
  ]);
});

test('renumberEndnotes: nota huérfana (sin marcador) se descarta', () => {
  // «7» está en el mapa pero ningún marcador lo referencia: no sale.
  const content = new Map([
    ['1', 'Citada'],
    ['7', 'Huérfana'],
  ]);
  const out = renumberEndnotes(['1'], content);
  expect(out).toEqual([{ seq: 1, label: 'i', content: 'Citada' }]);
  expect(out.some((e) => e.content === 'Huérfana')).toBe(false);
});

test('renumberEndnotes: marcador duplicado (copiado) → una entrada por aparición', () => {
  const content = new Map([['2', 'Repetida']]);
  expect(renumberEndnotes(['2', '2'], content)).toEqual([
    { seq: 1, label: 'i', content: 'Repetida' },
    { seq: 2, label: 'ii', content: 'Repetida' },
  ]);
});

test('renumberEndnotes: sin marcadores → lista vacía', () => {
  expect(renumberEndnotes([], new Map([['1', 'x']]))).toEqual([]);
});
