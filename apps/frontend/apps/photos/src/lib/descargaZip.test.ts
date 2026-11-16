/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { expect, test } from 'bun:test';
import { nombreDeZip, planDeZip, consultaDeZip, TOPE_ZIP } from './descargaZip.js';

const fotos = [
  { id: 'a', driveFileId: 'da' },
  { id: 'b', driveFileId: 'db' },
  { id: 'c', driveFileId: 'dc' },
];

test('la selección se traduce a ids de Drive en su orden', () => {
  expect(planDeZip(['c', 'a'], fotos)).toEqual({ ficheros: ['dc', 'da'], fuera: 0 });
});

test('una foto sin original conocido se cuenta fuera sin romper el resto', () => {
  const conHueco = [...fotos, { id: 'x', driveFileId: '  ' }];
  expect(planDeZip(['a', 'x', 'desconocida'], conHueco)).toEqual({ ficheros: ['da'], fuera: 2 });
});

test('el mismo original no se pide dos veces', () => {
  const repetida = [...fotos, { id: 'a2', driveFileId: 'da' }];
  expect(planDeZip(['a', 'a2'], repetida)).toEqual({ ficheros: ['da'], fuera: 1 });
});

test('el ZIP se recorta al tope de Drive y cuenta lo que queda fuera', () => {
  const muchas = Array.from({ length: TOPE_ZIP + 5 }, (_, i) => ({
    id: `f${i}`,
    driveFileId: `d${i}`,
  }));
  const plan = planDeZip(
    muchas.map((f) => f.id),
    muchas,
  );
  expect(plan.ficheros).toHaveLength(TOPE_ZIP);
  expect(plan.ficheros[0]).toBe('d0');
  expect(plan.fuera).toBe(5);
});

test('el nombre lleva la fecha local y la ruta codifica ids y nombre', () => {
  expect(nombreDeZip('Fotos', new Date(2026, 0, 5))).toBe('Fotos 2026-01-05');
  expect(consultaDeZip(['d1', 'd2'], 'Fotos 2026-01-05')).toBe(
    'file_ids=d1%2Cd2&name=Fotos+2026-01-05',
  );
});
