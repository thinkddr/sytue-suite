/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El BFF de la cola del operador reenvía con el token de la sesión: lo que no esté
 * aquí, anclado y con su método, no puede llegar a ninguna otra ruta de Cast o de
 * Rooms en nombre de quien opera la plataforma.
 */
import { expect, test } from 'bun:test';
import { destinoDeModeracion } from './moderacion.js';

const ID = '7f0c2f4e-8f3a-4f5e-9d3b-0a1b2c3d4e5f';

test('reenvía las cuatro rutas de la cola, a su servicio', () => {
  expect(destinoDeModeracion('GET', 'cast/reports')).toEqual({
    servicio: 'cast',
    resto: 'reports',
  });
  expect(destinoDeModeracion('POST', `cast/reports/${ID}`)).toEqual({
    servicio: 'cast',
    resto: `reports/${ID}`,
  });
  expect(destinoDeModeracion('GET', 'rooms/reports')).toEqual({
    servicio: 'rooms',
    resto: 'reports',
  });
  expect(destinoDeModeracion('POST', `rooms/reports/${ID}/resolve`)).toEqual({
    servicio: 'rooms',
    resto: `reports/${ID}/resolve`,
  });
});

test('nada más: ni otro método, ni otra ruta, ni un salto fuera', () => {
  for (const [metodo, path] of [
    ['POST', 'cast/reports'],
    ['GET', `cast/reports/${ID}`],
    ['POST', `rooms/reports/${ID}`],
    ['DELETE', `cast/reports/${ID}`],
    ['GET', 'cast/reports/../../videos'],
    ['POST', `cast/reports/${ID}/../../blocks`],
    ['GET', 'drive/reports'],
    ['GET', 'cast/moderation/audit'],
    ['GET', 'cast/reportsx'],
  ] as const) {
    expect(destinoDeModeracion(metodo, path)).toBeNull();
  }
});
