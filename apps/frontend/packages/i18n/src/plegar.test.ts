/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';

import { contiene, plegar, rangos, rangosDeConsulta, terminos } from './plegar.js';

// Los MISMOS casos que `buscar.Plegar` en Go y que la función SQL `plegar(text)`.
const CASOS = new URL('../../../../nucleo/buscar/testdata/casos.json', import.meta.url);

test('plegar dice lo mismo que Go y que SQL', async () => {
  const casos = (await Bun.file(CASOS).json()) as [string, string][];
  expect(casos.length).toBeGreaterThan(10);
  const distintos = casos
    .filter(([de, a]) => plegar(de) !== a)
    .map(
      ([de, a]) =>
        `plegar(${JSON.stringify(de)}) = ${JSON.stringify(plegar(de))}, quiero ${JSON.stringify(a)}`,
    );
  expect(distintos).toEqual([]);
});

test('contiene, en los dos sentidos', () => {
  expect(contiene('Acta de la reunión', 'reunion')).toBe(true);
  expect(contiene('Acta de la reunion', 'REUNIÓN')).toBe(true);
  expect(contiene('Café con leche', 'cafe')).toBe(true);
  expect(contiene('El niño pequeño', 'pequeno')).toBe(true);
  expect(contiene('Frau Muller', 'Müller')).toBe(true);
  expect(contiene('cualquier cosa', '')).toBe(true);
  expect(contiene('Café', 'té')).toBe(false);
});

test('rangos marca lo encontrado sobre el texto original', () => {
  const texto = 'Acta de la REUNIÓN y otra reunion';
  expect(rangos(texto, 'reunion').map(([a, b]) => texto.slice(a, b))).toEqual([
    'REUNIÓN',
    'reunion',
  ]);
  // Tilde combinada (dos caracteres): entra en el rango.
  const nfd = 'cafe\u0301 solo';
  expect(rangos(nfd, 'café').map(([a, b]) => nfd.slice(a, b))).toEqual(['cafe\u0301']);
  expect(rangos('Straße', 'strasse')).toEqual([[0, 6]]);
  expect(rangos('Straße', 's')).toEqual([
    [0, 1],
    [4, 5],
  ]);
  expect(rangos('Müller', 'u')).toEqual([[1, 2]]);
  expect(rangos('algo', '')).toEqual([]);
});

test('terminos quita la sintaxis de la búsqueda', () => {
  expect(terminos('acta "reunión" -borrador or  cafe')).toEqual(['acta', 'reunión', 'cafe']);
});

test('rangosDeConsulta ordena y funde los de cada término', () => {
  // Los mismos que TestRangosDeConsultaFundeYOrdena de apps/nucleo/buscar.
  expect(rangosDeConsulta('Café y reunión del café', 'reunion cafe caf')).toEqual([
    [0, 4],
    [7, 14],
    [19, 23],
  ]);
});
