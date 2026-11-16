/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { diasParaPurga } from './plazoPapelera.js';

const purga = '2026-12-01T10:00:00Z';
const en = (iso: string) => Date.parse(iso);

test('cuenta los días hacia arriba, como Fyne', () => {
  expect(diasParaPurga(purga, en('2026-11-01T10:00:00Z'))).toBe(30);
  // A 30 h quedan dos días, no uno.
  expect(diasParaPurga(purga, en('2026-11-30T04:00:00Z'))).toBe(2);
  expect(diasParaPurga(purga, en('2026-12-01T09:00:00Z'))).toBe(1);
});

test('pasada la fecha le toca hoy: el servicio purga por lotes', () => {
  expect(diasParaPurga(purga, en('2026-12-02T00:00:00Z'))).toBe(0);
});

test('sin fecha de purga no promete ninguna', () => {
  expect(diasParaPurga(undefined, Date.now())).toBeNull();
  expect(diasParaPurga('no es una fecha', Date.now())).toBeNull();
});
