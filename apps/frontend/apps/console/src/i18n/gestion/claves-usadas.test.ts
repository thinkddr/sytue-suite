/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ninguna clave del catálogo de gestión sin usar: la misma mecánica que el de la
 * consola (`../claves-usadas.test.ts`). Una clave huérfana aquí son 26 traducciones
 * que nadie lee, y la que alguien quita del código sin quitar del catálogo es la
 * siguiente que se traduce de más.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';
import { clavesSinUsar, codigoDe } from '@sytue/i18n/claves-usadas';
import { en } from './messages/en';

test('ninguna clave del catálogo de gestión se queda sin usar', () => {
  const codigo = codigoDe(join(import.meta.dir, '..', '..'));
  expect(clavesSinUsar(Object.keys(en), codigo, {})).toEqual([]);
});
