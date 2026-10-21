/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { esUuid } from './uuid.js';

// Un id que no es UUID no llega a componer la ruta del servicio: `..` y `.` la
// cambiaban (fetch normaliza los segmentos) y, junto a «vaciar», eso es peligroso.
test('solo un UUID es un id de documento', () => {
  expect(esUuid('5f3d1f8e-7c2a-4e5b-8f1a-2b3c4d5e6f70')).toBe(true);
  for (const malo of ['..', '.', '', 'x/../trash', '5f3d1f8e-7c2a-4e5b-8f1a-2b3c4d5e6f70/..']) {
    expect(esUuid(malo)).toBe(false);
  }
});
