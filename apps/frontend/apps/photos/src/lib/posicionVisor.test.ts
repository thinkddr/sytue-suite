/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { posicionVisor, saltoVisor } from './posicionVisor.js';

test('la posición cuenta desde 1 y dice si quedan más por cargar', () => {
  expect(posicionVisor(11, 240)).toEqual({ indice: 12, total: 240, hayMas: false });
  expect(posicionVisor(0, 100, true)).toEqual({ indice: 1, total: 100, hayMas: true });
});

test('una foto sola, o fuera de la lista, no lleva contador', () => {
  expect(posicionVisor(0, 1)).toBeNull();
  expect(posicionVisor(3, 3)).toBeNull();
  expect(posicionVisor(0, 1, true)).toEqual({ indice: 1, total: 1, hayMas: true });
});

test('Inicio va a la primera y Fin a la última; ya allí, no saltan', () => {
  expect(saltoVisor('Home', 11, 240)).toBe(0);
  expect(saltoVisor('End', 11, 240)).toBe(239);
  expect(saltoVisor('Home', 0, 240)).toBeNull();
  expect(saltoVisor('End', 239, 240)).toBeNull();
  expect(saltoVisor('ArrowLeft', 11, 240)).toBeNull();
});
