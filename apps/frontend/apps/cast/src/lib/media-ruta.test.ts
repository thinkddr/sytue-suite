/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /cast/media/… se sirve SIN sesión y compone el destino con la ruta pedida: lo único
 * que la ata a `/media/` del servicio es que no pueda salirse de ahí (FE-01).
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';
import { GET } from '../pages/media/[...ruta].js';

const real = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = real;
});

function pedir(ruta: string) {
  const f = mock(() => Promise.resolve(new Response('x', { status: 200 })));
  globalThis.fetch = f as unknown as typeof fetch;
  const request = new Request('https://sytue.test/cast/media/x');
  const ctx = { params: { ruta }, request, url: new URL(request.url) } as unknown as APIContext;
  return { respuesta: GET(ctx) as Promise<Response>, f };
}

test('una ruta que se sale de /media/ no llega al servicio', async () => {
  for (const ruta of [
    'v/t/../../healthz',
    'v/.\t./.\t./healthz',
    'v\\..\\..\\healthz',
    'v%2F..%2F..%2Fhealthz',
    'v/t/x\n',
  ]) {
    const { respuesta, f } = pedir(ruta);
    expect((await respuesta).status, JSON.stringify(ruta)).toBe(400);
    expect(f).not.toHaveBeenCalled();
  }
});

test('la de un fichero del vídeo, sí', async () => {
  const { respuesta, f } = pedir('7b0c/tok/master.m3u8');
  expect((await respuesta).status).toBe(200);
  expect(String((f.mock.calls[0] as unknown[])[0])).toEndWith('/media/7b0c/tok/master.m3u8');
});
