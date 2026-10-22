/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { destinoTrasConsentimiento } from './consentimiento.js';

test('conectada: vuelve a su sección con la instancia, y la salida abre el asistente de salida', () => {
  expect(
    destinoTrasConsentimiento('/admin', { instance: 'i1', section: 'migrations', out: true }),
  ).toBe('/admin/migrations?instance=i1&oauth=connected&purpose=out');
  expect(destinoTrasConsentimiento('/admin/', { instance: 'i1', section: 'migrations' })).toBe(
    '/admin/migrations?instance=i1&oauth=connected',
  );
});

test('un fallo vuelve con su código y sin «conectada»', () => {
  expect(destinoTrasConsentimiento('/admin', { section: 'apps', error: 'state' })).toBe(
    '/admin/apps?oauth_error=state',
  );
  expect(
    destinoTrasConsentimiento('/admin', {
      instance: 'i1',
      section: 'migrations',
      out: true,
      error: 'exchange',
    }),
  ).toBe('/admin/migrations?instance=i1&oauth_error=exchange');
});

test('una sección fuera de la lista cae en el catálogo de apps, nunca fuera de la consola', () => {
  for (const section of [
    '../evil',
    '//evil.example',
    'https://evil.example',
    'MIGRATIONS',
    undefined,
  ]) {
    expect(
      destinoTrasConsentimiento('/admin', { instance: 'i1', ...(section ? { section } : {}) }),
    ).toBe('/admin/apps?instance=i1&oauth=connected');
  }
});
