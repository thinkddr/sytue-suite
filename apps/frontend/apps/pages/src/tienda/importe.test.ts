/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 */
import { expect, test } from 'bun:test';
import { parseImporte, textoImporte } from './importe.js';

test('parseImporte admite coma o punto y rechaza lo que no es un importe', () => {
  expect(parseImporte('12,50')).toBe(1250);
  expect(parseImporte('12.5')).toBe(1250);
  expect(parseImporte(' 7 ')).toBe(700);
  expect(parseImporte('0,05')).toBe(5);
  expect(parseImporte('')).toBeNull();
  expect(parseImporte('-3')).toBeNull();
  expect(parseImporte('1,234')).toBeNull();
  expect(parseImporte('abc')).toBeNull();
});

test('textoImporte da dos decimales con la coma o el punto del idioma', () => {
  expect(textoImporte(1250, 'es')).toBe('12,50');
  expect(textoImporte(1250, 'en')).toBe('12.50');
  expect(textoImporte(123456, 'es')).toBe('1234,56');
});
