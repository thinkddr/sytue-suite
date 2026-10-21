/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del cotejo de Buscar y reemplazar (canvas/exec.matchIndices, puro):
 * apariciones no solapadas, sensibilidad a mayúsculas y palabra completa con
 * fronteras Unicode (acentos, ñ). Sin DOM: es la lógica que findTextRanges
 * materializa como rangos y sobre la que actúa el reemplazo.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { matchIndices } from '../src/editor/canvas/exec';

test('matchIndices: apariciones no solapadas en orden; aguja vacía = nada', () => {
  expect(matchIndices('a b a', 'a')).toEqual([
    [0, 1],
    [4, 5],
  ]);
  expect(matchIndices('aaa', 'aa')).toEqual([[0, 2]]); // no solapa: siguiente busca tras el fin
  expect(matchIndices('lo que sea', '')).toEqual([]);
});

test('matchIndices: insensible a mayúsculas por defecto; caseSensitive las distingue', () => {
  expect(matchIndices('Foo foo FOO', 'foo')).toEqual([
    [0, 3],
    [4, 7],
    [8, 11],
  ]);
  expect(matchIndices('Foo foo FOO', 'foo', { caseSensitive: true })).toEqual([[4, 7]]);
});

test('matchIndices: palabra completa respeta fronteras (y las Unicode)', () => {
  // «cat» dentro de «category» queda excluido; los dos «cat» sueltos, no.
  expect(matchIndices('cat category cat', 'cat', { wholeWord: true })).toEqual([
    [0, 3],
    [13, 16],
  ]);
  // Acento como carácter de palabra: «niño» pegado a «od» no es palabra completa.
  expect(matchIndices('niño niñood', 'niño', { wholeWord: true })).toEqual([[0, 4]]);
  // Borde de cadena cuenta como frontera.
  expect(matchIndices('sol', 'sol', { wholeWord: true })).toEqual([[0, 3]]);
  // Signos de puntuación son frontera (no son carácter de palabra).
  expect(matchIndices('¿sol?', 'sol', { wholeWord: true })).toEqual([[1, 4]]);
});

test('matchIndices: caseSensitive + wholeWord combinados', () => {
  expect(matchIndices('Sol sol solar', 'sol', { caseSensitive: true, wholeWord: true })).toEqual([
    [4, 7],
  ]);
});
