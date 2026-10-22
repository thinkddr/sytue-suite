/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mano alzada: limpieza y simplificación del trazo del puntero.
 */
import { expect, test } from 'bun:test';
import { limpiar } from './trazo.js';

test('un toque se conserva, lo no finito se descarta', () => {
  expect(limpiar([5, 5])).toEqual([5, 5]);
  expect(limpiar([Number.NaN, 1, 5, 5, Infinity, 2])).toEqual([5, 5]);
  expect(limpiar([])).toEqual([]);
});

test('los duplicados del puntero parado desaparecen', () => {
  expect(limpiar([0, 0, 0.1, 0, 0, 0.2, 10, 10])).toEqual([0, 0, 10, 10]);
});

test('una recta con ruido se queda en sus extremos y una esquina real se conserva', () => {
  const ruido: number[] = [];
  for (let x = 0; x <= 100; x += 2) ruido.push(x, 50 + (x % 4 === 0 ? 0.3 : -0.3));
  expect(limpiar(ruido)).toHaveLength(4);
  const l: number[] = [];
  for (let x = 0; x <= 50; x += 5) l.push(x, 0);
  for (let y = 5; y <= 50; y += 5) l.push(50, y);
  expect(limpiar(l)).toEqual([0, 0, 50, 0, 50, 50]);
});

test('acaba donde el usuario soltó', () => {
  const out = limpiar([0, 0, 20, 20, 40, 40.2, 40.3, 40.1]);
  expect(out.slice(-2)).toEqual([40.3, 40.1]);
});

test('una curva de miles de puntos se reduce mucho sin apartarse más que la tolerancia', () => {
  const pts: number[] = [];
  for (let i = 0; i <= 3000; i++) {
    const a = (i / 3000) * Math.PI * 2;
    pts.push(200 + 100 * Math.cos(a), 200 + 100 * Math.sin(a));
  }
  const out = limpiar(pts);
  expect(out.length / 2).toBeLessThan(120);
  expect(out.length / 2).toBeGreaterThan(20);
});

test('un trazo larguísimo no desborda la pila', () => {
  const pts: number[] = [];
  for (let i = 0; i < 20000; i++) pts.push(i, Math.sin(i / 7) * 30);
  expect(limpiar(pts).length).toBeGreaterThan(4);
});
