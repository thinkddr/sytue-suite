/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
/**
 * «?» abre la hoja de atajos del ERP fuera de los campos de texto, y Escape la
 * cierra. Se pinta la app entera porque lo que se vigila es el oyente de verdad:
 * la tabla de `lib/atajos.ts` ya tiene su test.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { SytueUser } from '@sytue/auth';
// Sin la puerta del almacenamiento cifrado (MatrixStorageGate): la hoja de atajos no
// depende de él, y aquí no hay cliente de Matrix.
import { ErpAppBody as ErpApp } from './ErpApp.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const original = globalThis.fetch;
let host: HTMLDivElement;
beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  // Hay sesión, pero el catálogo y lo demás no contestan: la hoja de atajos no
  // depende de ellos.
  globalThis.fetch = (async (input: RequestInfo | URL) =>
    String(input).includes('/auth/token')
      ? new Response(JSON.stringify({ accessToken: 't', expiresAt: Date.now() + 3_600_000 }))
      : new Response(JSON.stringify({ error: { code: 'unavailable', message: 'Down' } }), {
          status: 503,
        })) as typeof fetch;
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = original;
});

const usuario = {
  id: 'u1',
  email: 'ana@example.test',
  emailVerified: true,
  name: 'Ana',
  preferredUsername: 'ana',
  roles: [],
} as unknown as SytueUser;

function pulsar(key: string, destino: EventTarget = window): void {
  destino.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
}

function hoja(): Element | undefined {
  return [...document.querySelectorAll('[role="dialog"]')].find((d) =>
    d.textContent?.includes(t('shortcuts.title')),
  );
}

test('«?» abre la hoja de atajos con las teclas del ERP, y Escape la cierra', async () => {
  await act(async () =>
    render(<ErpApp user={usuario} hubBaseUrl="https://hub.example.test" locale="es" />, host),
  );
  expect(hoja()).toBeUndefined();
  await act(async () => pulsar('?'));
  const dialogo = hoja();
  expect(dialogo).toBeDefined();
  for (const texto of [t('shortcuts.palette'), t('shortcuts.new'), t('shortcuts.help')]) {
    expect(dialogo?.textContent).toContain(texto);
  }
  await act(async () => pulsar('Escape', document.activeElement ?? window));
  expect(hoja()).toBeUndefined();
});

test('«?» escrito en un campo de texto es un carácter, no la hoja', async () => {
  await act(async () =>
    render(<ErpApp user={usuario} hubBaseUrl="https://hub.example.test" locale="es" />, host),
  );
  const campo = document.createElement('input');
  campo.type = 'text';
  host.appendChild(campo);
  campo.focus();
  await act(async () => pulsar('?', campo));
  expect(hoja()).toBeUndefined();
});
