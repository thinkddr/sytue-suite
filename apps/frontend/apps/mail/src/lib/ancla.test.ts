/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El ancla S/MIME: una organización sin autoridad (404) y un fallo de red se dicen
 * con su motivo —la barra lateral los convierte en aviso— en vez de dejar a la
 * persona mirando el JSON crudo del BFF; lo bueno se guarda con su nombre.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { bajarAncla } = await import('./ancla.js');
const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

function responder(r: Response | Error): void {
  globalThis.fetch = (async () => {
    if (r instanceof Error) throw r;
    return r;
  }) as unknown as typeof fetch;
}

test('sin autoridad todavía (404) es «missing», no una descarga', async () => {
  responder(new Response('{"error":"mail_error"}', { status: 404 }));
  expect(await bajarAncla()).toBe('missing');
});

test('un 502 o la red caída es «failed»', async () => {
  responder(new Response('{}', { status: 502 }));
  expect(await bajarAncla()).toBe('failed');
  responder(new TypeError('Failed to fetch'));
  expect(await bajarAncla()).toBe('failed');
});

test('el PEM se guarda con su nombre', async () => {
  responder(new Response('-----BEGIN CERTIFICATE-----', { status: 200 }));
  let nombre = '';
  const click = HTMLAnchorElement.prototype.click;
  HTMLAnchorElement.prototype.click = function (this: HTMLAnchorElement) {
    nombre = this.download;
  };
  try {
    expect(await bajarAncla()).toBe('ok');
  } finally {
    HTMLAnchorElement.prototype.click = click;
  }
  expect(nombre).toBe('sytue-smime-ca.pem');
});
