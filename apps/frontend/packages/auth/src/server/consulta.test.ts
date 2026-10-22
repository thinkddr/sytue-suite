/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { consulta } from './consulta.js';

const url = new URL('https://ejemplo.test/api/search?q=de-la-url&limit=7');

function peticion(body: string, tipo = 'application/x-www-form-urlencoded'): Request {
  return new Request(url, {
    method: 'QUERY',
    body,
    headers: {
      'content-type': tipo,
      'content-length': String(new TextEncoder().encode(body).length),
    },
  });
}

test('con GET los parámetros salen de la URL', async () => {
  const p = await consulta(new Request(url), url);
  expect(p.get('q')).toBe('de-la-url');
  expect(p.get('limit')).toBe('7');
});

test('con QUERY salen del cuerpo y la URL no pinta nada', async () => {
  const p = await consulta(peticion('q=del+cuerpo&limit=3'), url);
  expect(p.get('q')).toBe('del cuerpo');
  expect(p.get('limit')).toBe('3');
});

test('un término con & y = viaja entero, que es media gracia de QUERY', async () => {
  const crudo = new URLSearchParams({ q: 'a=b & c "d"' }).toString();
  expect((await consulta(peticion(crudo), url)).get('q')).toBe('a=b & c "d"');
});

test('un cuerpo que no es un formulario no se adivina: queda vacío', async () => {
  const p = await consulta(peticion('{"q":"json"}', 'application/json'), url);
  expect([...p.keys()]).toEqual([]);
});

test('un cuerpo enorme no se lee', async () => {
  const grande = 'q=' + 'x'.repeat(64 * 1024 + 1);
  expect([...(await consulta(peticion(grande), url)).keys()]).toEqual([]);
});
