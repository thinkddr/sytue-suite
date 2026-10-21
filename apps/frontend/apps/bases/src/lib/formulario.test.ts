/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El formulario público no pide sesión: lo que este servidor hace por quien abre el enlace
 * tiene que estar acotado ANTES de llamar al servicio. Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import { MAX_CUERPO_ENVIO, leerFormulario, reenviar } from './formulario.js';

const TOKEN = 'a'.repeat(43);
const real = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = real;
});

function espia(respuesta = new Response(null, { status: 204 })) {
  const f = mock(() => Promise.resolve(respuesta));
  globalThis.fetch = f as unknown as typeof fetch;
  return f;
}

const envio = (cuerpo: string, cabeceras: Record<string, string> = {}) =>
  new Request(`https://bases.sytue.test/bases/f/${TOKEN}/enviar`, {
    method: 'POST',
    body: cuerpo,
    headers: { 'content-length': String(cuerpo.length), ...cabeceras },
  });

test('un token que no tiene forma de token no llega al servicio', async () => {
  const f = espia();
  for (const malo of ['', 'corto', `${TOKEN}x`, '../../api/v1/bases', `${'a'.repeat(42)}/`]) {
    expect((await reenviar(malo, '', envio('{}'))).status).toBe(404);
    expect(await leerFormulario(malo, envio('{}'))).toBeNull();
  }
  expect(f).not.toHaveBeenCalled();
});

test('un envío más grande que el techo se rechaza sin leerlo ni reenviarlo', async () => {
  const f = espia();
  const gordo = envio('{}', { 'content-length': String(MAX_CUERPO_ENVIO + 1) });
  expect((await reenviar(TOKEN, '', gordo)).status).toBe(413);
  expect(gordo.bodyUsed).toBe(false);
  const sinLargo = new Request(`https://bases.sytue.test/bases/f/${TOKEN}/enviar`, {
    method: 'POST',
    body: '{}',
  });
  sinLargo.headers.delete('content-length');
  expect((await reenviar(TOKEN, '', sinLargo)).status).toBe(411);
  expect(f).not.toHaveBeenCalled();
});

test('un envío normal se reenvía solo con la IP de quien pregunta, sin cookies ni sesión', async () => {
  const f = espia();
  const res = await reenviar(
    TOKEN,
    '',
    envio('{"values":{}}', {
      cookie: 'sesion=secreta',
      authorization: 'Bearer robado',
      'x-forwarded-for': '203.0.113.7',
    }),
  );
  expect(res.status).toBe(204);
  expect(f).toHaveBeenCalledTimes(1);
  const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
  expect(url.endsWith(`/_bases/v1/forms/${TOKEN}`)).toBe(true);
  expect(init.headers).toEqual({
    'x-forwarded-for': '203.0.113.7',
    'content-type': 'application/json',
  });
});
