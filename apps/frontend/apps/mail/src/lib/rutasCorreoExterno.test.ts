/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué rutas del correo externo pasan por el BFF de Mail. VISTO EN ROJO
 * (2026-11-06): sin `summary` en la lista, el lateral pedía el resumen y el proxy
 * lo cortaba. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { rutaCorreoExternoDenegada } from './rutasCorreoExterno.js';

const cuenta = '0b4a3f1e-8f7c-4d2a-9b1e-2c3d4e5f6a7b';

test('pasan la lista, el alta, el resumen y las rutas de una cuenta', () => {
  for (const ruta of [
    'accounts',
    'connect',
    'summary',
    cuenta,
    `${cuenta}/folders`,
    `${cuenta}/messages`,
    `${cuenta}/message`,
    `${cuenta}/send`,
    `${cuenta}/actions`,
  ])
    expect(rutaCorreoExternoDenegada(`/api/v1/external-mail/${ruta}`)).toBe(false);
});

test('se corta lo demás, también lo que cuelga del resumen', () => {
  for (const ruta of [
    'summary/x',
    'summary/',
    'accounts/otra',
    `${cuenta}/folders/x`,
    `${cuenta}/secrets`,
    'no-es-una-cuenta/folders',
    '../instances',
  ])
    expect(rutaCorreoExternoDenegada(`/api/v1/external-mail/${ruta}`)).toBe(true);
});
