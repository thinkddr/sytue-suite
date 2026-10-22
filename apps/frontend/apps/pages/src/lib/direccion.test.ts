/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { direccionDelRender } from './direccion';

test('usa el host canónico real, tanto de la suite como un dominio propio', () => {
  expect(direccionDelRender({ site: { canonical_host: 'aurora.sites.staging.sytue.eu' } })).toBe(
    'https://aurora.sites.staging.sytue.eu/',
  );
  expect(
    direccionDelRender({
      site: { canonical_host: 'www.aurora.es', sites_host: 'aurora.sites.sytue.eu' },
    }),
  ).toBe('https://www.aurora.es/');
});

test('sin host canónico no inventa un dominio o una ruta', () => {
  for (const datos of [
    null,
    {},
    { site: { sites_host: 'sites.sytue.eu' } },
    { site: { canonical_host: '' } },
  ]) {
    expect(direccionDelRender(datos)).toBeNull();
  }
});

test('un valor que contiene URL, credenciales o ruta no es un hostname canónico', () => {
  for (const host of [
    'https://www.aurora.es',
    'usuario@www.aurora.es',
    'www.aurora.es/ruta',
    'www.aurora.es?otro=1',
    'www.aurora.es#seccion',
    'www.aurora.es:1234',
  ]) {
    expect(direccionDelRender({ site: { canonical_host: host } })).toBeNull();
  }
});
