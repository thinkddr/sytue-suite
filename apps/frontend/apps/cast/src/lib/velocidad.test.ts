/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { VELOCIDADES, siguiente } from './velocidad.js';

test('la escalera va de 0,5× a 2× y tiene la normal', () => {
  expect(VELOCIDADES[0]).toBe(0.5);
  expect(VELOCIDADES.at(-1)).toBe(2);
  expect(VELOCIDADES).toContain(1);
});

test('las teclas suben y bajan un peldaño y se paran en los extremos', () => {
  expect(siguiente(1, 1)).toBe(1.25);
  expect(siguiente(1, -1)).toBe(0.75);
  expect(siguiente(2, 1)).toBe(2);
  expect(siguiente(0.5, -1)).toBe(0.5);
});

test('una velocidad fuera de la escalera salta a la vecina en ese sentido', () => {
  expect(siguiente(1.1, 1)).toBe(1.25);
  expect(siguiente(1.1, -1)).toBe(1);
  expect(siguiente(3, -1)).toBe(2);
  expect(siguiente(0.25, 1)).toBe(0.5);
});
