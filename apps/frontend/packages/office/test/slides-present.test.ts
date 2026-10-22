/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de la fase Slides-Presentación: mapa de
 * teclado del modo presentación (presentKeyTarget) y reordenación de
 * diapositivas desde el carril (reorderSlides).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { presentKeyTarget, reorderSlides } from '../src/editor/slides/SlidesCanvas';
import type { Slide } from '../src/editor/types';

const deck = (n: number): Slide[] =>
  Array.from({ length: n }, (_, i) => ({ id: `s${i}`, boxes: [] }));

test('presentKeyTarget: avanza (→/Espacio/AvPág) y retrocede (←/RePág) con topes', () => {
  expect(presentKeyTarget('ArrowRight', 0, 4)).toBe(1);
  expect(presentKeyTarget(' ', 2, 4)).toBe(3);
  expect(presentKeyTarget('PageDown', 3, 4)).toBe(4);
  expect(presentKeyTarget('ArrowRight', 4, 4)).toBe(4); // tope final: no pasa de la última
  expect(presentKeyTarget('ArrowLeft', 3, 4)).toBe(2);
  expect(presentKeyTarget('PageUp', 1, 4)).toBe(0);
  expect(presentKeyTarget('ArrowLeft', 0, 4)).toBe(0); // tope inicial
});

test('presentKeyTarget: Home/End saltan, Escape sale y el resto no navega', () => {
  expect(presentKeyTarget('Home', 3, 4)).toBe(0);
  expect(presentKeyTarget('End', 0, 4)).toBe(4);
  expect(presentKeyTarget('Escape', 2, 4)).toBe('exit');
  expect(presentKeyTarget('a', 2, 4)).toBeNull();
  expect(presentKeyTarget('Enter', 2, 4)).toBeNull();
});

test('reorderSlides: mueve conservando la identidad de los objetos y sin mutar', () => {
  const d = deck(4);
  const out = reorderSlides(d, 0, 2);
  expect(out.map((s) => s.id)).toEqual(['s1', 's2', 's0', 's3']);
  expect(out[2]).toBe(d[0]!); // misma referencia: la activa se sigue con findIndex
  expect(d.map((s) => s.id)).toEqual(['s0', 's1', 's2', 's3']); // pura: el original intacto
  // Mover hacia atrás también funciona.
  expect(reorderSlides(d, 3, 0).map((s) => s.id)).toEqual(['s3', 's0', 's1', 's2']);
});

test('reorderSlides: `from` fuera de rango devuelve el array original', () => {
  const d = deck(2);
  expect(reorderSlides(d, 9, 0)).toBe(d);
});
