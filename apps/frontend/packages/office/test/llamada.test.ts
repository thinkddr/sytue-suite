/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Llamar desde el documento»: validación de la respuesta del BFF.
 * Ejecutar: cd apps/frontend/packages/office && bun test test/llamada.test.ts
 */
import { expect, test } from 'bun:test';
import { SIN_LLAMADA, enLlamada, nombreDe, parseLlamada } from '../src/editor/state/llamada';

test('Rooms ausente o respuesta rara: la función no está, sin error', () => {
  expect(parseLlamada(null)).toEqual(SIN_LLAMADA);
  expect(parseLlamada({ available: false, active: true })).toEqual(SIN_LLAMADA);
  expect(parseLlamada('x')).toEqual(SIN_LLAMADA);
});

test('con llamada activa: quién está dentro, descartando entradas inválidas', () => {
  const e = parseLlamada({
    available: true,
    active: true,
    participants: [{ sub: 'a', name: ' Ada ' }, { sub: '' }, { name: 'x' }, 3, { sub: 'b' }],
  });
  expect(e.active).toBe(true);
  expect(e.participants).toEqual([{ sub: 'a', name: 'Ada' }, { sub: 'b' }]);
  expect(enLlamada(e, 'a')).toBe(true);
  expect(enLlamada(e, 'z')).toBe(false);
  expect(nombreDe(e, 'a', new Map())).toBe('Ada');
  expect(nombreDe(e, 'b', new Map([['b', 'Beto']]))).toBe('Beto');
});

test('disponible sin llamada', () => {
  expect(parseLlamada({ available: true, active: false })).toEqual({
    available: true,
    active: false,
    participants: [],
  });
});
